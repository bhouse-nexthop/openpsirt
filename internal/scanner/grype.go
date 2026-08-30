package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// Grype runs the scanner of that name.
//
// It is a hard requirement of a deployment rather than a packaging
// consideration: without it there is nothing to triage, since the
// vulnerability data is produced here rather than sent to us.
type Grype struct {
	// Path is the executable. Empty means whatever the environment resolves.
	Path string
	// Timeout bounds one execution. A scanner that has stopped making progress
	// must fail as a run that failed rather than hold a worker for ever.
	Timeout time.Duration
}

// Name identifies this scanner in everything it finds.
func (g Grype) Name() string { return "grype" }

// executable is what to run.
func (g Grype) executable() string {
	if g.Path != "" {
		return g.Path
	}
	return "grype"
}

// Scan feeds an inventory to the scanner and reads back what it matched.
func (g Grype) Scan(ctx context.Context, inventory io.Reader) (Result, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var out, errs bytes.Buffer
	// Reading the inventory from standard input rather than from a file: a
	// component name is somebody else's text, and nothing from a scan file is
	// ever used as a path.
	// The inventory goes to the program's input rather than being named as a
	// path. That is deliberate — nothing from a scan file is ever used as a
	// path, so a component called something hostile stays data — and it is
	// also what the scanner actually accepts: asking it to read a file called
	// "-" makes it look for one.
	//
	// The executable is an operator's configuration and the arguments are
	// fixed.
	cmd := exec.CommandContext(ctx, g.executable(), "--output", "json") // #nosec G204
	cmd.Stdin = inventory
	cmd.Stdout = &out
	cmd.Stderr = &errs

	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("run %s: %w: %s", g.executable(), err, tail(errs.String()))
	}
	return ParseGrype(&out)
}

// grypeDocument is the part of the scanner's output that is read.
type grypeDocument struct {
	Matches []struct {
		Vulnerability struct {
			ID          string `json:"id"`
			Severity    string `json:"severity"`
			Description string `json:"description"`
			// Where the issue is written up. Every match carries one, and for
			// the great majority it is the only route to a patch.
			DataSource string   `json:"dataSource"`
			URLs       []string `json:"urls"`
			Advisories []struct {
				Link string `json:"link"`
			} `json:"advisories"`
			// What the published estimates say about it being used.
			EPSS []struct {
				EPSS float64 `json:"epss"`
			} `json:"epss"`
			Risk float64 `json:"risk"`
			KEV  []struct {
				ID string `json:"id"`
			} `json:"knownExploited"`
			CVSS []struct {
				Version string `json:"version"`
				Vector  string `json:"vector"`
				Metrics struct {
					BaseScore float64 `json:"baseScore"`
				} `json:"metrics"`
			} `json:"cvss"`
			Fix struct {
				State     string   `json:"state"`
				Versions  []string `json:"versions"`
				Available []struct {
					Version string `json:"version"`
					Date    string `json:"date"`
				} `json:"available"`
			} `json:"fix"`
		} `json:"vulnerability"`
		RelatedVulnerabilities []struct {
			ID string `json:"id"`
		} `json:"relatedVulnerabilities"`
		Artifact struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Purl    string `json:"purl"`
		} `json:"artifact"`
	} `json:"matches"`
	Descriptor struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		// Where the database describes itself moved between versions of the
		// scanner: it used to sit directly under db and now sits under a
		// status within it. Both are read, because an operator running an
		// older build should not silently lose the record of what their
		// findings were matched against.
		DB struct {
			Built  string `json:"built"`
			Status struct {
				Built         string `json:"built"`
				SchemaVersion string `json:"schemaVersion"`
			} `json:"status"`
		} `json:"db"`
	} `json:"descriptor"`
}

// ParseGrype reads a scanner's output.
//
// Separate from running it so that what the output means is testable without
// the scanner and its database being installed, which is the part most likely
// to be wrong and the part least convenient to reproduce.
func ParseGrype(r io.Reader) (Result, error) {
	var doc grypeDocument
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return Result{}, fmt.Errorf("read the scanner's output: %w", err)
	}

	result := Result{
		Version:         doc.Descriptor.Version,
		DatabaseVersion: databaseVersion(doc),
	}
	for _, match := range doc.Matches {
		if match.Artifact.Name == "" {
			continue
		}
		score, vector := rating(match.Vulnerability.CVSS)
		aliases := make([]string, 0, len(match.RelatedVulnerabilities))
		for _, related := range match.RelatedVulnerabilities {
			if related.ID != "" && related.ID != match.Vulnerability.ID {
				aliases = append(aliases, related.ID)
			}
		}
		result.Reported = append(result.Reported, finding.Reported{
			Issue: finding.Named{
				Identifier:  match.Vulnerability.ID,
				Aliases:     aliases,
				Severity:    strings.ToLower(match.Vulnerability.Severity),
				Description: strings.TrimSpace(match.Vulnerability.Description),
				Advisory:    strings.TrimSpace(match.Vulnerability.DataSource),
				References:  references(match.Vulnerability.URLs, advisoryLinks(match.Vulnerability.Advisories)),
				Exploited:   len(match.Vulnerability.KEV) > 0,
				Likelihood:  firstEPSS(match.Vulnerability.EPSS),
				Score:       score,
				Vector:      vector,
			},
			Component: graph.Described{
				Name: match.Artifact.Name, Version: match.Artifact.Version,
				Purl: match.Artifact.Purl,
			},
			FixState: fixState(match.Vulnerability.Fix.State),
			FixedIn:  strings.Join(match.Vulnerability.Fix.Versions, ", "),
			FixedAt:  firstFixDate(match.Vulnerability.Fix.Available),
		})
	}
	return result, nil
}

