package finding_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// seenAt backdates a scan run, so a test can say when a finding was first
// seen. Nothing else can: the run stamps itself with the clock.
func (f *fixture) seenAt(t *testing.T, runID int64, when time.Time) {
	t.Helper()
	if _, err := f.db.DB.NewUpdate().Table("scan_run").
		Set("started_at = ?", when.UTC().Truncate(time.Microsecond)).
		Where("id = ?", runID).Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
}

var testWindows = finding.Windows{
	Exploited: 3 * 24 * time.Hour,
	Critical:  7 * 24 * time.Hour,
	High:      30 * 24 * time.Hour,
	Medium:    90 * 24 * time.Hour,
	Low:       180 * 24 * time.Hour,
}

func TestWhatIsRunningOutIsOrderedByDeadlineNotByAge(t *testing.T) {
	// The defect this replaced: the statement took the oldest findings and a
	// loop then discarded whatever was not due, so an exploited finding due
	// tomorrow lost its place to an old low that had filled the buffer.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())

		// A low, first seen a hundred days ago. Its window is a hundred and
		// eighty days, so it is not due for another eighty.
		oldRun := f.run(t)
		f.seenAt(t, oldRun, time.Now().UTC().Add(-100*24*time.Hour))
		mild := found("CVE-2026-OLD", swss)
		mild.Issue.Severity = "low"
		if _, err := f.store.Apply(t.Context(), f.target, oldRun,
			[]finding.Reported{mild}); err != nil {
			t.Fatal(err)
		}

		// An exploited finding seen two days ago. Its window is three days,
		// so it is due tomorrow — sooner than anything else here.
		newRun := f.run(t)
		f.seenAt(t, newRun, time.Now().UTC().Add(-2*24*time.Hour))
		urgent := found("CVE-2026-NOW", teamd)
		urgent.Issue.Severity = "medium"
		urgent.Issue.Exploited = true
		if _, err := f.store.Apply(t.Context(), f.target, newRun,
			[]finding.Reported{mild, urgent}); err != nil {
			t.Fatal(err)
		}

		// Ninety days ahead, so both are in range and the order is the thing
		// being tested. The low was seen first and is due last.
		who := f.holding(t, access.PublicTriage)
		late, err := f.store.RunningOut(t.Context(), who, finding.Scope{}, 90*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(late) != 2 {
			t.Fatalf("%d rows are running out of time, want both", len(late))
		}
		if late[0].Vulnerability != "CVE-2026-NOW" {
			t.Errorf("first row is %s, want the exploited finding due soonest",
				late[0].Vulnerability)
		}

		// And a fortnight ahead the low is not in range at all.
		soon, err := f.store.RunningOut(t.Context(), who, finding.Scope{}, 14*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range soon {
			if row.Vulnerability == "CVE-2026-OLD" {
				t.Error("a low with eighty days left is on the fortnight's list")
			}
		}
	})
}

func TestWhatIsRunningOutIsOneRowPerIssueAtAComponent(t *testing.T) {
	// A kernel flaw at sixty places is one thing somebody has to answer.
	// Sixty rows of it is a list with one entry in it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		f.seenAt(t, run, time.Now().UTC().Add(-60*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		if places := len(f.open(t)); places != 2 {
			t.Fatalf("expected one issue at two places, got %d", places)
		}

		late, err := f.store.RunningOut(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			14*24*time.Hour, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(late) != 1 {
			t.Fatalf("one issue at two places produced %d rows", len(late))
		}
		if late[0].Places != 2 {
			t.Errorf("the row says %d places, want 2", late[0].Places)
		}
	})
}

func TestAnUnratedFindingDoesNotGetTheLongestDeadline(t *testing.T) {
	// Nobody having scored it is not a claim that it is mild. Under the old
	// mapping it took the low window, which put the findings least is known
	// about at the back of the queue.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		f.seenAt(t, run, time.Now().UTC().Add(-100*24*time.Hour))
		unrated := found("CVE-2026-QUIET", swss)
		unrated.Issue.Severity = ""
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{unrated}); err != nil {
			t.Fatal(err)
		}

		// A hundred days in: past the ninety-day medium window, well inside
		// the hundred-and-eighty-day low one.
		late, err := f.store.RunningOut(t.Context(), f.holding(t, access.PublicTriage), finding.Scope{},
			0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(late) != 1 {
			t.Fatalf("an unrated finding a hundred days old produced %d rows, want 1", len(late))
		}
	})
}

