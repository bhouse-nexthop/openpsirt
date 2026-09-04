package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Embargoed is one finding nobody has announced, and when that ends.
type Embargoed struct {
	Vulnerability string `bun:"vulnerability"`
	Summary       string `bun:"summary"`
	Component     string `bun:"component"`
	Product       string `bun:"product"`
	Stream        string `bun:"stream"`
	Variant       string `bun:"variant"`
	Severity      string `bun:"severity"`
	// DiscloseAt is when the embargo ends. Reaching it discloses nothing: it
	// is a date to answer, not a trigger (ACC-47).
	DiscloseAt time.Time `bun:"disclose_at"`
	AssignedTo *int64    `bun:"assigned_to"`
	// Places is how many findings this covers.
	Places int `bun:"places"`
}

// Passed says the date has arrived and nothing has been decided about it.
func (e Embargoed) Passed(now time.Time) bool { return !e.DiscloseAt.After(now) }

// Disclosing reports what is approaching disclosure, and what is past it,
// soonest first.
//
// **Before the date, not on it** (ACC-49). The date arriving is the last
// moment to act on it rather than the first useful warning, and a list that
// only ever showed what was already overdue would be a list of decisions
// somebody has already failed to make.
//
// **Nothing here discloses anything.** Reaching the date escalates: the row
// appears, and the people who can act on it are told. Publishing embargoed
// detail because a timer expired is the wrong default — if the fix is not
// ready, disclosing anyway is a decision a person makes.
//
// Narrowed like everything else, and more consequentially: this is a list of
// findings nobody has announced, so somebody who may not read undisclosed work
// in a product sees none of that product's. What that costs them is a shorter
// list; what the alternative costs is the disclosure the whole split exists to
// prevent.
func (s *Store) Disclosing(ctx context.Context, subject access.Subject, scope Scope,
	within time.Duration, limit int) ([]Embargoed, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if within <= 0 {
		within = 30 * 24 * time.Hour
	}

	// Only what this person may see undisclosed work about. A product where
	// they hold public access alone contributes nothing, because every row
	// here is undisclosed by definition — and a count would say as much as a
	// row.
	private := make([]int64, 0, len(products))
	if all {
		private = nil
	} else {
		for _, id := range products {
			if subject.Reads(access.Private, id) {
				private = append(private, id)
			}
		}
		if len(private) == 0 {
			return nil, nil
		}
	}

	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("v.description AS summary").
		ColumnExpr(EffectiveSeverityExpr+" AS severity").
		ColumnExpr("c.name AS component").
		ColumnExpr("p.display_name AS product").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		ColumnExpr("MIN(f.disclose_at) AS disclose_at").
		ColumnExpr("MIN(f.assigned_to) AS assigned_to").
		ColumnExpr("COUNT(*) AS places").
		Where("f.visibility = ?", access.Private).
		Where("f.closed_run_id IS NULL").
		Where("f.disclose_at IS NOT NULL").
		Where("f.disclose_at <= ?", s.now().UTC().Add(within)).
		GroupExpr("v.identifier, v.description, " + EffectiveSeverityExpr +
			", c.name, p.display_name, st.display_name, va.display_name").
		OrderExpr("disclose_at, v.identifier").
		Limit(limit)
	if len(private) > 0 {
		query = query.Where("st.product_id IN (?)", bun.List(private))
	}
	query = scope.Narrow(query)

	var rows []Embargoed
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read what is approaching disclosure: %w", err)
	}
	return rows, nil
}
