package finding_test

import (
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestAReleaseNoteDoesNotCallAChurnedVersionAFix(t *testing.T) {
	// The one thing this document must not do. It goes to customers and they
	// keep it, so listing a bump that carried the issue with it under "Fixed"
	// is telling somebody something untrue in writing.
	notes := finding.Notes("2.4.0", &finding.Comparison{
		Fixed: []finding.Changed{
			{Vulnerability: "CVE-2026-1", Component: "libnl", Severity: "high",
				Because: finding.Upgraded},
			{Vulnerability: "CVE-2026-2", Component: "zlib", Severity: "critical",
				Because: finding.Superseded},
			{Vulnerability: "CVE-2026-3", Component: "busybox", Severity: "low",
				Because: finding.Unexplained},
		},
	})
	fixed, rest, found := strings.Cut(notes, "### Moved but not fixed")
	if !found {
		t.Fatalf("what moved without being fixed is not told apart:\n%s", notes)
	}
	if !strings.Contains(fixed, "CVE-2026-1") {
		t.Error("a real fix is missing from the fixed section")
	}
	for _, churn := range []string{"CVE-2026-2", "CVE-2026-3"} {
		if strings.Contains(fixed, churn) {
			t.Errorf("%s is listed as fixed and it was not", churn)
		}
		if !strings.Contains(rest, churn) {
			t.Errorf("%s is not listed at all", churn)
		}
	}
}

func TestAFixSaysHowItWasFixed(t *testing.T) {
	// "Upgraded to 3.9.0" and "a carried patch" are different sentences to
	// whoever reads this, and the closure reason already tells them apart.
	notes := finding.Notes("2.4.0", &finding.Comparison{
		Fixed: []finding.Changed{
			{Vulnerability: "CVE-2026-1", Component: "libnl", Because: finding.Upgraded},
			{Vulnerability: "CVE-2026-2", Component: "openssl", Because: finding.Revised},
			{Vulnerability: "CVE-2026-3", Component: "telnetd", Because: finding.Removed},
		},
	})
	for _, want := range []string{"upgraded", "carried patch", "no longer shipped"} {
		if !strings.Contains(notes, want) {
			t.Errorf("the note does not say %q:\n%s", want, notes)
		}
	}
}

func TestTheSameComparisonRendersTheSameDocumentTwice(t *testing.T) {
	// A release note that reorders between reads is one nobody can diff, and
	// the map iteration underneath is not ordered.
	comparison := &finding.Comparison{
		Still: []finding.Changed{
			{Vulnerability: "CVE-2026-9", Component: "a", Severity: "low"},
			{Vulnerability: "CVE-2026-1", Component: "b", Severity: "critical"},
			{Vulnerability: "CVE-2026-5", Component: "c", Severity: "critical"},
		},
	}
	first := finding.Notes("2.4.0", comparison)
	for i := 0; i < 5; i++ {
		if again := finding.Notes("2.4.0", comparison); again != first {
			t.Fatalf("the document changed between runs:\n%s\n---\n%s", first, again)
		}
	}
	// Worst first, so somebody reading from the top reads the worst first.
	if strings.Index(first, "CVE-2026-1") > strings.Index(first, "CVE-2026-9") {
		t.Errorf("a low is listed above a critical:\n%s", first)
	}
}

func TestAnEmptySectionIsNotWritten(t *testing.T) {
	// A heading with nothing under it is a question in the reader's mind about
	// whether something is missing.
	notes := finding.Notes("2.4.0", &finding.Comparison{
		Fixed: []finding.Changed{
			{Vulnerability: "CVE-2026-1", Component: "libnl", Because: finding.Upgraded},
		},
	})
	for _, absent := range []string{"Newly present", "Still present", "Moved but not fixed"} {
		if strings.Contains(notes, absent) {
			t.Errorf("an empty %q section was written:\n%s", absent, notes)
		}
	}
}

func TestABumpThatFellShortSaysWhatItCameFrom(t *testing.T) {
	// STA-18 from the other side: still present, and the version moved, so
	// somebody upgraded and it did not reach the fix.
	notes := finding.Notes("2.4.0", &finding.Comparison{
		Still: []finding.Changed{
			{Vulnerability: "CVE-2026-1", Component: "libnl", ArrivedFrom: "3.7.0"},
		},
	})
	if !strings.Contains(notes, "bumped from 3.7.0") {
		t.Errorf("the note does not say the bump fell short:\n%s", notes)
	}
}
