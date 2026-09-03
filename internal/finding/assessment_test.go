package finding_test

import (
	"errors"
	"sync"
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
		second, err := f.store.Assess(t.Context(), who, id, "critical", "Worse again.")
		if !errors.Is(err, finding.ErrAlreadyAssessed) {
			t.Errorf("a second claim about one issue got %v (%+v), want ErrAlreadyAssessed", err, second)
		}
	})
}

func TestAWithdrawnAssessmentDoesNotStandInTheWayOfAFreshOne(t *testing.T) {
	// The other half of one claim per issue. A withdrawn claim is history: it
	// stays readable, and it does not stop anybody claiming again. Enforced by
	// a key released on withdrawal rather than by a state check, so what makes
	// room is the same thing that took it (TRI-33).
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		one := found("CVE-2026-AGAIN", swss)
		one.Issue.Severity = "medium"
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{one}); err != nil {
			t.Fatal(err)
		}
		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		id := f.issue(t, "CVE-2026-AGAIN")

		first, err := f.store.Assess(t.Context(), who, id, "high", "Worse.")
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.Withdraw(t.Context(), who, first.ID); err != nil {
			t.Fatal(err)
		}
		again, err := f.store.Assess(t.Context(), who, id, "critical", "Worse than that.")
		if err != nil {
			t.Fatalf("a withdrawn claim blocked a fresh one: %v", err)
		}
		if again.ID == first.ID {
			t.Error("the fresh claim overwrote the withdrawn one rather than standing beside it")
		}
		// And the withdrawn one is still readable, which is why it was
		// withdrawn rather than deleted.
		if held := f.assessment(t, first.ID); held.State != finding.AssessmentWithdrawn {
			t.Errorf("the withdrawn claim reads as %q", held.State)
		}
	})
}

func TestTwoAssessmentsProposedAtOnceLeaveOneStanding(t *testing.T) {
	// The shape a read-then-write check cannot hold: both proposals read
	// "nothing stands here" before either writes, and both then write. What
	// refuses the second is the unique constraint over the issue a live claim
	// is about, which no amount of checking beforehand can substitute for
	// (TRI-33).
	each(t, func(t *testing.T, f *fixture) {
		f.shipped(t, twoConsumers())
		one := found("CVE-2026-RACE", swss)
		one.Issue.Severity = "medium"
		if _, err := f.store.Apply(t.Context(), f.target, f.run(t),
			[]finding.Reported{one}); err != nil {
			t.Fatal(err)
		}
		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		id := f.issue(t, "CVE-2026-RACE")

		const proposers = 6
		var wg sync.WaitGroup
		results := make([]error, proposers)
		start := make(chan struct{})
		for i := range proposers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, results[i] = f.store.Assess(t.Context(), who, id, "high", "Worse.")
			}()
		}
		close(start)
		wg.Wait()

		stood := 0
		for i, err := range results {
			switch {
			case err == nil:
				stood++
			case errors.Is(err, finding.ErrAlreadyAssessed):
			default:
				t.Errorf("proposal %d failed with %v, want ErrAlreadyAssessed", i, err)
			}
		}
		if stood != 1 {
			t.Errorf("%d of %d proposals got through, want exactly one", stood, proposers)
		}
		if live := f.liveAssessments(t, id); live != 1 {
			t.Errorf("%d claims stand about one issue", live)
		}
	})
}
