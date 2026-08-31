package finding

import (
	"context"
	"fmt"
	"sort"
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
	// Places is how many places this issue sits at in this component. One row
	// covers all of them, because they are one thing to answer.
	Places int `bun:"places"`
	// AssignedHigh and AssignedCount decide whether one name can be reported
	// for the group. They are read and then discarded.
	AssignedHigh  *int64 `bun:"assigned_high"`
	AssignedCount int    `bun:"assigned_count"`
	// Due is worked out from FirstSeen and the window this finding gets, and
	// is not stored: a window is a setting somebody changes, and a stored date
	// would be the answer under whatever it used to be.
	Due time.Time
}

// band is one window and the findings it applies to.
//
// The list is asked band by band, because within a band the oldest finding is
// the most overdue and one query per band answers in the right order. Asked as
// one query it cannot be: the deadline is the first sighting plus a window
// that differs per band, and expressing that as a sort key means date
// arithmetic spelled four ways for four engines (DAT-02).
type band struct {
	window time.Duration
	where  func(*bun.SelectQuery) *bun.SelectQuery
}

// bands orders exploited first and then downwards, and each rules out the ones
// above it, so no finding is counted in two.
//
// Anything the reports did not rate falls in with the mediums, not with the
// lows. Nobody having scored it is not a claim that it is mild, and giving it
// the longest window is the one reading of silence that cannot be defended —
// it puts the findings least is known about at the back of the queue.
func bands(w Windows) []band {
	rated := func(severity string) func(*bun.SelectQuery) *bun.SelectQuery {
		return func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("f.urgency_exploited = ?", false).Where("v.severity = ?", severity)
		}
	}
	return []band{
		{w.Exploited, func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("f.urgency_exploited = ?", true)
		}},
		{w.Critical, rated("critical")},
		{w.High, rated("high")},
		{w.Low, func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("f.urgency_exploited = ?", false).
				Where("v.severity IN (?)", bun.In([]string{"low", "negligible", "none"}))
		}},
		// Everything else, which is medium and anything nobody rated.
		{w.Medium, func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("f.urgency_exploited = ?", false).
				Where("COALESCE(v.severity, '') NOT IN (?)",
					bun.In([]string{"critical", "high", "low", "negligible", "none"}))
		}},
	}
}

// RunningOut reports findings whose deadline is within this many days and
// which nobody has decided about, most pressing first.
//
// **Undecided only.** A deadline that has been answered is not a deadline
// running out: a dismissal takes a finding off the clock, because the claim is
// that it will not be fixed, and a deferral replaces the deadline with its own
// date. What is left is time passing with nothing said, which is the only part
// worth interrupting somebody about.
//
// **One row per issue at a component**, not per place. A kernel flaw sitting
// at sixty places is one thing somebody has to answer, and sixty rows of it is
// a list with one entry in it.
//
// The window is not applied after the rows are read. It used to be, and that
// is a different list than the one it claims to be: the statement took the
// oldest findings and the loop then discarded whatever was not due, so an
// exploited finding first seen yesterday and due tomorrow lost its place to a
// low from two years ago. Each band is now asked for its own, with the window
// turned into a first-sighting cutoff before the statement runs.
func (s *Store) RunningOut(ctx context.Context, subject access.Subject, w Windows,
	within time.Duration, limit int) ([]Late, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	cutoff := s.now().UTC().Add(within)
	var late []Late
	for _, each := range bands(w) {
		// Due within the window means first seen at or before this instant.
		// The arithmetic happens here, and the statement compares timestamps —
		// which every engine spells the same way.
		seenBy := cutoff.Add(-each.window)

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
			ColumnExpr("MIN(COALESCE(v.severity, '')) AS severity").
			ColumnExpr("f.urgency_exploited AS exploited").
			ColumnExpr("p.display_name AS product").
			ColumnExpr("st.display_name AS stream").
			ColumnExpr("va.display_name AS variant").
			// The earliest place, because that is the one the deadline runs
			// from and the one that makes the whole group late.
			ColumnExpr("MIN(o.started_at) AS first_seen").
			ColumnExpr("COUNT(*) AS places").
			// Named only where every place has the same person. Reporting one
			// of several would tell somebody a finding is being dealt with
			// when most of it is not.
			ColumnExpr("MIN(f.assigned_to) AS assigned_to").
			ColumnExpr("MAX(f.assigned_to) AS assigned_high").
			ColumnExpr("COUNT(f.assigned_to) AS assigned_count").
			Where("f.closed_run_id IS NULL").
			Where("o.started_at <= ?", seenBy).
			// Nothing the build already argued away, and nothing a person has
			// answered. A decision standing here means the clock is not running.
			Where("f.suppressed_by IS NULL").
			Where(`NOT EXISTS (SELECT 1 FROM "decision" AS de
				WHERE de.product_id = st.product_id
				  AND de.vulnerability_id = f.vulnerability_id
				  AND de.place_identity = f.place_identity
				  AND de.live_key IS NOT NULL)`).
			GroupExpr("v.identifier, c.name, f.urgency_exploited, p.display_name, " +
				"st.display_name, va.display_name, f.target_id, f.vulnerability_id, f.component_id").
			OrderExpr("MIN(o.started_at)").
			Limit(limit)
		query = each.where(query)

		if !all {
			query = query.Where("st.product_id IN (?)", bun.List(products))
		}
		query = onlyVisible(query, subject, products, all)

		var rows []Late
		if err := query.Scan(ctx, &rows); err != nil {
			return nil, fmt.Errorf("read what is running out of time: %w", err)
		}
		for _, row := range rows {
			row.Due = row.FirstSeen.Add(each.window)
			if row.AssignedCount != row.Places || row.AssignedTo == nil ||
				row.AssignedHigh == nil || *row.AssignedTo != *row.AssignedHigh {
				row.AssignedTo = nil
			}
			late = append(late, row)
		}
	}

	// Merged across the bands. Each list arrived ordered, and what somebody
	// wants to see is the whole estate by deadline rather than five lists.
	sort.SliceStable(late, func(i, j int) bool { return late[i].Due.Before(late[j].Due) })
	if len(late) > limit {
		late = late[:limit]
	}
	return late, nil
}
