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

// Claim is one proposer's action: what an approver reads and agrees to.
//
// A judgment about a finding writes one decision per place, and a judgment
// about many issues writes one per issue and per place. Those rows stay that
// fine, because each is keyed and lapses on its own. What a second person
// reads is the action — one argument, with its reach — and the queue, approval,
// sending back and undoing all work on that rather than on rows (TRI-45).
type Claim struct {
	bun.BaseModel `bun:"table:claim,alias:cl"`

	ID         int64     `bun:"id,pk,autoincrement"`
	Kind       ClaimKind `bun:"kind,notnull"`
	ProposedBy int64     `bun:"proposed_by,notnull"`
	ProposedAt time.Time `bun:"proposed_at,notnull"`
	// DerivedFrom is the claim this one came from: the approved claim an
	// extension carries to a new issue, or the claim an approver set some rows
	// aside from when agreeing to the rest.
	DerivedFrom *int64 `bun:"derived_from"`
	// SelectedBy is how a bulk set was narrowed. Held on the claim as well as
	// on its rows, so a claim whose rows were all set aside still says how it
	// was found.
	SelectedBy *string `bun:"selected_by"`
}

// ClaimKind is what sort of action a claim was.
type ClaimKind string

const (
	// FindingClaim is one judgment about one issue in one component,
	// covering the places it sits at. A re-affirmation is one too.
	FindingClaim ClaimKind = "finding"
	// TogetherClaim is one judgment about many issues at one component.
	TogetherClaim ClaimKind = "together"
	// ExtensionClaim carries an approved claim to a new issue at the same
	// component under the same consumer, with the same justification.
	ExtensionClaim ClaimKind = "extension"
	// ReturnedClaim holds the rows an approver set aside from a claim they
	// agreed the rest of. It goes back to whoever proposed them.
	ReturnedClaim ClaimKind = "returned"
)

// newClaim records an action, inside whatever transaction is writing its rows.
func (s *Store) newClaim(ctx context.Context, kind ClaimKind, by int64, derivedFrom *int64,
	selectedBy string) (*Claim, error) {

	claim := &Claim{
		Kind: kind, ProposedBy: by, ProposedAt: s.now().Truncate(time.Microsecond),
		DerivedFrom: derivedFrom,
	}
	if strings.TrimSpace(selectedBy) != "" {
		how := selectedBy
		claim.SelectedBy = &how
	}
	if _, err := s.db.NewInsert().Model(claim).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record a claim: %w", err)
	}
	return claim, nil
}

// ErrNotExtendable is returned when a claim cannot be carried to a new issue.
var ErrNotExtendable = errors.New("that claim cannot be extended")

// Extend records the same judgment an approved claim made, against a new issue
// at the same places.
//
// The everyday case rather than the rare one: every nightly scan adds issues
// to components that already carry agreed claims — a new flaw in a kernel
// driver the image does not build — and each arrived as a blank decision.
// An extension is a claim of its own, recorded as derived from the one it
// carries, and it needs a second person like any other dismissal: the approver
// is told the argument was read once already, not that it was agreed to twice
// (TRI-47).
//
// Everything it turns on is read inside the transaction that writes (DAT-31):
// whether the source is approved, and what it was a claim about.
func (s *Store) Extend(ctx context.Context, subject access.Subject, from int64,
	proposals []Proposal) ([]*Decision, error) {

	if len(proposals) == 0 {
		return nil, nil
	}
	for _, p := range proposals {
		if !mayDecide(subject, p.Place.ProductID, visibilityOf(p.Place)) {
			return nil, ErrNotTheirs
		}
		if err := p.valid(); err != nil {
			return nil, err
		}
		if p.By != subject.ID {
			return nil, fmt.Errorf("a decision is recorded as made by whoever made it")
		}
	}

	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}

	var recorded []*Decision
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		recorded = recorded[:0]

		source, err := within.extendable(ctx, subject, from, proposals)
		if err != nil {
			return err
		}
		claim, err := within.newClaim(ctx, ExtensionClaim, subject.ID, &source.ID, "")
		if err != nil {
			return err
		}
		for _, p := range proposals {
			one, err := within.propose(ctx, claim.ID, p)
			if err != nil {
				return err
			}
			recorded = append(recorded, one)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyDecided) {
			for _, p := range proposals {
				if standing, found := s.liveAt(ctx, liveKeyFor(p.Place)); found {
					return nil, fmt.Errorf(
						"%w: decision %d is already %s at one of these places — revise that one "+
							"rather than recording a second claim about the same code",
						ErrAlreadyDecided, standing.ID, standing.State)
				}
			}
		}
		return nil, err
	}
	return recorded, nil
}

