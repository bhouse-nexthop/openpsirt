package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Windows is how long a finding may stay open before it is late, by how urgent
// it is.
//
// Exploited is separate and shortest. Severity says how bad the flaw is; being
// exploited says somebody is using it, and that is the one that should decide
// how long there is.
type Windows struct {
	Exploited time.Duration
	Critical  time.Duration
	High      time.Duration
	Medium    time.Duration
	Low       time.Duration
}

// For returns the window this finding gets.
func (w Windows) For(severity string, exploited bool) time.Duration {
	if exploited {
		return w.Exploited
	}
	switch severity {
	case "critical":
		return w.Critical
	case "high":
		return w.High
	case "medium":
		return w.Medium
	}
	return w.Low
}

// Late is a finding whose time is running out with nobody having decided about
// it.
type Late struct {
	Vulnerability string    `bun:"vulnerability"`
	Component     string    `bun:"component"`
	Severity      string    `bun:"severity"`
	Exploited     bool      `bun:"exploited"`
	Product       string    `bun:"product"`
	Stream        string    `bun:"stream"`
	Variant       string    `bun:"variant"`
	AssignedTo    *int64    `bun:"assigned_to"`
	FirstSeen     time.Time `bun:"first_seen"`
	// Due is worked out from FirstSeen and the window this finding gets, and
	// is not stored: a window is a setting somebody changes, and a stored date
	// would be the answer under whatever it used to be.
	Due time.Time
}

// RunningOut reports findings whose deadline is within this many days and
// which nobody has decided about, most pressing first.
//
// **Undecided only.** A deadline that has been answered is not a deadline
// running out: a dismissal takes a finding off the clock, because the claim is
// that it will not be fixed, and a deferral replaces the deadline with its own
// date. What is left is time passing with nothing said, which is the only part
// worth interrupting somebody about.
func (s *Store) RunningOut(ctx context.Context, subject access.Subject, w Windows,
	within time.Duration, limit int) ([]Late, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("JOIN scan_run AS o ON o.id = f.opened_run_id").
		ColumnExpr("v.identifier AS vulnerability").
		ColumnExpr("c.name AS component").
		ColumnExpr("v.severity AS severity").
		ColumnExpr("f.urgency_exploited AS exploited").
		ColumnExpr("p.display_name AS product").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		ColumnExpr("f.assigned_to AS assigned_to").
		ColumnExpr("o.started_at AS first_seen").
		Where("f.closed_run_id IS NULL").
		// Nothing the build already argued away, and nothing a person has
		// answered. A decision standing here means the clock is not running.
		Where("f.suppressed_by IS NULL").
		Where(`NOT EXISTS (SELECT 1 FROM "decision" AS de
			WHERE de.product_id = st.product_id
			  AND de.vulnerability_id = f.vulnerability_id
			  AND de.place_identity = f.place_identity
			  AND de.live_key IS NOT NULL)`).
		OrderExpr("o.started_at").
		Limit(limit * 4)

	if !all {
		query = query.Where("st.product_id IN (?)", bun.List(products))
	}
	query = query.Where("(f.visibility = ? OR st.product_id IN (?))",
		access.Public, bun.List(privateFor(subject, products, all)))

	var rows []Late
	if err := query.Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read what is running out of time: %w", err)
	}

	// The window is applied here rather than in the statement: it is four
	// settings and a comparison, and putting it in SQL would mean expressing
	// date arithmetic four ways for four engines to answer a question a loop
	// answers exactly.
	cutoff := s.now().UTC().Add(within)
	late := make([]Late, 0, len(rows))
	for _, row := range rows {
		row.Due = row.FirstSeen.Add(w.For(row.Severity, row.Exploited))
		if row.Due.After(cutoff) {
			continue
		}
		late = append(late, row)
		if len(late) == limit {
			break
		}
	}
	return late, nil
}
