package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
)

// Declaring is what the catalog endpoints need.
//
// Everything a scan is filed against is declared before it can be targeted, so
// this has to be reachable from whatever cuts a branch — a step that can only
// be done by hand is the step every pipeline works around.
type Declaring struct{ Store func() *catalog.Store }

// ProductBody is a product as the API states it.
type ProductBody struct {
	Name        string `json:"name" minLength:"1" maxLength:"191" doc:"How scans name this product"`
	DisplayName string `json:"display_name,omitempty" doc:"What people see. Defaults to the name"`
}

// StreamBody is a branch or a tag.
type StreamBody struct {
	Name string `json:"name" minLength:"1" maxLength:"191" doc:"How scans name this branch or tag"`
	Kind string `json:"kind" enum:"branch,tag" doc:"Whether this line moves. A branch is rebuilt; a tag never changes"`
	// Parent is the branch a tag was cut from, which is what lets a branch be
	// compared against its last release.
	Parent string `json:"parent,omitempty" doc:"For a tag, the branch it was cut from"`
}

// VariantBody is one of the ways a stream is built.
type VariantBody struct {
	Name string `json:"name" minLength:"1" maxLength:"191" doc:"How scans name this build of the stream"`
	// CustomerFacing is a pointer so that leaving it out is not the same as
	// saying no. An unclassified artifact should rank as though it ships,
	// which means the default is yes and silence must not read as a denial.
	CustomerFacing *bool `json:"customer_facing,omitempty" doc:"Whether this reaches customers. Defaults to yes"`
}

// declaredOutput reports what a declaration did.
type declaredOutput[T any] struct {
	Status int
	Body   declared[T]
}

type declared[T any] struct {
	// Created says whether this declaration made something. Declaring the same
	// thing twice succeeds, so a caller that needs to know reads this.
	Created bool `json:"created"`
	Item    T    `json:"item"`
}

type listOutput[T any] struct {
	Body listBody[T]
}

type listBody[T any] struct {
	Items []T `json:"items"`
}

