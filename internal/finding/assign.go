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

// Assign records who is dealing with an issue in a component, across every
// build of one product that holds it.
//
// Set for the whole group at once rather than per place: assigning one place
// of an issue and not another is not something anybody means to do, and the
// places of a group are the same problem seen from several parents. **Across
// builds for the same reason** — the same code built as several variants is
// one piece of work, and it is answered by one judgment (REL-01).
//
// Assigning to nobody is how something is handed back, and is deliberately the
// same operation — taking work back is not a different kind of act from giving
// it out, and making it one produces two paths that drift.
// The build is named rather than the product, and the product is resolved from
// it here. Both are int64, so a caller passing the wrong one is invisible to
// the compiler — and it is invisible on SQLite too, where a fresh database
// makes the first product and the first build both 1. Taking the build keeps
// every call site saying what it is looking at and leaves one place that knows
// the grain.
func (s *Store) Assign(ctx context.Context, subject access.Subject, targetID, vulnerabilityID,
	componentID int64, to *int64) (moved int64, undisclosed bool, err error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, false, err
	}
	// Deciding who deals with something is a write, so it asks for a right.
	// Being able to *see* a finding is not being able to hand it around, and
	// narrowing by visibility is not an authorization check — it stops
	// somebody assigning what they cannot see, not somebody assigning.
	//
	// Which right depends on who it lands on (ACC-61). Taking work nobody
	// owns, and handing back your own, are part of triaging: the constant
	// stream of unowned findings ACC-54 produces would otherwise need
	// somebody's attention before anybody could start. Putting work on
	// somebody else, or taking what they are holding, is a different act and
	// asks for the right that names it.
	triages := subject.Holds(access.PublicTriage, productID) ||
		subject.Holds(access.PrivateTriage, productID)
	if !triages {
		return 0, false, access.Denied(fmt.Sprintf("decide who deals with findings in product %d", productID))
	}
	if !subject.Holds(access.Assigner, productID) {
		mine, err := onlyMine(ctx, s.db, subject, productID, vulnerabilityID, componentID, to)
		if err != nil {
			return 0, false, err
		}
		if !mine {
			return 0, false, access.Denied(fmt.Sprintf(
				"give work to somebody else in product %d — you may take what nobody owns, "+
					"and hand back your own", productID))
		}
	}
	visible := access.Visible(subject, productID)
	if len(visible) == 0 {
		return 0, false, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	now := s.now().UTC().Truncate(time.Microsecond)
	update := s.db.NewUpdate().Model((*Finding)(nil)).
		// Across the product's builds, not one of them. The same code built as
		// several variants is one piece of work — a judgment carries no
		// variant, so somebody taking this on has taken on every build of the
		// product holding the same component (REL-01). Assigning one build
		// left the identical work unassigned beside it, which is how a person
		// ends up holding half of what they think they hold.
		Where(`target_id IN (SELECT tg.id FROM "target" AS tg
			JOIN "stream" AS st ON st.id = tg.stream_id
			WHERE st.product_id = ?)`, productID).
		Where("vulnerability_id = ?", vulnerabilityID).
		Where("component_id = ?", componentID).
		Where("closed_at IS NULL").
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
		return 0, false, fmt.Errorf("record who is dealing with this: %w", err)
	}
	moved, err = result.RowsAffected()
	if err != nil {
		return 0, false, fmt.Errorf("record who is dealing with this: %w", err)
	}
	if moved == 0 {
		return 0, false, nil
	}

	// Whether what was just handed over is a finding nobody has announced.
	//
	// Answered here because the caller has to know it to decide what may be
	// said about it outside the application (NTF-15) and cannot see the rows
	// from where it stands. Asked after the write and narrowed the same way,
	// so it describes what actually moved. Any one row answers: they are one
	// issue at one component in one build, and visibility belongs to the
	// finding rather than to the place.
	private, err := s.db.NewSelect().Model((*Finding)(nil)).
		Column("id").
		Where(`target_id IN (SELECT tg.id FROM "target" AS tg
			WHERE tg.stream_id IN (SELECT st.id FROM "stream" AS st
				WHERE st.product_id = ?))`, productID).
		Where("vulnerability_id = ?", vulnerabilityID).
		Where("component_id = ?", componentID).
		Where("closed_at IS NULL").
		Where("visibility = ?", access.Private).
		Exists(ctx)
	if err != nil {
		return moved, false, fmt.Errorf("read whether that finding is disclosed: %w", err)
	}
	return moved, private, nil
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
			Where("closed_at IS NULL").
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
			Where("closed_at IS NULL")
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
	// Open is how many pieces of work they hold — an issue in a component in
	// a product, which is the unit everything else here counts in (REL-01).
	Open int `bun:"open"`
	// Places is how many findings those cover. The fan-out, kept alongside
	// rather than instead: one kernel flaw at forty-eight places is one thing
	// to answer and forty-eight rows to write, and both are worth saying.
	Places int `bun:"places"`
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
//
// **Counted in pieces of work, not in rows** — an issue in a component in a
// product, the same unit Unassigned and AssignedTo list in (REL-01). Counting
// rows made this screen disagree with every screen it links to: measured
// against a real image, one kernel flaw assigned to one person read as 48 held
// against her here and as the single item it is in her own list. The larger
// number is not even a worse version of the smaller one, because it moves with
// how far a component fans out rather than with how much anybody has to do.
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
			Where("f.closed_at IS NULL").
			Where("f.assigned_to IS NOT NULL")
		if !all {
			query = query.Where("st.product_id IN (?)", bun.List(products))
		}
		// The same narrowing every other query here carries. A count is as
		// much a disclosure as a row: "somebody holds six" tells a reader
		// there are six.
		return onlyVisible(query, subject, products, all)
	}

	// One row per person per piece of work, counted by grouping and counting
	// the groups. The derived table is named and quoted, because GROUPS is a
	// reserved word in MySQL 8 and the obvious alias is a syntax error on one
	// engine and fine on the other three (DAT-33).
	pieces := mine().
		ColumnExpr("f.assigned_to AS person_id").
		GroupExpr("f.assigned_to, f.vulnerability_id, f.component_id, st.product_id")

	var held []Holding
	if err := s.db.NewSelect().
		TableExpr(`(?) AS "work"`, pieces).
		ColumnExpr(`"work".person_id AS person_id`).
		ColumnExpr("COUNT(*) AS open").
		ColumnExpr("0 AS places").
		ColumnExpr("0 AS overdue").
		GroupExpr(`"work".person_id`).
		Scan(ctx, &held); err != nil {
		return nil, fmt.Errorf("read who is holding what: %w", err)
	}

	// The fan-out, as its own pass rather than summed out of the grouping.
	// Summing a count across a derived table comes back as a decimal on two of
	// the four engines, and a cast to make it an integer is exactly the kind
	// of engine-specific spelling that has already been wrong here once.
	var spread []struct {
		PersonID int64 `bun:"person_id"`
		Places   int   `bun:"places"`
	}
	if err := mine().
		ColumnExpr("f.assigned_to AS person_id").
		ColumnExpr("COUNT(*) AS places").
		GroupExpr("f.assigned_to").
		Scan(ctx, &spread); err != nil {
		return nil, fmt.Errorf("read how far what they hold reaches: %w", err)
	}
	places := map[int64]int{}
	for _, row := range spread {
		places[row.PersonID] = row.Places
	}
	for i := range held {
		held[i].Places = places[held[i].PersonID]
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
	//
	// Counted in the same units as the rest of this: a piece of work is late
	// when any of its places is. The alternative reads worse in both
	// directions — a group with one late place among forty is late, and
	// calling it a fortieth of one is a number nobody acts on.
	standing, args := OffTheClock("st.product_id", s.now())
	late := mine().
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.assigned_to AS person_id").
		Where("f.due_at IS NOT NULL").
		Where("f.due_at < ?", s.now().UTC()).
		Where("f.suppressed_by IS NULL").
		Where("NOT "+standing, args...).
		GroupExpr("f.assigned_to, f.vulnerability_id, f.component_id, st.product_id")
	err := s.db.NewSelect().
		TableExpr(`(?) AS "work"`, late).
		ColumnExpr(`"work".person_id AS person_id`).
		ColumnExpr("COUNT(*) AS overdue").
		GroupExpr(`"work".person_id`).
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
	// Stream and Variant name **a** build holding this, not the only one. A
	// screen needs somewhere to link to and an action needs a finding to name,
	// and where several builds hold the same code any of them will do. What
	// says there are several is Builds, so a screen can show that instead of
	// naming one of them as though it were the answer.
	Stream  string `bun:"stream"`
	Variant string `bun:"variant"`
	Urgency int64  `bun:"urgency"`
	// Places is how many findings this covers, across every build it is in.
	// It is what a judgment here would be recorded against.
	Places int `bun:"places"`
	// Builds is how many builds hold it. One is the ordinary answer and reads
	// as it always did; more than one is the whole point of collapsing them.
	Builds int `bun:"builds"`
}

