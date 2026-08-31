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

	// Every open and closed moment in range, once, and the counting happens
	// here. The alternative is a statement per point per severity, which is
	// sixty round trips to answer one question.
	var rows []struct {
		Severity string     `bun:"severity"`
		OpenedAt time.Time  `bun:"opened_at"`
		ClosedAt *time.Time `bun:"closed_at"`
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
		ColumnExpr("c.started_at AS closed_at")
	if !all {
		query = query.Where("st.product_id IN (?)", bun.List(products))
	}
	query = query.Where("(f.visibility = ? OR st.product_id IN (?))",
		access.Public, bun.List(privateFor(subject, products, all)))

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
			if row.ClosedAt != nil && row.ClosedAt.After(from) && !row.ClosedAt.After(to) {
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
