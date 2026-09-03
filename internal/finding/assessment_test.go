package finding_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
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

func TestAnApproverIsToldWhatAgreeingTakesOffTheList(t *testing.T) {
	// TRI-41 gates a downgrade on a second person because it pushes a deadline
	// out. Since TRI-43 exists, a downgrade that crosses a product's triage
	// line does something different in kind: the finding stops being work
	// rather than becoming later work, and REM-27 takes its deadline away
	// entirely. Those are two things to agree to and an approver was told
	// neither.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		settings := setting.NewStore(f.db.DB)
		if err := settings.Set(ctx, setting.TriageFloor, "medium"); err != nil {
			t.Fatal(err)
		}

		f.shipped(t, twoConsumers())
		bad := found("CVE-2026-CROSS", libnl)
		bad.Issue.Severity = "high"
		if _, err := f.store.Apply(ctx, f.target, f.run(t),
			[]finding.Reported{bad}); err != nil {
			t.Fatal(err)
		}
		open := f.open(t)
		if len(open) != 2 {
			t.Fatalf("opened %d findings", len(open))
		}

		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		id := f.issue(t, "CVE-2026-CROSS")

		// Milder, but still above the line: later work, not no work.
		claim, err := f.store.Assess(ctx, who, id, "medium", "Not as bad as published.")
		if err != nil {
			t.Fatal(err)
		}
		would, err := f.store.WhatAgreeingWouldDo(ctx, who, claim.ID)
		if err != nil {
			t.Fatal(err)
		}
		if would.Findings != 2 || would.Products != 1 {
			t.Errorf("agreeing was measured against %+v, want two findings in one product", would)
		}
		if would.Crosses() {
			t.Errorf("a downgrade that stays above the line reported %d off the list",
				would.OffTheList)
		}

		// Milder still, and now it crosses.
		if err := f.store.Withdraw(ctx, who, claim.ID); err != nil {
			t.Fatal(err)
		}
		crossing, err := f.store.Assess(ctx, who, id, "low", "Not worth an afternoon.")
		if err != nil {
			t.Fatal(err)
		}
		would, err = f.store.WhatAgreeingWouldDo(ctx, who, crossing.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !would.Crosses() {
			t.Fatal("a downgrade below the line reported nothing coming off it")
		}
		if would.OffTheList != 2 || would.ProductsAffected != 1 {
			t.Errorf("it takes %d findings off the list in %d products, want 2 in 1",
				would.OffTheList, would.ProductsAffected)
		}
	})
}

func TestWhatAgreeingWouldDoCountsOnlyWhatTheReaderMaySee(t *testing.T) {
	// An approver who cannot see a product is not told how many of its
	// findings this would hide. That understates the effect for them, which is
	// the right way for it to be wrong: the alternative discloses a count of
	// undisclosed work.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		if err := setting.NewStore(f.db.DB).Set(ctx, setting.TriageFloor, "medium"); err != nil {
			t.Fatal(err)
		}
		f.shipped(t, twoConsumers())
		bad := found("CVE-2026-QUIET", libnl)
		bad.Issue.Severity = "high"
		if _, err := f.store.Apply(ctx, f.target, f.run(t),
			[]finding.Reported{bad}); err != nil {
			t.Fatal(err)
		}
		// One of the two places undisclosed, so a public reader sees a smaller
		// number rather than none — with everything hidden the check below
		// could only fail by returning something it should not.
		one := f.open(t)[0]
		if _, err := f.db.DB.NewUpdate().Model((*finding.Finding)(nil)).
			Set("visibility = ?", access.Private).
			Where("id = ?", one.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		f.recorded(t, 1, "someone")
		who := f.holding(t, access.PublicTriage)
		claim, err := f.store.Assess(ctx, who, f.issue(t, "CVE-2026-QUIET"),
			"low", "Not worth an afternoon.")
		if err != nil {
			t.Fatal(err)
		}

		public, err := f.store.WhatAgreeingWouldDo(ctx, who, claim.ID)
		if err != nil {
			t.Fatal(err)
		}
		if public.Findings != 1 || public.OffTheList != 1 {
			t.Errorf("a public reader was told %+v, want the one place they may see", public)
		}

		everything := f.holding(t, access.PublicTriage, access.PrivateRead)
		all, err := f.store.WhatAgreeingWouldDo(ctx, everything, claim.ID)
		if err != nil {
			t.Fatal(err)
		}
		if all.Findings != 2 || all.OffTheList != 2 {
			t.Errorf("a reader who may see both was told %+v, want both places", all)
		}
	})
}

func TestWhatAgreeingWouldDoCountsWhatCrossesTheLineAndNotWhatIsAlreadyBelow(t *testing.T) {
	// The number an approver is weighing is what *this* rating takes off a
	// working list. A finding already below its product's line is not taken
	// off anything by agreeing, and counting it would inflate what somebody is
	// being asked to weigh — which is the one number in front of them.
	//
	// Products differ in what they can afford to ignore, so one issue can sit
	// above the line in one product and below it in another. That is the case
	// that tells the two counts apart.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		products := catalog.NewStore(f.db.DB)
		if err := setting.NewStore(f.db.DB).Set(ctx, setting.TriageFloor, "medium"); err != nil {
			t.Fatal(err)
		}

		// This product triages from medium, so a high is on its working list.
		f.shipped(t, twoConsumers())
		bad := found("CVE-2026-BOTH", libnl)
		bad.Issue.Severity = "high"
		if _, err := f.store.Apply(ctx, f.target, f.run(t),
			[]finding.Reported{bad}); err != nil {
			t.Fatal(err)
		}

		// And another that triages from critical, where the same high is
		// already off the list before anybody says anything.
		strict := f.inAnotherProduct(t, "strict-product")
		if err := products.SetTriageFloor(ctx, f.productOf(t, strict), "critical"); err != nil {
			t.Fatal(err)
		}
		f.shippedTo(t, strict, twoConsumers())
		if _, err := f.store.Apply(ctx, strict, f.runOn(t, strict),
			[]finding.Reported{bad}); err != nil {
			t.Fatal(err)
		}

		f.recorded(t, 1, "someone")
		who := access.NewPerson(1, "someone", true, nil)
		claim, err := f.store.Assess(ctx, who, f.issue(t, "CVE-2026-BOTH"),
			"low", "Not worth an afternoon.")
		if err != nil {
			t.Fatal(err)
		}

		would, err := f.store.WhatAgreeingWouldDo(ctx, who, claim.ID)
		if err != nil {
			t.Fatal(err)
		}
		if would.Findings != 4 || would.Products != 2 {
			t.Fatalf("measured against %+v, want four findings across two products", would)
		}
		if would.OffTheList != 2 || would.ProductsAffected != 1 {
			t.Errorf("agreeing takes %d findings off the list in %d products, want 2 in 1 — "+
				"the two in the product that already hid them are not taken off anything",
				would.OffTheList, would.ProductsAffected)
		}
	})
}
