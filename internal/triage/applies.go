package triage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Applying returns the decision standing against a place, if one is.
//
// Expiry happens here and is not a mechanism. A decision was stored under the
// versions it was a claim about; a place asks under the versions it has now.
// When a version moves the two keys stop matching and the decision simply does
// not come back — nothing sweeps, nothing expires on a timer, and there is no
// second rule that could disagree with the first.
//
// That is why only the *upstream* versions are compared. A shipped package is
// rebuilt constantly and carries a version of its own that moves each time; a
// rebuild is not somebody's reasoning becoming wrong. What changes the
// reasoning is the code changing, which is what an upstream version moving
// says.
//
// It follows that a producer which patches rather than bumps will not lapse
// decisions this way at all. That is accepted rather than worked around: a
// patch is our own change to our own build, and it would be a poor trade to
// re-ask every question every night in order to catch the few that a patch
// made stale. What compensates is that a decision's age is shown wherever it
// appears, so an old judgment looks like one.
// It takes no subject, unlike everything else read here, because the question
// is not a person's: it is whether this finding is suppressed, asked while
// recording what a scan found and while listing findings for anybody at all.
// The answer does not vary by who is asking. Every path that reaches a place
// resolved it through a lookup that carried a subject, so nothing gets here
// without the finding itself having been authorized.
func (s *Store) Applying(ctx context.Context, at Place) (*Decision, error) {
	decision := new(Decision)
	query := s.db.NewSelect().Model(decision).
		Where("product_id = ?", at.ProductID).
		Where("vulnerability_id = ?", at.VulnerabilityID).
		Where("place_identity = ?", at.PlaceIdentity).
		// Approved, or proposed and never needing agreement. A claim that
		// hides risk and is waiting for a second person does not suppress
		// anything in the meantime — otherwise the queue is decorative and one
		// person can dismiss a finding on their own, which is the whole thing
		// the second pair of eyes exists to prevent.
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereOr("state = ?", Approved).
				WhereOr("state = ? AND needs_approval = ?", Proposed, false)
		})

	query = matchVersion(query, "component_upstream_version", at.ComponentUpstream)
	query = matchVersion(query, "consumer_upstream_version", at.ConsumerUpstream)

	// An agreed claim outranks a waiting one. Ordering by identifier alone let
	// a newer unapproved claim shadow an approved one, which is a way for one
	// person to overturn a decision two people made.
	if err := query.OrderExpr("CASE WHEN state = ? THEN 0 ELSE 1 END, id DESC", Approved).
		Limit(1).Scan(ctx); err != nil {
		// No decision standing is an answer. Anything else is a fault, and
		// reporting it as "nothing stands" would turn a lost race or a lock
		// timeout into a suppressed finding reappearing — or, in an export,
		// into an agreed dismissal silently dropped.
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read what stands here: %w", err)
	}

	// A deferral says "not now, ask again on this date". Once the date has
	// passed it stops standing, and the finding is back in the queue — flagged
	// as something that was deferred rather than as something new, which the
	// kept decision is what says.
	if decision.Outcome == Deferred && decision.DeferredUntil != nil {
		if !s.now().Before(*decision.DeferredUntil) {
			return nil, nil
		}
	}
	return decision, nil
}

// PreviouslyAt returns decisions once made about a place, whatever versions
// they were made against.
//
// The structural half of the key on its own. When a version moves and a
// decision stops applying, somebody has to make the judgment again — and
// making them start from a blank page, having thrown away the reasoning that
// was written the last time, is how a tool teaches people to stop writing
// reasoning at all.
//
// So what comes back is history rather than an answer: the claim that used to
// stand, and the versions it was about. Whether it still holds is the
// question being put, not something this decides.
func (s *Store) PreviouslyAt(ctx context.Context, subject access.Subject, at Place) ([]Decision, error) {
	var previous []Decision
	if err := readableBy(s.db.NewSelect().Model(&previous), subject, "de").
		Where("product_id = ?", at.ProductID).
		Where("vulnerability_id = ?", at.VulnerabilityID).
		Where("place_identity = ?", at.PlaceIdentity).
		Order("id DESC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what was decided here before: %w", err)
	}
	return previous, nil
}

// matchVersion constrains a query to one stated version, or to none.
//
// A version nobody stated is not the same as a version that is empty, and
// comparing the two as equal would let a decision made about a component with
// no known version stand over one whose version is simply blank. Two absences
// match each other and nothing else.
func matchVersion(query *bun.SelectQuery, column, stated string) *bun.SelectQuery {
	if stated == "" {
		return query.Where(column + " IS NULL")
	}
	return query.Where(column+" = ?", stated)
}

