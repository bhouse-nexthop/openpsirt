package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Repeated is one place that keeps being put off (TRI-19).
//
// A cumulative threshold already refuses a *further* deferral past a point,
// which acts on one item at a time. What that cannot show is the shape across
// everything: one item deferred three times is a judgment, and forty of them
// is a policy nobody wrote down and nobody agreed to.
type Repeated struct {
	Product       string `bun:"product"`
	Vulnerability string `bun:"vulnerability"`
	Severity      string `bun:"severity"`
	// PlaceIdentity names the place rather than describing it. What it is
	// called depends on the build being looked at, and this list is not about
	// one build.
	PlaceIdentity string `bun:"place_identity"`
	// Times is how often it has been put off, and Total is how long it has
	// been put off for, added up. The two answer different questions: three
	// short deferrals and one long one are different situations, and a list
	// carrying only one of the numbers hides whichever it is not.
	Times int `bun:"times"`
	// TotalDays is the sum of every deferral's span, in days.
	//
	// **A fraction, not a whole number.** Adding two moments' difference gives
	// a fractional day on all four engines, and declaring it as an integer
	// scanned on none of them: one refused a float outright and three handed
	// back a decimal string. Rounded where it is shown rather than where it is
	// read, so nothing rounds twice.
	TotalDays float64 `bun:"total_days"`
	// Standing says a deferral is in force now rather than all of them having
	// run out. Something put off three times and now decided is history; the
	// same thing still being put off is the pattern this list is for.
	Standing bool `bun:"standing"`
	// LastUntil is the furthest any of them reached.
	LastUntil time.Time `bun:"last_until"`
}

// DefaultRepeatedAt is how many deferrals make something worth listing.
//
// Two, because one is an ordinary judgment and the list exists for the pattern
// rather than for the act. It is a starting point rather than a rule: how many
// is too many is a judgment about a product, and the caller may ask for more.
const DefaultRepeatedAt = 2

// Repeats lists places that have been deferred more than once, worst first.
//
// **Counted over the decisions rather than over the findings.** A deferral is
// one judgment about a place; the places fan out into as many findings as the
// component has consumers, and counting those would order the list by how far
// a component spreads through an image.
func (s *Store) Repeats(ctx context.Context, subject access.Subject, productID int64,
	atLeast, limit int) ([]Repeated, error) {

	if subject.Kind != access.Person {
		return nil, nil
	}
	if atLeast <= 0 {
		atLeast = DefaultRepeatedAt
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var rows []Repeated
	q := s.db.NewSelect().
		TableExpr(`"decision" AS de`).
		Join(`JOIN "vulnerability" AS v ON v.id = de.vulnerability_id`).
		Join(`JOIN "product" AS p ON p.id = de.product_id`).
		ColumnExpr("p.display_name AS product").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("COALESCE(v.assessed_severity, v.severity, '') AS severity").
		ColumnExpr("de.place_identity AS place_identity").
		ColumnExpr("COUNT(*) AS times").
		ColumnExpr("MAX(de.deferred_until) AS last_until").
		// Summed in days here rather than as intervals, because the four
		// engines return an interval as four different things and a caller
		// would have to know which one it was talking to.
		ColumnExpr(deferredDays(s.db)+" AS total_days").
		ColumnExpr("MAX(CASE WHEN de.live_key IS NOT NULL AND de.deferred_until > ?"+
			" THEN 1 ELSE 0 END) AS standing", s.now().UTC()).
		Where("de.outcome = ?", Deferred).
		// What was taken back was never time anything spent put off.
		Where("de.state <> ?", Withdrawn).
		Where("de.deferred_until IS NOT NULL").
		GroupExpr("p.display_name, v.identifier, v.assessed_severity, v.severity, de.place_identity").
		Having("COUNT(*) >= ?", atLeast).
		// The most put-off first, and then the longest: a list read from the
		// top should start with the thing somebody has avoided most.
		OrderExpr("times DESC, total_days DESC, vulnerability").
		Limit(limit)

	if productID > 0 {
		q = q.Where("de.product_id = ?", productID)
	}
	q = onlyDecisionsReadable(q, subject)
	if err := q.Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read what keeps being put off: %w", err)
	}
	return rows, nil
}

// onlyDecisionsReadable narrows to the products somebody holds a role on.
//
// A decision carries no visibility of its own — the findings it covers do — so
// the product is what this can narrow by, and an issue nobody has disclosed is
// in a product somebody either reads or does not.
func onlyDecisionsReadable(q *bun.SelectQuery, subject access.Subject) *bun.SelectQuery {
	products, all := subject.Products()
	if all {
		return q
	}
	if len(products) == 0 {
		// Nothing anywhere, said as a condition nothing satisfies rather than
		// left off, so a caller cannot forget to check the empty case.
		return q.Where("1 = 0")
	}
	return q.Where("de.product_id IN (?)", bun.List(products))
}

// deferredDays sums how long each deferral ran for, in whole days.
//
// One of the few places an engine has to be asked directly: subtracting two
// moments has no portable spelling, and the four disagree about what comes
// back as much as about how to ask.
func deferredDays(db bun.IDB) string {
	switch db.Dialect().Name().String() {
	case "pg":
		return "COALESCE(SUM(EXTRACT(EPOCH FROM (de.deferred_until - de.proposed_at)) / 86400), 0)"
	case "mysql":
		return "COALESCE(SUM(TIMESTAMPDIFF(SECOND, de.proposed_at, de.deferred_until)) / 86400, 0)"
	default:
		return "COALESCE(SUM(julianday(de.deferred_until) - julianday(de.proposed_at)), 0)"
	}
}