func registerCatalog(api huma.API, d Declaring) {
	huma.Register(api, huma.Operation{
		OperationID: "declare-product", Method: http.MethodPost, Path: "/v1/products",
		Summary: "Declare a product",
		Description: "Records a product so scans may be filed against it. Declaring one that " +
			"already exists succeeds without changing anything, so this can run on every build.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body ProductBody
	}) (*declaredOutput[ProductBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, created, err := store.EnsureProduct(ctx, in.Body.Name, in.Body.DisplayName)
		if err != nil {
			return nil, declineDeclaration(err)
		}
		return answer(created, ProductBody{Name: product.Name, DisplayName: product.DisplayName}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "declare-stream", Method: http.MethodPost, Path: "/v1/products/{product}/streams",
		Summary: "Declare a branch or a tag",
		Description: "Records a line of a product. A branch moves and is rebuilt; a tag never " +
			"changes and is what somebody received.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Body    StreamBody
	}) (*declaredOutput[StreamBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, err := store.ProductByName(ctx, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}

		var parentID *int64
		if in.Body.Parent != "" {
			parent, err := store.StreamByName(ctx, product.ID, in.Body.Parent)
			if err != nil {
				return nil, huma.Error404NotFound(err.Error())
			}
			parentID = &parent.ID
		}

		stream, created, err := store.EnsureStream(ctx, product.ID, in.Body.Name,
			catalog.Kind(in.Body.Kind), parentID)
		if err != nil {
			return nil, declineDeclaration(err)
		}
		return answer(created, StreamBody{
			Name: stream.Name, Kind: string(stream.Kind), Parent: in.Body.Parent,
		}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "declare-variant", Method: http.MethodPost,
		Path:    "/v1/products/{product}/streams/{stream}/variants",
		Summary: "Declare a way a stream is built",
		Description: "Records one of the parallel builds of a stream — a chip variant, an " +
			"architecture, a platform. A variant belongs to its stream, so one introduced in a " +
			"later release does not appear to have existed in earlier ones.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
		Body    VariantBody
	}) (*declaredOutput[VariantBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		stream, err := findStream(ctx, store, in.Product, in.Stream)
		if err != nil {
			return nil, err
		}

		facing := true
		if in.Body.CustomerFacing != nil {
			facing = *in.Body.CustomerFacing
		}
		variant, created, err := store.EnsureVariant(ctx, stream.ID, in.Body.Name, facing)
		if err != nil {
			return nil, declineDeclaration(err)
		}
		return answer(created, VariantBody{Name: variant.Name, CustomerFacing: &variant.CustomerFacing}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-products", Method: http.MethodGet, Path: "/v1/products",
		Summary:     "List declared products",
		Description: "What a scan may be filed against. The first question after an upload is refused.",
		Tags:        []string{"Catalog"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[ProductBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		rows, err := store.Products(ctx)
		if err != nil {
			return nil, huma.Error500InternalServerError("cannot list products", err)
		}
		out := &listOutput[ProductBody]{}
		out.Body.Items = make([]ProductBody, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items,
				ProductBody{Name: row.Name, DisplayName: row.DisplayName})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-streams", Method: http.MethodGet, Path: "/v1/products/{product}/streams",
		Summary: "List a product's branches and tags", Tags: []string{"Catalog"},
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
	}) (*listOutput[StreamBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, err := store.ProductByName(ctx, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		rows, err := store.Streams(ctx, product.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("cannot list streams", err)
		}
		out := &listOutput[StreamBody]{}
		out.Body.Items = make([]StreamBody, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items,
				StreamBody{Name: row.Name, Kind: string(row.Kind)})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-variants", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants",
		Summary: "List the ways a stream is built", Tags: []string{"Catalog"},
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
	}) (*listOutput[VariantBody], error) {
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		stream, err := findStream(ctx, store, in.Product, in.Stream)
		if err != nil {
			return nil, err
		}
		rows, err := store.Variants(ctx, stream.ID)
		if err != nil {
			return nil, huma.Error500InternalServerError("cannot list variants", err)
		}
		out := &listOutput[VariantBody]{}
		out.Body.Items = make([]VariantBody, 0, len(rows))
		for _, row := range rows {
			facing := row.CustomerFacing
			out.Body.Items = append(out.Body.Items, VariantBody{Name: row.Name, CustomerFacing: &facing})
		}
		return out, nil
	})
}

// findStream resolves a product and stream, saying which of the two is missing.
func findStream(ctx context.Context, store *catalog.Store, product, stream string) (*catalog.Stream, error) {
	p, err := store.ProductByName(ctx, product)
	if err != nil {
		return nil, huma.Error404NotFound(err.Error())
	}
	st, err := store.StreamByName(ctx, p.ID, stream)
	if err != nil {
		return nil, huma.Error404NotFound(err.Error())
	}
	return st, nil
}

// answer reports a declaration, distinguishing one that made something from
// one that found it already there.
func answer[T any](created bool, item T) *declaredOutput[T] {
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &declaredOutput[T]{Status: status, Body: declared[T]{Created: created, Item: item}}
}

// declineDeclaration turns a refusal into the answer that describes it.
func declineDeclaration(err error) error {
	switch {
	case errors.Is(err, catalog.ErrDiffers):
		// Declared before, meaning something else. Answering with success
		// would let a pipeline quietly redefine what a name refers to.
		return huma.NewError(http.StatusConflict, err.Error())
	case errors.Is(err, catalog.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	default:
		return huma.Error400BadRequest(err.Error())
	}
}

// storeFor gives the handlers a catalog, or says plainly that this process
// has none.
func storeFor(d Declaring) (*catalog.Store, error) {
	if d.Store == nil {
		return nil, huma.Error500InternalServerError("this process cannot declare anything")
	}
	store := d.Store()
	if store == nil {
		return nil, huma.Error500InternalServerError("this process cannot declare anything")
	}
	return store, nil
}
