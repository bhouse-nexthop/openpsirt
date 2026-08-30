package triage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Decision is one claim about one combination of code.
type Decision struct {
	bun.BaseModel `bun:"table:decision,alias:de"`

	ID              int64  `bun:"id,pk,autoincrement"`
	ProductID       int64  `bun:"product_id,notnull"`
	VulnerabilityID int64  `bun:"vulnerability_id,notnull"`
	PlaceIdentity   string `bun:"place_identity,notnull"`
	// The versions the claim was made against. Absent where nothing states
	// one, and absent for the consumer where the thing above is the product
	// itself — whose version changes every build and is excluded from expiry.
	ComponentUpstreamVersion *string    `bun:"component_upstream_version"`
	ConsumerUpstreamVersion  *string    `bun:"consumer_upstream_version"`
	Outcome                  Outcome    `bun:"outcome,notnull"`
	Justification            *string    `bun:"justification"`
	DeferredUntil            *time.Time `bun:"deferred_until"`
	State                    State      `bun:"state,notnull"`
	ProposedBy               int64      `bun:"proposed_by,notnull"`
	ProposedAt               time.Time  `bun:"proposed_at,notnull"`
	RevisionID               *int64     `bun:"revision_id"`
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
	// ComponentUpstream and ConsumerUpstream are the versions expiry compares.
	// They are the *upstream* versions: a shipped package carries a version of
	// its own that moves whenever it is rebuilt, and rebuilding is not a
	// reason to ask somebody the same question again.
	ComponentUpstream string
	ConsumerUpstream  string
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
}

// Propose records a claim and the reasoning behind it.
//
// The two are written together. A claim with no reasoning is not something a
// second person can agree to, and leaving the reasoning to a later write is
// how a decision ends up in the queue with nothing in it to review.
func (s *Store) Propose(ctx context.Context, p Proposal) (*Decision, error) {
	if err := p.valid(); err != nil {
		return nil, err
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
		ComponentUpstreamVersion: text(p.Place.ComponentUpstream),
		ConsumerUpstreamVersion:  text(p.Place.ConsumerUpstream),
		Outcome:                  p.Outcome,
		DeferredUntil:            p.DeferredUntil,
		State:                    Proposed,
		ProposedBy:               p.By, ProposedAt: now,
	}
	if p.Outcome == NotApplicable {
		stated := string(p.Justification)
		decision.Justification = &stated
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