// extendable reads the claim being carried and checks it can be.
//
// Three things have to hold, and each is a claim the extension would otherwise
// be making on the source's behalf: the source is agreed to — every row of it
// approved, none withdrawn or lapsed — so an extension never carries an
// argument nobody agreed with; the new rows sit at places the source sits at,
// in the same product, because "the same argument" is about the same code
// under the same consumer; and the outcome and justification are the source's,
// because a different conclusion is a different claim.
func (s *Store) extendable(ctx context.Context, subject access.Subject, from int64,
	proposals []Proposal) (*Claim, error) {

	source := new(Claim)
	if err := s.db.NewSelect().Model(source).Where("id = ?", from).Scan(ctx); err != nil {
		return nil, ErrNotTheirs
	}
	var rows []Decision
	if err := s.db.NewSelect().Model(&rows).Where("claim_id = ?", from).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what that claim covers: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrNotTheirs
	}
	places := map[string]bool{}
	for _, row := range rows {
		if !readable(subject, row.ProductID, row.Visibility) {
			return nil, ErrNotTheirs
		}
		if row.State != Approved {
			return nil, fmt.Errorf("%w: it is %s, and only an approved claim carries", ErrNotExtendable, row.State)
		}
		places[row.PlaceIdentity] = true
	}
	first := rows[0]
	for _, p := range proposals {
		if p.Place.ProductID != first.ProductID {
			return nil, fmt.Errorf("%w: it is about a different product", ErrNotExtendable)
		}
		if !places[p.Place.PlaceIdentity] {
			return nil, fmt.Errorf("%w: it is about a different component or consumer", ErrNotExtendable)
		}
		if p.Place.VulnerabilityID == first.VulnerabilityID {
			return nil, fmt.Errorf("%w: it already covers this issue", ErrNotExtendable)
		}
		if p.Outcome != first.Outcome || string(p.Justification) != orEmpty(first.Justification) {
			return nil, fmt.Errorf("%w: an extension keeps the outcome and justification it carries", ErrNotExtendable)
		}
	}
	return source, nil
}

// ClaimApproved is what agreeing to a claim did.
type ClaimApproved struct {
	// Approved is how many decisions were agreed to.
	Approved int
	// Returned is the claim the rows set aside went into, where any were.
	Returned *Claim
}

