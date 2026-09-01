package finding_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestTheLineKeepsThingsOutOfTheListAndSaysHowMany(t *testing.T) {
	// TRI-43 and TRI-44. Below the line a finding is still recorded and still
	// counted — it leaves the working list, not the system — and the list says
	// what it is keeping out rather than showing a smaller number with nothing
	// explaining it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)

		low := found("CVE-2026-LOW", swss)
		low.Issue.Severity = "low"
		high := found("CVE-2026-HIGH", teamd)
		high.Issue.Severity = "high"
		// Rated in a word nobody recognizes. Unknown is not harmless, so it
		// is judged as a medium wherever severity is read.
		odd := found("CVE-2026-ODD", swss)
		odd.Issue.Severity = "unknown"
		// Being exploited is a fact about the world rather than a rating, so
		// no line sets it aside however it is scored.
		used := found("CVE-2026-USED", teamd)
		used.Issue.Severity = "low"
		used.Issue.Exploited = true

		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{low, high, odd, used}); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicTriage)
		line := finding.Filter{Floor: finding.Floor{Word: "medium"}}

		rows, total, err := f.store.Groups(t.Context(), who, f.target, 50, 0, line)
		if err != nil {
			t.Fatal(err)
		}
		named := map[string]bool{}
		for _, row := range rows {
			named[row.Vulnerability] = true
		}
		for _, want := range []string{"CVE-2026-HIGH", "CVE-2026-ODD", "CVE-2026-USED"} {
			if !named[want] {
				t.Errorf("%s is at or above the line and was kept out", want)
			}
		}
		if named["CVE-2026-LOW"] {
			t.Error("a low was shown with the line set at medium")
		}
		if total != 3 {
			t.Errorf("the total counts %d, want the three the line admits", total)
		}

		hidden, err := f.store.Hidden(t.Context(), who, f.target, line)
		if err != nil {
			t.Fatal(err)
		}
		if hidden != 1 {
			t.Errorf("the list says it is hiding %d, want the one low", hidden)
		}

		// And asking for them brings them back rather than needing the line
		// changed.
		open := line
		open.BelowFloor = true
		_, all, err := f.store.Groups(t.Context(), who, f.target, 50, 0, open)
		if err != nil {
			t.Fatal(err)
		}
		if all != 4 {
			t.Errorf("asking below the line counts %d, want all four", all)
		}
	})
}

func TestNothingBelowTheLineIsOnAClock(t *testing.T) {
	// REM-27. A line says this is not work and a deadline says this is work
	// and it is late; holding both means one of them is lying.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if err := f.setting(t, "triage.floor", "medium"); err != nil {
			t.Fatal(err)
		}
		run := f.run(t)
		low := found("CVE-2026-QUIET", swss)
		low.Issue.Severity = "low"
		high := found("CVE-2026-LOUD", teamd)
		high.Issue.Severity = "high"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{low, high}); err != nil {
			t.Fatal(err)
		}

		if got := f.deadlineOrZero(t, "CVE-2026-QUIET"); !got.IsZero() {
			t.Errorf("something below the line is on a clock, due %s", got)
		}
		if got := f.deadlineOrZero(t, "CVE-2026-LOUD"); got.IsZero() {
			t.Error("something above the line has no deadline")
		}
	})
}
