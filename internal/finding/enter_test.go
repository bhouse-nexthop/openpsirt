package finding_test

import (
	"errors"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

func TestAFlawInWhatWeShipIsRecordedAndSurvivesTheNextScan(t *testing.T) {
	// The case Phase 2 exists for: somebody knows about a flaw in their own
	// product before anybody outside does. It has no CVE, no scanner said
	// anything about it, and it still has to be triaged, assigned, decided and
	// reported like everything else.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}

		who := f.planner(t, access.PrivateTriage)
		row, identifier, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Component: swss.Name, Severity: "high",
			Summary: "The management socket accepts a request nobody authenticated.",
		})
		if err != nil {
			t.Fatalf("recording a flaw: %v", err)
		}
		if identifier != "SONIC-2026-0001" {
			t.Errorf("filed under %q, want the product's own first identifier of the year",
				identifier)
		}
		if row.Visibility != access.Private {
			t.Errorf("a flaw nobody has announced was recorded as %q", row.Visibility)
		}
		if row.OpenedRunID != nil {
			t.Errorf("a finding a person recorded claims run %d opened it", *row.OpenedRunID)
		}
		if row.OpenedAt.IsZero() {
			t.Error("a recorded finding does not say when it opened")
		}
		// On the clock like anything else. A finding that expired differently
		// because a person typed it would be a second policy nobody chose.
		if row.DueAt == nil {
			t.Error("a recorded finding carries no deadline")
		}

		// The next scan says nothing about it, and leaves it alone.
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		var open int
		if err := f.db.DB.NewSelect().Model((*finding.Finding)(nil)).
			Where("kind = ?", finding.Entered).Where("closed_run_id IS NULL").
			ColumnExpr("COUNT(*)").Scan(t.Context(), &open); err != nil {
			t.Fatal(err)
		}
		if open != 1 {
			t.Errorf("%d recorded findings are open after a scan, want 1", open)
		}

		// The second one this year counts on from the first.
		_, next, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Severity: "medium",
			Summary: "The recovery console does not clear the previous session.",
		})
		if err != nil {
			t.Fatal(err)
		}
		if next != "SONIC-2026-0002" {
			t.Errorf("the second identifier this year is %q", next)
		}
	})
}

func TestRecordingAnUndisclosedFlawNeedsThePrivateRight(t *testing.T) {
	// Somebody who may argue about known issues in shipped components has not
	// been handed the ones nobody has announced. The two rights are separate
	// for exactly this, and recording is the act that creates one.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		entering := finding.Entering{
			TargetID: f.target, Severity: "high", Summary: "Something we have not announced.",
		}

		if _, _, err := f.store.Enter(t.Context(), f.planner(t, access.PublicTriage),
			entering); err == nil {
			t.Error("somebody holding only public triage recorded an undisclosed flaw")
		}
		if _, _, err := f.store.Enter(t.Context(), f.planner(t, access.PublicRead),
			entering); err == nil {
			t.Error("somebody who may only read recorded a flaw")
		}
		if _, _, err := f.store.Enter(t.Context(), f.planner(t, access.PrivateTriage),
			entering); err != nil {
			t.Errorf("somebody holding private triage could not record one: %v", err)
		}

		// Already public is the other case, and it asks for the ordinary right.
		disclosed := entering
		disclosed.Disclosed = true
		disclosed.Summary = "Something already announced."
		if _, _, err := f.store.Enter(t.Context(), f.planner(t, access.PublicTriage),
			disclosed); err != nil {
			t.Errorf("a disclosed finding needed the private right: %v", err)
		}
	})
}

func TestARecordedFlawSaysWhatItIsAndWhereItIs(t *testing.T) {
	// A row with no summary is a row nobody can act on, and one hung off a
	// component the build does not contain is a claim about somebody else's
	// software.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		who := f.planner(t, access.PrivateTriage)

		// Whitespace passes a minimum length and is not a summary, so this
		// arrives from a request rather than only from inside this process —
		// which is why it is a sentinel rather than a sentence, and why the
		// caller is told to fix it rather than that something went wrong here.
		if _, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Severity: "high", Summary: "   ",
		}); !errors.Is(err, finding.ErrNothingSaid) {
			t.Errorf("a finding with nothing said about it was recorded: %v", err)
		}
		if _, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Severity: "urgent", Summary: "Something.",
		}); err == nil {
			t.Error("a severity that is not one was accepted")
		}
		_, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Component: "not-in-this-build", Severity: "high",
			Summary: "Something.",
		})
		if !errors.Is(err, finding.ErrNoSuchComponent) {
			t.Errorf("a component the build does not hold was accepted: %v", err)
		}

		// Naming nothing puts it on the build itself, which is the honest
		// answer where the flaw is in how the pieces fit together.
		row, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Severity: "high",
			Summary: "The pieces are wired together wrongly.",
		})
		if err != nil {
			t.Fatal(err)
		}
		var name string
		if err := f.db.DB.NewSelect().TableExpr("component AS c").
			ColumnExpr("c.name").Where("c.id = ?", row.ComponentID).
			Scan(t.Context(), &name); err != nil {
			t.Fatal(err)
		}
		if name != root.Name {
			t.Errorf("a flaw naming no component hangs off %q, want the build itself", name)
		}
	})
}

func TestRecordingAgainstANameTheBuildHoldsTwiceIsRefusedRatherThanGuessed(t *testing.T) {
	// A name is not unique within a build, and not rarely. This lookup took
	// the first row a name matched, so a flaw recorded against one of several
	// vendored versions of a library was filed against whichever had been
	// interned first, with nothing saying which — the same guess that was
	// measured wrong when a real image shipped three of one library and two of
	// the three findings answered about a version nobody asked about.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, graph.Snapshot{
			Root:       root,
			Components: []graph.Described{swss, libnl, libnlNew},
			Dependencies: []graph.Dependency{
				{Parent: root, Child: swss},
				{Parent: swss, Child: libnl},
				{Parent: swss, Child: libnlNew},
			},
		})
		who := f.planner(t, access.PrivateTriage)
		const said = "The parser accepts a message it should refuse."

		_, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Component: libnl.Name, Severity: "high", Summary: said,
		})
		var several *graph.Ambiguous
		if !errors.As(err, &several) {
			t.Fatalf("a name held at two versions resolved to one of them: %v", err)
		}
		// The versions, not only the fact. "Say which one" is not answerable
		// by somebody who does not know what the choices are.
		if got := several.Versions(); len(got) != 2 {
			t.Errorf("the refusal offers %v, want both versions", got)
		}

		// Naming the version settles it, and it settles it on the one named
		// rather than on whichever came first.
		row, _, err := f.store.Enter(t.Context(), who, finding.Entering{
			TargetID: f.target, Component: libnl.Name, Version: libnlNew.Version,
			Severity: "high", Summary: said,
		})
		if err != nil {
			t.Fatalf("naming the version: %v", err)
		}
		var version string
		if err := f.db.DB.NewSelect().TableExpr("component AS c").
			ColumnExpr("c.version").Where("c.id = ?", row.ComponentID).
			Scan(t.Context(), &version); err != nil {
			t.Fatal(err)
		}
		if version != libnlNew.Version {
			t.Errorf("recorded against %s, want the version that was named", version)
		}
	})
}
