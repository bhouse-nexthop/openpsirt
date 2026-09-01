package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// UnassignedBody is one finding nobody is dealing with.
type UnassignedBody struct {
	Vulnerability string `json:"vulnerability"`
	Severity      string `json:"severity,omitempty"`
	Exploited     bool   `json:"exploited,omitempty"`
	Component     string `json:"component"`
	Version       string `json:"version"`
	Product       string `json:"product"`
	Stream        string `json:"stream"`
	Variant       string `json:"variant"`
	Places        int    `json:"places" doc:"How many places this issue sits at in that build"`
}

// HoldingBody is how much work one person has.
type HoldingBody struct {
	Person  string `json:"person"`
	Open    int    `json:"open" doc:"Open findings assigned to them"`
	Overdue int    `json:"overdue" doc:"How many of those are past their deadline"`
}

func registerAssignment(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "assign-finding", Method: http.MethodPut,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/findings/{vulnerability}/components/{component}/assignment",
		Summary: "Assign a finding to somebody",
		Description: "Records who is dealing with this issue in this component. It covers every " +
			"place the component sits at in this build, because they are the same problem seen " +
			"from several parents.\n\n" +
			"Send `person` as an empty string to hand it back to nobody. Handing back is the " +
			"same operation as giving out, so there is one path rather than two that drift.\n\n" +
			"Findings arriving later under the same component start unassigned.",
		Tags: []string{"Findings"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability"`
		Component     string `path:"component"`
		Body          struct {
			Person string `json:"person" doc:"Their sign-in identity, or empty for nobody"`
		}
	}) (*struct{}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		product, target, issue, component, err := locateFinding(ctx, in, subject,
			input.Product, input.Stream, input.Variant, input.Vulnerability, input.Component)
		if err != nil {
			return nil, err
		}

		// Authorized to hand work around before any name is looked up.
		// Resolving first and refusing after answers "does this person have an
		// account here" for anybody who can merely read the product, which is
		// a directory of the organization for the price of one request.
		if !subject.Holds(access.PublicTriage, product) &&
			!subject.Holds(access.PrivateTriage, product) {
			return nil, noSuchFinding()
		}

		var to *int64
		if input.Body.Person != "" {
			person, err := access.NewStore(in.DB.DB).ByIdentity(ctx, input.Body.Person)
			if err != nil {
				return nil, noSuchPerson()
			}
			to = &person.ID
		}

		if _, err := finding.NewStore(in.DB.DB).Assign(ctx, subject, target, issue, component, to); err != nil {
			return nil, refusedFinding(in, err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-unassigned", Method: http.MethodGet, Path: "/v1/unassigned",
		Summary: "List findings nobody is dealing with",
		Description: "Returns open findings with no assignee, across every product you can see, " +
			"most urgent first.\n\n" +
			"Deliberately not scoped to one product: work falling between people is exactly what " +
			"hides when every screen shows one product and nobody looks at the others.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Limit  int `query:"limit" default:"50" minimum:"1" maximum:"200"`
		Offset int `query:"offset" minimum:"0"`
	}) (*struct {
		Body struct {
			Items []UnassignedBody `json:"items"`
			Total int              `json:"total"`
		}
	}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		rows, total, err := finding.NewStore(in.DB.DB).Unassigned(ctx, subject, input.Limit, input.Offset)
		if err != nil {
			return nil, wentWrong(in.Logger, "what nobody is dealing with could not be read", err)
		}
		out := &struct {
			Body struct {
				Items []UnassignedBody `json:"items"`
				Total int              `json:"total"`
			}
		}{}
		out.Body.Items = make([]UnassignedBody, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, UnassignedBody{
				Vulnerability: row.Vulnerability, Severity: row.Severity, Exploited: row.Exploited,
				Component: row.Component, Version: row.Version,
				Product: row.Product, Stream: row.Stream, Variant: row.Variant,
				Places: row.Places,
			})
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-holdings", Method: http.MethodGet, Path: "/v1/assignments",
		Summary: "List how much each person is dealing with",
		Description: "Returns everyone holding open findings you can see, with how many.\n\n" +
			"The number worth watching is not how many findings exist but how many are waiting " +
			"behind somebody: an idle account holding nothing is harmless, and work stuck behind " +
			"a person who has gone is the problem — nothing tells this software that somebody " +
			"has left.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[HoldingBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		held, err := finding.NewStore(in.DB.DB).HeldBy(ctx, subject)
		if err != nil {
			return nil, wentWrong(in.Logger, "who is holding what could not be read", err)
		}
		ids := make([]int64, 0, len(held))
		for _, h := range held {
			ids = append(ids, h.PersonID)
		}
		names, err := access.NewStore(in.DB.DB).Names(ctx, ids)
		if err != nil {
			return nil, wentWrong(in.Logger, "who is holding what could not be read", err)
		}
		out := &listOutput[HoldingBody]{}
		out.Body.Items = make([]HoldingBody, 0, len(held))
		for _, h := range held {
			out.Body.Items = append(out.Body.Items, HoldingBody{
				Person: names[h.PersonID], Open: h.Open, Overdue: h.Overdue,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "release-assignments", Method: http.MethodPost,
		Path:    "/v1/people/{identity}/assignments/release",
		Summary: "Hand back everything one person is dealing with",
		Description: "Returns all their open findings to the unassigned list. For when somebody " +
			"has left, or their last role is removed.\n\n" +
			"Nothing tells this software that somebody has gone — membership is read at sign-in, " +
			"and a person who has left never signs in again. So this is an action an " +
			"administrator takes rather than something that happens on its own, and until it is " +
			"taken their work is in no list at all: not in the shared one because it is assigned, " +
			"and not in anybody's own because they are not here.\n\n" +
			"Send `to` instead to hand it to a named person rather than to nobody.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, input *struct {
		Identity string `path:"identity"`
		Body     struct {
			To string `json:"to,omitempty" doc:"Who takes it on. Omit to return it to nobody"`
		}
	}) (*struct {
		Body struct {
			Moved int64 `json:"moved"`
		}
	}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		// Authorized before the name is looked up. Resolving first and
		// refusing after answers "does this person have an account here" for
		// anybody signed in: a name nobody holds and a name somebody holds
		// come back differently, which is a directory of the organization
		// readable by every account.
		if !subject.Admin {
			return nil, huma.Error403Forbidden("not authorized")
		}
		rights := access.NewStore(in.DB.DB)
		from, err := rights.ByIdentity(ctx, input.Identity)
		if err != nil {
			return nil, noSuchPerson()
		}

		findings := finding.NewStore(in.DB.DB)
		var moved int64
		if input.Body.To == "" {
			moved, err = findings.Release(ctx, subject, from.ID)
		} else {
			var to *access.Account
			if to, err = rights.ByIdentity(ctx, input.Body.To); err != nil {
				return nil, noSuchPerson()
			} else {
				moved, err = findings.HandOver(ctx, subject, from.ID, to.ID)
			}
		}
		if err != nil {
			return nil, refusedFinding(in, err)
		}
		out := &struct {
			Body struct {
				Moved int64 `json:"moved"`
			}
		}{}
		out.Body.Moved = moved
		return out, nil
	})
}

// locateFinding resolves the names in a path to the finding they address.
func locateFinding(ctx context.Context, in Ingest, subject access.Subject,
	product, stream, variant, vulnerability, component string) (int64, int64, int64, int64, error) {

	names := catalog.NewStore(in.DB.DB)
	named, err := names.LocateVisible(ctx, subject, product, stream, variant)
	if err != nil {
		return 0, 0, 0, 0, noSuchProduct()
	}
	target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
	if err != nil {
		return 0, 0, 0, 0, nothingScannedThere()
	}
	issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, vulnerability)
	if err != nil {
		return 0, 0, 0, 0, noSuchIssue()
	}
	held, err := graph.NewStore(in.DB.DB).ComponentAt(ctx, target.ID, component)
	if err != nil {
		return 0, 0, 0, 0, noSuchFinding()
	}
	return named.ProductID, target.ID, issue, held, nil
}

// refusedFinding turns a store's refusal about a finding into an answer.
//
// Somebody who may not reach a finding is told it is not there, the same
// answer a name nobody ever used gets — otherwise the two differ and guessing
// becomes informative.
//
// Only the refusals it recognizes are reported to the caller. Anything else is
// a database that could not answer, and returning those as 422 told somebody
// their request was wrong and put a driver's error text — table names,
// statement fragments — in the response body. What is not a recognized refusal
// is logged and answered as ours.
func refusedFinding(in Ingest, err error) error {
	switch {
	case errors.Is(err, access.ErrDenied):
		return noSuchFinding()
	case errors.Is(err, finding.ErrSamePerson):
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return wentWrong(in.Logger, "that could not be recorded", err)
}
