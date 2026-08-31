package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

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
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
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
		return 0, nil
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
		return 0, fmt.Errorf("that would hand their work to themselves")
	}
	return s.handOver(ctx, subject, from, &to)
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
	PersonID int64
	Open     int
	Overdue  int
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

	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("f.assigned_to AS person_id").
		ColumnExpr("COUNT(*) AS open").
		Where("f.closed_run_id IS NULL").
		Where("f.assigned_to IS NOT NULL").
		GroupExpr("f.assigned_to")
	if !all {
		query = query.Where("st.product_id IN (?)", bun.List(products))
	}

	var held []Holding
	if err := query.Scan(ctx, &held); err != nil {
		return nil, fmt.Errorf("read who is holding what: %w", err)
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
func (s *Store) Unassigned(ctx context.Context, subject access.Subject,
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
		// Visibility per product, the way it is everywhere else: holding
		// private read on one product does not make undisclosed findings on
		// another visible.
		return q.Where("(f.visibility = ? OR st.product_id IN (?))",
			access.Public, bun.List(privateFor(subject, products, all)))
	}

	total, err := narrow(s.db.NewSelect()).
		ColumnExpr("COUNT(DISTINCT f.vulnerability_id || '-' || f.component_id)").
		Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what nobody is dealing with: %w", err)
	}

	var rows []Owned
	err = narrow(s.db.NewSelect()).
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN product AS p ON p.id = st.product_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("MIN(v.identifier) AS vulnerability").
		ColumnExpr("MIN(c.name) AS component").
		ColumnExpr("MIN(c.version) AS version").
		ColumnExpr("MIN(v.severity) AS severity").
		ColumnExpr("MAX(CASE WHEN f.urgency_exploited THEN 1 ELSE 0 END) AS exploited").
		ColumnExpr("MIN(p.display_name) AS product").
		ColumnExpr("MIN(st.display_name) AS stream").
		ColumnExpr("MIN(va.display_name) AS variant").
		ColumnExpr("MAX(f.urgency) AS urgency").
		ColumnExpr("COUNT(*) AS places").
		GroupExpr("f.vulnerability_id, f.component_id, f.target_id").
		OrderExpr("urgency DESC, vulnerability").
		Limit(limit).Offset(offset).
		Scan(ctx, &rows)
	if err != nil {
		return nil, 0, fmt.Errorf("read what nobody is dealing with: %w", err)
	}
	return rows, total, nil
}

// privateFor is the products this subject may see undisclosed findings in.
func privateFor(subject access.Subject, products []int64, all bool) []int64 {
	if all {
		return products
	}
	held := make([]int64, 0, len(products))
	for _, id := range products {
		if subject.Reads(access.Private, id) {
			held = append(held, id)
		}
	}
	if len(held) == 0 {
		// An empty list would make the condition always false, which is the
		// right answer: this person sees no undisclosed findings anywhere.
		return []int64{0}
	}
	return held
}
