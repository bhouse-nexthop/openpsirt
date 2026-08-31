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
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
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
		// The same answer a decision somebody may not reach gets, so that
		// guessing identifiers says nothing about what exists.
		return ErrNotTheirs
	}
	// Read from the row rather than from what the caller said about it. A
	// caller that could name the product would be deciding what it may reach.
	//
	// Agreeing is its own right, not the right to argue: somebody granted
	// exactly the approver capability may do this, and asking for the triage
	// role instead made that grant do nothing at all.
	if !mayApprove(subject, decision.ProductID, decision.Visibility) {
		return ErrNotTheirs
	}
	// Compared against whoever wrote the words being agreed to, not against
	// whoever first proposed the claim. An approval names one revision, so the
	// control is about the text — and anybody who may triage can revise, which
	// made "did you propose this" the wrong question: revise somebody else's
	// claim in your own words and you could then approve your own words.
	author, err := s.authorOf(ctx, *decision)
	if err != nil {
		return err
	}
	if author == subject.ID || decision.ProposedBy == subject.ID {
		return ErrSamePerson
	}
	if decision.RevisionID == nil {
		return ErrNothingToApprove
	}
	if decision.State == Withdrawn || decision.State == LapsedState {
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
	// Counted now, and kept. A build appearing later is covered without
	// anybody acting, so asking again in a year answers what it covers then —
	// which is a useful question, and a different one from what was agreed to.
	if covered, err := s.covering(ctx, *decision); err == nil {
		approval.Covered = &covered
	}
	if _, err := s.db.NewInsert().Model(approval).Exec(ctx); err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	// Conditional on the revision that was read at the top still being the
	// current one. A revision landing in between replaces the words and
	// withdraws every agreement to them; without this condition the approval
	// would then mark the decision agreed while naming words that no longer
	// stand — an agreement floating free of the text, which is the one thing
	// naming a revision exists to prevent.
	result, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Approved).
		Where("id = ?", decisionID).
		Where("revision_id = ?", *decision.RevisionID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	if moved, err := result.RowsAffected(); err == nil && moved == 0 {
		return fmt.Errorf("the reasoning changed while this was being agreed to; read it again")
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
	if err := markdown.Check(reasoning); err != nil {
		return nil, err
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
		return nil, ErrNotTheirs
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
		// Whatever was asked for has been answered, or at least responded to.
		// Leaving the mark would keep the claim out of the approval queue
		// forever, which is the failure that makes sending back unusable.
		Set("sent_back_at = ?", nil).
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
	db, ok := s.db.(*bun.DB)
	if !ok {
		return fmt.Errorf("this store is already inside a transaction")
	}
	// Both writes or neither. Half of this leaves a decision reading as agreed
	// to with every agreement marked withdrawn — which is the state the whole
	// approval record exists to make impossible.
	return database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		if err := within.reachable(ctx, subject, decisionID); err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		if _, err := tx.NewUpdate().Model((*Approval)(nil)).
			Set("withdrawn_at = ?", now).
			Where("decision_id = ?", decisionID).
			Where("withdrawn_at IS NULL").Exec(ctx); err != nil {
			return fmt.Errorf("withdraw a decision: %w", err)
		}
		if _, err := tx.NewUpdate().Model((*Decision)(nil)).
			Set("state = ?", Withdrawn).
			// Released, so the place is open to a fresh claim. A withdrawn
			// decision is history, and history must not stop anybody deciding.
			Set("live_key = ?", nil).
			Where("id = ?", decisionID).Exec(ctx); err != nil {
			return fmt.Errorf("withdraw a decision: %w", err)
		}
		return nil
	})
}

// UndoBatch takes back everything one bulk approval agreed to.
//
// A reviewer may approve a long selection in one action, so undoing has to be
// available at the same size. Hunting for what a bulk approval touched, one
// row at a time, is not an undo anybody will actually use.
func (s *Store) UndoBatch(ctx context.Context, subject access.Subject, batch string) (int64, error) {
	db, ok := s.db.(*bun.DB)
	if !ok {
		return 0, fmt.Errorf("this store is already inside a transaction")
	}
	var undone int64
	// Applied whole, and every read it decides from is inside it. Reading
	// which decisions a batch covered and then writing outside that read lets
	// a decision withdrawn in between be flipped back to waiting.
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		var err error
		undone, err = (&Store{db: tx, now: s.now}).undoBatch(ctx, subject, batch)
		return err
	})
	return undone, err
}

