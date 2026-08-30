package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Waiting is one claim somebody has to look at, with what an approver needs in
// order to judge it.
//
// Everything here is carried rather than left to be fetched per row. A
// reviewer works down a list, and a list where judging each row means opening
// it is a list that gets approved without being read — which is the failure
// the queue exists to prevent, arriving by a different route.
type Waiting struct {
	Decision Decision
	// Reasoning is what the proposer wrote, as it currently stands.
	Reasoning string
	// PreviouslyApproved says this was agreed to before and came back — either
	// because the reasoning was revised under the approval or because the code
	// moved. Somebody meeting it again should know they are re-reading rather
	// than seeing it for the first time.
	PreviouslyApproved bool
	// DeferredSoFar is the total time this finding has been put off, across
	// every deferral. What decides whether a deferral needs agreement is the
	// cumulative time, not the length of the one being asked about.
	DeferredSoFar time.Duration
}

// Queue returns what is waiting to be judged, most urgent first.
//
// Narrowed to what the asker may act on, in the query. A reviewer who cannot
// triage a product should not be shown its claims at all — a queue is a work
// list, and one containing work somebody cannot do teaches them to skip rows.
func (s *Store) Queue(ctx context.Context, subject access.Subject, limit, offset int) ([]Waiting, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	counting := reachableBy(s.db.NewSelect().Model((*Decision)(nil)).
		Where("state = ?", Proposed), subject, "de")
	total, err := counting.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is waiting: %w", err)
	}

	var proposed []Decision
	listing := reachableBy(s.db.NewSelect().Model(&proposed).
		Where("state = ?", Proposed), subject, "de")
	if err := listing.Order("id DESC").Limit(limit).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read what is waiting: %w", err)
	}
	if len(proposed) == 0 {
		return nil, total, nil
	}

	ids := make([]int64, 0, len(proposed))
	for _, decision := range proposed {
		ids = append(ids, decision.ID)
	}

	reasoning, err := s.currentReasoning(ctx, proposed)
	if err != nil {
		return nil, 0, err
	}
	seenBefore, err := s.everApproved(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	waiting := make([]Waiting, 0, len(proposed))
	for _, decision := range proposed {
		deferred, err := s.DeferredSoFar(ctx, decision)
		if err != nil {
			return nil, 0, err
		}
		waiting = append(waiting, Waiting{
			Decision: decision, Reasoning: reasoning[decision.ID],
			PreviouslyApproved: seenBefore[decision.ID], DeferredSoFar: deferred,
		})
	}
	return waiting, total, nil
}

// currentReasoning reads the words each claim currently rests on.
func (s *Store) currentReasoning(ctx context.Context, decisions []Decision) (map[int64]string, error) {
	wanted := make([]int64, 0, len(decisions))
	for _, decision := range decisions {
		if decision.RevisionID != nil {
			wanted = append(wanted, *decision.RevisionID)
		}
	}
	if len(wanted) == 0 {
		return map[int64]string{}, nil
	}

	var revisions []Revision
	if err := s.db.NewSelect().Model(&revisions).
		Where("id IN (?)", bun.List(wanted)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read the reasoning: %w", err)
	}
	byDecision := make(map[int64]string, len(revisions))
	for _, revision := range revisions {
		byDecision[revision.DecisionID] = revision.Body
	}
	return byDecision, nil
}

// everApproved reports which of these were agreed to at some point.
func (s *Store) everApproved(ctx context.Context, ids []int64) (map[int64]bool, error) {
	var approvals []Approval
	if err := s.db.NewSelect().Model(&approvals).
		Where("decision_id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what has been agreed to before: %w", err)
	}
	seen := make(map[int64]bool, len(approvals))
	for _, approval := range approvals {
		seen[approval.DecisionID] = true
	}
	return seen, nil
}

// DeferredSoFar is the total time a finding has been put off, across every
// deferral ever recorded about the same place.
//
// Cumulative rather than per deferral, because otherwise deferring repeatedly
// for just under the threshold never needs agreement — and four consecutive
// twenty-nine day deferrals are a year nobody approved.
func (s *Store) DeferredSoFar(ctx context.Context, decision Decision) (time.Duration, error) {
	var deferrals []Decision
	if err := s.db.NewSelect().Model(&deferrals).
		Where("product_id = ?", decision.ProductID).
		Where("vulnerability_id = ?", decision.VulnerabilityID).
		Where("place_identity = ?", decision.PlaceIdentity).
		Where("outcome = ?", Deferred).
		Where("deferred_until IS NOT NULL").Scan(ctx); err != nil {
		return 0, fmt.Errorf("read how long this has been put off: %w", err)
	}

	var total time.Duration
	for _, deferral := range deferrals {
		if deferral.DeferredUntil == nil {
			continue
		}
		// Measured from when it was asked for, so a deferral that has not yet
		// run out counts the whole of what it asked for rather than only the
		// part already spent. The question is how long this has been put off
		// for, not how long it has been put off so far.
		if span := deferral.DeferredUntil.Sub(deferral.ProposedAt); span > 0 {
			total += span
		}
	}
	return total, nil
}

// NeedsApproval reports whether a proposal may stand on its own.
//
// Hiding risk needs a second person. The exception is a short deferral: a
// quick "not this sprint" is ordinary triage and gating it would put every
// routine act through a queue, which is how a queue stops being read.
//
// "Short" is measured against everything this finding has already been put off
// for. Otherwise the exception swallows the rule one twenty-nine day deferral
// at a time.
func (s *Store) NeedsApproval(ctx context.Context, p Proposal, threshold time.Duration) (bool, error) {
	if !p.Outcome.HidesRisk() {
		return false, nil
	}
	if p.Outcome != Deferred {
		return true, nil
	}
	if threshold <= 0 {
		return true, nil
	}

	asking := time.Duration(0)
	if p.DeferredUntil != nil {
		asking = time.Until(*p.DeferredUntil)
	}

	// What has already been asked for about this same place.
	already, err := s.DeferredSoFar(ctx, Decision{
		ProductID: p.Place.ProductID, VulnerabilityID: p.Place.VulnerabilityID,
		PlaceIdentity: p.Place.PlaceIdentity,
	})
	if err != nil {
		return false, err
	}
	return already+asking >= threshold, nil
}
