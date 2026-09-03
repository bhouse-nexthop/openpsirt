package triage

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Judged is one judgment as an auditor reads it: what was decided, about what,
// on whose say-so, agreed to by whom, and when each of those happened.
//
// Assembled rather than looked up one decision at a time. The question an
// auditor asks is about a period and a product — "show me every dismissal we
// made this year" — and a screen that answered it by opening each row would be
// one they read by clicking a hundred times.
type Judged struct {
	Decision
	// Reasoning is the words the standing approval was given for. Where the
	// text has been revised since, the approval was withdrawn by that act
	// (TRI-24), so what is shown and what was agreed to cannot drift apart.
	Reasoning string
	// Issue is the identifier the vulnerability is filed under here.
	Issue string
	// Component and Version are what it is about; Consumer is what pulls that
	// in, or empty where the build holds it directly.
	Component string
	Version   string
	Consumer  string
	Product   string
	// ProposedByName and Approvals are the separation-of-duties record: who
	// made the claim, and who agreed to it. Two different people is the whole
	// control, so both names are carried rather than a count.
	ProposedByName string
	Approvals      []Agreed
}

// Agreed is one person agreeing to one revision of the reasoning.
type Agreed struct {
	By string
	At time.Time
	// WithdrawnAt is when the agreement was taken back — by the approver, or
	// by somebody editing the words it was given for. An approval that was
	// later withdrawn is part of the record rather than removed from it
	// (TRI-25): what somebody agreed to and then stopped agreeing to is
	// exactly what an audit is looking for.
	WithdrawnAt *time.Time
}

// Standing reports whether this judgment applies now.
func (j Judged) Standing() bool { return j.State == Approved && j.LiveKey != nil }

// BySomebodyElse reports whether the people who proposed and agreed differ,
// which is the control TRI-41 exists for stated as a fact about the record.
//
// Read from the names rather than trusted: an audit that reported the rule as
// satisfied because the rule exists would be reporting on itself.
func (j Judged) BySomebodyElse() bool {
	for _, agreed := range j.Approvals {
		if agreed.WithdrawnAt == nil && agreed.By != j.ProposedByName {
			return true
		}
	}
	return false
}

// Audit returns the judgments made in a period, newest first, with everything
// needed to read one without opening it.
//
// Narrowed by what the reader may see, like everything else here. An auditor
// holding one product's findings sees that product's judgments; nothing about
// this view is exempt from the visibility rules, because a report that showed
// more than the screens it summarizes would be a way around them.
func (s *Store) Audit(ctx context.Context, subject access.Subject, f Filter,
	from, to time.Time, limit, offset int) ([]Judged, int, error) {

	if limit <= 0 || limit > 500 {
		limit = 100
	}

	narrow := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = readableBy(q, subject, "de")
		if f.ProductID != 0 {
			q = q.Where("de.product_id = ?", f.ProductID)
		}
		if f.Outcome != "" {
			q = q.Where("de.outcome = ?", f.Outcome)
		}
		if f.State != "" {
			q = q.Where("de.state = ?", f.State)
		}
		// The period is the proposal's date, not the approval's. A judgment
		// belongs to when it was made; dating it by its agreement would move
		// it out of the period it was argued in whenever an approval came
		// late, which is the ordinary case.
		if !from.IsZero() {
			q = q.Where("de.proposed_at >= ?", from)
		}
		if !to.IsZero() {
			q = q.Where("de.proposed_at < ?", to)
		}
		return q
	}

	total, err := narrow(s.db.NewSelect().Model((*Decision)(nil))).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count the judgments made: %w", err)
	}

	var decisions []Decision
	if err := narrow(s.db.NewSelect().Model(&decisions)).
		Order("de.id DESC").Limit(limit).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read the judgments made: %w", err)
	}
	if len(decisions) == 0 {
		return nil, total, nil
	}

	reasoning, err := s.currentReasoning(ctx, decisions)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(decisions))
	people := map[int64]bool{}
	for _, decision := range decisions {
		ids = append(ids, decision.ID)
		people[decision.ProposedBy] = true
	}

	// Every approval for the page in one statement, not one per row. A year of
	// judgments is a page of a hundred, and a query each would make the cost
	// of the report a count of rows rather than a count of pages.
	var approvals []Approval
	if err := s.db.NewSelect().Model(&approvals).
		Where("decision_id IN (?)", bun.List(ids)).
		Order("decision_id ASC", "id ASC").Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read who agreed: %w", err)
	}
	for _, approval := range approvals {
		people[approval.ApprovedBy] = true
	}

	named, err := s.namesOf(ctx, people)
	if err != nil {
		return nil, 0, err
	}
	about, err := s.aboutEach(ctx, decisions)
	if err != nil {
		return nil, 0, err
	}

	agreed := make(map[int64][]Agreed, len(decisions))
	for _, approval := range approvals {
		agreed[approval.DecisionID] = append(agreed[approval.DecisionID], Agreed{
			By: named[approval.ApprovedBy], At: approval.ApprovedAt,
			WithdrawnAt: approval.WithdrawnAt,
		})
	}

	out := make([]Judged, 0, len(decisions))
	for _, decision := range decisions {
		row := Judged{
			Decision: decision, Reasoning: reasoning[decision.ID],
			ProposedByName: named[decision.ProposedBy],
			Approvals:      agreed[decision.ID],
		}
		if what, held := about[decision.ID]; held {
			row.Issue, row.Component, row.Version = what.Issue, what.Component, what.Version
			row.Consumer, row.Product = what.Consumer, what.Product
		}
		out = append(out, row)
	}
	return out, total, nil
}

