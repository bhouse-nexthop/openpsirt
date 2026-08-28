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
	// A finding that appeared or vanished because the scanner or its database
	// moved is unexplainable without this.
	result := parse(t)
	if result.Version != "0.100.0" || result.DatabaseVersion != "2026-08-28T01:31:04Z" {
		t.Errorf("recorded %q against %q", result.Version, result.DatabaseVersion)
	}
}

func TestEveryMatchBecomesAReportedIssue(t *testing.T) {
	result := parse(t)
	if len(result.Reported) != 4 {
		t.Fatalf("read %d matches, want 4", len(result.Reported))
	}
	first := result.Reported[0]
	if first.Issue.Identifier != "CVE-2026-31951" {
		t.Errorf("first issue is %q", first.Issue.Identifier)
	}
	if first.Component.Name != "frr" || first.Component.Version != "10.5.4-sonic-0" {
		t.Errorf("reported against %+v", first.Component)
	}
	if first.Component.Purl == "" {
		t.Error("the package identifier was dropped, and it is what matches this back to what we hold")
	}
}

func TestTheOtherNamesForAnIssueAreCarried(t *testing.T) {
	// The scanner files this one under an advisory identifier and knows the
	// national one alongside. Dropping the alias would make it a second issue
	// the first time another scanner reported it the other way round.
	result := parse(t)
	for _, r := range result.Reported {
		if r.Issue.Identifier != "GHSA-aaaa-bbbb-cccc" {
			continue
		}
		if len(r.Issue.Aliases) != 1 || r.Issue.Aliases[0] != "CVE-2026-1111" {
			t.Errorf("aliases are %v", r.Issue.Aliases)
		}
		return
	}
	t.Fatal("the issue reported under an advisory identifier is missing")
}

func TestTheThreeFixStatesAreToldApart(t *testing.T) {
	// "No fix exists yet" and "upstream will not fix this" are different
	// situations, and the second changes the outcome somebody should reach.
	want := map[string]finding.FixState{
		"frr":         finding.FixedUpstream,
		"tokio":       finding.FixedUpstream,
		"libc6":       finding.WontFix,
		"libnl-3-200": finding.NoFix,
	}
	for _, r := range parse(t).Reported {
		if got := r.FixState; got != want[r.Component.Name] {
			t.Errorf("%s reads as %q, want %q", r.Component.Name, got, want[r.Component.Name])
		}
	}
	for _, r := range parse(t).Reported {
		if r.Component.Name == "frr" && r.FixedIn != "10.6.1" {
			t.Errorf("the version that fixes it is %q", r.FixedIn)
		}
	}
}

func TestSeverityIsAWordInOneCase(t *testing.T) {
	// Scanners disagree about capitalisation, and two spellings of one
	// severity would sort and group as two.
	for _, r := range parse(t).Reported {
		if r.Issue.Severity != strings.ToLower(r.Issue.Severity) {
			t.Errorf("%s is rated %q", r.Component.Name, r.Issue.Severity)
		}
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
		`{"matches": [], "descriptor": {"name": "grype", "version": "0.100.0"}}`))
	if err != nil {
		t.Fatalf("an empty run: %v", err)
	}
	if len(result.Reported) != 0 {
		t.Errorf("read %d issues from an empty run", len(result.Reported))
	}
}
