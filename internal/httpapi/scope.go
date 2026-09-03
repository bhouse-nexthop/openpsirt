package httpapi

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

// ScopeQuery is what the picker selected, as the screens that span products
// receive it (UIX-38).
//
// Every level is optional and empty means all of them, which is what an
// unselected level means. Embedded rather than repeated so the three screens
// that take it cannot drift into describing it differently.
type ScopeQuery struct {
	Product string `query:"product" doc:"Limit to one product, by name. Empty means every product you can see"`
	Stream  string `query:"stream" doc:"Limit to one branch or tag. Only meaningful with a product"`
	Variant string `query:"variant" doc:"Limit to one variant. Only meaningful with a product, and independent of the branch"`
}

// scoped resolves a selection into identifiers.
//
// Names are turned into identifiers through the catalog, which owns the rule
// for matching one — a second copy of that rule here is how two screens come
// to disagree about whether a name is the same name.
//
// A branch or variant without a product is refused rather than guessed at. It
// cannot be reached through the interface, which leaves those unselectable
// until a product is chosen, and guessing which product was meant is how a
// number quietly answers a different question from the one on screen.
func scoped(ctx context.Context, in Ingest, subject access.Subject, q ScopeQuery) (finding.Scope, error) {
	var scope finding.Scope
	if q.Product == "" {
		if q.Stream != "" || q.Variant != "" {
			return scope, huma.Error422UnprocessableEntity(
				"a branch or a variant needs a product to belong to — name one, or leave all three empty")
		}
		return scope, nil
	}

	names := catalog.NewStore(in.DB.DB)
	product, err := names.ProductByName(ctx, q.Product)
	if err != nil || !subject.Sees(product.ID) {
		// The same answer either way. A product somebody may not see reads as
		// one that was never declared, so this cannot be used to find out
		// which products exist.
		return scope, noSuchProduct()
	}
	scope.ProductID = &product.ID

	if q.Stream != "" {
		stream, err := names.StreamByName(ctx, product.ID, q.Stream)
		if err != nil {
			return finding.Scope{}, huma.Error404NotFound(
				"that product has no branch or tag by that name")
		}
		scope.StreamID = &stream.ID
	}
	if q.Variant != "" {
		variant, err := names.VariantByName(ctx, product.ID, q.Variant)
		if err != nil {
			return finding.Scope{}, huma.Error404NotFound(
				"that product has no variant by that name")
		}
		scope.VariantID = &variant.ID
	}
	return scope, nil
}
