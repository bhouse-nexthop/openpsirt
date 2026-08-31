package triage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
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
	NeedsApproval bool  `bun:"needs_approval,notnull"`
	State         State `bun:"state,notnull"`
	// SentBackAt marks that an approver asked for more before they would
	// agree, and is cleared when the author revises. Deliberately not a state:
	// the claim is still proposed and still suppresses nothing, and what
	// changed is whose turn it is.
	SentBackAt *time.Time `bun:"sent_back_at"`
	ProposedBy int64      `bun:"proposed_by,notnull"`
	ProposedAt time.Time  `bun:"proposed_at,notnull"`
	RevisionID *int64     `bun:"revision_id"`
	// LiveKey is what this decision is a claim about, while it is still a live
	// claim. Null once it is withdrawn or has lapsed, which is what lets a
	// fresh claim be made at a place a dead one used to cover.
	LiveKey *string `bun:"live_key"`
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
	// Covered is how many findings this claim covered when it was agreed to.
	//
	// Kept rather than worked out later. A decision reaches by matching, so a
	// build appearing afterwards is covered without anybody acting — asking
	// what it covers *now* answers a different question from what somebody
	// consented to, and only one of those two can be recovered after the fact.
	Covered *int `bun:"covered"`
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

// mayApprove reports whether a subject may agree to somebody else's claim.
//
// Approving is not deciding, and requiring the triage role for it made the
// approver capability decorative: somebody granted exactly the right to
// approve could not approve anything. It is a capability rather than a grant
// of visibility, so it is asked alongside whether they may read the finding —
// otherwise handing somebody the ability to approve hands them everything
// there is to approve.
//
// A triager may also approve, on somebody else's claim. Two triagers agreeing
// to each other's work is the ordinary shape of a small team, and the control
// that matters is that the two are different people — which is checked
// separately and has no override.
func mayApprove(subject access.Subject, productID int64, visibility access.Visibility) bool {
	if subject.Kind != access.Person {
		return false
	}
	if !subject.Reads(visibility, productID) {
		return false
	}
	return subject.Holds(access.Approver, productID) || mayDecide(subject, productID, visibility)
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
	if errors.Is(err, ErrAlreadyDecided) {
		// Read now the transaction has unwound, so the refusal can say which
		// claim to go and read rather than which constraint was violated.
		if standing, found := s.liveAt(ctx, liveKeyFor(p.Place)); found {
			return nil, fmt.Errorf(
				"%w: decision %d is already %s here — revise that one rather than recording a "+
					"second claim about the same code",
				ErrAlreadyDecided, standing.ID, standing.State)
		}
	}
	return recorded, err
}

