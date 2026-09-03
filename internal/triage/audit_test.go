package triage_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

func TestTheRecordCarriesWhoProposedAndWhoAgreed(t *testing.T) {
	// What an auditor asks for: the judgment, the words it rests on, and two
	// different people with the date each of them acted. Assembled for a page
	// rather than looked up one decision at a time, because the question is
	// about a period rather than about a row.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		// A finding at the place, because a decision stores a hash of names
		// rather than the names — what it was about is recovered from a
		// finding sitting there.
		at := f.at()
		in := f.build(t, f.product, "for-the-record")
		f.finds(t, in, f.component(t, "libfoo", "1.2.3"), at.PlaceIdentity, access.Public)
		claimed := f.agreed(t, at)

		rows, total, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			time.Time{}, time.Time{}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(rows) != 1 {
			t.Fatalf("the record holds %d judgments, want the one that was made", total)
		}
		row := rows[0]
		if row.ID != claimed.ID {
			t.Errorf("the record names decision %d, want %d", row.ID, claimed.ID)
		}
		if row.Reasoning == "" {
			t.Error("a judgment in the record carries no reasoning, which is the thing being audited")
		}
		if row.ProposedByName == "" || row.ProposedAt.IsZero() {
			t.Errorf("no record of who proposed it or when: %+v", row)
		}
		if len(row.Approvals) != 1 {
			t.Fatalf("%d agreements recorded, want 1", len(row.Approvals))
		}
		if row.Approvals[0].By == "" || row.Approvals[0].At.IsZero() {
			t.Errorf("no record of who agreed or when: %+v", row.Approvals[0])
		}
		if row.Approvals[0].By == row.ProposedByName {
			t.Errorf("the same person proposed and agreed: %q", row.Approvals[0].By)
		}
		// The control stated as a fact about this record rather than as a rule
		// that exists. A report that said the rule held because the rule
		// exists would be reporting on itself.
		if !row.BySomebodyElse() {
			t.Error("a judgment two people made does not read as one")
		}
		if !row.Standing() {
			t.Error("an agreed judgment does not read as standing")
		}
		// And what it was about, named from the finding at the place: a
		// decision stores a hash of names rather than the names.
		if row.Issue == "" || row.Component == "" || row.Product == "" {
			t.Errorf("the record does not say what the judgment was about: %+v", row)
		}
	})
}

func TestTheRecordKeepsAnAgreementThatWasTakenBack(t *testing.T) {
	// What somebody agreed to and then stopped agreeing to is exactly what an
	// audit is looking for, so a withdrawn approval is part of the record
	// rather than removed from it (TRI-25).
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.agreed(t, f.at())

		// Editing the words withdraws the agreement given for them.
		if _, err := f.store.Revise(ctx, f.triager, claimed.ID,
			"On reflection, the parser is reachable after all."); err != nil {
			t.Fatal(err)
		}

		rows, _, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			time.Time{}, time.Time{}, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 {
			t.Fatalf("the record holds %d judgments", len(rows))
		}
		row := rows[0]
		if len(row.Approvals) != 1 {
			t.Fatalf("%d agreements recorded, want the one that was taken back", len(row.Approvals))
		}
		if row.Approvals[0].WithdrawnAt == nil {
			t.Error("an agreement withdrawn by a revision reads as still standing")
		}
		// And it no longer counts toward the control.
		if row.BySomebodyElse() {
			t.Error("a judgment whose only agreement was withdrawn still reads as two people's")
		}
		// The words shown are the words in force, so what is read and what was
		// agreed to cannot drift apart.
		if row.Reasoning != "On reflection, the parser is reachable after all." {
			t.Errorf("the record shows %q, want the revised words", row.Reasoning)
		}
	})
}

func TestTheRecordIsNarrowedToWhatTheReaderMaySee(t *testing.T) {
	// Nothing about this view is exempt from the visibility rules. A report
	// showing more than the screens it summarizes would be a way around them.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		hidden := f.at()
		hidden.Visibility = access.Private
		insider := f.privately(t)
		if _, err := f.store.Propose(ctx, insider, triage.Proposal{
			Place: hidden, Outcome: triage.NotApplicable,
			Justification: triage.CodeNotInExecutePath,
			Reasoning:     "The parser is never reached.",
			By:            insider.ID, NeedsApproval: true,
		}); err != nil {
			t.Fatal(err)
		}

		// A public reader is shown nothing about it.
		if _, total, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			time.Time{}, time.Time{}, 50, 0); err != nil || total != 0 {
			t.Errorf("an undisclosed judgment was in a public reader's record: %d (%v)", total, err)
		}
		// And somebody who may see it does, so the check above is not passing
		// on a record that shows nothing to anybody.
		if _, total, err := f.store.Audit(ctx, insider, triage.Filter{},
			time.Time{}, time.Time{}, 50, 0); err != nil || total != 1 {
			t.Errorf("somebody who may read it was shown %d judgments (%v)", total, err)
		}
	})
}

func TestTheRecordIsBoundedByWhenAJudgmentWasProposed(t *testing.T) {
	// The period is the proposal's date, not the approval's: a judgment
	// belongs to when it was argued, and dating it by its agreement would move
	// it out of the period it was made in whenever an approval came late,
	// which is the ordinary case.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		claimed := f.claims(t, f.at())

		before := claimed.ProposedAt.Add(-time.Hour)
		after := claimed.ProposedAt.Add(time.Hour)

		if _, total, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			before, after, 50, 0); err != nil || total != 1 {
			t.Errorf("a judgment inside the period was not in the record: %d (%v)", total, err)
		}
		if _, total, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			after, time.Time{}, 50, 0); err != nil || total != 0 {
			t.Errorf("a judgment proposed before the period was in it: %d (%v)", total, err)
		}
		if _, total, err := f.store.Audit(ctx, f.reviewer, triage.Filter{},
			time.Time{}, before, 50, 0); err != nil || total != 0 {
			t.Errorf("a judgment proposed after the period was in it: %d (%v)", total, err)
		}
	})
}
