package finding_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

func TestRatingSomethingWorseTakesEffectAtOnce(t *testing.T) {
	// TRI-41. Nobody needs protecting from being told something is worse than
	// the world says, so raising is not gated — and it has to reach the order,
	// or it is a note nobody acts on.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		mild := found("CVE-2026-RAISE", swss)
		mild.Issue.Severity = "low"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{mild}); err != nil {
			t.Fatal(err)
		}
		before := f.urgency(t, "CVE-2026-RAISE")

		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		claim, err := f.store.Assess(t.Context(), who, f.issue(t, "CVE-2026-RAISE"),
			"critical", "Reachable from the network in how we ship it.")
		if err != nil {
			t.Fatal(err)
		}
		if claim.State != finding.AssessmentLive {
			t.Errorf("rating something worse is %s, want it in force at once", claim.State)
		}
		if claim.NeedsApproval {
			t.Error("rating something worse asked for a second person")
		}
		if after := f.urgency(t, "CVE-2026-RAISE"); after <= before {
			t.Errorf("the order did not move: %d then %d", before, after)
		}
		// And the published word is not overwritten by ours (TRI-42).
		published, assessed := f.ratings(t, "CVE-2026-RAISE")
		if published != "low" || assessed != "critical" {
			t.Errorf("ratings are published=%q assessed=%q, want low and critical",
				published, assessed)
		}
	})
}

func TestRatingSomethingMilderWaitsForSomebodyElse(t *testing.T) {
	// The direction that hides things, and it hides more than a position:
	// severity sets the deadline, so a downgrade pushes it out by months.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		bad := found("CVE-2026-LOWER", swss)
		bad.Issue.Severity = "critical"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{bad}); err != nil {
			t.Fatal(err)
		}
		before := f.urgency(t, "CVE-2026-LOWER")

		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		claim, err := f.store.Assess(t.Context(), who, f.issue(t, "CVE-2026-LOWER"),
			"low", "The affected feature is compiled out of our build.")
		if err != nil {
			t.Fatal(err)
		}
		if claim.State != finding.AssessmentProposed || !claim.NeedsApproval {
			t.Fatalf("a milder rating is %s and needs approval %v, want it waiting",
				claim.State, claim.NeedsApproval)
		}
		if after := f.urgency(t, "CVE-2026-LOWER"); after != before {
			t.Errorf("a claim nobody has agreed to moved the order: %d then %d", before, after)
		}

		// Nobody may agree with themselves.
		if _, err := f.store.Agree(t.Context(), who, claim.ID); err == nil {
			t.Error("the proposer agreed with themselves")
		}

		f.recorded(t, who.ID+1, "somebody-else")
		other := f.holding(t, access.PublicTriage)
		other.ID = who.ID + 1
		if _, err := f.store.Agree(t.Context(), other, claim.ID); err != nil {
			t.Fatal(err)
		}
		if after := f.urgency(t, "CVE-2026-LOWER"); after >= before {
			t.Errorf("agreeing did not move the order down: %d then %d", before, after)
		}
	})
}

func TestWithdrawingTakesThePublishedRatingBack(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		mild := found("CVE-2026-BACK", swss)
		mild.Issue.Severity = "low"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{mild}); err != nil {
			t.Fatal(err)
		}
		before := f.urgency(t, "CVE-2026-BACK")

		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		claim, err := f.store.Assess(t.Context(), who, f.issue(t, "CVE-2026-BACK"),
			"critical", "Worse than published.")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.Withdraw(t.Context(), who, claim.ID); err != nil {
			t.Fatal(err)
		}
		if after := f.urgency(t, "CVE-2026-BACK"); after != before {
			t.Errorf("withdrawing did not put the order back: %d then %d", before, after)
		}
		if _, assessed := f.ratings(t, "CVE-2026-BACK"); assessed != "" {
			t.Errorf("a withdrawn rating is still in force: %q", assessed)
		}
	})
}

func TestOneClaimStandsPerIssue(t *testing.T) {
	// A second claim about the same issue is a revision of the first rather
	// than a rival to it, which is the rule decisions already hold to.
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		run := f.run(t)
		one := found("CVE-2026-ONCE", swss)
		one.Issue.Severity = "medium"
		if _, err := f.store.Apply(t.Context(), f.target, run,
			[]finding.Reported{one}); err != nil {
			t.Fatal(err)
		}
		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		id := f.issue(t, "CVE-2026-ONCE")
		if _, err := f.store.Assess(t.Context(), who, id, "high", "Worse."); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Assess(t.Context(), who, id, "critical", "Worse again."); err == nil {
			t.Error("a second claim stood beside the first")
		}
	})
}
