package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// AtComponentBody is one issue open against a component, as something to
// select.
type AtComponentBody struct {
	Vulnerability string `json:"vulnerability"`
	Severity      string `json:"severity,omitempty"`
	Places        int    `json:"places" doc:"How many places in this build it sits at"`
	FixedIn       string `json:"fixed_in,omitempty"`
}

// togetherCap is how many findings one action may claim about.
//
// A limit rather than none, because one action writing an unbounded number of
// rows is a denial of service somebody triggers by accident rather than
// maliciously. Generous, because the case this exists for is a kernel.
const togetherCap = 2000

func registerBulk(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-issues-at-component", Method: http.MethodGet,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/components/{component}/issues",
		Summary: "List the issues open against one component",
		Description: "Returns the distinct issues open against this component in this build, " +
			"most urgent first.\n\n" +
			"The set somebody narrows before claiming something about all of it. `contains` " +
			"matches the text of a report — it is how a candidate is found, never why a claim is " +
			"true, and nothing here knows a kernel from a font library.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Product   string `path:"product"`
		Stream    string `path:"stream"`
		Variant   string `path:"variant"`
		Component string `path:"component"`
		Contains  string `query:"contains" doc:"Match the report's text. A way to narrow, not a judgment"`
		Limit     int    `query:"limit" default:"50" minimum:"1" maximum:"500"`
		Offset    int    `query:"offset" minimum:"0"`
	}) (*struct {
		Body struct {
			Items []AtComponentBody `json:"items"`
			Total int               `json:"total"`
		}
	}, error) {
		subject, _, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		target, err := browsing(ctx, in, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, err
		}
		component, err := graph.NewStore(in.DB.DB).ComponentAt(ctx, target, input.Component)
		if err != nil {
			return nil, noSuchFinding()
		}

		at, total, err := finding.NewStore(in.DB.DB).AtComponent(ctx, subject, target, component,
			input.Contains, input.Limit, input.Offset)
		if err != nil {
			return nil, refusedFinding(in, err)
		}

		issues := make([]int64, 0, len(at))
		for _, each := range at {
			issues = append(issues, each.VulnerabilityID)
		}
		named, err := finding.NewVulnerabilities(in.DB.DB).NamesByID(ctx, issues)
		if err != nil {
			return nil, wentWrong(in.Logger, "what these issues are called could not be read", err)
		}

		out := &struct {
			Body struct {
				Items []AtComponentBody `json:"items"`
				Total int               `json:"total"`
			}
		}{}
		out.Body.Items = make([]AtComponentBody, 0, len(at))
		for _, each := range at {
			out.Body.Items = append(out.Body.Items, AtComponentBody{
				Vulnerability: named[each.VulnerabilityID], Places: each.Places,
			})
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "decide-together", Method: http.MethodPost,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/components/{component}/decisions",
		Summary: "Record one judgment about several issues at once",
		Description: "Records the same claim against every issue you name: one outcome, one " +
			"justification, one reasoning, and a **separate decision per issue**, each keyed and " +
			"expiring on its own.\n\n" +
			"That separateness is what makes one action across many findings defensible. It is " +
			"not a blanket claim: a decision lapses when the code it was about moves, and these " +
			"lapse one at a time as they should.\n\n" +
			"You name the issues. Nothing here selects them for you, and how you narrowed the " +
			"list is not the claim — the reasoning has to hold for every issue in it, since " +
			"\"these matched a word\" is not a defence anybody would accept.\n\n" +
			"Bounded: one action may cover at most 2000 findings, because an unbounded write is " +
			"something somebody triggers by accident.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		Product   string `path:"product"`
		Stream    string `path:"stream"`
		Variant   string `path:"variant"`
		Component string `path:"component"`
		Body      struct {
			Vulnerabilities []string `json:"vulnerabilities" minItems:"1" doc:"The issues this claim covers, by name"`
			Outcome         string   `json:"outcome" enum:"affected,not-applicable,deferred,wont-fix"`
			Justification   string   `json:"justification,omitempty" doc:"Required when it does not apply"`
			Reasoning       string   `json:"reasoning" minLength:"1" doc:"Why this holds for every issue named"`
		}
	}) (*struct {
		Body struct {
			Recorded int     `json:"recorded"`
			IDs      []int64 `json:"ids"`
		}
	}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		target, err := browsing(ctx, in, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, err
		}
		component, err := graph.NewStore(in.DB.DB).ComponentAt(ctx, target, input.Component)
		if err != nil {
			return nil, noSuchFinding()
		}

		// Every named issue is resolved to the place it actually occupies,
		// rather than taken as stated. A caller free to name a place would be
		// choosing which decisions apply where.
		issues := finding.NewVulnerabilities(in.DB.DB)
		findings := finding.NewStore(in.DB.DB)
		places := make([]triage.Place, 0, len(input.Body.Vulnerabilities))
		for _, name := range input.Body.Vulnerabilities {
			id, err := issues.ByName(ctx, name)
			if err != nil {
				return nil, noSuchIssue()
			}
			at, _, err := findings.AtComponent(ctx, subject, target, component, "", 500, 0)
			if err != nil {
				return nil, refusedFinding(in, err)
			}
			found := false
			for _, each := range at {
				if each.VulnerabilityID != id {
					continue
				}
				places = append(places, triage.Place{
					ProductID: each.ProductID, VulnerabilityID: each.VulnerabilityID,
					PlaceIdentity: each.PlaceIdentity, Visibility: each.Visibility,
					ComponentUpstream: each.ComponentUpstream,
					ConsumerUpstream:  each.ConsumerUpstream,
				})
				found = true
				break
			}
			if !found {
				return nil, noSuchFinding()
			}
		}

		recorded, err := store.Together(ctx, subject, places, triage.Proposal{
			Outcome:       triage.Outcome(input.Body.Outcome),
			Justification: triage.Justification(input.Body.Justification),
			Reasoning:     input.Body.Reasoning,
			By:            subject.ID,
			NeedsApproval: true,
		}, togetherCap)
		if err != nil {
			return nil, refusedDecision(err)
		}

		out := &struct {
			Body struct {
				Recorded int     `json:"recorded"`
				IDs      []int64 `json:"ids"`
			}
		}{}
		out.Body.Recorded = len(recorded)
		out.Body.IDs = recorded
		return out, nil
	})
}
