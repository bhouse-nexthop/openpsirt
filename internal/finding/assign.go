package finding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// ErrSamePerson says work was handed from somebody to themselves.
//
// A sentinel because it is the one refusal here that is about what was asked
// rather than about what went wrong. Everything else this returns is a
// database that could not answer, and reporting those to a caller as though
// they were its fault is how a driver's error text ends up on a screen.
var ErrSamePerson = errors.New("that would hand their work to themselves")

// Assign records who is dealing with an issue in a component.
//
// Set for the whole group at once rather than per place: assigning one place
// of an issue and not another is not something anybody means to do, and the
// places of a group are the same problem seen from several parents.
//
// Assigning to nobody is how something is handed back, and is deliberately the
// same operation — taking work back is not a different kind of act from giving
// it out, and making it one produces two paths that drift.
func (s *Store) Assign(ctx context.Context, subject access.Subject, targetID, vulnerabilityID,
	componentID int64, to *int64) (int64, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, err
	}
	// Deciding who deals with something is a write, so it asks for the right
	// that names it. Being able to *see* a finding is not being able to hand
	// it around, and narrowing by visibility is not an authorization check —
	// it stops somebody assigning what they cannot see, not somebody
	// assigning.
	if !subject.Holds(access.PublicTriage, productID) &&
		!subject.Holds(access.PrivateTriage, productID) {
		return 0, access.Denied(fmt.Sprintf("decide who deals with findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	update := s.db.NewUpdate().Model((*Finding)(nil)).
		Where("target_id = ?", targetID).
		Where("vulnerability_id = ?", vulnerabilityID).
		Where("component_id = ?", componentID).
		Where("closed_run_id IS NULL").
		// Narrowed by what this person may see, like every other query here. A
		// finding nobody has disclosed is not one somebody may hand around.
		Where("visibility IN (?)", bun.List(visible))

	if to == nil {
		update = update.Set("assigned_to = ?", nil).Set("assigned_at = ?", nil)
	} else {
		update = update.Set("assigned_to = ?", *to).Set("assigned_at = ?", now)
	}

	result, err := update.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("record who is dealing with this: %w", err)
	}
	moved, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("record who is dealing with this: %w", err)
	}
	return moved, nil
}

// Release hands everything one person holds back to nobody.
//
// The case this exists for is somebody leaving. Their work is then invisible
// twice over — not in the shared list because it is assigned, and not in
// anybody's own list because they are not here — so it is in no view at all
// rather than visibly orphaned.
//
// Nothing tells us somebody has left. Membership is read at sign-in, and
// somebody who has gone never signs in again, so this is an action an
// administrator takes rather than something the software discovers.
func (s *Store) Release(ctx context.Context, subject access.Subject, personID int64) (int64, error) {
	return s.handOver(ctx, subject, personID, nil)
}

// HandOver moves everything one person holds to somebody else.
//
// Kept apart from Release because they answer different questions. Handing
// over says who is dealing with it now; releasing says nobody is, and puts it
// back where it can be picked up — which is the honest answer when whoever
// takes it on has not been decided.
func (s *Store) HandOver(ctx context.Context, subject access.Subject, from, to int64) (int64, error) {
	if from == to {
		return 0, ErrSamePerson
	}
	return s.handOver(ctx, subject, from, &to)
}

// ReleaseIn hands back what one person holds in one product.
//
// The narrow form of Release, for when somebody's last role on a product is
// withdrawn. What they hold elsewhere is untouched, because nothing about that
// product changed.
//
// Administrative like Release: this moves work somebody else was given.
func (s *Store) ReleaseIn(ctx context.Context, subject access.Subject, personID, productID int64) (int64, error) {
	if !subject.Admin {
		return 0, access.Denied("move work assigned to somebody else")
	}
	var moved int64
	err := database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		result, err := tx.NewUpdate().Model((*Finding)(nil)).
			Set("assigned_to = ?", nil).Set("assigned_at = ?", nil).
			Where("assigned_to = ?", personID).
			Where("closed_run_id IS NULL").
			Where(`target_id IN (SELECT tg.id FROM "target" AS tg
				JOIN "stream" AS st ON st.id = tg.stream_id
				WHERE st.product_id = ?)`, productID).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("hand back what they were dealing with: %w", err)
		}
		moved, err = result.RowsAffected()
		return err
	})
	return moved, err
}

