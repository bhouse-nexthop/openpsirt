package httpapi

import (
	"context"
	"errors"
	"net/http"

	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
)

// Declaring is what the catalog endpoints need.
//
// Everything a scan is filed against is declared before it can be targeted, so
// this has to be reachable from whatever cuts a branch — a step that can only
// be done by hand is the step every pipeline works around.
type Declaring struct {
	Store  func() *catalog.Store
	Logger *slog.Logger
}

// reading is who a handler is answering, where the answer is something to
// read.
//
// A pipeline is refused rather than shown an empty list. A key may send scans
// and nothing else, and answering "here is nothing" is a different statement
// from "you cannot ask" — the first invites a caller to believe the list is
// empty.
func reading(ctx context.Context) (access.Subject, error) {
	subject, err := requester(ctx)
	if err != nil {
		return access.Subject{}, err
	}
	if subject.Kind != access.Person {
		return access.Subject{}, huma.Error403Forbidden("not authorized")
	}
	return subject, nil
}

// requester is who a handler is answering, resolved once before it ran.
//
// Nothing here reaches for a request or a header: resolution happens in one
// place for every route, so a handler cannot answer for everybody by
// forgetting to ask.
func requester(ctx context.Context) (access.Subject, error) {
	subject, err := access.From(ctx)
	if err != nil {
		// Nobody is attached, which for a route behind the resolver means
		// nobody was recognized. The same answer whoever they are.
		return access.Subject{}, huma.Error401Unauthorized("not authorized")
	}
	return subject, nil
}

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
		Summary: "Create a product",
		Description: "Records a product so scans may be filed against it. Declaring one that " +
			"already exists succeeds without changing anything, so this can run on every build.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body ProductBody
	}) (*declaredOutput[ProductBody], error) {
		if err := administrating(ctx); err != nil {
			return nil, err
		}
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
		Summary: "Create a branch or tag",
		Description: "Records a line of a product. A branch moves and is rebuilt; a tag never " +
			"changes and is what somebody received.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Body    StreamBody
	}) (*declaredOutput[StreamBody], error) {
		if err := administrating(ctx); err != nil {
			return nil, err
		}
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
		Path:    "/v1/products/{product}/variants",
		Summary: "Create a build variant",
		Description: "Records one of the parallel builds of a product — a chip variant, an " +
			"architecture, an operating system. Declared once for the product, not once per " +
			"release: a release is filed against it the first time a scan arrives, so nobody " +
			"restates the list and no release ends up with the name spelled differently.",
		Tags: []string{"Catalog"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Body    VariantBody
	}) (*declaredOutput[VariantBody], error) {
		if err := administrating(ctx); err != nil {
			return nil, err
		}
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, err := store.ProductByName(ctx, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}

		facing := true
		if in.Body.CustomerFacing != nil {
			facing = *in.Body.CustomerFacing
		}
		variant, created, err := store.EnsureVariant(ctx, product.ID, in.Body.Name, facing)
		if err != nil {
			return nil, declineDeclaration(err)
		}
		return answer(created, VariantBody{Name: variant.Name, CustomerFacing: &variant.CustomerFacing}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-products", Method: http.MethodGet, Path: "/v1/products",
		Summary:     "List products",
		Description: "What a scan may be filed against. The first question after an upload is refused.",
		Tags:        []string{"Catalog"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[ProductBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		rows, err := store.Products(ctx, subject)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot list products", err)
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
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, err := store.VisibleProduct(ctx, subject, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		rows, err := store.Streams(ctx, subject, product.ID)
		if err != nil {
			return nil, refused(d.Logger, err, "cannot list streams")
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
		Path:    "/v1/products/{product}/variants",
		Summary: "List a product's build variants", Tags: []string{"Catalog"},
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
	}) (*listOutput[VariantBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		product, err := store.VisibleProduct(ctx, subject, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		rows, err := store.Variants(ctx, subject, product.ID)
		if err != nil {
			return nil, refused(d.Logger, err, "cannot list variants")
		}
		return variantList(rows), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-release-variants", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants",
		Summary: "List the variants a release was built as",
		Description: "A subset of what the product builds. A release predating a variant has " +
			"never been filed against it, which is what keeps something introduced later from " +
			"appearing to have shipped years ago.",
		Tags: []string{"Catalog"},
	}, func(ctx context.Context, in *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
	}) (*listOutput[VariantBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		store, err := storeFor(d)
		if err != nil {
			return nil, err
		}
		_, stream, err := store.VisibleStream(ctx, subject, in.Product, in.Stream)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		rows, err := store.BuiltAs(ctx, subject, stream.ID)
		if err != nil {
			return nil, refused(d.Logger, err, "cannot list what a release is built as")
		}
		return variantList(rows), nil
	})
}

// variantList renders variants for a response.
func variantList(rows []catalog.Variant) *listOutput[VariantBody] {
	out := &listOutput[VariantBody]{}
	out.Body.Items = make([]VariantBody, 0, len(rows))
	for _, row := range rows {
		facing := row.CustomerFacing
		out.Body.Items = append(out.Body.Items, VariantBody{Name: row.Name, CustomerFacing: &facing})
	}
	return out
}

// refused turns a refusal from the data layer into one the caller sees as a
// refusal. Anything else is a fault here rather than a decision about them.
func refused(logger *slog.Logger, err error, what string) error {
	if errors.Is(err, access.ErrDenied) {
		return huma.Error403Forbidden("not authorized")
	}
	return wentWrong(logger, what, err)
}

// administrating refuses anybody who is not an administrator.
//
// Declaring what exists is administration: it decides what scans may be filed
// against and what every later grant is written in terms of. A pipeline cannot
// do it at all, and neither can somebody who merely reads — a role on a
// product says what they may see in it, not that they may invent another.
func administrating(ctx context.Context) error {
	subject, err := requester(ctx)
	if err != nil {
		return err
	}
	if !subject.Admin {
		return huma.Error403Forbidden("not authorized")
	}
	return nil
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
