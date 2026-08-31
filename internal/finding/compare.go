package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Changed is one issue that differs between two builds.
type Changed struct {
	Vulnerability string `bun:"vulnerability"`
	Component     string `bun:"component"`
	Severity      string `bun:"severity"`
	// Because says why it went, for something that is no longer there. The
	// four explanations are different sentences to whoever reads a release
	// note: upgraded, patched, the component removed, and superseded — which
	// is a bump that did not reach the fix and is not a fix at all.
	Because Closure `bun:"because"`
	// ArrivedFrom is the version this place held before, where the version
	// moved and the issue came with it. Set on still-present entries, which is
	// where it says something a reader cannot get any other way: this was
	// bumped, and the bump fell short (STA-18).
	ArrivedFrom string `bun:"arrived_from"`
}

// Comparison is what changed between two builds.
type Comparison struct {
	Fixed []Changed
	Newly []Changed
	Still []Changed
}

// Compare reports what was fixed, what is newly present, and what is still
// there between two builds.
//
// Between any two, not only adjacent ones: the question a release note answers
// is usually about the last release a customer has, which is rarely the
// previous one.
//
// Each fixed entry says **why** it went. "Fixed by upgrading to 2.4" and
// "fixed by a carried patch" are different things to a reader, and a bump that
// did not reach the fix is not a fix — it appears as superseded rather than as
// something resolved.
func (s *Store) Compare(ctx context.Context, subject access.Subject, fromTarget, toTarget int64,
	includePrivate bool) (*Comparison, error) {

	// Both builds, not one. The first version authorized the later target and
	// applied that answer to the earlier one as well, so a caller who could
	// reach one product could read findings out of another through the
	// comparison — enforcement lives in the data layer precisely so the next
	// caller of this cannot open that.
	visible, err := s.mayCompare(ctx, subject, toTarget)
	if err != nil {
		return nil, err
	}
	earlier, err := s.mayCompare(ctx, subject, fromTarget)
	if err != nil {
		return nil, err
	}
	// What may be read of the two, which is the narrower of the two answers.
	if len(earlier) < len(visible) {
		visible = earlier
	}
	if !includePrivate {
		// Its destination is usually a public document, so including
		// something undisclosed is a deliberate act rather than something
		// somebody pastes in without noticing.
		visible = []access.Visibility{access.Public}
	}

	at := func(targetID int64) *bun.SelectQuery {
		q := s.db.NewSelect().
			TableExpr("finding AS f").
			Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
			Join("JOIN component AS c ON c.id = f.component_id").
			ColumnExpr("v.identifier AS vulnerability").
			ColumnExpr("c.name AS component").
			ColumnExpr("COALESCE(v.severity, '') AS severity").
			ColumnExpr("COALESCE(f.closed_because, '') AS because").
			ColumnExpr("MIN(COALESCE(f.arrived_from, '')) AS arrived_from").
			Where("f.target_id = ?", targetID).
			Where("f.visibility IN (?)", bun.List(visible)).
			GroupExpr("v.identifier, c.name, v.severity, f.closed_because")
		return q.Where("f.closed_run_id IS NULL")
	}

	var was, now []Changed
	if err := at(fromTarget).Scan(ctx, &was); err != nil {
		return nil, fmt.Errorf("read what the earlier build had: %w", err)
	}
	if err := at(toTarget).Scan(ctx, &now); err != nil {
		return nil, fmt.Errorf("read what the later build has: %w", err)
	}

	key := func(c Changed) string { return c.Vulnerability + "\x00" + c.Component }
	here := map[string]Changed{}
	for _, c := range now {
		here[key(c)] = c
	}

	comparison := &Comparison{}
	for _, c := range was {
		if standing, still := here[key(c)]; still {
			// The later build's row, not the earlier one: what a reader wants
			// to know about something still present is whether somebody tried
			// to move it since, and that is recorded where it landed.
			c.ArrivedFrom = standing.ArrivedFrom
			comparison.Still = append(comparison.Still, c)
			continue
		}
		// Why it went is read from the row that closed in the later build,
		// because the earlier build's row is still open in its own history.
		c.Because = s.whyGone(ctx, toTarget, c)
		comparison.Fixed = append(comparison.Fixed, c)
	}

	had := map[string]bool{}
	for _, c := range was {
		had[key(c)] = true
	}
	for _, c := range now {
		if !had[key(c)] {
			comparison.Newly = append(comparison.Newly, c)
		}
	}
	return comparison, nil
}

// mayCompare reports what a subject may read of one build, refusing where they
// may read nothing.
func (s *Store) mayCompare(ctx context.Context, subject access.Subject, targetID int64) ([]access.Visibility, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	return visible, nil
}

// whyGone reads the explanation recorded when a finding closed in the later
// build. Unexplained where the later build never had it at all, which happens
// when a component was gone before that line was cut.
func (s *Store) whyGone(ctx context.Context, targetID int64, c Changed) Closure {
	var because string
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS cp ON cp.id = f.component_id").
		ColumnExpr("COALESCE(f.closed_because, '')").
		Where("f.target_id = ?", targetID).
		Where("v.identifier = ?", c.Vulnerability).
		Where("cp.name = ?", c.Component).
		Where("f.closed_run_id IS NOT NULL").
		OrderExpr("f.id DESC").Limit(1).
		Scan(ctx, &because)
	if database.IsNoRows(err) || because == "" {
		// The later build never carried this at all, which is the ordinary
		// case when a fix landed before that line was first scanned. Saying
		// "removed" here publishes "we dropped the component" into a release
		// note about a component that is still there at a newer version.
		return Unexplained
	}
	if err != nil {
		return Unexplained
	}
	return Closure(because)
}
