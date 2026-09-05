package finding_test

import (
	"errors"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestAPersonClosesAFlawTheyRecordedAndNothingElseCan(t *testing.T) {
	// Resolution is computed from scans everywhere else, and that rule needs
	// evidence. For a flaw somebody recorded there is none and never will be —
	// no run reports it — so the computation has no input and the finding
	// stays open forever. The exception is exactly that wide.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		who := f.planner(t, access.PrivateTriage)

		rows, identifier, err := f.store.Enter(ctx, who, finding.Entering{
			TargetIDs: []int64{f.target}, Component: swss.Name, Severity: "high",
			Summary: "The management socket accepts a request nobody authenticated.",
		})
		if err != nil {
			t.Fatal(err)
		}
		row := rows[0]
		if err != nil {
			t.Fatal(err)
		}
		issueID := row.VulnerabilityID

		// A reason is required, for the same reason moving a disclosure date
		// needs one: a closure with no reason is a record saying somebody
		// closed it and nothing else.
		if _, err := f.store.Resolve(ctx, who, f.target, issueID, "   "); !errors.Is(err, finding.ErrNoReason) {
			t.Errorf("a closure with no reason was accepted: %v", err)
		}

		const because = "Fixed by the authentication check added in 1.0.1."
		done, err := f.store.Resolve(ctx, who, f.target, issueID, because)
		if err != nil {
			t.Fatalf("closing %s: %v", identifier, err)
		}
		if done.Closed != 1 {
			t.Errorf("closed %d rows, want the one place it sits at", done.Closed)
		}

		var closed struct {
			ClosedAt      *string `bun:"closed_at"`
			ClosedBy      *int64  `bun:"closed_by"`
			ClosedNote    string  `bun:"closed_note"`
			ClosedBecause string  `bun:"closed_because"`
			ClosedRunID   *int64  `bun:"closed_run_id"`
		}
		if err := f.db.DB.NewSelect().TableExpr("finding AS f").
			ColumnExpr("f.closed_at AS closed_at").
			ColumnExpr("f.closed_by AS closed_by").
			ColumnExpr("COALESCE(f.closed_note, '') AS closed_note").
			ColumnExpr("COALESCE(f.closed_because, '') AS closed_because").
			ColumnExpr("f.closed_run_id AS closed_run_id").
			Where("f.id = ?", row.ID).Scan(ctx, &closed); err != nil {
			t.Fatal(err)
		}
		if closed.ClosedAt == nil {
			t.Fatal("it is still open")
		}
		// Who and why, on the record. And no run: a run closed nothing here,
		// and saying one did would put a scan's authority behind a person's
		// judgment.
		if closed.ClosedBy == nil || *closed.ClosedBy != who.ID {
			t.Errorf("closed by %v, want the person who said so", closed.ClosedBy)
		}
		if closed.ClosedNote != because {
			t.Errorf("the closure records %q", closed.ClosedNote)
		}
		if closed.ClosedBecause != string(finding.Fixed) {
			t.Errorf("closed because %q, want the word a person writes", closed.ClosedBecause)
		}
		if closed.ClosedRunID != nil {
			t.Errorf("a person's closure claims run %d closed it", *closed.ClosedRunID)
		}

		// Closing it again finds nothing open, rather than rewriting the first
		// closure's reason with a second one.
		if _, err := f.store.Resolve(ctx, who, f.target, issueID, "again"); !errors.Is(err, finding.ErrNothingOpenThere) {
			t.Errorf("closing it twice: %v", err)
		}
	})
}

func TestAScannersFindingIsNotClosedByHand(t *testing.T) {
	// The half of REM-09 that stands. For a scanner's finding the evidence
	// exists — a scan of the release either finds it or does not — and letting
	// somebody overrule that is exactly the gap between marking work done and
	// the work being done that computing resolution was chosen to remove.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		issueID, err := finding.NewVulnerabilities(f.db.DB).ByName(ctx, "CVE-2026-1")
		if err != nil {
			t.Fatal(err)
		}

		who := f.planner(t, access.PrivateTriage)
		_, err = f.store.Resolve(ctx, who, f.target, issueID, "We fixed it, honestly.")
		if !errors.Is(err, finding.ErrNotOursToClose) {
			t.Errorf("a scanner's finding was closed by hand: %v", err)
		}

		// And it is still open, so nothing was written on the way to refusing.
		var open int
		if err := f.db.DB.NewSelect().TableExpr("finding AS f").
			ColumnExpr("COUNT(*)").
			Where("f.vulnerability_id = ?", issueID).
			Where("f.closed_at IS NULL").Scan(ctx, &open); err != nil {
			t.Fatal(err)
		}
		if open != 2 {
			t.Errorf("%d of the scanner's findings are still open, want both", open)
		}
	})
}

func TestClosingAnUndisclosedFlawNeedsThePrivateRight(t *testing.T) {
	// Closing one is a judgment about a finding nobody has announced, so it
	// asks what recording one asks. Somebody who may argue about disclosed
	// findings has not been handed the undisclosed ones here either.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		rows, _, err := f.store.Enter(ctx, f.planner(t, access.PrivateTriage), finding.Entering{
			TargetIDs: []int64{f.target}, Component: swss.Name, Severity: "high",
			Summary: "The management socket accepts a request nobody authenticated.",
		})
		if err != nil {
			t.Fatal(err)
		}
		row := rows[0]
		if err != nil {
			t.Fatal(err)
		}

		public := f.planner(t, access.PublicTriage)
		_, err = f.store.Resolve(ctx, public, f.target, row.VulnerabilityID, "Fixed.")
		if !errors.Is(err, access.ErrDenied) {
			t.Errorf("somebody without the private right closed an undisclosed flaw: %v", err)
		}
	})
}