// ApproveClaim records a second person agreeing to a claim: every decision in
// it, as one action, under the same rules each decision is approved under.
//
// Rows may be set aside. An approver of a bulk claim who has found the handful
// of rows that do not look like the rest should not have to choose between
// refusing everything and agreeing to everything: the rest is approved as one
// claim, and the rows set aside go back to the proposer as a claim of their
// own, carrying the reason the way sending back does (TRI-46).
func (s *Store) ApproveClaim(ctx context.Context, subject access.Subject, claimID int64,
	batch string, except []int64, because string) (*ClaimApproved, error) {

	if len(except) > 0 {
		if strings.TrimSpace(because) == "" {
			return nil, fmt.Errorf("say why those are set aside: rows returned with no reason " +
				"are a round trip nobody learns from")
		}
		if err := markdown.Check(because); err != nil {
			return nil, err
		}
	}
	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}

	var result *ClaimApproved
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		var err error
		result, err = within.approveClaim(ctx, subject, claimID, batch, except, because)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) approveClaim(ctx context.Context, subject access.Subject, claimID int64,
	batch string, except []int64, because string) (*ClaimApproved, error) {

	claim, rows, err := s.claimRows(ctx, subject, claimID, mayApprove)
	if err != nil {
		return nil, err
	}
	aside := make(map[int64]bool, len(except))
	for _, id := range except {
		aside[id] = true
	}
	// Every row named as set aside has to be one of this claim's. A stray
	// identifier is more likely a mistake than a wish, and silently ignoring
	// it would approve a row somebody meant to hold back.
	named := make(map[int64]bool, len(rows))
	for _, row := range rows {
		named[row.ID] = true
	}
	for id := range aside {
		if !named[id] {
			return nil, fmt.Errorf("decision %d is not part of claim %d", id, claimID)
		}
	}

	// The same rules each row is approved under, asked of every row before
	// anything is written: not by whoever wrote the words or proposed the
	// claim, and only what is waiting and has words to agree to. A row
	// already approved, withdrawn or lapsed is left alone.
	authors, err := s.authorsOf(ctx, rows)
	if err != nil {
		return nil, err
	}
	var approving, returning []int64
	for _, row := range rows {
		if row.State != Proposed {
			continue
		}
		samePerson := authors[row.ID] == subject.ID || row.ProposedBy == subject.ID
		if aside[row.ID] {
			// Setting rows aside is an approver's act as much as agreeing
			// is — the rest of the claim is approved in the same action — so
			// the proposer naming their own rows as set aside would be acting
			// on their own claim.
			if samePerson {
				return nil, ErrSamePerson
			}
			returning = append(returning, row.ID)
			continue
		}
		// A row already with the author is not waiting on an approver, and
		// agreeing to it before they answer would undo the sending back. It
		// is not in the queue for the same reason.
		if row.SentBackAt != nil {
			continue
		}
		if row.RevisionID == nil {
			return nil, ErrNothingToApprove
		}
		if samePerson {
			return nil, ErrSamePerson
		}
		approving = append(approving, row.ID)
	}
	if len(approving) == 0 && len(returning) == 0 {
		return nil, ErrNothingToApprove
	}

	now := s.now().Truncate(time.Microsecond)
	result := &ClaimApproved{}
	if len(approving) > 0 {
		if err := s.approveRows(ctx, subject, approving, batch, now); err != nil {
			return nil, err
		}
		result.Approved = len(approving)
	}

	if len(returning) > 0 {
		returned, err := s.newClaim(ctx, ReturnedClaim, claim.ProposedBy, &claim.ID,
			orEmpty(claim.SelectedBy))
		if err != nil {
			return nil, err
		}
		// The reason travels as a comment, the way sending back records it:
		// the author needs the words, and a reason kept anywhere else is one
		// nobody reads.
		if err := s.sayToEach(ctx, subject, returning, because, now); err != nil {
			return nil, err
		}
		if _, err := s.db.NewUpdate().Model((*Decision)(nil)).
			Set("claim_id = ?", returned.ID).
			Set("sent_back_at = ?", now).
			Where("id IN (?)", bun.List(returning)).Exec(ctx); err != nil {
			return nil, fmt.Errorf("set rows aside: %w", err)
		}
		result.Returned = returned
	}
	return result, nil
}

// approveRows records one person agreeing to a set of decisions, as a set.
//
// A claim over a kernel is two thousand rows. Agreed to one row at a time —
// a count of what each covers and a conditional update per row — 1,760 rows
// took 15.6 s on the demo. As a set it is three statements whatever the size:
// the approvals inserted from a select over the rows, each carrying what its
// row covers as a correlated count; the rows moved to approved in one
// conditional update; and the matched count checked against what was meant.
//
// The revision-bound control is the condition on the update: an approval
// names the revision it was inserted against, and a row is moved only where
// that is still the revision it rests on. A revision landing in between
// leaves its row unmoved, the matched count falls short, and the whole claim
// is refused rather than half of it agreed to (DAT-35).
func (s *Store) approveRows(ctx context.Context, subject access.Subject, ids []int64,
	batch string, now time.Time) error {

	var named *string
	if strings.TrimSpace(batch) != "" {
		named = &batch
	}
	// What each row covers is counted at the moment of approval and kept,
	// for the same reason the per-row approval keeps it: a decision reaches
	// by matching, so asking later answers a different question from what
	// was agreed to. Narrowed to what this person may read, as the per-row
	// count is, so that the stored number discloses nothing.
	readable := readableVisibilities(subject, ids, s, ctx)
	inserted, err := s.db.ExecContext(ctx, `INSERT INTO "decision_approval" `+
		`("decision_id", "revision_id", "approved_by", "approved_at", "batch", "covered") `+
		`SELECT de.id, de.revision_id, ?, ?, ?, (`+
		`SELECT COUNT(*) FROM "finding" AS f `+
		`JOIN "target" AS tg ON tg.id = f.target_id `+
		`JOIN "stream" AS st ON st.id = tg.stream_id `+
		`JOIN "component" AS c ON c.id = f.component_id `+
		`LEFT JOIN "component" AS uc ON uc.id = f.consumer_id `+
		`WHERE st.product_id = de.product_id `+
		`AND f.vulnerability_id = de.vulnerability_id `+
		`AND f.place_identity = de.place_identity `+
		`AND f.closed_at IS NULL `+
		`AND COALESCE(de.component_upstream_version, '') = `+finding.ComponentUpstreamExpr+` `+
		`AND COALESCE(de.consumer_upstream_version, '') = `+finding.ConsumerUpstreamExpr+` `+
		`AND f.visibility IN (?)) `+
		`FROM "decision" AS de WHERE de.id IN (?) AND de.state = ? AND de.revision_id IS NOT NULL`,
		subject.ID, now, named, bun.List(readable), bun.List(ids), Proposed)
	if err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	if n, err := inserted.RowsAffected(); err == nil && n != int64(len(ids)) {
		return fmt.Errorf("the claim changed while this was being agreed to; read it again")
	}

	moved, err := s.db.ExecContext(ctx, `UPDATE "decision" SET "state" = ? `+
		`WHERE "id" IN (?) AND "state" = ? AND EXISTS (`+
		`SELECT 1 FROM "decision_approval" AS da WHERE da.decision_id = "decision"."id" `+
		`AND da.revision_id = "decision"."revision_id" `+
		`AND da.approved_by = ? AND da.approved_at = ? AND da.withdrawn_at IS NULL)`,
		Approved, bun.List(ids), Proposed, subject.ID, now)
	if err != nil {
		return fmt.Errorf("record an approval: %w", err)
	}
	if n, err := moved.RowsAffected(); err == nil && n != int64(len(ids)) {
		return fmt.Errorf("the reasoning changed while this was being agreed to; read it again")
	}
	return nil
}

