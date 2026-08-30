package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

// FindingBody is one issue in one component, with the places it occupies.
type FindingBody struct {
	Vulnerability string `json:"vulnerability" doc:"The issue, under the name it is most widely known by"`
	Severity      string `json:"severity,omitempty" doc:"As the scanner rated it. A word, not a score"`
	Component     string `json:"component" doc:"What carries it"`
	Version       string `json:"version" doc:"The version that ships"`
	Upstream      string `json:"upstream,omitempty" doc:"What a fork was made from, where it is one"`
	FixState      string `json:"fix_state,omitempty" enum:"fixed,none,wont-fix" doc:"What upstream has done about it"`
	FixedIn       string `json:"fixed_in,omitempty" doc:"The version that resolves it, where one exists"`
	// Places is how many consumers pull this component in here, and Answered
	// how many of those the build has already argued about.
	Places   int `json:"places" doc:"How many places this component sits at here"`
	Answered int `json:"answered,omitempty" doc:"How many of those the build has already argued do not apply"`
	// Exploited is why something is at the top when it is. A position nobody
	// can explain is one people stop trusting, and then they sort by something
	// else and lose the point of the order entirely.
	Exploited bool `json:"exploited,omitempty" doc:"Somebody is known to be exploiting this"`
}

// FindingsOutput is a page of what is open.
type FindingsOutput struct {
	Body struct {
		Items []FindingBody `json:"items"`
		// Total is how many things there are to decide about, which is not the
		// number of findings: one issue in one component can occupy sixty
		// places and is one decision.
		Total int `json:"total"`
	}
}

func registerFindings(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-findings", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings",
		Summary: "List vulnerability findings",
		Description: "Returns one row per vulnerability-and-component pair, not one row per " +
			"place the component appears. Each row gives the number of places it occupies and " +
			"how many of those the build's VEX documents already answer.\n\n" +
			"Grouping matters at real scale: one switch image produced 335,021 individual " +
			"findings, which collapse to 7,906 rows here.\n\n" +
			"Ordered by urgency — known-exploited first, then whether the build ships to " +
			"customers, then likelihood, then severity. Supports limit and offset.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
		Variant string `path:"variant"`
		Limit   int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"How many to return"`
		Offset  int    `query:"offset" minimum:"0" doc:"How many to skip"`
	}) (*FindingsOutput, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot read findings")
		}

		// Resolved and authorized together, so a build somebody may not see
		// reads as one that was never declared.
		names := catalog.NewStore(in.DB.DB)
		named, err := names.LocateVisible(ctx, subject, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
		if err != nil {
			// Declared, but nothing was ever filed against it. Nothing is open
			// because nothing has been scanned.
			out := &FindingsOutput{}
			out.Body.Items = []FindingBody{}
			return out, nil
		}

		groups, total, err := finding.NewStore(in.DB.DB).Groups(ctx, subject, target.ID, input.Limit, input.Offset)
		if err != nil {
			return nil, refused(in.Logger, err, "cannot read what is open")
		}

		out := &FindingsOutput{}
		out.Body.Total = total
		out.Body.Items = make([]FindingBody, 0, len(groups))
		for _, group := range groups {
			out.Body.Items = append(out.Body.Items, FindingBody{
				Vulnerability: group.Vulnerability, Severity: group.Severity,
				Component: group.Component, Version: group.Version, Upstream: group.Upstream,
				FixState: string(group.FixState), FixedIn: group.FixedIn,
				Places: group.Places, Answered: group.Answered,
				Exploited: group.Exploited,
			})
		}
		return out, nil
	})
}
