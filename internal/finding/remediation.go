package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Remediation is how fast things are being fixed, and what is aging (RPT-03).
//
// **Counted in issues, not in places.** One kernel flaw across sixty modules is
// one thing that was fixed, and a mean time to remediate weighted by how far a
// component fans out through an image is a measurement of the dependency graph
// rather than of anybody's work.
type Remediation struct {
	// Fixed and Opened are how many distinct issues closed and opened in the
	// window. Together they say whether the team is keeping pace; separately they
	// are two numbers people quote at each other.
	Fixed  int
	Opened int
	// TimeToFix is the average time an issue closed in this window was open
	// for, by the severity it was rated. Absent where nothing of that rating
	// closed — a zero would read as "fixed instantly".
	TimeToFix map[string]time.Duration
	// Aging is what is open now, by how long it has been open. The buckets are
	// the ones people ask in, and the last one is open-ended because "older
	// than ninety days" is the answer that matters and its shape does not.
	Aging []Bucket
}

// Bucket is how many issues have been open for a stretch of time.
type Bucket struct {
	// Label names the stretch, and Days is where it starts, so a caller can
	// order them without parsing the label.
	Label string
	Days  int
	Open  int
}

// agingBuckets are the stretches what is open is counted into.
var agingBuckets = []struct {
	label string
	from  int
	to    int
}{
	{"under a week", 0, 7},
	{"one to four weeks", 7, 28},
	{"one to three months", 28, 90},
	{"over three months", 90, 0},
}

// resolvedExpr is what counts as an issue actually going away (RPT-15).
//
// **A closure is not a fix unless the issue went with it.** A bump that
// carried the issue into the next version closed one row and opened another
// with the same issue in it, and a scanner that silently stopped reporting
// something closed a row and explained nothing. Counting either as a fix
// measures churn and reports it as progress, which is worse than reporting
// nothing: the number moves in the right direction while nothing improves.
const resolvedExpr = `f.closed_because IN ('removed', 'upgraded', 'revised', 'fixed')`

// Remediation reports how fast issues are being closed and what is aging.
func (s *Store) Remediation(ctx context.Context, subject access.Subject, scope Scope,
	window time.Duration) (*Remediation, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}
	if window <= 0 {
		window = 30 * 24 * time.Hour
	}
	now := s.now().UTC()
	since := now.Add(-window)

	out := &Remediation{TimeToFix: map[string]time.Duration{}}

	// How long each issue took, averaged per severity band. Averaged over the
	// issue rather than over its rows: an issue is closed when the last of its
	// places is, and the places are what fans out.
	var spans []struct {
		Band    string  `bun:"band"`
		Seconds float64 `bun:"seconds"`
		Issues  int     `bun:"issues"`
	}
	closed := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr(BandExpr+" AS band").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("MAX(f.closed_at) AS closed_at").
		ColumnExpr("MIN(f.opened_at) AS opened_at").
		Where("f.closed_at IS NOT NULL").
		Where("f.closed_at >= ?", since).
		Where(resolvedExpr).
		GroupExpr("band, f.vulnerability_id")
	closed = scope.Narrow(onlyReadable(closed, subject, products, all))

	// The averaging happens over the grouped issues, in a statement of its
	// own, because averaging inside the grouping would average the places.
	if err := s.db.NewSelect().
		TableExpr("(?) AS per_issue", closed).
		ColumnExpr("per_issue.band AS band").
		ColumnExpr("COUNT(*) AS issues").
		ColumnExpr(secondsBetween(s.db)+" AS seconds").
		GroupExpr("per_issue.band").
		Scan(ctx, &spans); err != nil {
		return nil, fmt.Errorf("read how long fixes took: %w", err)
	}
	for _, row := range spans {
		out.Fixed += row.Issues
		if row.Issues > 0 {
			out.TimeToFix[row.Band] = time.Duration(row.Seconds) * time.Second
		}
	}

	// What opened in the same window, as distinct issues, so the two figures
	// are in the same unit and can be read against each other.
	opened := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("f.vulnerability_id").
		Where("f.opened_at >= ?", since).
		GroupExpr("f.vulnerability_id")
	count, err := s.db.NewSelect().
		TableExpr("(?) AS grouped", scope.Narrow(onlyReadable(opened, subject, products, all))).Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count what opened: %w", err)
	}
	out.Opened = count

	// What is open now, by how long it has been. One statement per bucket
	// rather than a case expression, because the boundaries are moments
	// computed here and a database that does its own date arithmetic does it
	// four different ways.
	for _, bucket := range agingBuckets {
		older := now.Add(-time.Duration(bucket.from) * 24 * time.Hour)
		q := s.db.NewSelect().
			TableExpr("finding AS f").
			Join("JOIN target AS tg ON tg.id = f.target_id").
			Join("JOIN stream AS st ON st.id = tg.stream_id").
			ColumnExpr("f.vulnerability_id").
			Where("f.closed_at IS NULL").
			Where("f.opened_at <= ?", older).
			GroupExpr("f.vulnerability_id")
		if bucket.to > 0 {
			q = q.Where("f.opened_at > ?", now.Add(-time.Duration(bucket.to)*24*time.Hour))
		}
		n, err := s.db.NewSelect().
			TableExpr("(?) AS grouped", scope.Narrow(onlyReadable(q, subject, products, all))).Count(ctx)
		if err != nil {
			return nil, fmt.Errorf("count what is aging: %w", err)
		}
		out.Aging = append(out.Aging, Bucket{Label: bucket.label, Days: bucket.from, Open: n})
	}
	return out, nil
}

// secondsBetween averages the gap between two moments, in the spelling each
// engine understands.
//
// There is no portable way to subtract two timestamps and get a number: one
// returns an interval, one a number of days, and two return something else
// again. So this is one of the few places the engine has to be asked, and it
// is confined to an expression rather than spread through a query.
func secondsBetween(db bun.IDB) string {
	switch db.Dialect().Name().String() {
	case "pg":
		return "AVG(EXTRACT(EPOCH FROM (per_issue.closed_at - per_issue.opened_at)))"
	case "mysql":
		return "AVG(TIMESTAMPDIFF(SECOND, per_issue.opened_at, per_issue.closed_at))"
	default:
		// SQLite keeps these as text and compares them as text, which is why
		// the format is fixed; julianday is its way of getting a number out.
		return "AVG((julianday(per_issue.closed_at) - julianday(per_issue.opened_at)) * 86400)"
	}
}
