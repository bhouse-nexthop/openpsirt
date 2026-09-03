package finding_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// withoutLibnl is the same shape with the library gone, so an issue at it
// closes as removed rather than as anything else.
func withoutLibnl() graph.Snapshot {
	return graph.Snapshot{
		Root: root, Components: []graph.Described{swss, teamd},
		Dependencies: []graph.Dependency{
			{Parent: root, Child: swss}, {Parent: root, Child: teamd},
		},
	}
}

// entries indexes a comparison's rows by issue, so an assertion names what it
// is about rather than an ordinal.
func entries(rows []finding.Changed) map[string]finding.Changed {
	at := map[string]finding.Changed{}
	for _, row := range rows {
		at[row.Vulnerability] = row
	}
	return at
}

func TestComparingTwoBuildsSaysWhatWentWhatCameAndWhatStayed(t *testing.T) {
	// What a release note is drawn from. The three groups are the whole of the
	// answer, and each fixed entry says why it went: "fixed by an upgrade" and
	// "the component is gone" are different things to whoever reads the note.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()

		// The release a customer is on: two issues, one at the library and one
		// at the daemon.
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-GONE", libnl),
			found("CVE-2026-STAYS", swss),
		}); err != nil {
			t.Fatal(err)
		}
		earlier := f.target

		// The release they would move to. It carried the same two, and then
		// the library was dropped and a new issue appeared at the other
		// daemon.
		later := f.anotherBuild(t, "v2")
		f.shippedTo(t, later, twoConsumers())
		if _, err := f.store.Apply(ctx, later, f.runOn(t, later), []finding.Reported{
			found("CVE-2026-GONE", libnl),
			found("CVE-2026-STAYS", swss),
		}); err != nil {
			t.Fatal(err)
		}
		f.shippedTo(t, later, withoutLibnl())
		if _, err := f.store.Apply(ctx, later, f.runOn(t, later), []finding.Reported{
			found("CVE-2026-STAYS", swss),
			found("CVE-2026-NEW", teamd),
		}); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicRead)
		comparison, err := f.store.Compare(ctx, who, earlier, later, false)
		if err != nil {
			t.Fatal(err)
		}

		fixed := entries(comparison.Fixed)
		if len(fixed) != 1 {
			t.Fatalf("%d entries were fixed, want 1: %+v", len(fixed), comparison.Fixed)
		}
		gone, listed := fixed["CVE-2026-GONE"]
		if !listed {
			t.Fatalf("the issue that went is not in the fixed list: %+v", comparison.Fixed)
		}
		// Read from the row that closed in the *later* build. The earlier
		// build's row is still open in its own history, so the explanation
		// cannot come from there.
		if gone.Because != finding.Removed {
			t.Errorf("it went because %q, want %q", gone.Because, finding.Removed)
		}

		if newly := entries(comparison.Newly); len(newly) != 1 {
			t.Errorf("%d entries are new, want 1: %+v", len(newly), comparison.Newly)
		} else if _, listed := newly["CVE-2026-NEW"]; !listed {
			t.Errorf("the new issue is not in the new list: %+v", comparison.Newly)
		}

		if still := entries(comparison.Still); len(still) != 1 {
			t.Errorf("%d entries are still there, want 1: %+v", len(still), comparison.Still)
		} else if _, listed := still["CVE-2026-STAYS"]; !listed {
			t.Errorf("the issue that stayed is not in the still list: %+v", comparison.Still)
		}
	})
}

