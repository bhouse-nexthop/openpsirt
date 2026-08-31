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

// Queue returns what is waiting for somebody, newest first.
//
// Narrowed to what the asker may act on, in the query. A reviewer who cannot
// triage a product should not be shown its claims at all — a queue is a work
// list, and one containing work somebody cannot do teaches them to skip rows.
func (s *Store) Queue(ctx context.Context, subject access.Subject, limit, offset int) ([]Waiting, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	counting := approvableBy(waiting(s.db.NewSelect().Model((*Decision)(nil)), s.now()), subject, "de")
	total, err := counting.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is waiting: %w", err)
	}

	var proposed []Decision
	listing := approvableBy(waiting(s.db.NewSelect().Model(&proposed), s.now()), subject, "de")
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
// ReasoningFor returns the reasoning each decision currently rests on, keyed
// by decision.
func (s *Store) ReasoningFor(ctx context.Context, decisions []Decision) (map[int64]string, error) {
	return s.currentReasoning(ctx, decisions)
}

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
		// What was taken back was not time the finding spent put off. Counting
		// a withdrawn deferral would make the number shown to an approver —
		// "how long has this been postponed" — include time it was not.
		Where("state <> ?", Withdrawn).
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
		// Measured from this store's own clock, like every other time decision
		// here, and never negative. A date already past asks for no time at
		// all; letting it come out negative would let a back-dated deferral
		// subtract from what a finding has already been postponed for and slip
		// under the threshold.
		if span := p.DeferredUntil.Sub(s.now()); span > 0 {
			asking = span
		}
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

// waiting narrows a query to what somebody has to look at.
//
// Three things, not one. A claim awaiting agreement is the obvious case. The
// other two are what happens when a judgment stops covering anything:
//
// A deferral that has run out has said what it was going to say. The finding
// is back, and if it does not appear here it simply reappears as new with the
// reasoning stranded behind it — which is the outcome marking a lapse exists
// to prevent.
//
// A decision the code moved out from under is the same shape: somebody made a
// judgment, it no longer applies, and they are the person who should be told.
//
// A claim that needed nobody — a short deferral — is not here at all. A work
// list containing work nobody has to do teaches people to skip rows.
func waiting(query *bun.SelectQuery, now time.Time) *bun.SelectQuery {
	return query.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.
			WhereOr("state = ? AND needs_approval = ? AND sent_back_at IS NULL", Proposed, true).
			WhereOr("state = ?", LapsedState).
			WhereOr("state IN (?, ?) AND outcome = ? AND deferred_until IS NOT NULL AND deferred_until <= ?",
				Proposed, Approved, Deferred, now)
	})
}
