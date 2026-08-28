package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
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
	// The executable is an operator's configuration and the arguments are
	// fixed. Nothing here comes from a scan file — the inventory is written to
	// the program's input, never named as a path, so a component called
	// something hostile stays data.
	cmd := exec.CommandContext(ctx, g.executable(), "sbom:-", "--output", "json") // #nosec G204
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
			Fix         struct {
				State    string   `json:"state"`
				Versions []string `json:"versions"`
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
		DB      struct {
			Built         string `json:"built"`
			SchemaVersion any    `json:"schemaVersion"`
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
		DatabaseVersion: doc.Descriptor.DB.Built,
	}
	for _, match := range doc.Matches {
		if match.Artifact.Name == "" {
			continue
		}
		aliases := make([]string, 0, len(match.RelatedVulnerabilities))
		for _, related := range match.RelatedVulnerabilities {
			if related.ID != "" && related.ID != match.Vulnerability.ID {
				aliases = append(aliases, related.ID)
			}
		}
		result.Reported = append(result.Reported, finding.Reported{
			Issue: finding.Named{
				Identifier: match.Vulnerability.ID,
				Aliases:    aliases,
				// A word, not a score, and often unspecified. Numeric scores
				// come from the ranking feeds instead.
				Severity: strings.ToLower(match.Vulnerability.Severity),
			},
			Component: graph.Described{
				Name: match.Artifact.Name, Version: match.Artifact.Version,
				Purl: match.Artifact.Purl,
			},
			FixState: fixState(match.Vulnerability.Fix.State),
			FixedIn:  strings.Join(match.Vulnerability.Fix.Versions, ", "),
		})
	}
	return result, nil
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