func TestWhyEveryFixedEntryWentIsReadWithoutAQueryEach(t *testing.T) {
	// A comparison against the release a customer has been on for a year has
	// as many fixed entries as the note is long, and asking about each
	// separately makes the screen's cost a count of round trips rather than a
	// count of rows.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		const issues = 40

		var carried []finding.Reported
		for i := range issues {
			carried = append(carried, found(vulnerability(i), libnl))
		}
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), carried); err != nil {
			t.Fatal(err)
		}
		earlier := f.target

		later := f.anotherBuild(t, "v2")
		f.shippedTo(t, later, twoConsumers())
		if _, err := f.store.Apply(ctx, later, f.runOn(t, later), carried); err != nil {
			t.Fatal(err)
		}
		f.shippedTo(t, later, withoutLibnl())
		if _, err := f.store.Apply(ctx, later, f.runOn(t, later), nil); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicRead)
		count := &counter{}
		f.db.AddQueryHook(count)
		comparison, err := f.store.Compare(ctx, who, earlier, later, false)
		if err != nil {
			t.Fatal(err)
		}
		statements := count.queries.Load()

		if len(comparison.Fixed) != issues {
			t.Fatalf("%d entries were fixed, want %d", len(comparison.Fixed), issues)
		}
		for _, row := range comparison.Fixed {
			if row.Because != finding.Removed {
				t.Errorf("%s went because %q, want %q", row.Vulnerability, row.Because, finding.Removed)
			}
		}
		// Authorization for each build, the two builds' open rows, and one
		// read for the explanations. A statement per fixed entry would be
		// forty more.
		if statements > int64(issues) {
			t.Errorf("comparing %d fixed entries took %d statements, which is a query each",
				issues, statements)
		}
	})
}

func TestComparingBuildsAuthorizesBoth(t *testing.T) {
	// The earlier build as well as the later one. Authorizing only the target
	// somebody asked to compare *to* and applying that answer to the other
	// lets a reader who can reach one product read findings out of another
	// through the comparison — which is why this is enforced here rather than
	// in the handler.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t),
			[]finding.Reported{found("CVE-2026-1", libnl)}); err != nil {
			t.Fatal(err)
		}
		mine := f.target

		// A build of a product this reader holds nothing on.
		theirs := f.inAnotherProduct(t, "somebody-elses")
		f.shippedTo(t, theirs, twoConsumers())
		if _, err := f.store.Apply(ctx, theirs, f.runOn(t, theirs),
			[]finding.Reported{found("CVE-2026-2", libnl)}); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicRead)
		if _, err := f.store.Compare(ctx, who, theirs, mine, false); err == nil {
			t.Error("comparing from a build in a product this reader holds nothing on was allowed")
		}
		if _, err := f.store.Compare(ctx, who, mine, theirs, false); err == nil {
			t.Error("comparing to a build in a product this reader holds nothing on was allowed")
		}
		// And the pair they may read is allowed, so the checks above are not
		// passing because everything is refused.
		second := f.anotherBuild(t, "v2")
		f.shippedTo(t, second, twoConsumers())
		if _, err := f.store.Apply(ctx, second, f.runOn(t, second), nil); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Compare(ctx, who, mine, second, false); err != nil {
			t.Errorf("comparing two builds this reader may read was refused: %v", err)
		}
	})
}

func TestAComparisonLeavesOutWhatIsNotDisclosed(t *testing.T) {
	// Its destination is usually a public document, so including something
	// undisclosed is a deliberate act rather than something somebody pastes in
	// without noticing.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		f.shipped(t, twoConsumers())
		if _, err := f.store.Apply(ctx, f.target, f.run(t), []finding.Reported{
			found("CVE-2026-OPEN", swss),
			found("CVE-2026-QUIET", teamd),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.DB.NewUpdate().Model((*finding.Finding)(nil)).
			Set("visibility = ?", access.Private).
			Where("vulnerability_id = ?", f.issueID(t, "CVE-2026-QUIET")).
			Exec(ctx); err != nil {
			t.Fatal(err)
		}
		earlier := f.target

		later := f.anotherBuild(t, "v2")
		f.shippedTo(t, later, twoConsumers())
		if _, err := f.store.Apply(ctx, later, f.runOn(t, later), nil); err != nil {
			t.Fatal(err)
		}

		who := f.holding(t, access.PublicRead, access.PrivateRead)
		public, err := f.store.Compare(ctx, who, earlier, later, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, listed := entries(public.Fixed)["CVE-2026-QUIET"]; listed {
			t.Error("an undisclosed finding is in a comparison nobody asked to include one in")
		}
		if _, listed := entries(public.Fixed)["CVE-2026-OPEN"]; !listed {
			t.Error("a public finding is missing, so the check above proves nothing")
		}

		asked, err := f.store.Compare(ctx, who, earlier, later, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, listed := entries(asked.Fixed)["CVE-2026-QUIET"]; !listed {
			t.Error("a reader who may see it and asked for it did not get it")
		}
	})
}
