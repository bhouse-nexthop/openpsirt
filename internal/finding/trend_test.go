package finding_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestABumpThatFixedNothingIsNotCountedAsResolved(t *testing.T) {
	// Otherwise the two lines move together and say opposite things: work
	// completed, on the same chart where the same issue arrives as new.
	//
	// Counted as issues rather than places: the issue never leaves the set,
	// because its version moved and it came along, so nothing here needs a
	// rule about closure reasons to get the answer right.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		first := f.run(t)
		f.seenAt(t, first, time.Now().UTC().Add(-20*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, first,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		// Bumped, and the scanner still reports it. The old rows close as
		// superseded and new ones open against the new version.
		f.shipped(t, movedTo(libnlNew))
		second := f.run(t)
		f.seenAt(t, second, time.Now().UTC().Add(-10*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, second,
			[]finding.Reported{found("CVE-2026-1", libnlNew)}); err != nil {
			t.Fatal(err)
		}

		points, err := f.store.Trend(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			time.Now().UTC().Add(-30*24*time.Hour), 24*time.Hour, 30)
		if err != nil {
			t.Fatal(err)
		}
		var resolved int
		for _, point := range points {
			resolved += point.Resolved
		}
		if resolved != 0 {
			t.Errorf("a bump that fixed nothing counted %d findings as resolved", resolved)
		}
		// One issue, however many places it sits at. It was two here when the
		// trend counted finding rows, and counting rows reported 441,108 open
		// on a real image where 5,661 issues were open.
		if points[len(points)-1].Open != 1 {
			t.Errorf("%d open at the end, want the one issue against the new version",
				points[len(points)-1].Open)
		}
	})
}

func TestAnUpgradeThatFixedSomethingIsCountedAsResolved(t *testing.T) {
	// The other side of the same rule, so that excluding superseded does not
	// quietly become excluding everything.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		first := f.run(t)
		f.seenAt(t, first, time.Now().UTC().Add(-20*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, first,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		// Bumped, and the scanner stops reporting it.
		f.shipped(t, movedTo(libnlNew))
		second := f.run(t)
		f.seenAt(t, second, time.Now().UTC().Add(-10*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, second, nil); err != nil {
			t.Fatal(err)
		}

		points, err := f.store.Trend(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			time.Now().UTC().Add(-30*24*time.Hour), 24*time.Hour, 30)
		if err != nil {
			t.Fatal(err)
		}
		var resolved int
		for _, point := range points {
			resolved += point.Resolved
		}
		// One issue went away, not two places. What somebody reads as work
		// completed is the issue.
		if resolved != 1 {
			t.Errorf("an upgrade that fixed it counted %d resolved, want 1", resolved)
		}
	})
}

func TestATrendCountsNothingFromOutsideTheRangeItDraws(t *testing.T) {
	// The answer, not the cost. The statement also refuses to *read* what
	// cannot be in range — a finding closed before the first point, or opened
	// after the last — but that is about how much is fetched and is not
	// visible in what comes back, so nothing here can assert it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		old := f.run(t)
		f.seenAt(t, old, time.Now().UTC().Add(-400*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, old,
			[]finding.Reported{found("CVE-2026-ANCIENT", libnl)}); err != nil {
			t.Fatal(err)
		}
		f.shipped(t, movedTo(libnlNew))
		closing := f.run(t)
		f.seenAt(t, closing, time.Now().UTC().Add(-380*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, closing, nil); err != nil {
			t.Fatal(err)
		}

		// A fortnight's window, a year after all of that ended.
		points, err := f.store.Trend(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			time.Now().UTC().Add(-14*24*time.Hour), 24*time.Hour, 14)
		if err != nil {
			t.Fatal(err)
		}
		for _, point := range points {
			if point.Open != 0 || point.Opened != 0 || point.Resolved != 0 {
				t.Fatalf("a point in the last fortnight counted %d open, %d new, %d resolved "+
					"from a year ago", point.Open, point.Opened, point.Resolved)
			}
		}
	})
}

func TestATrendWithNoStepStillDrawsARange(t *testing.T) {
	// The step arrives as a query parameter and the loop walks whatever it
	// says, so zero would draw one instant however many times.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		f.seenAt(t, run, time.Now().UTC().Add(-20*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		points, err := f.store.Trend(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			time.Time{}, 0, 4)
		if err != nil {
			t.Fatal(err)
		}
		if len(points) != 4 {
			t.Fatalf("%d points, want 4", len(points))
		}
		for i := 1; i < len(points); i++ {
			if !points[i].At.After(points[i-1].At) {
				t.Fatalf("point %d is at %v, the same instant or earlier than point %d",
					i, points[i].At, i-1)
			}
		}
	})
}
