package triage_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

func TestTheQueueCarriesWhatAnApproverNeedsToJudge(t *testing.T) {
	// A reviewer works down a list. A list where judging each row means
	// opening it is a list that gets approved without being read — which is
	// the failure the queue exists to prevent, arriving by another route.
	each(t, func(t *testing.T, f *fixture) {
		f.claims(t, f.at())

		waiting, total, err := f.store.Queue(t.Context(), f.reviewer, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(waiting) != 1 {
			t.Fatalf("%d waiting (total %d), want 1", len(waiting), total)
		}
		if waiting[0].Reasoning == "" {
			t.Error("a row arrived with nothing to read")
		}
		if waiting[0].PreviouslyApproved {
			t.Error("a fresh claim reads as one that was agreed to before")
		}
	})
}

func TestSomethingComingBackSaysSo(t *testing.T) {
	// An approver meeting a claim again should know they are re-reading rather
	// than seeing it for the first time.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.claims(t, f.at())
		if err := f.store.Approve(ctx, f.reviewer, claimed.ID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Revise(ctx, f.triager, claimed.ID, "Different reasoning entirely."); err != nil {
			t.Fatal(err)
		}

		waiting, _, err := f.store.Queue(ctx, f.reviewer, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(waiting) != 1 {
			t.Fatalf("%d waiting, want the revised one", len(waiting))
		}
		if !waiting[0].PreviouslyApproved {
			t.Error("a claim that came back after being agreed to reads as fresh")
		}
		if waiting[0].Reasoning != "Different reasoning entirely." {
			t.Errorf("the queue shows %q, not what now stands", waiting[0].Reasoning)
		}
	})
}

func TestAQueueShowsOnlyWorkTheReaderCanDo(t *testing.T) {
	// A work list containing work somebody cannot do teaches them to skip
	// rows, which is the opposite of what a review queue is for.
	each(t, func(t *testing.T, f *fixture) {
		f.claims(t, f.at())

		waiting, total, err := f.store.Queue(t.Context(), f.onlooker, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(waiting) != 0 || total != 0 {
			t.Errorf("somebody holding only a read role was shown %d rows (total %d)", len(waiting), total)
		}
	})
}

func TestAShortDeferralNeedsNobodyAndALongOneDoes(t *testing.T) {
	// A quick "not this sprint" is ordinary triage. Gating it would put every
	// routine act through a queue, which is how a queue stops being read.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		const threshold = 30 * 24 * time.Hour

		soon := time.Now().UTC().Add(7 * 24 * time.Hour)
		short := triage.Proposal{
			Place: f.at(), Outcome: triage.Deferred, DeferredUntil: &soon,
			Reasoning: "Not this sprint.", By: f.proposer,
		}
		needs, err := f.store.NeedsApproval(ctx, short, threshold)
		if err != nil {
			t.Fatal(err)
		}
		if needs {
			t.Error("a week-long deferral was made to wait for a second person")
		}

		distant := time.Now().UTC().Add(200 * 24 * time.Hour)
		long := short
		long.DeferredUntil = &distant
		needs, err = f.store.NeedsApproval(ctx, long, threshold)
		if err != nil {
			t.Fatal(err)
		}
		if !needs {
			t.Error("a deferral of most of a year stood on its own")
		}
	})
}

func TestDeferringRepeatedlyStopsBeingShort(t *testing.T) {
	// Otherwise the exception swallows the rule: four consecutive twenty-nine
	// day deferrals are a year nobody approved.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		const threshold = 30 * 24 * time.Hour
		nearly := 29 * 24 * time.Hour

		asking := func() triage.Proposal {
			until := time.Now().UTC().Add(nearly)
			return triage.Proposal{
				Place: f.at(), Outcome: triage.Deferred, DeferredUntil: &until,
				Reasoning: "Still not this sprint.", By: f.proposer,
			}
		}

		first := asking()
		needs, err := f.store.NeedsApproval(ctx, first, threshold)
		if err != nil {
			t.Fatal(err)
		}
		if needs {
			t.Fatal("the first short deferral needed approval")
		}
		if _, err := f.store.Propose(ctx, f.triager, first); err != nil {
			t.Fatal(err)
		}

		// The second one asks for another twenty-nine days on a finding
		// already put off for twenty-nine, which is fifty-eight.
		needs, err = f.store.NeedsApproval(ctx, asking(), threshold)
		if err != nil {
			t.Fatal(err)
		}
		if !needs {
			t.Error("deferring twice for just under the threshold stayed unapproved")
		}
	})
}

func TestHidingRiskNeedsAgreementAndPuttingItBackDoesNot(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		for _, c := range []struct {
			outcome triage.Outcome
			needs   bool
		}{
			{triage.Affected, false},
			{triage.NotApplicable, true},
			{triage.WontFix, true},
		} {
			p := triage.Proposal{
				Place: f.at(), Outcome: c.outcome,
				Reasoning: "x", By: f.proposer,
			}
			if c.outcome == triage.NotApplicable {
				p.Justification = triage.CodeNotPresent
			}
			needs, err := f.store.NeedsApproval(ctx, p, 30*24*time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			if needs != c.needs {
				t.Errorf("%q needing approval = %v, want %v", c.outcome, needs, c.needs)
			}
		}
	})
}