func (s *Store) handOver(ctx context.Context, subject access.Subject, from int64, to *int64) (int64, error) {
	if !subject.Admin {
		// Moving work somebody else was given is an administrative act. A
		// person hands back their own by assigning it to nobody.
		return 0, access.Denied("move work assigned to somebody else")
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	var moved int64
	err := database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		update := tx.NewUpdate().Model((*Finding)(nil)).
			Where("assigned_to = ?", from).
			Where("closed_run_id IS NULL")
		if to == nil {
			update = update.Set("assigned_to = ?", nil).Set("assigned_at = ?", nil)
		} else {
			update = update.Set("assigned_to = ?", *to).Set("assigned_at = ?", now)
		}
		result, err := update.Exec(ctx)
		if err != nil {
			return fmt.Errorf("move what they were dealing with: %w", err)
		}
		moved, _ = result.RowsAffected()
		return nil
	})
	return moved, err
}

// Holding is how much work one person has, and how much of it is late.
type Holding struct {
	PersonID int64 `bun:"person_id"`
	Open     int   `bun:"open"`
	// Overdue is how much of it is past its deadline. The number that says
	// whether somebody is holding work or sitting on it — a large open count
	// on somebody who is keeping up is not the same signal at all.
	Overdue int `bun:"overdue"`
}

// HeldBy reports what each person is dealing with, for the people this subject
// may see findings about.
//
// The number that matters is not how many findings exist but how many are
// stuck behind somebody: an idle account holding nothing is harmless, and work
// waiting on a person who is not here is the thing worth surfacing.
func (s *Store) HeldBy(ctx context.Context, subject access.Subject) ([]Holding, error) {
	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, nil
	}

	mine := func() *bun.SelectQuery {
		query := s.db.NewSelect().
			TableExpr("finding AS f").
			Join("JOIN target AS tg ON tg.id = f.target_id").
			Join("JOIN stream AS st ON st.id = tg.stream_id").
			Where("f.closed_run_id IS NULL").
			Where("f.assigned_to IS NOT NULL")
		if !all {
			query = query.Where("st.product_id IN (?)", bun.List(products))
		}
		// The same narrowing every other query here carries. A count is as
		// much a disclosure as a row: "somebody holds six" tells a reader
		// there are six.
		return onlyVisible(query, subject, products, all)
	}

	var held []Holding
	if err := mine().
		ColumnExpr("f.assigned_to AS person_id").
		ColumnExpr("COUNT(*) AS open").
		ColumnExpr("0 AS overdue").
		GroupExpr("f.assigned_to").
		Scan(ctx, &held); err != nil {
		return nil, fmt.Errorf("read who is holding what: %w", err)
	}

	// How much of it is late. One pass, against the deadline stored on the
	// finding (REM-26) — it used to be a pass per urgency band, each with its
	// own cutoff, because the deadline was derived. Overdue has to mean the
	// same thing here as on the screen that lists what is running out, and the
	// surest way to keep two answers equal is for there to be one of them: the
	// deadline is the stored one, and what takes a finding off the clock is
	// the one condition both read — a decision that applies, and nothing the
	// build argued away. Counting every late finding regardless made a
	// dismissed finding overdue against whoever held it while the list of
	// what is running out, rightly, left it off.
	var counted []struct {
		PersonID int64 `bun:"person_id"`
		Overdue  int   `bun:"overdue"`
	}
	standing, args := OffTheClock("st.product_id", s.now())
	err := mine().
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.assigned_to AS person_id").
		ColumnExpr("COUNT(*) AS overdue").
		Where("f.due_at IS NOT NULL").
		Where("f.due_at < ?", s.now().UTC()).
		Where("f.suppressed_by IS NULL").
		Where("NOT "+standing, args...).
		GroupExpr("f.assigned_to").
		Scan(ctx, &counted)
	if err != nil {
		return nil, fmt.Errorf("read how much of it is late: %w", err)
	}
	overdue := map[int64]int{}
	for _, row := range counted {
		overdue[row.PersonID] += row.Overdue
	}
	for i := range held {
		held[i].Overdue = overdue[held[i].PersonID]
	}
	return held, nil
}

