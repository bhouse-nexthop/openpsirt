package scanner_test

import (
	"os"
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/scanner"
)

func parse(t *testing.T) scanner.Result {
	t.Helper()
	f, err := os.OpenInRoot("testdata", "grype-output.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result, err := scanner.ParseGrype(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return result
}

func TestWhatRanAndAgainstWhatIsRecorded(t *testing.T) {
	// A finding that appeared or vanished because the scanner or its data
	// moved is unexplainable without this.
	result := parse(t)
	if result.Version != "0.112.0" {
		t.Errorf("scanner version reads as %q", result.Version)
	}
	if result.DatabaseVersion != "2026-08-28T09:21:39Z" {
		t.Errorf("database version reads as %q", result.DatabaseVersion)
	}
}

func TestTheDatabaseVersionIsFoundWhereverItSits(t *testing.T) {
	// Where the database describes itself moved between versions of the
	// scanner. An operator running an older build should not silently lose the
	// record of what their findings were matched against.
	older := `{"matches": [], "descriptor": {"name": "grype", "version": "0.90.0",
	 "db": {"built": "2026-01-01T00:00:00Z", "schemaVersion": 5}}}`
	result, err := scanner.ParseGrype(strings.NewReader(older))
	if err != nil {
		t.Fatalf("an older scanner's output: %v", err)
	}
	if result.DatabaseVersion != "2026-01-01T00:00:00Z" {
		t.Errorf("database version reads as %q", result.DatabaseVersion)
	}
}

func TestEveryMatchBecomesAReportedIssue(t *testing.T) {
	result := parse(t)
	if len(result.Reported) != 7 {
		t.Fatalf("read %d matches, want 7", len(result.Reported))
	}
	for _, r := range result.Reported {
		if r.Issue.Identifier == "" {
			t.Error("an issue was read with no identifier")
		}
		if r.Component.Name == "" || r.Component.Version == "" {
			t.Errorf("an issue was read against %+v", r.Component)
		}
		if r.Component.Purl == "" {
			t.Error("a package identifier was dropped, and it is what matches this back to what we hold")
		}
	}
}

func TestTheOtherNamesForAnIssueAreCarried(t *testing.T) {
	// The scanner files many issues under an advisory identifier and knows the
	// national one alongside. Dropping the alias would make it a second issue
	// the first time another scanner reported it the other way round.
	var withAliases int
	for _, r := range parse(t).Reported {
		if len(r.Issue.Aliases) == 0 {
			continue
		}
		withAliases++
		for _, alias := range r.Issue.Aliases {
			if alias == r.Issue.Identifier {
				t.Error("an issue lists itself as one of its other names")
			}
		}
	}
	if withAliases == 0 {
		t.Fatal("no aliases were read from output that carries them")
	}
}

func TestTheThreeFixStatesAreToldApart(t *testing.T) {
	// "No fix exists yet" and "upstream will not fix this" are different
	// situations, and the second changes the outcome somebody should reach.
	seen := map[finding.FixState]int{}
	for _, r := range parse(t).Reported {
		seen[r.FixState]++
		if r.FixState == finding.FixedUpstream && r.FixedIn == "" {
			t.Errorf("%s is fixed upstream with no version to move to", r.Component.Name)
		}
	}
	for _, state := range []finding.FixState{finding.FixedUpstream, finding.NoFix, finding.WontFix} {
		if seen[state] == 0 {
			t.Errorf("no issue read as %q, and the recorded output has one", state)
		}
	}
}

func TestSeverityIsAWordInOneCase(t *testing.T) {
	// The scanner capitalizes them and two spellings of one severity would
	// sort and group as two.
	var rated int
	for _, r := range parse(t).Reported {
		if r.Issue.Severity == "" {
			continue
		}
		rated++
		if r.Issue.Severity != strings.ToLower(r.Issue.Severity) {
			t.Errorf("%s is rated %q", r.Component.Name, r.Issue.Severity)
		}
	}
	if rated == 0 {
		t.Fatal("nothing was rated, and the recorded output rates everything")
	}
}

func TestOutputThatIsNotUnderstoodIsRefused(t *testing.T) {
	if _, err := scanner.ParseGrype(strings.NewReader("not json")); err == nil {
		t.Error("unreadable scanner output was accepted")
	}
}

func TestAnEmptyRunIsNotAFailure(t *testing.T) {
	// Nothing found is an ordinary answer, and treating it as an error would
	// make a clean product look like a broken scanner.
	result, err := scanner.ParseGrype(strings.NewReader(
		`{"matches": [], "descriptor": {"name": "grype", "version": "0.112.0"}}`))
	if err != nil {
		t.Fatalf("an empty run: %v", err)
	}
	if len(result.Reported) != 0 {
		t.Errorf("read %d issues from an empty run", len(result.Reported))
	}
}

func TestHowGrypeReachedAMatchIsRead(t *testing.T) {
	// The scanner tries advisory data for the package's own ecosystem first
	// and falls back to comparing a published identifier against an upstream
	// version range. On a distribution's package those two mean very
	// different things, and the output says which happened.
	//
	// The real shape, from a scan of an Alpine image: an apk package whose
	// only match detail is a cpe-match, with no fix information at all.
	const output = `{
	  "matches": [
	    {
	      "vulnerability": {"id": "CVE-2025-60876", "severity": "Medium",
	        "dataSource": "https://nvd.nist.gov/vuln/detail/CVE-2025-60876",
	        "fix": {"state": "", "versions": []}},
	      "artifact": {"name": "busybox", "version": "1.37.0-r14",
	        "purl": "pkg:apk/alpine/busybox@1.37.0-r14?distro=alpine-3.21.7"},
	      "matchDetails": [{"type": "cpe-match"}]
	    },
	    {
	      "vulnerability": {"id": "CVE-2026-1234", "severity": "High",
	        "dataSource": "https://secdb.alpinelinux.org/",
	        "fix": {"state": "fixed", "versions": ["1.37.0-r15"]}},
	      "artifact": {"name": "curl", "version": "8.11.0-r2",
	        "purl": "pkg:apk/alpine/curl@8.11.0-r2?distro=alpine-3.21.7"},
	      "matchDetails": [{"type": "exact-direct-match"}]
	    },
	    {
	      "vulnerability": {"id": "CVE-2026-9999", "severity": "Low",
	        "dataSource": "https://example.test/"},
	      "artifact": {"name": "mystery", "version": "1.0"},
	      "matchDetails": []
	    }
	  ],
	  "descriptor": {"name": "grype", "version": "0.118.0"}
	}`

	result, err := scanner.ParseGrype(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reported) != 3 {
		t.Fatalf("%d reported, want 3", len(result.Reported))
	}
	how := map[string]finding.Reported{}
	for _, r := range result.Reported {
		how[r.Component.Name] = r
	}

	// Compared against an upstream range. A distribution that backported the
	// fix looks the same as one that did not, which is what makes this the
	// question worth surfacing.
	if got := how["busybox"].Matched; got != finding.ByIdentifier {
		t.Errorf("a cpe-match reads as %q, want it marked as reached by identifier", got)
	}
	if got := how["busybox"].MatchedFrom; got != "https://nvd.nist.gov/vuln/detail/CVE-2025-60876" {
		t.Errorf("the match came from %q", got)
	}

	// The people who package it said so, and what they say about a fix is
	// about the version actually installed.
	if got := how["curl"].Matched; got != finding.ByAdvisory {
		t.Errorf("an exact match against advisory data reads as %q", got)
	}

	// Nothing said. Left empty rather than guessed: a word this does not know
	// is a word whose strength nobody has checked.
	if got := how["mystery"].Matched; got != "" {
		t.Errorf("a match with no details reads as %q, want nothing claimed", got)
	}
}

func TestAnUnknownMatchKindIsTreatedAsTheWeakerOne(t *testing.T) {
	// A scanner adding a match kind we have not seen must not have it read as
	// "the people who package this confirmed it". Unrecognized is the
	// direction that hides something, so it goes the other way.
	const output = `{
	  "matches": [{
	    "vulnerability": {"id": "CVE-2026-1", "severity": "High"},
	    "artifact": {"name": "thing", "version": "1.0"},
	    "matchDetails": [{"type": "exact-direct-match"}, {"type": "cpe-match"}]
	  }],
	  "descriptor": {"name": "grype", "version": "0.118.0"}
	}`
	result, err := scanner.ParseGrype(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Reported[0].Matched; got != finding.ByIdentifier {
		t.Errorf("a match reached both ways reads as %q, want the weaker of the two", got)
	}
}
