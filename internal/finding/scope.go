package finding

import "github.com/uptrace/bun"

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
// matched by a rule the catalogue owns and there should not be a second copy
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
