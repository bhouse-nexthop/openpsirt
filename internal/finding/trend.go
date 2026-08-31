package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Point is what was true at one moment.
type Point struct {
	At       time.Time
	Open     int
	Opened   int
	Resolved int
	// BySeverity is the open count split up. A total that barely moves while
	// its critical share rises is getting worse, and one line hides that.
	BySeverity map[string]int
}

// Trend reports new, resolved and open over time, split by severity.
//
// Worked out when it is asked for. Nothing is precomputed or refreshed on a
// schedule until a measurement says it has to be: a stale answer is a cost
// paid up front for a benefit nobody has demonstrated, and it brings its own
// invalidation bugs.
//
// Three series rather than one. Separately they are three numbers; together
// they say whether the team is keeping pace, and new consistently outrunning
// resolved is a growing backlog that should be visible before somebody works
// it out from a chart of open alone.
func (s *Store) Trend(ctx context.Context, subject access.Subject, since time.Time,
	step time.Duration, steps int) ([]Point, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}
	if steps <= 0 || steps > 104 {
		steps = 12
	}
	// A step of nothing makes every point the same instant, so the chart is
	// one number drawn twelve times. A week is what the callers ask for.
	if step <= 0 {
		step = 7 * 24 * time.Hour
	}
	// Longer than the whole history is a range nothing falls in. Bounded here
	// rather than trusted, because the range is a query parameter and the loop
	// below walks it whatever it says.
	if step > 366*24*time.Hour {
		step = 366 * 24 * time.Hour
	}
	if since.IsZero() {
		since = s.now().UTC().Add(-time.Duration(steps) * step)
	}
	until := since.Add(time.Duration(steps) * step)

	// Every open and closed moment in range, once, and the counting happens
	// here. The alternative is a statement per point per severity, which is
	// sixty round trips to answer one question.
	var rows []struct {
		Severity      string     `bun:"severity"`
		OpenedAt      time.Time  `bun:"opened_at"`
		ClosedAt      *time.Time `bun:"closed_at"`
		ClosedBecause string     `bun:"closed_because"`
	}
	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN scan_run AS o ON o.id = f.opened_run_id").
		Join("LEFT JOIN scan_run AS c ON c.id = f.closed_run_id").
		ColumnExpr("COALESCE(v.severity, 'unknown') AS severity").
		ColumnExpr("o.started_at AS opened_at").
		ColumnExpr("c.started_at AS closed_at").
		ColumnExpr("COALESCE(f.closed_because, '') AS closed_because").
		// Only what can fall in the range. A finding opened after the last
		// point contributes to nothing, and one closed before the first
		// contributes to nothing either — reading the whole table to discard
		// most of it grows the cost of a chart with the age of the deployment.
		Where("o.started_at <= ?", until).
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereOr("f.closed_run_id IS NULL").
				WhereOr("c.started_at > ?", since)
		})
	if !all {
		query = query.Where("st.product_id IN (?)", bun.List(products))
	}
	query = onlyVisible(query, subject, products, all)

	if err := query.Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read what changed over time: %w", err)
	}

	points := make([]Point, 0, steps)
	for i := 0; i < steps; i++ {
		from := since.Add(time.Duration(i) * step)
		to := from.Add(step)
		point := Point{At: to, BySeverity: map[string]int{}}
		for _, row := range rows {
			if row.OpenedAt.After(from) && !row.OpenedAt.After(to) {
				point.Opened++
			}
			if row.ClosedAt != nil && row.ClosedAt.After(from) && !row.ClosedAt.After(to) &&
				resolving(Closure(row.ClosedBecause)) {
				point.Resolved++
			}
			// Open at the end of this step: opened by then, and either still
			// open or closed afterwards.
			if row.OpenedAt.After(to) {
				continue
			}
			if row.ClosedAt != nil && !row.ClosedAt.After(to) {
				continue
			}
			point.Open++
			point.BySeverity[row.Severity]++
		}
		points = append(points, point)
	}
	return points, nil
}

// resolving says whether a finding closing means the issue went away.
//
// Superseded does not. The component's version moved and the issue came with
// it: this row closed and the same issue reopened against the new version, so
// counting it as resolved draws a line saying work was completed while the
// same chart's new-findings line rises by exactly as much. Unexplained does
// not either — the scanner stopped saying it, which is not the same as
// somebody having fixed it.
func resolving(because Closure) bool {
	switch because {
	case Superseded, Unexplained:
		return false
	}
	return true
}
