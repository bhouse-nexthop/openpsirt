package triage_test

import (
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

func TestACommentNeverDisturbsAnApproval(t *testing.T) {
	// The two are different things and the obvious mistake is treating all
	// text on a finding as one. Annotating an approved decision months later
	// is ordinary, and an approval that fell over each time somebody added a
	// note would teach people not to add notes.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.agreed(t, f.at())
		if _, err := f.store.Say(ctx, f.triager, claimed.ID, "Re-checked against 1.2.4; still true."); err != nil {
			t.Fatal(err)
		}

		standing, err := f.store.Applying(ctx, f.at())
		if err != nil {
			t.Fatal(err)
		}
		if standing == nil || standing.State != triage.Approved {
			t.Errorf("a comment disturbed the approval: %+v", standing)
		}
		approvals, err := f.store.Approvals(ctx, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(approvals) != 1 || approvals[0].WithdrawnAt != nil {
			t.Errorf("a comment withdrew the agreement: %+v", approvals)
		}
	})
}

func TestOnlyTheAuthorMayChangeTheirOwnWords(t *testing.T) {
	// An edit anybody could make is not a correction. It is a forgery with a
	// timestamp.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.agreed(t, f.at())
		said, err := f.store.Say(ctx, f.triager, claimed.ID, "First thought.")
		if err != nil {
			t.Fatal(err)
		}

		if err := f.store.Reword(ctx, f.reviewer, said.ID, "Words I did not write."); err == nil {
			t.Error("somebody rewrote another person's comment")
		}
		if err := f.store.Reword(ctx, f.triager, said.ID, "Second thought."); err != nil {
			t.Fatal(err)
		}

		discussion, err := f.store.Discussion(ctx, f.triager, claimed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(discussion) != 1 {
			t.Fatalf("%d comments, want 1", len(discussion))
		}
		if discussion[0].Body != "Second thought." {
			t.Errorf("the comment reads %q", discussion[0].Body)
		}
		// Overwritten rather than revised, and marked as changed — discussion
		// is not the record a decision rests on.
		if discussion[0].EditedAt == nil {
			t.Error("an edited comment does not say it was edited")
		}
	})
}

func TestTextOnADecisionGoesThroughTheSamePolicy(t *testing.T) {
	// Every field somebody types into is checked before it is stored, so that
	// stored text is known to have passed what was in force when it arrived.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		const dangerous = "See ![this](https://evil.example/pixel.gif)"

		if _, err := f.store.Propose(ctx, f.triager, triage.Proposal{
			Place: f.at(), Outcome: triage.WontFix,
			Reasoning: dangerous, By: f.proposer,
		}); err == nil {
			t.Error("a remote image was accepted as reasoning")
		}

		claimed := f.agreed(t, f.at())
		if _, err := f.store.Revise(ctx, f.triager, claimed.ID, dangerous); err == nil {
			t.Error("a remote image was accepted as a revision")
		}
		if _, err := f.store.Say(ctx, f.triager, claimed.ID, dangerous); err == nil {
			t.Error("a remote image was accepted as a comment")
		}
	})
}

func TestDiscussionIsNotReachableWithoutTheRightToTriage(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.agreed(t, f.at())
		if _, err := f.store.Say(ctx, f.onlooker, claimed.ID, "Adding my thoughts."); err == nil {
			t.Error("somebody holding only a read role commented")
		}
		if _, err := f.store.Discussion(ctx, f.onlooker, claimed.ID); err == nil {
			t.Error("somebody holding only a read role read the discussion")
		}
	})
}