// namesOf reads sign-in identities for a set of people.
func (s *Store) namesOf(ctx context.Context, ids map[int64]bool) (map[int64]string, error) {
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}
	wanted := make([]int64, 0, len(ids))
	for id := range ids {
		wanted = append(wanted, id)
	}
	var rows []struct {
		ID       int64  `bun:"id"`
		Identity string `bun:"identity"`
	}
	if err := s.db.NewSelect().
		TableExpr(`"person" AS p`).
		ColumnExpr("p.id AS id").ColumnExpr("p.identity AS identity").
		Where("p.id IN (?)", bun.List(wanted)).Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read who these people are: %w", err)
	}
	named := make(map[int64]string, len(rows))
	for _, row := range rows {
		named[row.ID] = row.Identity
	}
	return named, nil
}

// what one decision was about, in the words a reader knows it by.
type what struct {
	Issue     string
	Component string
	Version   string
	Consumer  string
	Product   string
}

// aboutEach names what each decision was about, for the whole page at once.
//
// A decision stores the place as a hash of names rather than the names, so
// what it was about is recovered from a finding at that place. **Findings in
// any state, not only open ones**: a judgment about something that has since
// been fixed or removed is exactly what an audit asks for, and joining only
// open findings would leave the oldest and most interesting rows unnamed.
//
// The earliest matching finding, so the answer is stable between reads rather
// than moving as rows open and close underneath it.
func (s *Store) aboutEach(ctx context.Context, decisions []Decision) (map[int64]what, error) {
	keys := make([]int64, 0, len(decisions))
	for _, decision := range decisions {
		keys = append(keys, decision.ID)
	}
	var rows []struct {
		DecisionID int64  `bun:"decision_id"`
		Issue      string `bun:"issue"`
		Component  string `bun:"component"`
		Version    string `bun:"version"`
		Consumer   string `bun:"consumer"`
		Product    string `bun:"product"`
	}
	err := s.db.NewSelect().
		TableExpr(`"decision" AS de`).
		Join(`JOIN "finding" AS f ON f.vulnerability_id = de.vulnerability_id`+
			` AND f.place_identity = de.place_identity`).
		Join(`JOIN "target" AS tg ON tg.id = f.target_id`).
		Join(`JOIN "stream" AS st ON st.id = tg.stream_id AND st.product_id = de.product_id`).
		Join(`JOIN "product" AS p ON p.id = st.product_id`).
		Join(`JOIN "vulnerability" AS v ON v.id = f.vulnerability_id`).
		Join(`JOIN "component" AS c ON c.id = f.component_id`).
		Join(`LEFT JOIN "component" AS uc ON uc.id = f.consumer_id`).
		ColumnExpr("de.id AS decision_id").
		ColumnExpr("MIN(v.identifier) AS issue").
		ColumnExpr("MIN(c.name) AS component").
		ColumnExpr("MIN(c.version) AS version").
		ColumnExpr("MIN(COALESCE(uc.name, ?)) AS consumer", "").
		ColumnExpr("MIN(p.display_name) AS product").
		Where("de.id IN (?)", bun.List(keys)).
		GroupExpr("de.id").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what these judgments were about: %w", err)
	}
	about := make(map[int64]what, len(rows))
	for _, row := range rows {
		about[row.DecisionID] = what{
			Issue: row.Issue, Component: row.Component, Version: row.Version,
			Consumer: row.Consumer, Product: row.Product,
		}
	}
	return about, nil
}
