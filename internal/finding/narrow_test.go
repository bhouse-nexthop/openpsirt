package finding_test

import (
	"context"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
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

		all, total, err := f.store.Groups(t.Context(), who, f.scope, 50, 0, finding.Filter{})
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
		kept, keptTotal, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
			finding.Filter{Ecosystem: "deb"})
		if err != nil {
			t.Fatal(err)
		}
		if len(kept) != len(all) || keptTotal != total {
			t.Errorf("asking for the kind they are kept %d of %d (total %d of %d)",
				len(kept), len(all), keptTotal, total)
		}

		none, noneTotal, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
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

		_, total, err := f.store.Groups(t.Context(), who, f.scope, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}

		// The consumer's own name. `swss` is what the Go variable is called;
		// the component is `libswsscommon`, and filtering on the variable's
		// name matched nothing — which passed every assertion below for the
		// wrong reason, because "narrowed to a subset" and "narrowed to
		// nothing" are both fewer than everything.
		inside, insideTotal, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
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
		_, absent, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
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
		_, direct, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
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
		// Live, and keyed on the version shipping here, so that the product
		// is the only thing keeping it from standing: a claim that failed to
		// match on its versions would leave a missing product condition
		// unexercised.
		place := finding.PlaceIdentity(swss.Name, "")
		if _, err := f.db.DB.NewInsert().
			Model(&map[string]any{
				"claim_id":                   claimBy(t, f.db, somebody.ID),
				"product_id":                 elsewhere.ID,
				"vulnerability_id":           issueID,
				"place_identity":             place,
				"visibility":                 "public",
				"outcome":                    "not-applicable",
				"state":                      "proposed",
				"needs_approval":             true,
				"proposed_by":                somebody.ID,
				"proposed_at":                time.Now().UTC(),
				"component_upstream_version": swss.Version,
				"live_key":                   "elsewhere-live-key",
			}).
			TableExpr("decision").Exec(ctx); err != nil {
			// A row this test cannot write is a reason to fail: skipping would
			// make it green in exactly the case where it proves nothing.
			t.Fatalf("could not record a claim in another product: %v", err)
		}

		_, waiting, err := f.store.Groups(ctx, who, f.scope, 50, 0,
			finding.Filter{State: "waiting"})
		if err != nil {
			t.Fatal(err)
		}
		if waiting != 0 {
			t.Errorf("a claim in another product made %d rows here read as waiting", waiting)
		}
		_, undecided, err := f.store.Groups(ctx, who, f.scope, 50, 0,
			finding.Filter{State: "undecided"})
		if err != nil {
			t.Fatal(err)
		}
		if undecided != 1 {
			t.Errorf("%d rows read as undecided, want the 1 that nobody here has decided",
				undecided)
		}

		// And the finding's own screen, which names the decision standing at
		// each place, names nothing: it showed the other product's claim as
		// standing here, an identifier the reader could not open.
		open := f.open(t)
		evidence, err := f.store.Detail(ctx, who, f.target, open[0].VulnerabilityID, open[0].ComponentID)
		if err != nil {
			t.Fatal(err)
		}
		for _, sitting := range evidence.Places {
			if sitting.Decision != nil {
				t.Errorf("a claim in another product reads as decision %d standing here",
					*sitting.Decision)
			}
		}
	})
}

func TestALapsedPlaceDecidedAgainReadsAsWaiting(t *testing.T) {
	// Lapsed means a decision here stopped applying and nothing replaced it.
	// Once somebody makes the claim again, the place is waiting — which is
	// what the row said, while the filter still listed it under lapsed: it
	// asked only that something had lapsed and nothing was approved. A filter
	// has to find a row by the word the row reads.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", swss),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)
		somebody, err := access.NewStore(f.db.DB).Ensure(ctx, "them@example.com", "Them", false)
		if err != nil {
			t.Fatal(err)
		}
		place := finding.PlaceIdentity(swss.Name, "")
		f.decided(t, somebody.ID, f.issueID(t, "CVE-2026-1"), place, "lapsed", "0.9.0", "")
		f.decided(t, somebody.ID, f.issueID(t, "CVE-2026-1"), place, "proposed", swss.Version, "again")

		groups, _, err := f.store.Groups(ctx, who, f.scope, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(groups) != 1 || groups[0].State != "waiting" {
			t.Fatalf("a lapsed place claimed again reads as %+v, want one row waiting", groups)
		}
		_, lapsed, err := f.store.Groups(ctx, who, f.scope, 50, 0, finding.Filter{State: "lapsed"})
		if err != nil {
			t.Fatal(err)
		}
		if lapsed != 0 {
			t.Errorf("lapsed kept %d rows that read as waiting", lapsed)
		}
		_, waiting, err := f.store.Groups(ctx, who, f.scope, 50, 0, finding.Filter{State: "waiting"})
		if err != nil {
			t.Fatal(err)
		}
		if waiting != 1 {
			t.Errorf("waiting kept %d, want the 1 row that reads as waiting", waiting)
		}
	})
}