// Unassigned reports the work nobody is dealing with, across every product the
// subject may see, most urgent first.
//
// Across products deliberately. Work falling between people is not a
// per-product problem — it is exactly the thing that hides when every screen
// is scoped to one product and nobody looks at the others.
//
// **One item per issue in a component in a product, not one per build**
// (REL-01). Variants are mostly the same thing built twice: a decision is keyed
// on the product, the place and the upstream versions and carries no variant,
// so answering this on one build answers it on every build of that product
// holding the same code. Listed per build, a second variant doubled this
// screen while doubling none of the work — which is how a queue stops being
// read.
//
// **Genuine differences still break out, and they break out by themselves.** A
// component row is one name at one version, shared by every build that ships
// it, so two variants at the same version group together and two at different
// versions do not. Nothing here has to decide which case it is looking at.
//
// The product stays in the key. A decision is a claim about one product's code
// (TRI-48), so two products shipping the identical component at the identical
// version are two judgments, and merging them would offer one answer for work
// that is answered separately.
func (s *Store) Unassigned(ctx context.Context, subject access.Subject, scope Scope,
	limit, offset int) ([]Owned, int, error) {
	return s.work(ctx, subject, scope, nil, limit, offset)
}

// AssignedTo reports what one person is dealing with, in the same units as
// what nobody is.
//
// The same shape deliberately. "What is waiting for me" and "what is waiting
// for nobody" are the same question asked of a different holder, and answering
// them in two shapes is how a screen ends up saying somebody holds ninety
// things because it counted places where the other counted judgments.
func (s *Store) AssignedTo(ctx context.Context, subject access.Subject, personID int64,
	scope Scope, limit, offset int) ([]Owned, int, error) {

	return s.work(ctx, subject, scope, &personID, limit, offset)
}

