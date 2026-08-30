package triage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Approve records a second person agreeing to what a decision currently says.
//
// Against one revision, not against the decision. The whole value of a second
// pair of eyes is that they read particular words; an approval that floats
// free of the words would still be standing after somebody rewrote them, and
// nothing would report that.
//
// The proposer may never be the approver, with no override. A one-person
// deployment therefore cannot approve anything, which is the control working
// rather than a gap in it — and it is better said plainly than quietly
// relaxed.
func (s *Store) Approve(ctx context.Context, subject access.Subject, decisionID int64, batch string) error {
	db, ok := s.db.(*bun.DB)
	if !ok {
		return fmt.Errorf("this store is already inside a transaction")
	}
	return database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		return (&Store{db: tx, now: s.now}).approve(ctx, subject, decisionID, batch)
	})
}

func (s *Store) approve(ctx context.Context, subject access.Subject, decisionID int64, batch string) error {
	decision := new(Decision)
	if err := s.db.NewSelect().Model(decision).
		Where("id = ?", decisionID).Scan(ctx); err != nil {
		return fmt.Errorf("no decision to approve: %w", err)
	}
	// Read from the row rather than from what the caller said about it. A
	// caller that could name the product would be deciding what it may reach.
	if !mayDecide(subject, decision.ProductID, decision.Visibility) {
		return ErrNotTheirs
	}
	if decision.ProposedBy == subject.ID {
		return ErrSamePerson
	}
	if decision.RevisionID == nil {
		return ErrNothingToApprove
	}
	if decision.State == Withdrawn || decision.State == Lapsed {
		return fmt.Errorf("that decision is %s, so there is nothing standing to agree to", decision.State)
	}

	now := s.now().Truncate(time.Microsecond)
	approval := &Approval{
		DecisionID: decisionID, RevisionID: *decision.RevisionID,
		ApprovedBy: subject.ID, ApprovedAt: now,
	}
	if strings.TrimSpace(batch) != "" {
		approval.Batch = &batch
	}
	if _, err := s.db.NewInsert().Model(approval).Exec(ctx); err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Approved).Where("id = ?", decisionID).Exec(ctx); err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	return nil
}

// Revise states the reasoning again, and takes back any approval standing on
// what it said before.
//
// Withdrawing the approval is the point rather than a side effect. A second
// person agreed to particular words; different words are a claim nobody has
// agreed to, and letting the approval carry over would defeat the control
// silently — which is worse than not having it, because the record would say
// two people had read something only one of them had.
//
// It needs no approval of its own. Returning something to the queue re-exposes
// risk rather than hiding it, and the queue exists to stop risk being hidden
// unseen.
func (s *Store) Revise(ctx context.Context, subject access.Subject, decisionID int64, reasoning string) (*Revision, error) {
	if strings.TrimSpace(reasoning) == "" {
		return nil, errors.New("a revision has to say something")
	}
	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}

	var written *Revision
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		var err error
		written, err = within.revise(ctx, subject, decisionID, reasoning)
		return err
	})
	return written, err
}

func (s *Store) revise(ctx context.Context, subject access.Subject, decisionID int64, reasoning string) (*Revision, error) {
	if err := s.reachable(ctx, subject, decisionID); err != nil {
		return nil, err
	}

	var latest int64
	if err := s.db.NewSelect().Model((*Revision)(nil)).
		ColumnExpr("COALESCE(MAX(ordinal), 0)").
		Where("decision_id = ?", decisionID).Scan(ctx, &latest); err != nil {
		return nil, fmt.Errorf("read what has been said already: %w", err)
	}
	if latest == 0 {
		return nil, fmt.Errorf("no decision to revise")
	}

	now := s.now().Truncate(time.Microsecond)
	revision := &Revision{
		DecisionID: decisionID, Ordinal: latest + 1,
		Body: reasoning, WrittenBy: subject.ID, WrittenAt: now,
	}
	if _, err := s.db.NewInsert().Model(revision).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record a revision: %w", err)
	}

	// Every approval standing on the old words is taken back, and the decision
	// goes back to being a proposal. It is marked as having been approved
	// before — an approver meeting it again should know they are re-reading
	// something rather than seeing it for the first time — which is what the
	// kept approval rows say.
	if _, err := s.db.NewUpdate().Model((*Approval)(nil)).
		Set("withdrawn_at = ?", now).
		Where("decision_id = ?", decisionID).
		Where("withdrawn_at IS NULL").Exec(ctx); err != nil {
		return nil, fmt.Errorf("withdraw the approvals on what was revised: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("revision_id = ?", revision.ID).
		Set("state = ?", Proposed).
		Where("id = ?", decisionID).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record a revision: %w", err)
	}
	return revision, nil
}

// Withdraw takes a decision back.
//
// No approval needed, for the same reason revising needs none: it puts risk
// back on the table rather than taking it off.
func (s *Store) Withdraw(ctx context.Context, subject access.Subject, decisionID int64) error {
	if err := s.reachable(ctx, subject, decisionID); err != nil {
		return err
	}
	now := s.now().Truncate(time.Microsecond)
	if _, err := s.db.NewUpdate().Model((*Approval)(nil)).
		Set("withdrawn_at = ?", now).
		Where("decision_id = ?", decisionID).
		Where("withdrawn_at IS NULL").Exec(ctx); err != nil {
		return fmt.Errorf("withdraw a decision: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Withdrawn).Where("id = ?", decisionID).Exec(ctx); err != nil {
		return fmt.Errorf("withdraw a decision: %w", err)
	}
	return nil
}

// UndoBatch takes back everything one bulk approval agreed to.
//
// A reviewer may approve a long selection in one action, so undoing has to be
// available at the same size. Hunting for what a bulk approval touched, one
// row at a time, is not an undo anybody will actually use.
func (s *Store) UndoBatch(ctx context.Context, subject access.Subject, batch string) (int64, error) {
	now := s.now().Truncate(time.Microsecond)

	// Narrowed to what this person may reach before anything is undone. A
	// batch is one reviewer's afternoon and may span products, so undoing it
	// wholesale would let somebody act on products they hold nothing on.
	var decisions []int64
	covered := s.db.NewSelect().Model((*Approval)(nil)).
		ColumnExpr("da.decision_id").
		Join("JOIN decision AS d ON d.id = da.decision_id").
		Where("da.batch = ?", batch).Where("da.withdrawn_at IS NULL")
	covered = reachableBy(covered, subject, "d")
	if err := covered.Scan(ctx, &decisions); err != nil {
		return 0, fmt.Errorf("read what that approval covered: %w", err)
	}
	if len(decisions) == 0 {
		return 0, nil
	}

	if _, err := s.db.NewUpdate().Model((*Approval)(nil)).
		Set("withdrawn_at = ?", now).
		Where("batch = ?", batch).Where("withdrawn_at IS NULL").
		Where("decision_id IN (?)", bun.List(decisions)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("undo an approval: %w", err)
	}
	// Back to proposed rather than withdrawn: the claims still stand, it is
	// the agreement to them that was taken back.
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Proposed).
		Where("id IN (?)", bun.List(decisions)).Exec(ctx); err != nil {
		return 0, fmt.Errorf("undo an approval: %w", err)
	}
	return int64(len(decisions)), nil
}
