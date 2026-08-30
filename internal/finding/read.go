package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// Open returns the findings currently open against a target that this subject
// may read.
//
// The filtering happens here rather than in whatever asked. A check in a
// handler is a check somebody forgets the first time they add another handler,
// and the thing being forgotten is not a blank screen — it is somebody seeing
// an issue that has not been disclosed.
//
// Which product the target belongs to is read here too, rather than accepted
// from the caller. A caller that could name the product could name a different
// one, and then the check would be answering a question nobody asked.
func (s *Store) Open(ctx context.Context, subject access.Subject, targetID int64) ([]Finding, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		// Not merely empty: a product somebody holds nothing on does not
		// exist as far as they are concerned, and an empty list is a
		// different statement from a refusal.
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []Finding
	err = s.db.NewSelect().Model(&rows).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Order("id").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read open findings: %w", err)
	}
	return rows, nil
}

// CountOpen counts what Open would return.
//
// A count is a read. Counting rows somebody may not see and reporting the
// total is the same disclosure as listing them, just compressed — and it is
// the path that leaks when only row reads are guarded.
func (s *Store) CountOpen(ctx context.Context, subject access.Subject, targetID int64) (int, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, err
	}
	if !subject.Sees(productID) {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	n, err := s.db.NewSelect().Model((*Finding)(nil)).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open findings: %w", err)
	}
	return n, nil
}

// visibleTo is what a subject may read in a product, as values to compare
// against. Empty means nothing, which is a refusal rather than a filter.
func visibleTo(subject access.Subject, productID int64) []access.Visibility {
	var visible []access.Visibility
	for _, v := range []access.Visibility{access.Public, access.Private} {
		if subject.Reads(v, productID) {
			visible = append(visible, v)
		}
	}
	return visible
}

// productOf reads which product a target belongs to.
func productOf(ctx context.Context, db *bun.DB, targetID int64) (int64, error) {
	var productID int64
	err := db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("st.product_id").
		Where("tg.id = ?", targetID).
		Scan(ctx, &productID)
	if err != nil {
		return 0, fmt.Errorf("look up which product target %d belongs to: %w", targetID, err)
	}
	return productID, nil
}

// Group is one issue in one component, with the places it occupies.
//
// This is the unit somebody decides about. The places are what the decision is
// recorded against — one real image produced 335,021 findings and 305,487 of
// them were a single kernel across the modules built against it, so a list of
// places is six thousand screens of rows differing in a column nobody reads.
type Group struct {
	Vulnerability string
	Severity      string
	Component     string
	Version       string
	// Upstream is what a fork was made from, where it is one. A version nobody
	// recognizes needs it to be explainable.
	Upstream string
	FixState FixState
	FixedIn  string
	// Places is how many consumers pull this component in here. It is part of
	// what is read rather than a detail: sixty-two places and one place are
	// different situations to somebody deciding, and a group that hides its
	// size invites a judgment made about one being applied to sixty-one
	// unseen.
	Places int
	// Answered counts the places the build has already argued about.
	Answered int
	// Urgency is how far up the list this belongs, and Exploited says whether
	// it is there because somebody is using it. The flag is carried rather
	// than left to be inferred from the number: a position nobody can explain
	// is one people stop trusting and then work around.
	Urgency   int64
	Exploited bool
}

// Groups returns what is open against a target, as the things somebody decides
// about rather than as one row per place.
func (s *Store) Groups(ctx context.Context, subject access.Subject, targetID int64, limit, offset int) ([]Group, int, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, 0, err
	}
	if !subject.Sees(productID) {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}

	// Grouped in the database and named in a second pass. Reducing text across
	// rows has no portable spelling — the function differs on every engine —
	// and the counts are what this query is for.
	var rows []struct {
		VulnerabilityID int64  `bun:"vulnerability_id"`
		ComponentID     int64  `bun:"component_id"`
		Places          int    `bun:"places"`
		Answered        int    `bun:"answered"`
		Urgency         int64  `bun:"urgency"`
		Exploited       bool   `bun:"exploited"`
		FixState        string `bun:"fix_state"`
		FixedIn         string `bun:"fixed_in"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("COUNT(*) AS places").
		// The most urgent place this issue sits at. A group is one decision
		// about one issue in one component, so what should decide where that
		// decision appears is the worst of what it covers.
		ColumnExpr("MAX(f.urgency) AS urgency").
		// Folded in Go from the same maximum rather than aggregated: no
		// portable spelling reduces a boolean across rows, and one engine
		// rejects the obvious one outright.
		ColumnExpr("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) AS exploited").
		ColumnExpr("SUM(CASE WHEN f.suppressed_by IS NULL THEN 0 ELSE 1 END) AS answered").
		ColumnExpr("MIN(f.fix_state) AS fix_state").
		ColumnExpr("MIN(f.fixed_in) AS fixed_in").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.vulnerability_id, f.component_id").
		// Ordered by urgency rather than by how widespread something is.
		// Sorting by place count puts whatever ships in the most places at the
		// top, which on a real image is the kernel — everywhere, and not
		// therefore the thing to look at first. What somebody with an hour
		// needs at the top is what is being exploited.
		OrderExpr("urgency DESC, places DESC, f.vulnerability_id, f.component_id").
		Limit(limit).Offset(offset).
		Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("read what is open: %w", err)
	}

	total, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", s.db.NewSelect().
			TableExpr("finding AS f").
			ColumnExpr("f.vulnerability_id").
			Where("f.target_id = ?", targetID).
			Where("f.closed_run_id IS NULL").
			Where("f.visibility IN (?)", bun.List(visible)).
			GroupExpr("f.vulnerability_id, f.component_id")).
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is open: %w", err)
	}

	groups := make([]Group, 0, len(rows))
	for _, row := range rows {
		group := Group{
			Places: row.Places, Answered: row.Answered,
			Urgency: row.Urgency, Exploited: row.Exploited,
			FixState: FixState(row.FixState), FixedIn: row.FixedIn,
		}
		var issue Vulnerability
		if err := s.db.NewSelect().Model(&issue).Where("id = ?", row.VulnerabilityID).Scan(ctx); err == nil {
			group.Vulnerability, group.Severity = issue.Identifier, issue.Severity
		}
		var component graph.Component
		if err := s.db.NewSelect().Model(&component).Where("id = ?", row.ComponentID).Scan(ctx); err == nil {
			group.Component, group.Version = component.Name, component.Version
			if component.UpstreamVersion != "" {
				group.Upstream = component.UpstreamName + " " + component.UpstreamVersion
			}
		}
		groups = append(groups, group)
	}
	return groups, total, nil
}