// Owned is one finding nobody is dealing with, named rather than numbered.
type Owned struct {
	VulnerabilityID int64  `bun:"vulnerability_id"`
	ComponentID     int64  `bun:"component_id"`
	Vulnerability   string `bun:"vulnerability"`
	Component       string `bun:"component"`
	Version         string `bun:"version"`
	Severity        string `bun:"severity"`
	Exploited       bool   `bun:"exploited"`
	Product         string `bun:"product"`
	Stream          string `bun:"stream"`
	Variant         string `bun:"variant"`
	Urgency         int64  `bun:"urgency"`
	Places          int    `bun:"places"`
}

// Unassigned reports the work nobody is dealing with, across every product the
// subject may see, most urgent first.
//
// Across products deliberately. Work falling between people is not a
// per-product problem — it is exactly the thing that hides when every screen
// is scoped to one product and nobody looks at the others.
func (s *Store) Unassigned(ctx context.Context, subject access.Subject, scope Scope,
	limit, offset int) ([]Owned, int, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || (!all && len(products) == 0) {
		return nil, 0, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	narrow := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = q.TableExpr("finding AS f").
			Join("JOIN target AS tg ON tg.id = f.target_id").
			Join("JOIN stream AS st ON st.id = tg.stream_id").
			Where("f.closed_run_id IS NULL").
			Where("f.assigned_to IS NULL")
		if !all {
			q = q.Where("st.product_id IN (?)", bun.List(products))
		}
		return scope.Narrow(onlyVisible(q, subject, products, all))
	}

	// Counted by grouping and counting the groups, not by a COUNT DISTINCT
	// expression. Two reasons, and both were live at once.
	//
	// With no GROUP BY, bun's Count() emits its own count(*) and never appends
	// the columns — so the expression was dead text and the total was a count
	// of finding *rows* while the list is grouped. On a real image the fan-out
	// is the whole point of this model, so the total was not slightly wrong.
	//
	// And the expression concatenated with ||, which on two of the four
	// engines is logical OR: the operands coerce to numbers, the whole thing
	// collapses to 1, and the count comes back as 1 for any non-empty result.
	// Measured: 3 on SQLite and PostgreSQL, 1 on MySQL and MariaDB.
	//
	// The derived table is named "grouped" and quoted. GROUPS is a reserved
	// word in MySQL 8 — it names a window frame type — so the obvious alias
	// is a syntax error on one engine and fine on the other three (DAT-33).
	// The page in two statements: which groups, then their names. The first
	// groups and orders over finding and the two joins the scoping needs; the
	// names of the issue, the component and the build come from a second
	// statement over the page rather than from four more joins under the
	// aggregate, which is what the first version did — a text column reduced
	// with MIN once per row of the grouping to read fifty names.
	var heads []struct {
		VulnerabilityID int64 `bun:"vulnerability_id"`
		ComponentID     int64 `bun:"component_id"`
		TargetID        int64 `bun:"target_id"`
		Urgency         int64 `bun:"urgency"`
		Places          int   `bun:"places"`
		Total           int   `bun:"total"`
	}
	err := narrow(s.db.NewSelect()).
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("f.target_id AS target_id").
		ColumnExpr("MAX(f.urgency) AS urgency").
		ColumnExpr("COUNT(*) AS places").
		// The total rides on the page, as the findings list's does: the
		// groups the narrowing admits, counted after the grouping and before
		// the limit, in the statement that groups them.
		ColumnExpr("COUNT(*) OVER () AS total").
		GroupExpr("f.vulnerability_id, f.component_id, f.target_id").
		OrderExpr("urgency DESC, f.vulnerability_id, f.component_id, f.target_id").
		Limit(limit).Offset(offset).
		Scan(ctx, &heads)
	if err != nil {
		return nil, 0, fmt.Errorf("read what nobody is dealing with: %w", err)
	}
	total := 0
	if len(heads) > 0 {
		total = heads[0].Total
	} else {
		if total, err = s.db.NewSelect().
			TableExpr(`(?) AS "grouped"`, narrow(s.db.NewSelect()).
				ColumnExpr("f.vulnerability_id").
				GroupExpr("f.vulnerability_id, f.component_id, f.target_id")).
			Count(ctx); err != nil {
			return nil, 0, fmt.Errorf("count what nobody is dealing with: %w", err)
		}
	}

	issues := make([]int64, 0, len(heads))
	components := make([]int64, 0, len(heads))
	targets := make([]int64, 0, len(heads))
	for _, head := range heads {
		issues = append(issues, head.VulnerabilityID)
		components = append(components, head.ComponentID)
		targets = append(targets, head.TargetID)
	}
	named, err := issuesNamed(ctx, s.db, issues)
	if err != nil {
		return nil, 0, err
	}
	shipped, err := componentsNamed(ctx, s.db, components)
	if err != nil {
		return nil, 0, err
	}
	builds, err := targetsNamed(ctx, s.db, targets)
	if err != nil {
		return nil, 0, err
	}

	rows := make([]Owned, 0, len(heads))
	for _, head := range heads {
		row := Owned{
			VulnerabilityID: head.VulnerabilityID, ComponentID: head.ComponentID,
			Urgency: head.Urgency, Places: head.Places,
			Exploited: Rank(head.Urgency).Exploited(),
		}
		if issue, held := named[head.VulnerabilityID]; held {
			row.Vulnerability, row.Severity = issue.Identifier, issue.Severity
		}
		if component, held := shipped[head.ComponentID]; held {
			row.Component, row.Version = component.Name, component.Version
		}
		if build, held := builds[head.TargetID]; held {
			row.Product, row.Stream, row.Variant = build.Product, build.Stream, build.Variant
		}
		rows = append(rows, row)
	}
	return rows, total, nil
}