// databaseVersion says which vulnerability data a run matched against.
//
// When it was built identifies the data; the schema version only identifies
// its shape, so it stands in only when there is nothing better. Without either,
// a finding that appeared or vanished because the data moved is unexplainable.
func databaseVersion(doc grypeDocument) string {
	switch {
	case doc.Descriptor.DB.Status.Built != "":
		return doc.Descriptor.DB.Status.Built
	case doc.Descriptor.DB.Built != "":
		return doc.Descriptor.DB.Built
	default:
		return doc.Descriptor.DB.Status.SchemaVersion
	}
}

// fixState reads what upstream has done about an issue.
//
// The scanner's own words are mapped rather than kept, because a second
// scanner will use different ones for the same three situations and the
// difference between them is what a person acts on.
func fixState(state string) finding.FixState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "fixed":
		return finding.FixedUpstream
	case "wont-fix":
		return finding.WontFix
	case "not-fixed":
		return finding.NoFix
	default:
		return ""
	}
}

// tail bounds what a failure quotes back from a program's complaints.
func tail(s string) string {
	const most = 500
	s = strings.TrimSpace(s)
	if len(s) <= most {
		return s
	}
	return "…" + s[len(s)-most:]
}

// patchLike recognizes an address that is a change rather than a write-up.
//
// Matched on the shape of the address, which is the only thing available: a
// report gives a flat list of references and does not say what any of them
// is. Recognizing them is worth the guess — somebody deciding whether to
// backport rather than upgrade needs the change itself, and hunting for it by
// hand is the step that does not happen when a thousand findings are waiting.
//
// A wrong guess costs a label, not the address, so this errs toward saying
// less: an unrecognized reference is reported as a discussion rather than
// asserted to be a patch.
var patchLike = regexp.MustCompile(
	`(?i)(/commit/|/commits/|/pull/|/merge_requests?/|/changeset/|\.patch$|\.diff$|patchwork|git\.kernel\.org|cgit)`)

// advisoryLike recognizes a write-up of the issue itself.
var advisoryLike = regexp.MustCompile(
	`(?i)(/security/advisories/|/advisories/|nvd\.nist\.gov|cve\.org|security-tracker|\bGHSA-|\bCVE-)`)

// references turns what a report points at into what it is.
//
// Deduplicated, because the same address arrives from several places in one
// report and a person reading a list of eleven identical links learns nothing
// from ten of them.
func references(urls []string, advisories []string) []finding.Reference {
	seen := map[string]bool{}
	var kept []finding.Reference
	for _, url := range append(append([]string{}, urls...), advisories...) {
		url = strings.TrimSpace(url)
		if url == "" || seen[url] {
			continue
		}
		seen[url] = true
		kept = append(kept, finding.Reference{URL: url, Kind: kindOf(url)})
	}
	return kept
}

func kindOf(url string) finding.ReferenceKind {
	switch {
	case patchLike.MatchString(url):
		return finding.Patch
	case advisoryLike.MatchString(url):
		return finding.AdvisoryRef
	default:
		return finding.Report
	}
}

// advisoryLinks flattens the structured advisory list to its addresses.
func advisoryLinks(advisories []struct {
	Link string `json:"link"`
}) []string {
	links := make([]string, 0, len(advisories))
	for _, advisory := range advisories {
		links = append(links, advisory.Link)
	}
	return links
}

// rating picks the severity score to record, and the vector it assumes.
//
// The first that states both. A report carries several ratings from different
// sources and they disagree; taking the first stated is at least a stable
// answer, and the vector travels with the number so that what the number
// assumed is readable rather than lost.
func rating(ratings []struct {
	Version string `json:"version"`
	Vector  string `json:"vector"`
	Metrics struct {
		BaseScore float64 `json:"baseScore"`
	} `json:"metrics"`
}) (float64, string) {
	for _, rated := range ratings {
		if rated.Metrics.BaseScore > 0 && rated.Vector != "" {
			return rated.Metrics.BaseScore, rated.Vector
		}
	}
	return 0, ""
}

// firstEPSS reads the published estimate that an issue will be exploited.
func firstEPSS(estimates []struct {
	EPSS float64 `json:"epss"`
}) float64 {
	for _, estimate := range estimates {
		if estimate.EPSS > 0 {
			return estimate.EPSS
		}
	}
	return 0
}

// firstFixDate reads when a fix became available.
//
// The earliest stated, because what matters is how long the fix has existed
// rather than which version somebody happens to be looking at.
func firstFixDate(available []struct {
	Version string `json:"version"`
	Date    string `json:"date"`
}) *time.Time {
	var earliest *time.Time
	for _, fix := range available {
		stated, err := time.Parse(time.DateOnly, strings.TrimSpace(fix.Date))
		if err != nil {
			continue
		}
		if earliest == nil || stated.Before(*earliest) {
			copied := stated
			earliest = &copied
		}
	}
	return earliest
}