func (s *Store) undoBatch(ctx context.Context, subject access.Subject, batch string) (int64, error) {
	now := s.now().Truncate(time.Microsecond)

	// Narrowed to what this person may reach before anything is undone. A
	// batch is one reviewer's afternoon and may span products, so undoing it
	// wholesale would let somebody act on products they hold nothing on.
	var decisions []int64
	covered := s.db.NewSelect().Model((*Approval)(nil)).
		ColumnExpr("da.decision_id").
		Join("JOIN decision AS d ON d.id = da.decision_id").
		Where("da.batch = ?", batch).Where("da.withdrawn_at IS NULL")
	covered = approvableBy(covered, subject, "d")
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
	//
	// Only where nothing else still agrees. A decision may carry more than one
	// agreement, and undoing a batch is undoing that batch — sending a
	// decision back to the queue while somebody's standing agreement to it is
	// still recorded would discard an agreement nobody took back.
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("state = ?", Proposed).
		Where("id IN (?)", bun.List(decisions)).
		Where("NOT EXISTS (SELECT 1 FROM decision_approval AS still " +
			"WHERE still.decision_id = de.id AND still.withdrawn_at IS NULL)").
		Exec(ctx); err != nil {
		return 0, fmt.Errorf("undo an approval: %w", err)
	}
	return int64(len(decisions)), nil
}

// authorOf returns who wrote the reasoning a decision currently rests on.
func (s *Store) authorOf(ctx context.Context, decision Decision) (int64, error) {
	if decision.RevisionID == nil {
		return 0, ErrNothingToApprove
	}
	revision := new(Revision)
	if err := s.db.NewSelect().Model(revision).
		Where("id = ?", *decision.RevisionID).Scan(ctx); err != nil {
		return 0, fmt.Errorf("read who wrote this: %w", err)
	}
	return revision.WrittenBy, nil
}

// SendBack asks the author for more before agreeing.
//
// The third thing an approver needs. Approving and withdrawing were the only
// two, and withdrawing throws away somebody's work over a missing sentence —
// so what actually happened was a comment, and the claim sat in the queue
// looking untouched.
//
// The words are required. A claim sent back with no reason is a round trip
// nobody learns from, and it is the whole of what the author needs.
//
// It needs no approval of its own, for the same reason revising and
// withdrawing do not: it puts risk back on the table rather than taking it
// off.
func (s *Store) SendBack(ctx context.Context, subject access.Subject, decisionID int64,
	because string) error {

	if strings.TrimSpace(because) == "" {
		return fmt.Errorf("say what needs to change: sending something back without a reason " +
			"is a round trip nobody learns from")
	}
	if err := markdown.Check(because); err != nil {
		return err
	}

	db, ok := s.db.(*bun.DB)
	if !ok {
		return fmt.Errorf("this store is already inside a transaction")
	}
	return database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		decision, err := within.reaching(ctx, subject, decisionID, mayApprove)
		if err != nil {
			return err
		}
		if decision.State != Proposed {
			return fmt.Errorf("that decision is %s, so there is nothing waiting on anybody",
				decision.State)
		}
		author, err := within.authorOf(ctx, *decision)
		if err != nil {
			return err
		}
		if author == subject.ID {
			return fmt.Errorf("that is your own claim to revise, not one to send back")
		}

		// The reason travels as a comment, because that is what it is: the
		// author needs the words, and a reason recorded anywhere else is one
		// nobody reads.
		if _, err := within.Say(ctx, subject, decisionID, because); err != nil {
			return err
		}
		if _, err := tx.NewUpdate().Model((*Decision)(nil)).
			Set("sent_back_at = ?", s.now().Truncate(time.Microsecond)).
			Where("id = ?", decisionID).Exec(ctx); err != nil {
			return fmt.Errorf("record that this was sent back: %w", err)
		}
		return nil
	})
}

// covering counts the open findings a claim covers right now.
//
// The same match a finding makes when it asks whether a decision applies to
// it: the place, and both upstream versions. Read at the moment of approval
// and kept, because a decision reaches by matching and so covers more as
// builds appear — with nobody having acted, and nobody having agreed to the
// larger number.
func (s *Store) covering(ctx context.Context, decision Decision) (int, error) {
	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		Where("st.product_id = ?", decision.ProductID).
		Where("f.vulnerability_id = ?", decision.VulnerabilityID).
		Where("f.place_identity = ?", decision.PlaceIdentity).
		Where("f.closed_run_id IS NULL").
		Where("COALESCE(?, '') = "+finding.ComponentUpstreamExpr, decision.ComponentUpstreamVersion).
		Where("COALESCE(?, '') = "+finding.ConsumerUpstreamExpr, decision.ConsumerUpstreamVersion)

	covered, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count what this covers: %w", err)
	}
	return covered, nil
}
