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
		VulnerabilityID int64      `bun:"vulnerability_id"`
		Severity        string     `bun:"severity"`
		OpenedAt        time.Time  `bun:"opened_at"`
		ClosedAt        *time.Time `bun:"closed_at"`
		ClosedBecause   string     `bun:"closed_because"`
	}
	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN scan_run AS o ON o.id = f.opened_run_id").
		Join("LEFT JOIN scan_run AS c ON c.id = f.closed_run_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
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

	// Counted as issues, not as places.
	//
	// A finding is a component at a place, and one issue in one shared library
	// reaches every consumer of it — on a real image the kernel alone produced
	// 305,487 findings for a few thousand issues. Counting rows here reported
	// 441,108 open where 5,661 issues were open, which is not a chart anybody
	// can read: it measures how much the dependency graph shares rather than
	// how much there is to answer.
	//
	// So each point is a *set* of issues, and the three numbers come from the
	// same set. That also settles what "resolved" means without a second rule:
	// an issue whose version moved and came with it is still in the set, so a
	// bump that fixed nothing cannot appear as work completed.
	open := make([]map[int64]string, steps)
	for i := range open {
		open[i] = map[int64]string{}
	}
	// Where an issue stopped being present without explanation, per step. The
	// scanner going quiet is a fault to investigate rather than a fix, so it
	// is held back from the resolved count even though the issue has left the
	// set (RPT-15).
	quiet := make([]map[int64]bool, steps)
	for i := range quiet {
		quiet[i] = map[int64]bool{}
	}

	for _, row := range rows {
		for i := 0; i < steps; i++ {
			to := since.Add(time.Duration(i+1) * step)
			if row.OpenedAt.After(to) {
				continue
			}
			if row.ClosedAt != nil && !row.ClosedAt.After(to) {
				// Gone by this point. Note an unexplained disappearance so the
				// step it happened in does not read as work done.
				if Closure(row.ClosedBecause) == Unexplained {
					for j := 0; j < steps; j++ {
						from := since.Add(time.Duration(j) * step)
						until := from.Add(step)
						if row.ClosedAt.After(from) && !row.ClosedAt.After(until) {
							quiet[j][row.VulnerabilityID] = true
						}
					}
				}
				continue
			}
			open[i][row.VulnerabilityID] = row.Severity
		}
	}

	points := make([]Point, 0, steps)
	for i := 0; i < steps; i++ {
		point := Point{At: since.Add(time.Duration(i+1) * step), BySeverity: map[string]int{}}
		point.Open = len(open[i])
		for _, severity := range open[i] {
			point.BySeverity[severity]++
		}
		// New and resolved are the difference between this step's set and the
		// last one's, so all three numbers agree with each other rather than
		// counting events that may not change what is open. The first step has
		// nothing to differ from, so it reports neither.
		if i == 0 {
			points = append(points, point)
			continue
		}
		before := open[i-1]
		for id := range open[i] {
			if _, was := before[id]; !was {
				point.Opened++
			}
		}
		for id := range before {
			if _, still := open[i][id]; !still && !quiet[i][id] {
				point.Resolved++
			}
		}
		points = append(points, point)
	}

	return points, nil
}
