package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

// Scope narrows a cross-product answer to what somebody has selected.
//
// The screens that span products — what is running out, what nobody owns, how
// the estate is trending — used to answer for everything a reader may see and
// nothing else (UIX-08). Once a product is chosen in the picker, a page
// counting the others is answering a question nobody asked, so the picker
// narrows the whole interface and "all" is offered at each level rather than
// being the only option (UIX-38).
//
// Each level is a resolved identifier rather than a name, because a name is
// matched by a rule the catalog owns and there should not be a second copy
// of it here. Nil means "all of them", which is what an unselected level means.
//
// The levels are independent. A variant belongs to the product rather than to
// one branch of it (MDL-01), so "this product, every branch, this variant" is
// a real question — what a product is built as, across everything still being
// built. What cannot happen is a branch or variant without a product, which
// the interface prevents by leaving those unselectable and the caller refuses
// rather than guessing which product was meant.
type Scope struct {
	ProductID *int64
	StreamID  *int64
	VariantID *int64
}

// Narrow applies the scope to a query that already joins target AS tg and
// stream AS st.
//
// Filtering on the identifiers the target already carries rather than by
// joining names: every one of these queries has the joins for other reasons,
// and comparing an integer the row holds is cheaper than comparing a string
// through a join that exists to produce a label.
func (s Scope) Narrow(q *bun.SelectQuery) *bun.SelectQuery {
	if s.ProductID != nil {
		q = q.Where("st.product_id = ?", *s.ProductID)
	}
	if s.StreamID != nil {
		q = q.Where("tg.stream_id = ?", *s.StreamID)
	}
	if s.VariantID != nil {
		q = q.Where("tg.variant_id = ?", *s.VariantID)
	}
	return q
}

// Builds is the identifiers of the builds a scope covers, in a stable order.
//
// Resolved once and bound into the queries that follow rather than joined into
// each of them. The findings list reads its page from an index that leads with
// the build (`finding_group_idx`), so naming the builds keeps that index usable
// as one range per build; joining the catalog into the same statement would
// make the engine reach a row to discover what it already knew from the key.
//
// A selection matching nothing is not an error. A product whose releases have
// never been scanned, or a variant that arrived after the branch it is asked
// about, is an empty list rather than a failure — the answer is that nothing
// is open there.
func (s *Store) Builds(ctx context.Context, scope Scope) ([]int64, error) {
	var ids []int64
	q := s.db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("tg.id").
		OrderExpr("tg.id")
	if err := scope.Narrow(q).Scan(ctx, &ids); err != nil {
		return nil, fmt.Errorf("read which builds are in scope: %w", err)
	}
	return ids, nil
}
