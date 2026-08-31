package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
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

// ReferenceBody is somewhere an issue is written up, or fixed.
type ReferenceBody struct {
	URL  string `json:"url"`
	Kind string `json:"kind" enum:"patch,advisory,report,other" doc:"What it appears to be. A patch is the change itself"`
}

// SittingBody is one place a component occupies in this build.
type SittingBody struct {
	Place      string `json:"place" doc:"Name this when recording a decision about it"`
	Consumer   string `json:"consumer,omitempty" doc:"What pulls the component in here. Absent under the product itself"`
	Suppressed bool   `json:"suppressed,omitempty" doc:"The build has already argued this place away"`
}

// EvidenceBody is everything held about one issue in one component.
type EvidenceBody struct {
	Vulnerability string   `json:"vulnerability"`
	Aliases       []string `json:"aliases,omitempty" doc:"Other names the same issue is known by"`
	Severity      string   `json:"severity,omitempty" doc:"As the data rates it. A word"`
	Score         float64  `json:"score,omitempty" doc:"The same judgment as a number, where one is published"`
	Vector        string   `json:"vector,omitempty" doc:"What the score assumes — reachability, privilege, interaction"`
	Exploited     bool     `json:"exploited,omitempty" doc:"Somebody is known to be exploiting this"`
	Likelihood    float64  `json:"likelihood,omitempty" doc:"Published probability of exploitation, 0 to 1"`
	Weaknesses    []string `json:"weaknesses,omitempty" doc:"What kind of flaw this is, as CWE identifiers"`
	Description   string   `json:"description,omitempty"`
	Advisory      string   `json:"advisory,omitempty" doc:"Where the issue is written up"`
	// References carries patches first, because for somebody deciding whether
	// to backport rather than upgrade, the change itself is the answer.
	References []ReferenceBody `json:"references,omitempty"`

	Component string `json:"component"`
	Version   string `json:"version"`
	Upstream  string `json:"upstream,omitempty" doc:"What a fork was made from, where it is one"`
	FixState  string `json:"fix_state,omitempty" enum:"fixed,none,wont-fix"`
	FixedIn   string `json:"fixed_in,omitempty" doc:"The version that resolves it"`
	FixedAt   string `json:"fixed_at,omitempty" doc:"When that version became available"`
	// ArrivedFrom says somebody moved this version and the issue came with it.
	// A different sentence aimed at a different person: whoever did the bump,
	// rather than whoever triages.
	ArrivedFrom string `json:"arrived_from,omitempty" doc:"The version this was bumped from, where the bump did not resolve it"`

	Places []SittingBody `json:"places"`
}

func registerFindingDetail(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "get-finding", Method: http.MethodGet,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/findings/{vulnerability}/components/{component}",
		Summary: "Get everything known about one finding",
		Description: "Returns the full record for one issue in one component of a build: the " +
			"description, the advisory, every reference the data carries with patches listed " +
			"first, the score and what it assumes, whether it is known to be exploited and how " +
			"likely exploitation is, the weakness classification, what upstream has done about " +
			"it, and every place the component sits at here.\n\n" +
			"This is what a triage decision is made from, so it is gathered into one request. " +
			"Each entry in `places` carries the `place` identity to name when recording a " +
			"decision about it.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability" doc:"The issue, by any name it is known under"`
		Component     string `path:"component" doc:"The component's name, as the findings list gives it"`
	}) (*struct{ Body EvidenceBody }, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		names := catalog.NewStore(in.DB.DB)
		named, err := names.LocateVisible(ctx, subject, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, noSuchProduct()
		}
		target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
		if err != nil {
			return nil, nothingScannedThere()
		}

		issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, input.Vulnerability)
		if err != nil {
			return nil, noSuchIssue()
		}
		component, err := graph.NewStore(in.DB.DB).ComponentAt(ctx, target.ID, input.Component)
		if err != nil {
			return nil, noSuchFinding()
		}

		evidence, err := finding.NewStore(in.DB.DB).Detail(ctx, subject, target.ID, issue, component)
		if err != nil {
			return nil, noSuchFinding()
		}
		return &struct{ Body EvidenceBody }{Body: evidenceBody(*evidence)}, nil
	})
}

func evidenceBody(e finding.Evidence) EvidenceBody {
	body := EvidenceBody{
		Vulnerability: e.Vulnerability, Aliases: e.Aliases, Severity: e.Severity,
		Score: float64(e.ScoreCenti) / 100, Vector: e.Vector,
		Exploited: e.Exploited, Likelihood: float64(e.LikelihoodPPM) / 1_000_000,
		Weaknesses: e.Weaknesses, Description: e.Description, Advisory: e.Advisory,
		Component: e.Component, Version: e.Version, Upstream: e.Upstream,
		FixState: string(e.FixState), FixedIn: e.FixedIn, ArrivedFrom: e.ArrivedFrom,
		Places: make([]SittingBody, 0, len(e.Places)),
	}
	if e.FixedAt != nil {
		body.FixedAt = e.FixedAt.Format(time.DateOnly)
	}
	for _, reference := range e.References {
		body.References = append(body.References, ReferenceBody{
			URL: reference.URL, Kind: string(reference.Kind),
		})
	}
	for _, place := range e.Places {
		body.Places = append(body.Places, SittingBody{
			Place: place.PlaceIdentity, Consumer: place.Consumer, Suppressed: place.Suppressed,
		})
	}
	return body
}
