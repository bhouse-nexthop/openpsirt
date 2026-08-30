package triage_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// judged claims something with a severity recorded against it, which is what a
// later re-affirmation compares itself to.
func (f *fixture) judged(t *testing.T, at triage.Place, severity int) *triage.Decision {
	t.Helper()
	decision, err := f.store.Propose(t.Context(), f.triager, triage.Proposal{
		Place: at, Outcome: triage.NotApplicable,
		Justification: triage.CodeNotInExecutePath,
		Reasoning:     "The parser is never reached.",
		By:            f.proposer, SeverityCenti: severity,
		NeedsApproval: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return decision
}

func TestReAffirmingAfterABumpNeedsNoSecondPerson(t *testing.T) {
	// Two people already agreed to this claim. A version bump is a prompt to
	// re-check rather than a new claim, and asking for full approval every
	// time produces rubber-stamping — which costs the control its meaning
	// everywhere, not only here.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		agreed := f.judged(t, f.at(), 700)
		if err := f.store.Approve(ctx, f.reviewer, agreed.ID, ""); err != nil {
			t.Fatal(err)
		}

		moved := f.at()
		moved.ComponentUpstream = "1.2.4"
		again, err := f.store.Reaffirm(ctx, f.triager, triage.Reaffirmation{
			PreviousID: agreed.ID, Place: moved,
			Reasoning: "Checked again at the new version; still not reached.",
			By:        f.proposer,
		}, 700)
		if err != nil {
			t.Fatal(err)
		}
		if again.State != triage.Approved {
			t.Errorf("a re-affirmation of an agreed claim reads as %q", again.State)
		}
		// And it stands at the new versions.
		if standing, _ := f.store.Applying(ctx, moved); standing == nil {
			t.Error("the re-affirmed claim does not apply at the version it was re-made for")
		}
	})
}

func TestSeverityRisingSendsItBackForFullApproval(t *testing.T) {
	// What was agreed to was that this did not matter much. That is not an
	// agreement about what it has become.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		agreed := f.judged(t, f.at(), 400)
		if err := f.store.Approve(ctx, f.reviewer, agreed.ID, ""); err != nil {
			t.Fatal(err)
		}

		moved := f.at()
		moved.ComponentUpstream = "1.2.4"
		again, err := f.store.Reaffirm(ctx, f.triager, triage.Reaffirmation{
			PreviousID: agreed.ID, Place: moved,
			Reasoning: "Still not reached.", By: f.proposer,
		}, 950)
		if err != nil {
			t.Fatal(err)
		}
		if again.State == triage.Approved {
			t.Error("a claim about a much worse issue inherited the old agreement")
		}
	})
}

func TestNothingIsCarriedFromAClaimNobodyAgreedTo(t *testing.T) {
	// Otherwise a version bump manufactures an approval out of nothing.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		neverAgreed := f.judged(t, f.at(), 700)

		moved := f.at()
		moved.ComponentUpstream = "1.2.4"
		again, err := f.store.Reaffirm(ctx, f.triager, triage.Reaffirmation{
			PreviousID: neverAgreed.ID, Place: moved,
			Reasoning: "Still true.", By: f.proposer,
		}, 700)
		if err != nil {
			t.Fatal(err)
		}
		if again.State == triage.Approved {
			t.Error("a claim nobody ever agreed to came back approved")
		}
	})
}

func TestRepetitionAloneChangesNothing(t *testing.T) {
	// Deliberately left out. A count of re-affirmations would fire on nothing
	// having changed, which every other rule here refuses to do.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		previous := f.judged(t, f.at(), 700)
		if err := f.store.Approve(ctx, f.reviewer, previous.ID, ""); err != nil {
			t.Fatal(err)
		}

		at := f.at()
		for i, version := range []string{"1.2.4", "1.2.5", "1.2.6", "1.2.7"} {
			at.ComponentUpstream = version
			again, err := f.store.Reaffirm(ctx, f.triager, triage.Reaffirmation{
				PreviousID: previous.ID, Place: at,
				Reasoning: "Checked again; still not reached.", By: f.proposer,
			}, 700)
			if err != nil {
				t.Fatal(err)
			}
			if again.State != triage.Approved {
				t.Errorf("re-affirmation %d was sent for full approval on repetition alone", i+1)
			}
			previous = again
		}
	})
}

func TestAWithdrawnAgreementIsNotResurrectedByAVersionBump(t *testing.T) {
	// The case the state check actually guards. A withdrawn decision still has
	// its approval rows — that is deliberate, because who agreed and to what
	// is part of the record — so carrying an agreement forward without asking
	// what state it is in would undo a withdrawal with a version bump.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		agreed := f.judged(t, f.at(), 700)
		if err := f.store.Approve(ctx, f.reviewer, agreed.ID, ""); err != nil {
			t.Fatal(err)
		}
		// Somebody thought better of it.
		if err := f.store.Withdraw(ctx, f.triager, agreed.ID); err != nil {
			t.Fatal(err)
		}

		moved := f.at()
		moved.ComponentUpstream = "1.2.4"
		again, err := f.store.Reaffirm(ctx, f.triager, triage.Reaffirmation{
			PreviousID: agreed.ID, Place: moved,
			Reasoning: "Trying again.", By: f.proposer,
		}, 700)
		if err != nil {
			t.Fatal(err)
		}
		if again.State == triage.Approved {
			t.Error("a withdrawn agreement came back through a version bump")
		}
	})
}