func TestOverdueIsCountedAgainstWhoeverIsHoldingIt(t *testing.T) {
	// A large open count on somebody keeping up is not the same signal as the
	// same count on somebody sitting on it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		f.seenAt(t, run, time.Now().UTC().Add(-100*24*time.Hour))
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		triager := f.holding(t, access.PublicTriage)
		if _, err := f.store.Assign(t.Context(), triager, f.target,
			open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7))); err != nil {
			t.Fatal(err)
		}

		held, err := f.store.HeldBy(t.Context(), triager)
		if err != nil {
			t.Fatal(err)
		}
		if len(held) != 1 {
			t.Fatalf("%d people are holding something, want 1", len(held))
		}
		if held[0].Open != 2 {
			t.Errorf("they hold %d findings, want 2", held[0].Open)
		}
		// A high at a hundred days is seventy days past its thirty-day window.
		if held[0].Overdue != 2 {
			t.Errorf("%d of what they hold is overdue, want 2", held[0].Overdue)
		}
	})
}

// deadline reads the stored deadline for one issue, which is the thing a
// policy change has to move.
func (f *fixture) deadline(t *testing.T, identifier string) time.Time {
	t.Helper()
	var due time.Time
	err := f.db.DB.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("f.due_at").
		Where("v.identifier = ?", identifier).
		Where("f.closed_run_id IS NULL").
		Limit(1).Scan(t.Context(), &due)
	if err != nil {
		t.Fatalf("read the stored deadline for %s: %v", identifier, err)
	}
	return due
}

func TestChangingHowLongSomethingMayStayOpenMovesTheDeadline(t *testing.T) {
	// REM-26. The deadline is worked out when a finding is first seen and
	// stored, so the one event that makes it wrong is somebody changing the
	// policy that set it. A number an administrator just typed that moves
	// nothing is worse than a slow screen — which is the difference between
	// this and urgency, stale until the next scan because nobody edits it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())

		run := f.run(t)
		seen := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Microsecond)
		f.seenAt(t, run, seen)
		high := found("CVE-2026-HIGH", swss)
		high.Issue.Severity = "high"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{high}); err != nil {
			t.Fatal(err)
		}

		// Stored at ingest, as the first sighting plus the window for its
		// rating rather than as anything derived at read time.
		was := f.deadline(t, "CVE-2026-HIGH")
		want := seen.Add(testWindows.High)
		if was.Sub(want).Abs() > time.Second {
			t.Fatalf("stored deadline is %s, want the first sighting plus the high window, %s",
				was, want)
		}

		shorter := testWindows
		shorter.High = 15 * 24 * time.Hour
		changed, err := f.store.Recompute(t.Context(), shorter)
		if err != nil {
			t.Fatal(err)
		}
		if changed == 0 {
			t.Fatal("the policy changed and nothing was rewritten")
		}

		now := f.deadline(t, "CVE-2026-HIGH")
		if now.Sub(seen.Add(shorter.High)).Abs() > time.Second {
			t.Errorf("deadline after halving the window is %s, want %s",
				now, seen.Add(shorter.High))
		}
		if !now.Before(was) {
			t.Errorf("the window was halved and the deadline did not move earlier: %s then %s",
				was, now)
		}
	})
}

