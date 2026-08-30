package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Reaffirmation is somebody saying a lapsed claim still holds.
type Reaffirmation struct {
	// PreviousID names the decision that stopped applying — because the code
	// moved under it, not because anybody disagreed.
	//
	// Named rather than passed. Whether an agreement may be carried forward is
	// read from the row, because a caller holding a stale copy would carry one
	// that has since been withdrawn, and a caller inventing a copy would carry
	// one that never existed.
	PreviousID int64
	// Place is where it is being re-made, at the versions it has now.
	Place Place
	// Reasoning is the fresh reason. Required: "still true" with nothing
	// behind it is what a re-affirmation becomes when it is easy enough.
	Reasoning string
	By        int64
}

// Reaffirm re-makes a decision the code moved out from under.
//
// The person who made it may re-make it, with no second approver. Two people
// already agreed to this claim; a version bump is a prompt to re-check rather
// than a new claim, and requiring full approval on every bump produces
// rubber-stamping — which costs the control its meaning everywhere, not only
// here.
//
// Two things put it back through full approval, and both fire on something
// having actually changed:
//
// A different justification is a different claim, which nobody has reviewed.
// Letting it inherit an approval granted for other reasons is the same failure
// as an approval surviving a rewrite.
//
// A severity that has risen since means the original judgment was made about a
// smaller thing. What was agreed to was that this did not matter much; that is
// not an agreement about what it has become.
//
// A count of re-affirmations deliberately does not trigger it. That would fire
// on nothing having changed, which every other rule here refuses to do.
func (s *Store) Reaffirm(ctx context.Context, subject access.Subject, r Reaffirmation, severityNow int) (*Decision, error) {
	if !mayDecide(subject, r.Place.ProductID, visibilityOf(r.Place)) {
		return nil, ErrNotTheirs
	}
	if r.By != subject.ID {
		return nil, fmt.Errorf("a decision is recorded as made by whoever made it")
	}

	previous := new(Decision)
	if err := s.db.NewSelect().Model(previous).
		Where("id = ?", r.PreviousID).Scan(ctx); err != nil {
		return nil, ErrNotTheirs
	}
	// Authorized against the row, not against what the caller said about it.
	// Checking the stated visibility would let somebody trusted only with what
	// has been disclosed re-affirm an undisclosed claim — and, because the new
	// decision is written with the visibility it was authorized under, publish
	// it in the act of re-making it.
	if !mayDecide(subject, previous.ProductID, previous.Visibility) {
		return nil, ErrNotTheirs
	}
	// The claim being re-made has to be about the same thing. Otherwise a
	// re-affirmation is a way to attach one place's agreement to another's.
	if previous.ProductID != r.Place.ProductID ||
		previous.VulnerabilityID != r.Place.VulnerabilityID ||
		previous.PlaceIdentity != r.Place.PlaceIdentity {
		return nil, fmt.Errorf("that decision was about a different place")
	}
	// Re-affirming is a right the person who made the claim has, and nobody
	// else. Without this the approver could re-affirm — becoming proposer of
	// the new claim while their own earlier agreement is carried onto it, so
	// one person ends up on both sides of a control that says they may not be.
	if previous.ProposedBy != subject.ID {
		return nil, fmt.Errorf(
			"only the person who made a decision may re-affirm it; anybody else proposes it afresh")
	}

	// And it keeps the visibility it had. A re-affirmation says the same claim
	// still holds; it is not an occasion to change who may see it.
	place := r.Place
	place.Visibility = previous.Visibility

	justification := ""
	if previous.Justification != nil {
		justification = *previous.Justification
	}

	made, err := s.Propose(ctx, subject, Proposal{
		Place: place, Outcome: previous.Outcome,
		Justification: Justification(justification),
		DeferredUntil: previous.DeferredUntil,
		Reasoning:     r.Reasoning, By: r.By,
		SeverityCenti: severityNow,
	})
	if err != nil {
		return nil, err
	}

	if needsFullApproval(*previous, severityNow) {
		return made, nil
	}

	// Carried rather than re-agreed. Recorded as an approval like any other,
	// naming the words it stands on, so what somebody reading the record sees
	// is that this was agreed to — and by whom, and when — rather than a gap
	// where an agreement should be.
	if err := s.carryApproval(ctx, made, *previous); err != nil {
		return nil, err
	}
	return made, nil
}

