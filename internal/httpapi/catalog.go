package httpapi

import (
	"context"
	"errors"
	"net/http"

	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
)

// Declaring is what the catalog endpoints need.
//
// Everything a scan is filed against is declared before it can be targeted, so
// this has to be reachable from whatever cuts a branch — a step that can only
// be done by hand is the step every pipeline works around.
type Declaring struct {
	Store  func() *catalog.Store
	Logger *slog.Logger
	// Findings and Scans answer what is open against a catalogue entry and
	// when it was last scanned. A list of names alone makes somebody open
	// every row to find out whether there is anything behind it, which is the
	// question the list exists to answer.
	Findings func() *finding.Store
	Scans    func() *ingest.Store
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
	// What the product holds, so a catalogue answers what exists rather than
	// making somebody open each row to find out. Counts of what is open are
	// issues at components, the way the findings list counts, so the two
	// agree; a declaration returns them as zero because it has just been made.
	Branches int `json:"branches,omitempty" doc:"How many branches are declared"`
	Tags     int `json:"tags,omitempty" doc:"How many tags are declared"`
	Variants int `json:"variants,omitempty" doc:"How many variants are declared"`
	Open     int `json:"open,omitempty" doc:"Issues open against it, counted at components rather than at every place they sit"`
	// LastScanAt is absent where nothing has ever been filed against any of
	// this product's builds.
	LastScanAt string `json:"last_scan_at,omitempty" doc:"When a scan last arrived for any of its builds"`
}

// StreamBody is a branch or a tag.
type StreamBody struct {
	Name string `json:"name" minLength:"1" maxLength:"191" doc:"How scans name this branch or tag"`
	Kind string `json:"kind" enum:"branch,tag" doc:"Whether this line moves. A branch is rebuilt; a tag never changes"`
	// Parent is the branch a tag was cut from, which is what lets a branch be
	// compared against its last release.
	Parent string `json:"parent,omitempty" doc:"For a tag, the branch it was cut from"`
	// Open and LastScanAt, for the same reason the product list carries them:
	// a line that has stopped being built looks identical to a healthy one
	// until somebody opens it.
	Open       int    `json:"open,omitempty" doc:"Issues open against it, counted at components rather than at every place they sit"`
	LastScanAt string `json:"last_scan_at,omitempty" doc:"When a scan last arrived for any build of it"`
}

// VariantBody is one of the ways a stream is built.
type VariantBody struct {
	Name string `json:"name" minLength:"1" maxLength:"191" doc:"How scans name this build of the stream"`
	// CustomerFacing is a pointer so that leaving it out is not the same as
	// saying no. An unclassified artifact should rank as though it ships,
	// which means the default is yes and silence must not read as a denial.
	CustomerFacing *bool `json:"customer_facing,omitempty" doc:"Whether this reaches customers. Defaults to yes"`
	Open           int   `json:"open,omitempty" doc:"Issues open against it here, counted at components rather than at every place they sit"`
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
			Name: stream.DisplayName, Kind: string(stream.Kind), Parent: in.Body.Parent,
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
		return answer(created, VariantBody{Name: variant.DisplayName, CustomerFacing: &variant.CustomerFacing}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-products", Method: http.MethodGet, Path: "/v1/products",
		Summary: "List products",
		Description: "Lists the products declared here, with what each one holds.\n\n" +
			"A scan may only be filed against something declared, so this is the first question " +
			"to ask after an upload is refused for naming something unknown.",
		Tags: []string{"Catalog"},
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

		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		shapes, err := store.Shapes(ctx, subject, ids)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot count what the products hold", err)
		}
		open, err := d.Findings().OpenBy(ctx, subject, finding.Scope{}, finding.ByProduct)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot count what is open", err)
		}
		// When each product was last scanned comes from the same answer the
		// home page reads, rather than a second query that could disagree
		// with it about which build counts.
		seen, err := lastScans(ctx, d.Scans(), subject)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot read when these were last scanned", err)
		}

		out := &listOutput[ProductBody]{}
		out.Body.Items = make([]ProductBody, 0, len(rows))
		for _, row := range rows {
			shape := shapes[row.ID]
			out.Body.Items = append(out.Body.Items, ProductBody{
				Name: row.Name, DisplayName: row.DisplayName,
				Branches: shape.Branches, Tags: shape.Tags, Variants: shape.Variants,
				Open:       open[row.ID],
				LastScanAt: seen[row.Name],
			})
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
		open, err := d.Findings().OpenBy(ctx, subject, finding.Scope{ProductID: &product.ID}, finding.ByStream)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot count what is open", err)
		}
		seen, err := lastScansIn(ctx, d.Scans(), subject, product.ID, func(c ingest.Coverage) string {
			return c.Stream
		})
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot read when these were last scanned", err)
		}

		out := &listOutput[StreamBody]{}
		out.Body.Items = make([]StreamBody, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, StreamBody{
				Name: row.Name, Kind: string(row.Kind),
				Open: open[row.ID], LastScanAt: seen[row.Name],
			})
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
		open, err := d.Findings().OpenBy(ctx, subject, finding.Scope{ProductID: &product.ID}, finding.ByVariant)
		if err != nil {
			return nil, wentWrong(d.Logger, "cannot count what is open", err)
		}
		list := variantList(rows)
		for i := range list.Body.Items {
			list.Body.Items[i].Open = open[rows[i].ID]
		}
		return list, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-release-variants", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants",
		Summary: "List the variants a release was built as",
		Description: "Lists the variants one release was actually built as — a subset of what " +
			"the product declares.\n\n" +
			"A release predating a variant has never been filed against it and does not list " +
			"it, which is what keeps something introduced later from appearing to have shipped " +
			"years ago.",
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

// lastScans is when a scan last arrived for each product, by product name.
//
// It reads the same answer the front page reads rather than asking a question
// of its own: two queries about when something was last scanned are two
// numbers that can disagree, and the one on the catalogue would be the one
// nobody checks.
func lastScans(ctx context.Context, scans *ingest.Store, subject access.Subject) (map[string]string, error) {
	rows, err := scans.Scanning(ctx, subject, finding.Scope{}, 0)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.LastReceivedAt == nil {
			continue
		}
		at := stamp(*row.LastReceivedAt)
		// The newest across a product's builds. Rows arrive quietest first,
		// so the last one to win is the most recent.
		if at > seen[row.Product] {
			seen[row.Product] = at
		}
	}
	return seen, nil
}

// lastScansIn is the same within one product, keyed by whichever level the
// caller is listing.
// Narrowed in the query rather than filtered afterwards, so a deployment with
// many products does not read every build to answer about one.
func lastScansIn(ctx context.Context, scans *ingest.Store, subject access.Subject, productID int64,
	key func(ingest.Coverage) string) (map[string]string, error) {

	rows, err := scans.Scanning(ctx, subject, finding.Scope{ProductID: &productID}, 0)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.LastReceivedAt == nil {
			continue
		}
		if at := stamp(*row.LastReceivedAt); at > seen[key(row)] {
			seen[key(row)] = at
		}
	}
	return seen, nil
}