// buildName is what a build is called on a screen: its product, stream and
// variant, as people know them.
type buildName struct {
	TargetID int64  `bun:"target_id"`
	Product  string `bun:"product"`
	Stream   string `bun:"stream"`
	Variant  string `bun:"variant"`
}

// targetsNamed reads the names of the builds these targets are, by target.
func targetsNamed(ctx context.Context, db *bun.DB, ids []int64) (map[int64]buildName, error) {
	held := map[int64]buildName{}
	if len(ids) == 0 {
		return held, nil
	}
	var builds []buildName
	err := db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		ColumnExpr("tg.id AS target_id").
		ColumnExpr("p.display_name AS product").
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		Where("tg.id IN (?)", bun.List(ids)).
		Scan(ctx, &builds)
	if err != nil {
		return nil, fmt.Errorf("name the builds on the page: %w", err)
	}
	for _, build := range builds {
		held[build.TargetID] = build
	}
	return held, nil
}

// onlyVisible narrows a query to what this subject may read, per product.
//
// Holding private read on one product does not make undisclosed findings on
// another visible, so the clause is per product rather than a single flag.
//
// **An administrator is not narrowed at all**, and the first version of this
// got that exactly backwards: Products() reports "everything" as an empty list
// with a flag, the empty list rendered as IN (NULL) — which is never true —
// and the clause collapsed to public-only for the one subject who is supposed
// to see everything. Their dashboard, deadline list and trend all
// under-reported, with nothing saying so.
func onlyVisible(q *bun.SelectQuery, subject access.Subject, products []int64, all bool) *bun.SelectQuery {
	if all {
		return q
	}
	held := make([]int64, 0, len(products))
	for _, id := range products {
		if subject.Reads(access.Private, id) {
			held = append(held, id)
		}
	}
	if len(held) == 0 {
		// Nothing undisclosed anywhere, which is a real answer rather than an
		// empty condition to be filled in.
		return q.Where("f.visibility = ?", access.Public)
	}
	return q.Where("(f.visibility = ? OR st.product_id IN (?))",
		access.Public, bun.List(held))
}