func TestALiveDecisionCoversOnlyTheVersionsItWasKeyedOn(t *testing.T) {
	// A decision is a claim about a place at the versions it was keyed on,
	// and it lapses by those versions moving. Two builds of one product can
	// ship the same place at different versions, and the one that moved is
	// exactly the one the decision no longer covers — everything that asks
	// whether a decision applies says so, and the list's state, its filter and
	// the finding's screen were matching by place alone and saying "agreed".
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		directly := func(library graph.Described) graph.Snapshot {
			return graph.Snapshot{
				Root: root, Components: []graph.Described{library},
				Dependencies: []graph.Dependency{{Parent: root, Child: library}},
			}
		}
		f.shipped(t, directly(libnl))
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", libnl),
		}); err != nil {
			t.Fatal(err)
		}
		other := f.anotherBuild(t, "202411")
		f.shippedTo(t, other, directly(libnlNew))
		if _, err := f.store.Apply(ctx, other, f.runOn(t, other), []finding.Reported{
			found("CVE-2026-1", libnlNew),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)
		somebody, err := access.NewStore(f.db.DB).Ensure(ctx, "them@example.com", "Them", false)
		if err != nil {
			t.Fatal(err)
		}
		// Approved against the older version, in the build that ships it.
		f.decided(t, somebody.ID, f.issueID(t, "CVE-2026-1"),
			finding.PlaceIdentity(libnl.Name, ""), "approved", libnl.Version, "old")

		reads := func(target int64, want string) {
			t.Helper()
			groups, _, err := f.store.Groups(ctx, who, f.scopeOf(t, target), 50, 0, finding.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 1 || groups[0].State != want {
				t.Errorf("build %d reads as %+v, want one row %q", target, groups, want)
			}
			_, agreed, err := f.store.Groups(ctx, who, f.scopeOf(t, target), 50, 0, finding.Filter{State: "agreed"})
			if err != nil {
				t.Fatal(err)
			}
			if kept := want == "agreed"; (agreed == 1) != kept {
				t.Errorf("build %d: agreed kept %d rows, want %v", target, agreed, kept)
			}
		}
		decided := func(target int64) int {
			t.Helper()
			var open []finding.Finding
			if err := f.db.DB.NewSelect().Model(&open).
				Where("target_id = ?", target).Where("closed_at IS NULL").Scan(ctx); err != nil {
				t.Fatal(err)
			}
			evidence, err := f.store.Detail(ctx, who, target, open[0].VulnerabilityID, open[0].ComponentID)
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.Places) != 1 {
				t.Fatalf("build %d sits at %d places, want 1", target, len(evidence.Places))
			}
			n := 0
			for _, sitting := range evidence.Places {
				if sitting.Decision != nil {
					n++
				}
			}
			return n
		}

		reads(f.target, "agreed")
		if n := decided(f.target); n != 1 {
			t.Errorf("the build the decision was made against shows %d of 1 places decided", n)
		}
		reads(other, "undecided")
		if n := decided(other); n != 0 {
			t.Errorf("the build that moved on shows %d of 1 places decided, want 0", n)
		}
	})
}

// decided writes one decision of this product at a place, keyed on a
// component version, as the triage store would have written it. Live where
// the state is one a live claim can hold; a lapsed or withdrawn row holds no
// key, which is what says it no longer applies.
func (f *fixture) decided(t *testing.T, by, issueID int64, place, state, componentVersion, key string) {
	t.Helper()
	row := map[string]any{
		"claim_id":   claimBy(t, f.db, by),
		"product_id": f.productID, "vulnerability_id": issueID,
		"place_identity": place, "visibility": "public",
		"outcome": "not-applicable", "state": state,
		"needs_approval": true, "proposed_by": by,
		"proposed_at":                time.Now().UTC(),
		"component_upstream_version": componentVersion,
	}
	if key != "" {
		// Short, and not the place: the column holds sixty-four characters,
		// which a place identity fills on its own.
		row["live_key"] = key
	}
	if _, err := f.db.DB.NewInsert().Model(&row).TableExpr("decision").Exec(t.Context()); err != nil {
		t.Fatalf("record a %s claim: %v", state, err)
	}
}