// readableVisibilities is what this person may read across the products a
// set of rows sits in: private where they may read private on every one of
// those products, public only otherwise.
//
// A claim is one action on one build, so its rows share a product and this
// is the per-row rule asked once. Where a set did span products the answer
// is the narrower one, which discloses less rather than more.
func readableVisibilities(subject access.Subject, ids []int64, s *Store, ctx context.Context) []access.Visibility {
	var products []int64
	if err := s.db.NewSelect().Model((*Decision)(nil)).
		ColumnExpr("DISTINCT de.product_id").
		Where("de.id IN (?)", bun.List(ids)).Scan(ctx, &products); err != nil {
		return []access.Visibility{access.Public}
	}
	for _, product := range products {
		if !subject.Reads(access.Private, product) {
			return []access.Visibility{access.Public}
		}
	}
	return []access.Visibility{access.Public, access.Private}
}

// sayToEach records the same comment on every one of a set of decisions, in
// one statement.
func (s *Store) sayToEach(ctx context.Context, subject access.Subject, ids []int64, body string,
	now time.Time) error {

	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("a comment has to say something")
	}
	if err := markdown.Check(body); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO "decision_comment" `+
		`("decision_id", "body", "written_by", "written_at") `+
		`SELECT "id", ?, ?, ? FROM "decision" WHERE "id" IN (?)`,
		body, subject.ID, now, bun.List(ids)); err != nil {
		return fmt.Errorf("record a comment: %w", err)
	}
	return noting(ctx, s.db, body, now)
}

// authorsOf reads who wrote the words each decision currently rests on, in
// one statement.
func (s *Store) authorsOf(ctx context.Context, rows []Decision) (map[int64]int64, error) {
	wanted := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.RevisionID != nil {
			wanted = append(wanted, *row.RevisionID)
		}
	}
	authors := make(map[int64]int64, len(rows))
	if len(wanted) == 0 {
		return authors, nil
	}
	var revisions []Revision
	if err := s.db.NewSelect().Model(&revisions).
		Column("id", "decision_id", "written_by").
		Where("id IN (?)", bun.List(wanted)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read who wrote these: %w", err)
	}
	for _, revision := range revisions {
		authors[revision.DecisionID] = revision.WrittenBy
	}
	return authors, nil
}

// SentBack is what sending a claim back did.
type SentBack struct {
	// Authors is everybody whose words were sent back, each once, in the
	// order of the rows. Usually one person; a claim revised row by row can
	// rest on several people's words, and each of them is waiting to hear.
	Authors []int64
	// Sent is how many rows went back.
	Sent int
	// Decision is a representative of what went back: the earliest row.
	Decision Decision
	// Undisclosed says at least one row of the claim is about a finding
	// nobody has announced.
	//
	// Not read off Decision above. That row is a representative chosen by
	// identifier for naming the claim, and a claim is one action over many
	// places whose rows need not agree about visibility — so the earliest
	// row being public says nothing about the rest, and what may leave this
	// deployment is decided by the most careful row in the set (NTF-15).
	Undisclosed bool
}

// SendBackClaim asks the author for more before agreeing to any of a claim.
//
// Every waiting row leaves the queue together and comes back together when the
// author revises, because they are one argument: sending back half of it
// leaves an approver agreeing to words the author is about to change. As a
// set: one comment inserted per row from a select, one update.
func (s *Store) SendBackClaim(ctx context.Context, subject access.Subject, claimID int64,
	because string) (*SentBack, error) {

	if strings.TrimSpace(because) == "" {
		return nil, fmt.Errorf("say what needs to change: sending something back without a " +
			"reason is a round trip nobody learns from")
	}
	if err := markdown.Check(because); err != nil {
		return nil, err
	}
	db, ok := s.db.(*bun.DB)
	if !ok {
		return nil, fmt.Errorf("this store is already inside a transaction")
	}
	var result *SentBack
	err := database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		within := &Store{db: tx, now: s.now}
		result = &SentBack{}
		_, rows, err := within.claimRows(ctx, subject, claimID, mayApprove)
		if err != nil {
			return err
		}
		authors, err := within.authorsOf(ctx, rows)
		if err != nil {
			return err
		}
		var ids []int64
		told := map[int64]bool{}
		for _, row := range rows {
			if row.State != Proposed || row.SentBackAt != nil {
				continue
			}
			if authors[row.ID] == subject.ID {
				return fmt.Errorf("that is your own claim to revise, not one to send back")
			}
			if len(ids) == 0 {
				result.Decision = row
			}
			// Whether anything in this claim is undisclosed, which decides
			// what may be said about it outside the application (NTF-15). Any
			// row is enough: a claim is one action over many places and its
			// rows need not agree, so the representative row above answers
			// for the claim's identity and not for this.
			if row.Visibility == access.Private {
				result.Undisclosed = true
			}
			if author := authors[row.ID]; author != 0 && !told[author] {
				told[author] = true
				result.Authors = append(result.Authors, author)
			}
			ids = append(ids, row.ID)
		}
		if len(ids) == 0 {
			return fmt.Errorf("nothing in that claim is waiting on anybody")
		}
		now := s.now().Truncate(time.Microsecond)
		if err := within.sayToEach(ctx, subject, ids, because, now); err != nil {
			return err
		}
		marked, err := tx.NewUpdate().Model((*Decision)(nil)).
			Set("sent_back_at = ?", now).
			Where("id IN (?)", bun.List(ids)).Where("state = ?", Proposed).Exec(ctx)
		if err != nil {
			return fmt.Errorf("record that this was sent back: %w", err)
		}
		if n, err := marked.RowsAffected(); err == nil && n != int64(len(ids)) {
			return fmt.Errorf("the claim changed while it was being sent back; read it again")
		}
		result.Sent = len(ids)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// claimRows reads a claim and every decision in it, under a rule about what
// the subject may do with them.
//
// The rule is asked of every row, not of the first: a claim spans whatever one
// action wrote, and a person who may act on some of it and not the rest is
// refused the whole rather than handed the part — acting on a claim is acting
// on the argument, which does not come in halves. A claim they may reach none
// of answers as one that is not there, like everything else here.
func (s *Store) claimRows(ctx context.Context, subject access.Subject, claimID int64,
	allowed func(access.Subject, int64, access.Visibility) bool) (*Claim, []Decision, error) {

	claim := new(Claim)
	if err := s.db.NewSelect().Model(claim).Where("id = ?", claimID).Scan(ctx); err != nil {
		return nil, nil, ErrNotTheirs
	}
	var rows []Decision
	if err := s.db.NewSelect().Model(&rows).
		Where("claim_id = ?", claimID).Order("id ASC").Scan(ctx); err != nil {
		return nil, nil, fmt.Errorf("read what that claim covers: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil, ErrNotTheirs
	}
	for _, row := range rows {
		if !allowed(subject, row.ProductID, row.Visibility) {
			return nil, nil, ErrNotTheirs
		}
	}
	return claim, rows, nil
}