func (s *Store) propose(ctx context.Context, p Proposal) (*Decision, error) {
	now := s.now().Truncate(time.Microsecond)
	key := liveKeyFor(p.Place)
	liveKey := &key
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
		LiveKey:                  liveKey,
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
		if database.IsDuplicate(err) {
			// The unique index over the live key refused it, which is the only
			// thing that could: two proposals arriving together both walk
			// through any check made before the write. Naming the claim that
			// is already there happens outside this transaction — a failed
			// write leaves nothing else in it able to read.
			return nil, ErrAlreadyDecided
		}
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

// version is how a version string is read, everywhere it is read.
//
// Surrounding space is not part of a version. It has to be taken off in one
// place because the two halves disagreed otherwise: storing treated a
// whitespace-only version as absent, and matching treated it as a version
// that happened to be spaces — so a decision was written against nothing and
// looked for something, and could never apply to the place it was made about.
func version(s string) string { return strings.TrimSpace(s) }

// text keeps an absent version absent rather than storing it as an empty one.
//
// The difference matters: a version nobody stated and a version that is the
// empty string would otherwise compare equal, and expiry is exactly a
// comparison of versions.
func text(s string) *string {
	trimmed := version(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// liveKeyFor is what a decision is a claim about: the place, and both upstream
// versions it was made against.
//
// Hashed rather than stored as its parts, because it exists to be compared for
// equality under a unique index and nothing ever reads it back. The versions
// are normalized the same way they are everywhere else, so a claim written with
// spaces around a version collides with one written without — which is the
// whole point of a uniqueness rule.
func liveKeyFor(at Place) string {
	basis := strings.Join([]string{
		strconv.FormatInt(at.ProductID, 10),
		strconv.FormatInt(at.VulnerabilityID, 10),
		at.PlaceIdentity,
		version(at.ComponentUpstream),
		version(at.ConsumerUpstream),
	}, "\x00")
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

// ErrAlreadyDecided is returned when a live claim already covers this exact
// combination of code.
//
// The answer is to revise that claim rather than to make a second one beside
// it: two claims about one finding are a disagreement, and a disagreement
// belongs in one place where both sides are readable.
var ErrAlreadyDecided = errors.New("a decision already stands here")

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

// liveAt reads the claim currently standing over a combination of code, if
// there is one. Used to explain a refusal rather than to prevent one.
func (s *Store) liveAt(ctx context.Context, key string) (*Decision, bool) {
	standing := new(Decision)
	if err := s.db.NewSelect().Model(standing).
		Where("live_key = ?", key).Scan(ctx); err != nil {
		return nil, false
	}
	return standing, true
}

// reachable reports whether a subject may act on a decision, reading which
// product and visibility it belongs to from the row rather than from anything
// the caller stated.
func (s *Store) reachable(ctx context.Context, subject access.Subject, decisionID int64) error {
	_, err := s.reaching(ctx, subject, decisionID, mayDecide)
	return err
}

// reaching finds a decision a subject may act on, under a given rule.
//
// The rule differs by act: arguing about a finding needs the triage role,
// while agreeing to somebody else's claim is the approver capability alongside
// being able to read it. Reading what was decided follows whichever act
// produced it — anybody who could have taken part can see what came of it.
func (s *Store) reaching(ctx context.Context, subject access.Subject, decisionID int64,
	allowed func(access.Subject, int64, access.Visibility) bool) (*Decision, error) {

	decision := new(Decision)
	if err := s.db.NewSelect().Model(decision).
		Where("id = ?", decisionID).Scan(ctx); err != nil {
		// A decision somebody may not reach and one that does not exist get
		// the same answer, so that guessing identifiers says nothing.
		return nil, ErrNotTheirs
	}
	if !allowed(subject, decision.ProductID, decision.Visibility) {
		return nil, ErrNotTheirs
	}
	return decision, nil
}

// readable reports whether a subject may see what was decided.
//
// Wider than deciding and wider than approving, and deliberately so: an
// approver has to read a claim to judge it, and somebody who took part in a
// discussion has to be able to read it back. It is still bounded by what they
// may read, so a finding nobody has disclosed stays where it was.
func readable(subject access.Subject, productID int64, visibility access.Visibility) bool {
	return mayApprove(subject, productID, visibility) || mayDecide(subject, productID, visibility)
}

// approvableBy narrows a query to the decisions a subject may agree to, which
// is a wider set than the ones they may argue about.
func approvableBy(query *bun.SelectQuery, subject access.Subject, column string) *bun.SelectQuery {
	return narrowedBy(query, subject, column, mayApprove)
}

// readableBy narrows a query to the decisions a subject may see.
func readableBy(query *bun.SelectQuery, subject access.Subject, column string) *bun.SelectQuery {
	return narrowedBy(query, subject, column, readable)
}

// narrowedBy applies one of those rules as a condition on the query.
//
// Written as a condition rather than as filtering afterwards, because a count,
// an export or a report is exactly where filtering afterwards gets forgotten —
// and where the number is the leak even when no row is shown.
//
// Public and private are kept apart because they permit different things:
// reaching undisclosed findings implies reaching disclosed ones, and the
// reverse is exactly what must not happen. The rule is asked separately for
// each, per product, so a new right cannot widen one by being written into the
// other.
//
// The products are bound as values. They come from the subject's own grants
// rather than from anything typed, so writing them into the statement would be
// safe today and would be the shape somebody copies later when the list does
// come from outside.
func narrowedBy(query *bun.SelectQuery, subject access.Subject, column string,
	allowed func(access.Subject, int64, access.Visibility) bool) *bun.SelectQuery {

	if subject.Kind != access.Person {
		return query.Where("1 = 0")
	}
	products, all := subject.Products()
	if all {
		return query
	}

	var private, public []int64
	for _, id := range products {
		switch {
		case allowed(subject, id, access.Private):
			private = append(private, id)
		case allowed(subject, id, access.Public):
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