// Revisions returns the reasoning behind a decision, oldest first.
//
// All of it. An approval names one revision, so reading only the current one
// leaves somebody unable to see what was actually agreed to.
func (s *Store) Revisions(ctx context.Context, subject access.Subject, decisionID int64) ([]Revision, error) {
	if _, err := s.reaching(ctx, subject, decisionID, readable); err != nil {
		return nil, err
	}
	var revisions []Revision
	if err := s.db.NewSelect().Model(&revisions).
		Where("decision_id = ?", decisionID).
		Order("ordinal ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read the reasoning: %w", err)
	}
	return revisions, nil
}

// Approvals returns who agreed to a decision and to which words, including
// agreements later taken back.
func (s *Store) Approvals(ctx context.Context, subject access.Subject, decisionID int64) ([]Approval, error) {
	if _, err := s.reaching(ctx, subject, decisionID, readable); err != nil {
		return nil, err
	}
	var approvals []Approval
	if err := s.db.NewSelect().Model(&approvals).
		Where("decision_id = ?", decisionID).
		Order("id ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read who agreed: %w", err)
	}
	return approvals, nil
}

// Age is how long a decision has stood.
//
// Shown wherever a decision appears. It is the compensating control for expiry
// being inert against a producer that patches rather than bumps: an eight-year
// -old judgment should look like one rather than reading the same as
// yesterday's.
func (s *Store) Age(decision *Decision) time.Duration {
	return s.now().Sub(decision.ProposedAt)
}

// Read returns one decision with the reasoning it currently rests on.
//
// Everything a decision endpoint accepts has a matching way to read the
// result. A tool that lets somebody argue, agree and annotate, and then offers
// no way to see what any of it produced, sends every reader to the review
// queue — which by definition no longer holds what was agreed to.
func (s *Store) Read(ctx context.Context, subject access.Subject, decisionID int64) (*Decision, string, error) {
	decision, err := s.reaching(ctx, subject, decisionID, readable)
	if err != nil {
		return nil, "", err
	}
	reasoning, err := s.currentReasoning(ctx, []Decision{*decision})
	if err != nil {
		return nil, "", err
	}
	return decision, reasoning[decision.ID], nil
}

// Filter narrows a list of decisions to what somebody is looking for.
//
// An empty field is not a filter. Nobody asking about deferrals wants to be
// told which states exist first.
type Filter struct {
	// ProductID limits to one product. Zero is every product the reader may
	// reach, which is the ordinary case: somebody auditing dismissals is
	// asking across a release, not about one package.
	ProductID int64
	Outcome   Outcome
	State     State
	// Expired limits deferrals to those whose date has passed. It is the
	// question a deferral list is usually being asked — what did we put off
	// that has come back — rather than a state anything records.
	Expired bool
}

// List returns decisions matching a filter, newest first, with how many there
// are behind the page.
//
// Newest first because the question this answers is almost always about recent
// judgment: what have we dismissed, what did we defer, what is coming back.
// Oldest-first would put the page nobody wants at the front of every request.
func (s *Store) List(ctx context.Context, subject access.Subject, f Filter,
	limit, offset int) ([]Decision, map[int64]string, int, error) {

	narrow := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = readableBy(q, subject, "de")
		if f.ProductID != 0 {
			q = q.Where("product_id = ?", f.ProductID)
		}
		if f.Outcome != "" {
			q = q.Where("outcome = ?", f.Outcome)
		}
		if f.State != "" {
			q = q.Where("state = ?", f.State)
		}
		if f.Expired {
			q = q.Where("deferred_until IS NOT NULL").Where("deferred_until <= ?", s.now())
		}
		return q
	}

	total, err := narrow(s.db.NewSelect().Model((*Decision)(nil))).Count(ctx)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("count what was decided: %w", err)
	}

	var decisions []Decision
	if err := narrow(s.db.NewSelect().Model(&decisions)).
		Order("id DESC").Limit(limit).Offset(offset).Scan(ctx); err != nil {
		return nil, nil, 0, fmt.Errorf("read what was decided: %w", err)
	}

	// The reasoning comes with the row, for the same reason the review queue
	// carries it: a list where seeing why means opening each entry is a list
	// nobody reads before acting on.
	reasoning, err := s.currentReasoning(ctx, decisions)
	if err != nil {
		return nil, nil, 0, err
	}
	return decisions, reasoning, total, nil
}
