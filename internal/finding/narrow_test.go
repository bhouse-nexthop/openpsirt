package finding_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

// The filters that narrow a findings list by something other than how bad an
// issue is: what kind of package carries it, what holds that package, and how
// far it has been decided.
//
// Every one of them narrows the page *and* the total through the same clause,
// which is what keeps the number beside a list from counting something else
// (REJ-10) — so each case below asserts both.
func TestNarrowingByPackageKind(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl), found("CVE-2026-2", swss), found("CVE-2026-3", teamd),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

		all, total, err := f.store.Groups(t.Context(), who, f.target, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if total == 0 || len(all) == 0 {
			t.Fatal("wanted findings to narrow")
		}

		// The fixture's components are Debian packages, so the kind they are
		// keeps everything and a kind they are not keeps nothing. Both
		// directions, because a filter that never excludes and a filter that
		// excludes everything look the same from one of them.
		kept, keptTotal, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{Ecosystem: "deb"})
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != len(all) || keptTotal != total {
			t.Errorf("asking for the kind they are kept %d of %d (total %d of %d)",
				len(kept), len(all), keptTotal, total)
		}

		none, noneTotal, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{Ecosystem: "golang"})
		if err != nil {
			t.Fatal(err)
		}
		if len(none) != 0 || noneTotal != 0 {
			t.Errorf("asking for a kind nothing is kept %d rows and counted %d",
				len(none), noneTotal)
		}
	})
}

func TestNarrowingByWhatHoldsIt(t *testing.T) {
	// A place records what pulls a component in, so "what is inside this
	// container" is a question about consumers — and what the build holds
	// directly is the places that have none. The two together are every place,
	// which is what makes them worth asserting against each other.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl), found("CVE-2026-2", swss), found("CVE-2026-3", teamd),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

		_, total, err := f.store.Groups(t.Context(), who, f.target, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}

		// The consumer's own name. `swss` is what the Go variable is called;
		// the component is `libswsscommon`, and filtering on the variable's
		// name matched nothing — which passed every assertion below for the
		// wrong reason, because "narrowed to a subset" and "narrowed to
		// nothing" are both fewer than everything.
		inside, insideTotal, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{Under: swss.Name})
		if err != nil {
			t.Fatal(err)
		}
		if insideTotal >= total || insideTotal == 0 {
			t.Errorf("naming one consumer kept %d of %d, want some but not all",
				insideTotal, total)
		}
		if insideTotal != len(inside) {
			t.Errorf("the total counts %d and the page has %d; they are counted differently",
				insideTotal, len(inside))
		}

		// A consumer nothing is under keeps nothing rather than everything,
		// which is the failure a filter silently not applied would look like.
		_, absent, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{Under: "no-such-container"})
		if err != nil {
			t.Fatal(err)
		}
		if absent != 0 {
			t.Errorf("a consumer nothing sits under kept %d", absent)
		}

		// What the build holds directly is the other half. Together they are
		// every place, which is the assertion worth making: a filter that
		// quietly kept nothing would satisfy either half alone.
		_, direct, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{UnderTheBuild: true})
		if err != nil {
			t.Fatal(err)
		}
		if direct == 0 {
			t.Error("nothing reads as held by the build itself")
		}
	})
}

func TestAClaimInAnotherProductDoesNotDecideThisOne(t *testing.T) {
	// A place identity is a hash of a consumer and a component and carries no
	// product, and an issue is one row for the whole deployment — so anything
	// correlating a place to a decision without naming the product matches
	// every product there is.
	//
	// That is two failures at once. A reader is told a claim is pending in a
	// product they may not be able to see, and their own screen says "waiting"
	// when nothing here is waiting. This writes a real decision in a real
	// second product, at the same place and about the same issue, and asserts
	// it moves nothing here.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", swss),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

		// A second product, declared properly: the row has foreign keys to
		// product and person, so inventing numbers would fail the write rather
		// than test anything.
		elsewhere, err := catalog.NewStore(f.db.DB).DeclareProduct(ctx, "edge-router", "Edge")
		if err != nil {
			t.Fatal(err)
		}
		somebody, err := access.NewStore(f.db.DB).Ensure(ctx, "them@example.com", "Them", false)
		if err != nil {
			t.Fatal(err)
		}
		var issueID int64
		if err := f.db.DB.NewSelect().TableExpr("vulnerability AS v").
			Column("v.id").Where("v.identifier = ?", "CVE-2026-1").
			Scan(ctx, &issueID); err != nil {
			t.Fatal(err)
		}

		// The place a component sitting directly under the build hashes to,
		// which is the same string in every product — that being the point.
		place := finding.PlaceIdentity(swss.Name, "")
		if _, err := f.db.DB.NewInsert().
			Model(&map[string]any{
				"product_id":       elsewhere.ID,
				"vulnerability_id": issueID,
				"place_identity":   place,
				"visibility":       "public",
				"outcome":          "not-applicable",
				"state":            "proposed",
				"needs_approval":   true,
				"proposed_by":      somebody.ID,
				"proposed_at":      time.Now().UTC(),
			}).
			TableExpr("decision").Exec(ctx); err != nil {
			// A row this test cannot write is a reason to fail: skipping would
			// make it green in exactly the case where it proves nothing.
			t.Fatalf("could not record a claim in another product: %v", err)
		}

		_, waiting, err := f.store.Groups(ctx, who, f.target, 50, 0,
			finding.Filter{State: "waiting"})
		if err != nil {
			t.Fatal(err)
		}
		if waiting != 0 {
			t.Errorf("a claim in another product made %d rows here read as waiting", waiting)
		}
		_, undecided, err := f.store.Groups(ctx, who, f.target, 50, 0,
			finding.Filter{State: "undecided"})
		if err != nil {
			t.Fatal(err)
		}
		if undecided != 1 {
			t.Errorf("%d rows read as undecided, want the 1 that nobody here has decided",
				undecided)
		}
	})
}

func TestNarrowingByHowFarDecided(t *testing.T) {
	// A group covers every place an issue sits at in one component, so its
	// state is a statement about all of them: undecided means no place has a
	// decision, not that one of them does not.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl), found("CVE-2026-2", swss),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

		_, total, err := f.store.Groups(t.Context(), who, f.target, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}

		// Nothing has been decided, so every group is undecided and none is
		// answered. Both asserted: "undecided keeps everything" alone is what
		// a clause that never runs also looks like.
		_, undecided, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{State: "undecided"})
		if err != nil {
			t.Fatal(err)
		}
		if undecided != total {
			t.Errorf("nothing is decided, so undecided should be all %d, got %d",
				total, undecided)
		}
		for _, state := range []string{"agreed", "waiting", "lapsed"} {
			_, n, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
				finding.Filter{State: state})
			if err != nil {
				t.Fatalf("%s: %v", state, err)
			}
			if n != 0 {
				t.Errorf("nothing is decided, so %q should keep none, got %d", state, n)
			}
		}
	})
}
