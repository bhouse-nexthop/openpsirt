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
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
)

// Decision is one claim about one combination of code.
type Decision struct {
	bun.BaseModel `bun:"table:decision,alias:de"`

	ID              int64             `bun:"id,pk,autoincrement"`
	ProductID       int64             `bun:"product_id,notnull"`
	VulnerabilityID int64             `bun:"vulnerability_id,notnull"`
	PlaceIdentity   string            `bun:"place_identity,notnull"`
	Visibility      access.Visibility `bun:"visibility,notnull"`
	// The versions the claim was made against. Absent where nothing states
	// one, and absent for the consumer where the thing above is the product
	// itself — whose version changes every build and is excluded from expiry.
	ComponentUpstreamVersion *string    `bun:"component_upstream_version"`
	ConsumerUpstreamVersion  *string    `bun:"consumer_upstream_version"`
	Outcome                  Outcome    `bun:"outcome,notnull"`
	Justification            *string    `bun:"justification"`
	DeferredUntil            *time.Time `bun:"deferred_until"`
	// SeverityCenti is how bad this was judged to be when the claim was made,
	// in hundredths. Kept with the decision rather than read from the issue
	// later, because what a re-affirmation asks is whether severity has risen
	// *since* — and an issue's severity is rewritten in place as reports
	// revise it, so reading it now would compare a number against itself.
	SeverityCenti *int `bun:"severity_centi"`
	// NeedsApproval says a second person has to agree before this takes
	// effect. A short deferral does not, so it is a property of the claim
	// rather than of its outcome alone — and it has to be recorded, or a claim
	// that is waiting and one that is in force are indistinguishable.
	NeedsApproval bool      `bun:"needs_approval,notnull"`
	State         State     `bun:"state,notnull"`
	ProposedBy    int64     `bun:"proposed_by,notnull"`
	ProposedAt    time.Time `bun:"proposed_at,notnull"`
	RevisionID    *int64    `bun:"revision_id"`
}

// Revision is one statement of the reasoning behind a decision.
type Revision struct {
	bun.BaseModel `bun:"table:decision_revision,alias:dr"`

	ID         int64     `bun:"id,pk,autoincrement"`
	DecisionID int64     `bun:"decision_id,notnull"`
	Ordinal    int64     `bun:"ordinal,notnull"`
	Body       string    `bun:"body,notnull"`
	WrittenBy  int64     `bun:"written_by,notnull"`
	WrittenAt  time.Time `bun:"written_at,notnull"`
}

// Approval is a second person agreeing to one revision of the reasoning.
type Approval struct {
	bun.BaseModel `bun:"table:decision_approval,alias:da"`

	ID          int64      `bun:"id,pk,autoincrement"`
	DecisionID  int64      `bun:"decision_id,notnull"`
	RevisionID  int64      `bun:"revision_id,notnull"`
	ApprovedBy  int64      `bun:"approved_by,notnull"`
	ApprovedAt  time.Time  `bun:"approved_at,notnull"`
	WithdrawnAt *time.Time `bun:"withdrawn_at"`
	Batch       *string    `bun:"batch"`
}

// Place is what a decision is a claim about, as a finding presents it.
//
// Assembled from the finding and the components it points at rather than from
// anything a person typed, so that whether a decision applies is a question
// about the code and not about how somebody described it.
type Place struct {
	ProductID       int64
	VulnerabilityID int64
	PlaceIdentity   string
	// Visibility is the finding's, carried here so that what somebody may
	// decide about is answered where the query is rather than by whichever
	// handler happened to build this. A finding nobody has disclosed is one
	// only a private triager may argue about.
	Visibility access.Visibility
	// ComponentUpstream and ConsumerUpstream are the versions expiry compares.
	// They are the *upstream* versions: a shipped package carries a version of
	// its own that moves whenever it is rebuilt, and rebuilding is not a
	// reason to ask somebody the same question again.
	ComponentUpstream string
	ConsumerUpstream  string
}

// ErrNotTheirs is returned when somebody reaches for a decision about a
// product they may not triage.
//
// The same answer whether the product is one they cannot see or one they can
// only read: telling those apart would say which products exist to somebody
// who was told they may not ask.
var ErrNotTheirs = errors.New("not authorized")

