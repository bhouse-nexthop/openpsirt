package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/advisory"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
)

func registerAdvisory(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "get-advisory", Method: http.MethodGet,
		Path:    "/v1/products/{product}/issues/{vulnerability}/advisory",
		Summary: "Generate a CSAF advisory for an issue",
		Description: "Returns a CSAF 2.0 document for a flaw in this product: what it is, and " +
			"which releases hold it and which no longer do.\n\n" +
			"**The document is generated, not published.** Nothing is sent anywhere and " +
			"nothing here records that an advisory was issued — the triage record is ours and " +
			"the published advisory belongs to whoever publishes it, and keeping both as the " +
			"source of truth is how such an arrangement rots.\n\n" +
			"**Only for a flaw in what you ship.** An issue a scanner reported against a " +
			"third-party component is refused: that is dependency hygiene a consumer can " +
			"already read out of the inventory, and a vendor advisory for every upstream CVE " +
			"in a dependency is not what an advisory is.\n\n" +
			"**A document about an undisclosed flaw is a draft**, and says so in `tracking." +
			"status`. Reaching a disclosure date discloses nothing, so nothing here does " +
			"either.\n\n" +
			"Requires a publisher configured for this deployment: a document naming none is " +
			"not a valid CSAF document.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Vulnerability string `path:"vulnerability" doc:"The identifier the issue is filed under"`
	}) (*struct{ Body *advisory.Document }, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot generate advisories")
		}

		doc, err := advisory.NewStore(in.DB.DB).For(ctx, subject, in.Publisher,
			input.Product, input.Vulnerability)
		if err != nil {
			switch {
			case errors.Is(err, advisory.ErrNoPublisher):
				// A configuration gap rather than a bad request, and named as
				// one: whoever is asking cannot fix it from here, and an
				// operator can.
				return nil, huma.Error409Conflict(err.Error())
			case errors.Is(err, advisory.ErrNotOurs):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			case errors.Is(err, advisory.ErrNoSuchIssue), errors.Is(err, catalog.ErrNotFound):
				// The same answer for a product nobody holds and an issue that
				// is not there. Telling them apart turns a lookup into a
				// directory of what exists.
				return nil, noSuchIssue()
			}
			return nil, wentWrong(in.Logger, "the advisory could not be generated", err)
		}
		return &struct{ Body *advisory.Document }{Body: doc}, nil
	})
}
