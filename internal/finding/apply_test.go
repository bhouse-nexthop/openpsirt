package finding_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestAFindingWhoseLikelihoodMovedIsNotCountedAsChanged(t *testing.T) {
	// Urgency is a fact about the moment a finding opened, and a later scan
	// does not rewrite it. Comparing it anyway counted every finding of an
	// issue whose likelihood moved as changed, night after night, and moved
	// its last change forward without anything about it having moved.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		unlikely := finding.Reported{
			Issue:     finding.Named{Identifier: "CVE-2026-1", Severity: "high", Likelihood: 0.01},
			Component: libnl, FixState: finding.NoFix,
		}
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{unlikely}); err != nil {
			t.Fatal(err)
		}
		opened := f.open(t)
		if len(opened) != 2 {
			t.Fatalf("opened %d findings", len(opened))
		}
		was := opened[0].LastChangedAt
		urgency := opened[0].Urgency

		likely := unlikely
		likely.Issue.Likelihood = 0.9
		applied, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{likely})
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("a likelihood moving wrote %+v, want nothing", applied)
		}
		for _, row := range f.open(t) {
			if !row.LastChangedAt.Equal(was) {
				t.Error("a likelihood moving was recorded as the finding changing")
			}
			if row.Urgency != urgency {
				t.Errorf("urgency moved from %d to %d, want it kept as of opening", urgency, row.Urgency)
			}
		}
	})
}

func TestANewFindingIsClockedByTheRatingInForce(t *testing.T) {
	// Somebody rated this issue worse than the world says, and that rating is
	// what every later reading uses: where it sits, what the line admits, and
	// the deadline of everything already open (TRI-41, TRI-42). A finding of
	// the same issue opened afterwards, on the published word's deadline,
	// would sit beside them with months more to run for no reason anybody
	// chose.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		mild := found("CVE-2026-RAISE", swss)
		mild.Issue.Severity = "low"
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{mild}); err != nil {
			t.Fatal(err)
		}
		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		if _, err := f.store.Assess(t.Context(), who, f.issue(t, "CVE-2026-RAISE"),
			"critical", "Reachable from the network in how we ship it."); err != nil {
			t.Fatal(err)
		}

		// The same issue turns up in another build.
		other := f.anotherBuild(t, "2.4.0")
		f.shippedTo(t, other, twoConsumers())
		run := f.runOn(t, other)
		if _, err := f.store.Apply(t.Context(), other, run, []finding.Reported{mild}); err != nil {
			t.Fatal(err)
		}

		var rows []struct {
			DueAt     *time.Time `bun:"due_at"`
			StartedAt time.Time  `bun:"started_at"`
		}
		err := f.db.DB.NewSelect().
			TableExpr("finding AS f").
			Join("JOIN scan_run AS r ON r.id = f.opened_run_id").
			ColumnExpr("f.due_at AS due_at").
			ColumnExpr("r.started_at AS started_at").
			Where("f.target_id = ?", other).
			Scan(t.Context(), &rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 0 {
			t.Fatal("nothing was opened in the other build")
		}
		windows := finding.DefaultWindows()
		for _, row := range rows {
			if row.DueAt == nil {
				t.Fatal("a finding was opened with no deadline")
			}
			if got := row.DueAt.Sub(row.StartedAt); got != windows.Critical {
				t.Errorf("clocked at %v, want the %v a critical gets rather than the %v a low gets",
					got, windows.Critical, windows.Low)
			}
		}
	})
}
