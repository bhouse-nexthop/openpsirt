package finding_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// assigned reads who holds each open finding of this target.
func (f *fixture) assigned(t *testing.T) map[int64]int64 {
	t.Helper()
	held := map[int64]int64{}
	for _, row := range f.open(t) {
		if row.AssignedTo != nil {
			held[row.ID] = *row.AssignedTo
		}
	}
	return held
}

func TestAssigningCoversEveryPlaceOfOneIssue(t *testing.T) {
	// A group is one issue in one component, seen from several parents.
	// Assigning one of its places and not another is not something anybody
	// means to do, so the whole group moves together.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		if len(open) != 2 {
			t.Fatalf("expected one issue at two places, got %d", len(open))
		}

		moved, err := f.store.Assign(t.Context(), f.holding(t, access.PublicTriage),
			f.target, open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7)))
		if err != nil {
			t.Fatal(err)
		}
		if moved != 2 {
			t.Errorf("assigning covered %d places, want both", moved)
		}
		for id, who := range f.assigned(t) {
			if who != 7 {
				t.Errorf("finding %d went to %d", id, who)
			}
		}
	})
}

func TestHandingSomethingBackIsTheSameActionAsGivingItOut(t *testing.T) {
	// Taking work back is not a different kind of act, and making it one
	// produces two paths that drift apart.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		who := f.holding(t, access.PublicTriage)

		if _, err := f.store.Assign(t.Context(), who, f.target,
			open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7))); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Assign(t.Context(), who, f.target,
			open[0].VulnerabilityID, open[0].ComponentID, nil); err != nil {
			t.Fatal(err)
		}
		if held := f.assigned(t); len(held) != 0 {
			t.Errorf("%d findings are still assigned after being handed back", len(held))
		}
	})
}

func TestWhenSomebodyGoesTheirWorkComesBack(t *testing.T) {
	// The case this exists for. Work assigned to somebody who has gone is in
	// no view at all — not in the shared list because it is assigned, and not
	// in anybody's own list because they are not here.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl), found("CVE-2026-2", swss)}); err != nil {
			t.Fatal(err)
		}
		triager := f.holding(t, access.PublicTriage)
		for _, row := range f.open(t) {
			if _, err := f.store.Assign(t.Context(), triager, f.target,
				row.VulnerabilityID, row.ComponentID, ptr(int64(7))); err != nil {
				t.Fatal(err)
			}
		}
		if len(f.assigned(t)) == 0 {
			t.Fatal("nothing was assigned to begin with")
		}

		// Only an administrator moves work somebody else was given.
		if _, err := f.store.Release(t.Context(), triager, 7); err == nil {
			t.Error("a triager released somebody else's work")
		}

		admin := access.NewPerson(99, "admin", true, nil)
		released, err := f.store.Release(t.Context(), admin, 7)
		if err != nil {
			t.Fatal(err)
		}
		if released == 0 {
			t.Error("releasing an absent person's work moved nothing")
		}
		if held := f.assigned(t); len(held) != 0 {
			t.Errorf("%d findings are still held by somebody who has gone", len(held))
		}
	})
}

func TestWorkCanBeHandedToSomebodyElse(t *testing.T) {
	// The other answer, and a different question: handing over says who is
	// dealing with it now, where releasing says nobody is.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		if _, err := f.store.Assign(t.Context(), f.holding(t, access.PublicTriage), f.target,
			open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7))); err != nil {
			t.Fatal(err)
		}

		admin := access.NewPerson(99, "admin", true, nil)
		if _, err := f.store.HandOver(t.Context(), admin, 7, 7); err == nil {
			t.Error("work was handed to the person who already had it")
		}
		if _, err := f.store.HandOver(t.Context(), admin, 7, 8); err != nil {
			t.Fatal(err)
		}
		for id, who := range f.assigned(t) {
			if who != 8 {
				t.Errorf("finding %d is held by %d after being handed on", id, who)
			}
		}
	})
}

func TestSomebodyWhoMayNotSeeAFindingCannotAssignIt(t *testing.T) {
	// Assignment is narrowed the way every other query is. A finding nobody
	// has disclosed is not one somebody may hand around.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)

		outsider := access.NewPerson(5, "outsider", false, nil)
		if _, err := f.store.Assign(t.Context(), outsider, f.target,
			open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7))); err == nil {
			t.Error("somebody holding nothing assigned a finding")
		}
	})
}

func ptr[T any](v T) *T { return &v }

func TestWorkComesBackWhenTheirLastRoleOnAProductGoes(t *testing.T) {
	// ACC-43, the half that has a trigger today. Without it their work is in
	// no list at all — assigned, so not in the shared one, and assigned to
	// somebody who can no longer open it.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		triager := f.holding(t, access.PublicTriage)
		if _, err := f.store.Assign(t.Context(), triager, f.target,
			open[0].VulnerabilityID, open[0].ComponentID, ptr(int64(7))); err != nil {
			t.Fatal(err)
		}

		// Administrative, like every other move of somebody else's work.
		if _, err := f.store.ReleaseIn(t.Context(), triager, 7, f.productID); err == nil {
			t.Error("a triager handed back somebody else's work")
		}

		admin := access.NewPerson(99, "admin", true, nil)
		// Another product's findings are not this product's business.
		released, err := f.store.ReleaseIn(t.Context(), admin, 7, f.productID+1000)
		if err != nil {
			t.Fatal(err)
		}
		if released != 0 {
			t.Errorf("handing back in one product moved %d findings in another", released)
		}
		if len(f.assigned(t)) == 0 {
			t.Fatal("the work was released by a product it does not belong to")
		}

		released, err = f.store.ReleaseIn(t.Context(), admin, 7, f.productID)
		if err != nil {
			t.Fatal(err)
		}
		if released == 0 {
			t.Error("handing back in this product moved nothing")
		}
		if held := f.assigned(t); len(held) != 0 {
			t.Errorf("%d findings are still held by somebody with no role here", len(held))
		}
	})
}

func TestTheCountsBesideTheseListsAreAskedForOnEveryEngine(t *testing.T) {
	// Both lists carry a total, and both totals come from a derived table
	// wrapped around the grouped query. That shape is the one that breaks
	// per-engine: the obvious alias for it, "groups", is a reserved word in
	// MySQL 8 — it names a window frame type — so the statement parses on
	// three engines and is a syntax error on the fourth (DAT-33).
	//
	// Nothing else reaches these two, so without this the first anybody knew
	// was a 500 from a handler that had swallowed the driver's message.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl), found("CVE-2026-2", swss)}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

		nobodys, total, err := f.store.Unassigned(t.Context(), who, finding.Scope{}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		// Two issues, one of them at two places. The total counts issues at
		// components, not finding rows, which is what the list shows.
		if total != 2 || len(nobodys) != 2 {
			t.Errorf("%d rows and a total of %d, want 2 and 2", len(nobodys), total)
		}

		// Named, not whichever row came back first. Two components carry
		// findings here and they hold different numbers of places, so
		// indexing into an unordered read makes this assert a different
		// thing on different runs.
		library, err := graph.NewStore(f.db.DB).ComponentAt(t.Context(), f.target, libnl.Name)
		if err != nil {
			t.Fatal(err)
		}
		at, atTotal, err := f.store.AtComponent(t.Context(), who, f.target,
			library, "", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if atTotal != 1 {
			t.Errorf("the component holds a total of %d issues, want 1", atTotal)
		}
		// Every place of it, because that is what a claim is written against.
		if len(at) != 2 {
			t.Errorf("one issue at that component came back as %d places, want 2", len(at))
		}
	})
}