func TestEachDecisionStateSelectsWhatItNames(t *testing.T) {
	// The states asserted positively rather than by all being empty.
	//
	// Nothing-is-decided is a fixture where "correct" and "always false" look
	// identical, so three of the four states were pinned by a condition that
	// could have been anything. This records a claim at a place and walks it:
	// proposed reads as waiting, approved and live reads as agreed, and a
	// claim that stopped applying reads as lapsed and not as agreed.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-1", swss),
		}); err != nil {
			t.Fatal(err)
		}
		who := f.holding(t, access.PublicTriage)

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
		place := finding.PlaceIdentity(swss.Name, "")

		record := func(state string, live bool) {
			t.Helper()
			if _, err := f.db.DB.NewDelete().Table("decision").
				Where("vulnerability_id = ?", issueID).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			row := map[string]any{
				"claim_id":   claimBy(t, f.db, somebody.ID),
				"product_id": f.productID, "vulnerability_id": issueID,
				"place_identity": place, "visibility": "public",
				"outcome": "not-applicable", "state": state,
				"needs_approval": true, "proposed_by": somebody.ID,
				"proposed_at": time.Now().UTC(),
			}
			if live {
				// What makes a claim the one standing here: a key, and the
				// version the claim was made against being the one shipping.
				row["live_key"] = state + "-live-key"
				row["component_upstream_version"] = swss.Version
			}
			if _, err := f.db.DB.NewInsert().Model(&row).
				TableExpr("decision").Exec(ctx); err != nil {
				t.Fatalf("record a %s claim: %v", state, err)
			}
		}
		count := func(state string) int {
			t.Helper()
			_, n, err := f.store.Groups(ctx, who, f.scope, 50, 0, finding.Filter{State: state})
			if err != nil {
				t.Fatalf("%s: %v", state, err)
			}
			return n
		}
		// The row says the same word the filter would find it by, from the
		// same counts in the same statement.
		said := func(want string) {
			t.Helper()
			groups, _, err := f.store.Groups(ctx, who, f.scope, 50, 0, finding.Filter{})
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range groups {
				if group.Vulnerability == "CVE-2026-1" && group.State != want {
					t.Errorf("the row reads as %q, want %q", group.State, want)
				}
			}
		}

		said("undecided")
		// A proposed row that holds no key is a shape nothing writes — a
		// proposal is live until it is withdrawn or lapses, and both change
		// its state — and it is none of the four words, for the row as for
		// the filter.
		record("proposed", false)
		said("")
		record("proposed", true)
		said("waiting")
		if n := count("waiting"); n != 1 {
			t.Errorf("a proposed claim: waiting kept %d, want 1", n)
		}
		if n := count("undecided"); n != 0 {
			t.Errorf("a proposed claim: undecided kept %d, want 0", n)
		}
		if n := count("agreed"); n != 0 {
			t.Errorf("a proposed claim is not agreed, yet agreed kept %d", n)
		}

		record("approved", true)
		said("agreed")
		if n := count("agreed"); n != 1 {
			t.Errorf("an approved live claim: agreed kept %d, want 1", n)
		}
		if n := count("waiting"); n != 0 {
			t.Errorf("an approved claim is not waiting, yet waiting kept %d", n)
		}

		record("lapsed", false)
		said("lapsed")
		if n := count("lapsed"); n != 1 {
			t.Errorf("a lapsed claim: lapsed kept %d, want 1", n)
		}
		if n := count("agreed"); n != 0 {
			t.Errorf("a lapsed claim is not agreed, yet agreed kept %d", n)
		}

		// And a claim that was withdrawn long ago answers for nothing: it is
		// not live, so the place is undecided again.
		record("withdrawn", false)
		if n := count("agreed"); n != 0 {
			t.Errorf("a withdrawn claim read as agreed: %d", n)
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

		_, total, err := f.store.Groups(t.Context(), who, f.scope, 50, 0, finding.Filter{})
		if err != nil {
			t.Fatal(err)
		}

		// Nothing has been decided, so every group is undecided and none is
		// answered. Both asserted: "undecided keeps everything" alone is what
		// a clause that never runs also looks like.
		_, undecided, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
			finding.Filter{State: "undecided"})
		if err != nil {
			t.Fatal(err)
		}
		if undecided != total {
			t.Errorf("nothing is decided, so undecided should be all %d, got %d",
				total, undecided)
		}
		for _, state := range []string{"agreed", "waiting", "lapsed"} {
			_, n, err := f.store.Groups(t.Context(), who, f.scope, 50, 0,
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

// claimBy records the action a directly written decision belongs to. Every
// decision is one row of a claim, and a row written without one is refused.
func claimBy(t *testing.T, db *database.DB, personID int64) int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := db.DB.NewInsert().
		Model(&map[string]any{
			"kind": "finding", "proposed_by": personID, "proposed_at": time.Now().UTC(),
		}).
		TableExpr("claim").Exec(ctx); err != nil {
		t.Fatalf("record a claim: %v", err)
	}
	var id int64
	if err := db.DB.NewSelect().TableExpr("claim").ColumnExpr("MAX(id)").Scan(ctx, &id); err != nil {
		t.Fatalf("read the claim back: %v", err)
	}
	return id
}