// work is what somebody holds, or what nobody does.
func (s *Store) work(ctx context.Context, subject access.Subject, scope Scope,
	holder *int64, limit, offset int) ([]Owned, int, error) {

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
			Where("f.closed_at IS NULL")
		if holder == nil {
			q = q.Where("f.assigned_to IS NULL")
		} else {
			q = q.Where("f.assigned_to = ?", *holder)
		}
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
		ProductID       int64 `bun:"product_id"`
		TargetID        int64 `bun:"target_id"`
		Builds          int   `bun:"builds"`
		Urgency         int64 `bun:"urgency"`
		Places          int   `bun:"places"`
		Total           int   `bun:"total"`
	}
	err := narrow(s.db.NewSelect()).
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("st.product_id AS product_id").
		// One of the builds, and how many there are. The one is what a link
		// and an action need somewhere to point; the count is what stops a
		// screen presenting it as the only one. MIN rather than any other
		// choice because it is stable: a row that named a different build
		// between two reads would move under somebody.
		ColumnExpr("MIN(f.target_id) AS target_id").
		ColumnExpr("COUNT(DISTINCT f.target_id) AS builds").
		ColumnExpr("MAX(f.urgency) AS urgency").
		ColumnExpr("COUNT(*) AS places").
		// The total rides on the page, as the findings list's does: the
		// groups the narrowing admits, counted after the grouping and before
		// the limit, in the statement that groups them.
		ColumnExpr("COUNT(*) OVER () AS total").
		GroupExpr("f.vulnerability_id, f.component_id, st.product_id").
		OrderExpr("urgency DESC, f.vulnerability_id, f.component_id, st.product_id").
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
				GroupExpr("f.vulnerability_id, f.component_id, st.product_id")).
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
			Urgency: head.Urgency, Places: head.Places, Builds: head.Builds,
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

// onlyMine reports whether an assignment is one triage alone may make: taking
// something nobody owns, or handing back something already yours.
//
// Both halves are about who holds it now rather than about who is asking, so
// they are read from the rows rather than inferred. A finding somebody else
// holds is refused even when the caller is putting it on themselves — taking
// work off a colleague is the act the right exists to name, and doing it to
// yourself is still doing it.
func onlyMine(ctx context.Context, db bun.IDB, subject access.Subject,
	productID, vulnerabilityID, componentID int64, to *int64,
) (bool, error) {
	// Assigning to anybody but yourself is giving work away, whatever is
	// there now. Assigning to nobody is only ever handing back.
	if to != nil && *to != subject.ID {
		return false, nil
	}

	// Asked as "is any of this held by somebody else", rather than by reading
	// the holders and comparing them here. Scanning a nullable column into
	// pointers hands back a pointer to zero where the row is null, which reads
	// as "held by nobody at all" only if you remember that — and once it is a
	// comparison in the query, the engine answers it and there is nothing to
	// remember.
	somebodyElse, err := db.NewSelect().Model((*Finding)(nil)).
		Column("id").
		Where(`target_id IN (SELECT tg.id FROM "target" AS tg
			WHERE tg.stream_id IN (SELECT st.id FROM "stream" AS st
				WHERE st.product_id = ?))`, productID).
		Where("vulnerability_id = ?", vulnerabilityID).
		Where("component_id = ?", componentID).
		Where("closed_at IS NULL").
		Where("assigned_to IS NOT NULL").
		Where("assigned_to <> ?", subject.ID).
		Exists(ctx)
	if err != nil {
		return false, fmt.Errorf("read who is dealing with this: %w", err)
	}
	return !somebodyElse, nil
}
