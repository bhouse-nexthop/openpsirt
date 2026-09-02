package finding_test

import (
	"testing"

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

		inside, insideTotal, err := f.store.Groups(t.Context(), who, f.target, 50, 0,
			finding.Filter{Under: "swss"})
		if err != nil {
			t.Fatal(err)
		}
		if insideTotal >= total {
			t.Errorf("naming one consumer narrowed nothing: %d of %d", insideTotal, total)
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