func TestWhatIsRunningOutNarrowsToWhatIsSelected(t *testing.T) {
	// UIX-38. The screens that span products answer for whatever the picker
	// has selected, with "all" offered at each level rather than being the
	// only option — so the narrowing has to happen in the statement, not in
	// whatever is drawing the result.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		f.seenAt(t, run, time.Now().UTC().Add(-40*24*time.Hour))
		high := found("CVE-2026-SCOPE", swss)
		high.Issue.Severity = "high"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{high}); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicTriage)
		window := 90 * 24 * time.Hour

		everything, err := f.store.RunningOut(t.Context(), who, finding.Scope{}, window, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(everything) == 0 {
			t.Fatal("nothing is running out, so there is nothing to narrow")
		}

		here := f.productID
		mine, err := f.store.RunningOut(t.Context(), who,
			finding.Scope{ProductID: &here}, window, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(mine) != len(everything) {
			t.Errorf("narrowing to the only product there is returned %d of %d rows",
				len(mine), len(everything))
		}

		// A product this build does not belong to. Nothing here is in it, and
		// an empty answer is the whole point: a scoped page that quietly
		// ignored the scope would report another product's numbers under this
		// product's name.
		elsewhere := here + 1000
		none, err := f.store.RunningOut(t.Context(), who,
			finding.Scope{ProductID: &elsewhere}, window, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(none) != 0 {
			t.Errorf("narrowing to another product returned %d rows, want none", len(none))
		}
	})
}

// setting writes one deployment setting, so a test can put a line in place
// before a scan is applied.
func (f *fixture) setting(t *testing.T, name, value string) error {
	t.Helper()
	return setting.NewStore(f.db.DB).Set(t.Context(), name, value)
}

// deadlineOrZero reads a stored deadline, or the zero time where there is
// none. Below the line there is none, which is the thing being asserted.
func (f *fixture) deadlineOrZero(t *testing.T, identifier string) time.Time {
	t.Helper()
	var due *time.Time
	err := f.db.DB.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("f.due_at").
		Where("v.identifier = ?", identifier).
		Where("f.closed_run_id IS NULL").
		Limit(1).Scan(t.Context(), &due)
	if err != nil {
		t.Fatalf("read the stored deadline for %s: %v", identifier, err)
	}
	if due == nil {
		return time.Time{}
	}
	return *due
}

// urgency reads where an issue's findings sit in the order.
func (f *fixture) urgency(t *testing.T, identifier string) int64 {
	t.Helper()
	var rank int64
	err := f.db.DB.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("MAX(f.urgency)").
		Where("v.identifier = ?", identifier).
		Where("f.closed_run_id IS NULL").
		Scan(t.Context(), &rank)
	if err != nil {
		t.Fatalf("read where %s sits: %v", identifier, err)
	}
	return rank
}

// ratings reads what was published about an issue and what we say instead.
func (f *fixture) ratings(t *testing.T, identifier string) (string, string) {
	t.Helper()
	var row struct {
		Published string `bun:"published"`
		Assessed  string `bun:"assessed"`
	}
	err := f.db.DB.NewSelect().
		TableExpr("vulnerability AS v").
		ColumnExpr("COALESCE(v.severity, '') AS published").
		ColumnExpr("COALESCE(v.assessed_severity, '') AS assessed").
		Where("v.identifier = ?", identifier).
		Scan(t.Context(), &row)
	if err != nil {
		t.Fatalf("read the ratings for %s: %v", identifier, err)
	}
	return row.Published, row.Assessed
}

// issue resolves an identifier to what it is stored as.
func (f *fixture) issue(t *testing.T, identifier string) int64 {
	t.Helper()
	var id int64
	err := f.db.DB.NewSelect().
		TableExpr("vulnerability AS v").
		ColumnExpr("v.id").
		Where("v.identifier = ?", identifier).
		Scan(t.Context(), &id)
	if err != nil {
		t.Fatalf("read what %s is: %v", identifier, err)
	}
	return id
}

// recorded makes sure a person exists to hang a claim on.
//
// An assessment names whoever made it, and that is a real reference rather
// than a number in a column — the subjects these tests hold are made up, so
// the row has to be put there for them.
func (f *fixture) recorded(t *testing.T, id int64, identity string) {
	t.Helper()
	n, err := f.db.DB.NewSelect().Table("person").Where("id = ?", id).Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if n > 0 {
		return
	}
	_, err = f.db.DB.NewInsert().
		Model(&map[string]interface{}{
			"id": id, "identity": identity, "display_name": identity,
			"is_admin": false, "is_bootstrap": false, "admin_derived": false,
			"created_at": time.Now().UTC().Truncate(time.Microsecond),
		}).
		TableExpr("person").
		Exec(t.Context())
	if err != nil {
		t.Fatalf("record a person to hang a claim on: %v", err)
	}
}