// mayDecide reports whether a subject may argue about findings of this
// visibility on this product.
//
// Triage is its own right, held per product and per visibility. Reading a
// finding is not deciding about it — an approver or a reporter reaches plenty
// they may not judge — so this asks for the triage role rather than for
// whether they can see it.
func mayDecide(subject access.Subject, productID int64, visibility access.Visibility) bool {
	if subject.Kind != access.Person {
		return false
	}
	if visibility == access.Private {
		return subject.Holds(access.PrivateTriage, productID)
	}
	return subject.Holds(access.PublicTriage, productID) ||
		subject.Holds(access.PrivateTriage, productID)
}

// ErrSamePerson is returned when somebody tries to approve their own claim.
var ErrSamePerson = errors.New("the person who proposed a decision may not approve it")

// ErrNothingToApprove is returned when there is no current reasoning to
// approve, which means the decision is not in a state anybody can agree to.
var ErrNothingToApprove = errors.New("that decision has no reasoning to approve")

// Store reads and writes decisions.
type Store struct {
	db  bun.IDB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db bun.IDB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Proposal is somebody claiming something about a finding.
type Proposal struct {
	Place         Place
	Outcome       Outcome
	Justification Justification
	DeferredUntil *time.Time
	Reasoning     string
	By            int64
	// SeverityCenti is how bad this is judged to be right now, in hundredths.
	// Recorded with the claim so that a later re-affirmation can ask whether
	// it has risen since.
	SeverityCenti int
	// NeedsApproval says a second person must agree before this takes effect.
	// Worked out by the caller through NeedsApproval, and recorded, because a
	// claim that is waiting and one that is in force must be distinguishable
	// afterwards.
	NeedsApproval bool
}

// Propose records a claim and the reasoning behind it.
//
// The two are written together. A claim with no reasoning is not something a
// second person can agree to, and leaving the reasoning to a later write is
// how a decision ends up in the queue with nothing in it to review.
func (s *Store) Propose(ctx context.Context, subject access.Subject, p Proposal) (*Decision, error) {
	// Normalized before it is checked, not only before it is stored. Checking
	// the stated value and storing the careful one would let a place that
	// states nothing pass the check for disclosed findings and then be
	// recorded as undisclosed — authorized as one thing and kept as another.
	if !mayDecide(subject, p.Place.ProductID, visibilityOf(p.Place)) {
		return nil, ErrNotTheirs
	}
	if err := p.valid(); err != nil {
		return nil, err
	}
	if p.By != subject.ID {
		// A claim is somebody's, and recording it under another name would
		// make the second-person rule meaningless: anybody could propose as
		// somebody else and then agree with themselves.
		return nil, fmt.Errorf("a decision is recorded as made by whoever made it")
	}

	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}

	var recorded *Decision
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		var err error
		recorded, err = within.propose(ctx, p)
		return err
	})
	return recorded, err
}

func (s *Store) propose(ctx context.Context, p Proposal) (*Decision, error) {
	now := s.now().Truncate(time.Microsecond)
	decision := &Decision{
		ProductID: p.Place.ProductID, VulnerabilityID: p.Place.VulnerabilityID,
		PlaceIdentity:            p.Place.PlaceIdentity,
		Visibility:               visibilityOf(p.Place),
		ComponentUpstreamVersion: text(p.Place.ComponentUpstream),
		ConsumerUpstreamVersion:  text(p.Place.ConsumerUpstream),
		Outcome:                  p.Outcome,
		DeferredUntil:            p.DeferredUntil,
		State:                    Proposed,
		NeedsApproval:            p.NeedsApproval,
		ProposedBy:               p.By, ProposedAt: now,
	}
	if p.Outcome == NotApplicable {
		stated := string(p.Justification)
		decision.Justification = &stated
	}
	if p.SeverityCenti > 0 {
		judged := p.SeverityCenti
		decision.SeverityCenti = &judged
	}
	if _, err := s.db.NewInsert().Model(decision).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record a decision: %w", err)
	}

	revision := &Revision{
		DecisionID: decision.ID, Ordinal: 1,
		Body: p.Reasoning, WrittenBy: p.By, WrittenAt: now,
	}
	if _, err := s.db.NewInsert().Model(revision).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record the reasoning: %w", err)
	}
	if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
		Set("revision_id = ?", revision.ID).
		Where("id = ?", decision.ID).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record the reasoning: %w", err)
	}
	decision.RevisionID = &revision.ID
	return decision, nil
}

