package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
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

// Lapse marks the decisions this target's contents have moved out from under.
//
// A decision is stored against the upstream versions it was made about. When
// those versions move it stops applying, and that much is automatic, because
// what applies is matched on the versions. What is not automatic is anybody
// finding out. Without this the finding simply reappears as though nobody had
// ever looked at it, with the reasoning stranded on a row nothing points at —
// which is the outcome that keeping the old decision exists to prevent.
//
// Run after a scan records what it found, because that is when the versions
// have just changed. It is one statement rather than one per place: a real
// image holds tens of thousands of places, and a sweep costing a write per
// place is a sweep somebody turns off.
//
// **A decision covering nothing here is not lapsed.** A component that is gone
// altogether closed its findings and there is nothing to ask anybody about,
// where a component still present at a different version is exactly the
// question somebody has to answer again.
func (s *Store) Lapse(ctx context.Context, targetID int64) (int64, error) {
	// Every open finding of this target at the decision's place, with the
	// versions it currently has — stated the same way the decision was written
	// against them, from the same expression, so that a decision cannot lapse
	// on one path and stand on the other.
	openHere := func() *bun.SelectQuery {
		return s.db.NewSelect().
			ColumnExpr("1").
			TableExpr("finding AS f").
			Join("JOIN component AS c ON c.id = f.component_id").
			Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
			Where("f.target_id = ?", targetID).
			Where("f.closed_run_id IS NULL").
			Where("f.vulnerability_id = de.vulnerability_id").
			Where("f.place_identity = de.place_identity")
	}

	// Still found here, at versions that are not the ones this was decided
	// about. Absent and empty are the same answer on the finding's side, so a
	// decision recorded against no version matches a component stating none.
	result, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", LapsedState).
		// Released for the same reason a withdrawal is: the code moved out
		// from under this, so it covers nothing, and somebody has to be able
		// to decide about what is there now.
		Set("live_key = ?", nil).
		Where("state IN (?, ?)", Proposed, Approved).
		Where("EXISTS (?)", openHere()).
		Where("NOT EXISTS (?)", openHere().
			Where("COALESCE(de.component_upstream_version, '') = "+finding.ComponentUpstreamExpr).
			Where("COALESCE(de.consumer_upstream_version, '') = "+finding.ConsumerUpstreamExpr)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("mark what the code moved out from under: %w", err)
	}
	moved, err := result.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return moved, nil
}

// Carried is what a new line would inherit from an existing one.
//
// Four buckets, because they need four different things from a person. What
// already applies needs nothing. What moved needs a fresh answer, and gets the
// old reasoning to start from. A postponement is a scheduling judgment about a
// release rather than a claim about code, so it is offered separately. And
// what covers nothing there is left behind.
type Carried struct {
	// Applying reach the new line by matching. Nothing to choose.
	Applying int
	// Moved held a claim at a version the new line does not have. Each comes
	// across as a proposal carrying the old words — never as a decision,
	// because the version moved and the old conclusion is not a conclusion
	// about the new code.
	Moved []Inherited
	// Postponed were deferrals. "Not this sprint" was about that sprint, and
	// carrying it silently gives a new line expiry dates nobody chose.
	Postponed []Inherited
	// Absent is how many cover nothing in the new line at all.
	Absent int
}

// Inherited is one claim a new line could take on.
type Inherited struct {
	DecisionID    int64
	Vulnerability string
	Component     string
	Outcome       Outcome
	Was           string
	Now           string
	Reasoning     string
	// DeferredDays is how long this has already been put off, across every
	// line it has been carried through. The number that decides whether
	// carrying it again is reasonable.
	DeferredDays int
}

// WouldCarry reports what a new line would inherit from an existing one,
// without changing anything.
//
// Asked before a line is created, because the answer is what somebody is
// agreeing to — and a carry that happened silently is the one nobody reviews.
func (s *Store) WouldCarry(ctx context.Context, subject access.Subject,
	fromTarget, toTarget int64) (*Carried, error) {

	if subject.Kind != access.Person {
		return nil, ErrNotTheirs
	}

	var rows []struct {
		DecisionID    int64  `bun:"decision_id"`
		Vulnerability string `bun:"vulnerability"`
		Component     string `bun:"component"`
		Outcome       string `bun:"outcome"`
		Was           string `bun:"was"`
		Now           string `bun:"now_at"`
		Reasoning     string `bun:"reasoning"`
		StillThere    bool   `bun:"still_there"`
	}
	err := s.db.NewSelect().
		TableExpr("decision AS de").
		Join("JOIN vulnerability AS v ON v.id = de.vulnerability_id").
		Join("LEFT JOIN decision_revision AS dr ON dr.id = de.revision_id").
		ColumnExpr("de.id AS decision_id").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("COALESCE(de.component_upstream_version, '') AS was").
		ColumnExpr("de.outcome AS outcome").
		ColumnExpr("COALESCE(dr.body, '') AS reasoning").
		// What the new line has at that place, if anything.
		ColumnExpr(`COALESCE((SELECT MIN(c.name) FROM "finding" AS f
			JOIN "component" AS c ON c.id = f.component_id
			WHERE f.target_id = ? AND f.vulnerability_id = de.vulnerability_id
			  AND f.place_identity = de.place_identity AND f.closed_run_id IS NULL), '')
			AS component`, toTarget).
		ColumnExpr(`COALESCE((SELECT MIN(`+finding.ComponentUpstreamExpr+`) FROM "finding" AS f
			JOIN "component" AS c ON c.id = f.component_id
			LEFT JOIN "component" AS uc ON uc.id = f.consumer_id
			WHERE f.target_id = ? AND f.vulnerability_id = de.vulnerability_id
			  AND f.place_identity = de.place_identity AND f.closed_run_id IS NULL), '')
			AS now_at`, toTarget).
		ColumnExpr(`EXISTS (SELECT 1 FROM "finding" AS f
			WHERE f.target_id = ? AND f.vulnerability_id = de.vulnerability_id
			  AND f.place_identity = de.place_identity AND f.closed_run_id IS NULL)
			AS still_there`, toTarget).
		Where("de.live_key IS NOT NULL").
		Where(`EXISTS (SELECT 1 FROM "finding" AS g
			WHERE g.target_id = ? AND g.vulnerability_id = de.vulnerability_id
			  AND g.place_identity = de.place_identity)`, fromTarget).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what a new line would inherit: %w", err)
	}

	carried := &Carried{}
	for _, row := range rows {
		if !row.StillThere {
			carried.Absent++
			continue
		}
		if row.Was == row.Now {
			// The versions match, so it reaches the new line by matching.
			// Offering it would ask somebody to agree to something that has
			// already happened.
			carried.Applying++
			continue
		}
		one := Inherited{
			DecisionID: row.DecisionID, Vulnerability: row.Vulnerability,
			Component: row.Component, Outcome: Outcome(row.Outcome),
			Was: row.Was, Now: row.Now, Reasoning: row.Reasoning,
		}
		if Outcome(row.Outcome) == Deferred {
			carried.Postponed = append(carried.Postponed, one)
			continue
		}
		carried.Moved = append(carried.Moved, one)
	}
	return carried, nil
}