// needsFullApproval reports whether a re-affirmation is really a new claim.
func needsFullApproval(previous Decision, severityNow int) bool {
	// Never carried where nobody agreed in the first place. A claim that was
	// only ever proposed has nothing to carry, and treating its re-affirmation
	// as pre-agreed would manufacture an approval out of a version bump.
	if previous.State != Approved && previous.State != LapsedState {
		return true
	}
	if severityNow > previous.SeverityAtApproval() {
		return true
	}
	return false
}

// SeverityAtApproval is how bad this was judged to be when it was agreed to.
//
// Stored with the decision rather than read from the issue now, which is the
// whole point: the question is whether it has risen *since*, and an issue's
// severity is rewritten in place as reports revise it.
func (d Decision) SeverityAtApproval() int {
	if d.SeverityCenti == nil {
		return 0
	}
	return *d.SeverityCenti
}

// carryApproval records that a re-affirmed claim stands on the agreement its
// predecessor had.
func (s *Store) carryApproval(ctx context.Context, made *Decision, previous Decision) error {
	// Only an agreement that still stands may be carried, and only one given
	// by somebody other than whoever is re-affirming.
	//
	// A withdrawn agreement is kept because who agreed and to what is part of
	// the record — but carrying it forward would resurrect it against words
	// nobody read. The path is concrete: propose, have it agreed to, revise
	// (which withdraws the agreement), let a version bump lapse it, re-affirm.
	var earlier []Approval
	if err := s.db.NewSelect().Model(&earlier).
		Where("decision_id = ?", previous.ID).
		Where("withdrawn_at IS NULL").
		Where("approved_by <> ?", made.ProposedBy).
		Order("id DESC").Limit(1).Scan(ctx); err != nil || len(earlier) == 0 {
		// Nothing to carry. Not a fault: a decision may have lapsed before
		// anybody agreed to it, and the claim simply waits like any other.
		return nil //nolint:nilerr // an absent approval is an answer, not a failure
	}
	if made.RevisionID == nil {
		return fmt.Errorf("a re-affirmed decision has no reasoning to stand on")
	}

	now := s.now().Truncate(time.Microsecond)
	carried := &Approval{
		DecisionID: made.ID, RevisionID: *made.RevisionID,
		ApprovedBy: earlier[0].ApprovedBy, ApprovedAt: now,
	}
	if _, err := s.db.NewInsert().Model(carried).Exec(ctx); err != nil {
		return fmt.Errorf("carry an approval forward: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Approved).Where("id = ?", made.ID).Exec(ctx); err != nil {
		return fmt.Errorf("carry an approval forward: %w", err)
	}
	made.State = Approved
	return nil
}

// Lapsed marks the decisions a place has moved out from under.
//
// Applying finds them by not matching, which is enough for reading. This is
// for the queue: somebody has to be shown that a judgment they made no longer
// covers anything, or it simply disappears and the finding reappears as new
// with the reasoning stranded behind it.
func (s *Store) Lapsed(ctx context.Context, at Place) error {
	query := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", LapsedState).
		Where("product_id = ?", at.ProductID).
		Where("vulnerability_id = ?", at.VulnerabilityID).
		Where("place_identity = ?", at.PlaceIdentity).
		Where("state IN (?, ?)", Proposed, Approved)

	// Everything about this place except what it is now. A decision has moved
	// out from under the code when *either* version differs, not when both
	// do — a component bumped under an unchanged consumer is exactly the
	// ordinary case, and requiring both would leave it standing.
	query = query.WhereGroup(" AND ", func(q *bun.UpdateQuery) *bun.UpdateQuery {
		q = notVersion(q, "component_upstream_version", at.ComponentUpstream)
		return notVersionOr(q, "consumer_upstream_version", at.ConsumerUpstream)
	})

	if _, err := query.Exec(ctx); err != nil {
		return fmt.Errorf("mark what the code moved out from under: %w", err)
	}
	return nil
}

// notVersion matches rows recorded against a version other than this one.
//
// An absent version and a stated one are different things, so "not this
// version" means "states some other version" where one is stated, and "states
// any version at all" where none is.
func notVersion(query *bun.UpdateQuery, column, stated string) *bun.UpdateQuery {
	if stated == "" {
		return query.Where(column + " IS NOT NULL")
	}
	return query.Where("("+column+" IS NULL OR "+column+" <> ?)", stated)
}

// notVersionOr is the same test, joined with OR rather than AND.
func notVersionOr(query *bun.UpdateQuery, column, stated string) *bun.UpdateQuery {
	if stated == "" {
		return query.WhereOr(column + " IS NOT NULL")
	}
	return query.WhereOr("("+column+" IS NULL OR "+column+" <> ?)", stated)
}