// valid reports whether a proposal says enough to be recorded.
func (p Proposal) valid() error {
	if !p.Outcome.Valid() {
		return fmt.Errorf("%q is not an outcome", p.Outcome)
	}
	if strings.TrimSpace(p.Reasoning) == "" {
		return errors.New("a decision needs reasoning, because somebody else has to agree with it")
	}
	// The same policy every other typed field goes through, run before the
	// text is stored rather than when it is read back. Stored text is then
	// known to have passed what was in force when it arrived.
	if err := markdown.Check(p.Reasoning); err != nil {
		return err
	}
	if p.By == 0 {
		return errors.New("a decision needs somebody to have made it")
	}
	// The claim that something does not affect us *is* which of the
	// recognized reasons applies, so it is not optional there — and it is
	// meaningless on the others, which are claims about priority rather than
	// about applicability.
	switch p.Outcome {
	case NotApplicable:
		if !p.Justification.Valid() {
			return fmt.Errorf("%q is not a recognized reason for something not applying", p.Justification)
		}
	default:
		if p.Justification != "" {
			return fmt.Errorf("%q states why something does not apply, which %q does not claim",
				p.Justification, p.Outcome)
		}
	}
	if p.Outcome == Deferred && p.DeferredUntil == nil {
		return errors.New("a deferral needs a date it returns on, or it is a decision never to look again")
	}
	if p.Outcome != Deferred && p.DeferredUntil != nil {
		return fmt.Errorf("%q does not return on a date", p.Outcome)
	}
	return nil
}

// text keeps an absent version absent rather than storing it as an empty one.
//
// The difference matters: a version nobody stated and a version that is the
// empty string would otherwise compare equal, and expiry is exactly a
// comparison of versions.
func text(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// visibilityOf reads a place's visibility, treating anything unset as not
// disclosed.
//
// Unset has to read as private, or a place assembled by something that forgot
// to state it would make a private finding argueable by anybody who can triage
// the public ones.
func visibilityOf(at Place) access.Visibility {
	if at.Visibility == access.Public {
		return access.Public
	}
	return access.Private
}

// reachable reports whether a subject may act on a decision, reading which
// product and visibility it belongs to from the row rather than from anything
// the caller stated.
func (s *Store) reachable(ctx context.Context, subject access.Subject, decisionID int64) error {
	decision := new(Decision)
	if err := s.db.NewSelect().Model(decision).
		Where("id = ?", decisionID).Scan(ctx); err != nil {
		// A decision somebody may not reach and one that does not exist get
		// the same answer, so that guessing identifiers says nothing.
		return ErrNotTheirs
	}
	if !mayDecide(subject, decision.ProductID, decision.Visibility) {
		return ErrNotTheirs
	}
	return nil
}

// reachableBy narrows a query to the decisions a subject may act on.
//
// Applied as a condition rather than by filtering afterwards, because a count,
// an export or a report is exactly where filtering afterwards gets forgotten —
// and where the number is the leak even when no row is shown.
//
// The products are bound as values. They come from the subject's own grants
// rather than from anything typed, so writing them into the statement would be
// safe today and would be the shape somebody copies later when the list does
// come from outside.
func reachableBy(query *bun.SelectQuery, subject access.Subject, column string) *bun.SelectQuery {
	if subject.Kind != access.Person {
		return query.Where("1 = 0")
	}
	products, all := subject.Products()
	if all {
		return query
	}

	// Kept apart because they permit different things: triage on undisclosed
	// findings implies triage on disclosed ones, and the reverse is exactly
	// what must not happen.
	var private, public []int64
	for _, id := range products {
		switch {
		case subject.Holds(access.PrivateTriage, id):
			private = append(private, id)
		case subject.Holds(access.PublicTriage, id):
			public = append(public, id)
		}
	}
	if len(private) == 0 && len(public) == 0 {
		return query.Where("1 = 0")
	}

	return query.WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
		if len(private) > 0 {
			q = q.WhereOr(column+".product_id IN (?)", bun.List(private))
		}
		if len(public) > 0 {
			q = q.WhereOr(column+".product_id IN (?) AND "+column+".visibility = ?",
				bun.List(public), access.Public)
		}
		return q
	})
}
