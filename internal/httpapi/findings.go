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
	// Likelihood is why one medium sits above another, and above a high. It
	// ranks between whether something reaches customers and how severe it is,
	// so a list that orders by it and does not show it reads as unsorted.
	Likelihood float64 `json:"likelihood,omitempty" doc:"Published estimate that this will be exploited, 0 to 1"`
	// Score is what the ordering compares. The word beside it comes from
	// whichever scoring generation rated it — 10.0 reads "high" under CVSS v2
	// and "critical" under v3 — so two rows can tie on the number while their
	// words disagree, and without the number that looks mis-sorted.
	Score float64 `json:"score,omitempty" doc:"The severity as a number, which is what the order compares"`
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

// ComponentFindingBody is one component at one version, with what is open
// against it counted.
type ComponentFindingBody struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Upstream  string `json:"upstream,omitempty" doc:"What a fork was cut from, where one is known"`
	Issues    int    `json:"issues" doc:"Distinct vulnerabilities open against it, which is how many rows it contributes to the findings list"`
	Places    int    `json:"places" doc:"How many times those sit somewhere in the build"`
	Exploited bool   `json:"exploited" doc:"Whether any of them is known-exploited"`
}

// ComponentFindingsOutput is a page of what is open, by component.
type ComponentFindingsOutput struct {
	Body struct {
		Items []ComponentFindingBody `json:"items"`
		Total int                    `json:"total"`
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
			"customers, then likelihood, then severity. Supports limit and offset.\n\n" +
			"Narrowing happens here rather than in the client, and `total` counts what the " +
			"filter admits. A filter applied to a page already fetched answers a different " +
			"question from the one it appears to: `exploited` over fifty rows means exploited " +
			"among those fifty.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product   string   `path:"product"`
		Stream    string   `path:"stream"`
		Variant   string   `path:"variant"`
		Severity  string   `query:"severity" enum:"low,medium,high,critical" doc:"Keep only issues rated this badly or worse. 'low' excludes nothing, including issues carrying no rating"`
		Exploited bool     `query:"exploited" doc:"Keep only issues somebody is known to be exploiting"`
		Fixable   bool     `query:"fixable" doc:"Keep only issues where an upstream fixed version is known"`
		Component string   `query:"component" doc:"Keep only what is open against components of this name, whatever version"`
		Exclude   []string `query:"exclude" doc:"Drop components of these names. One package can drown the list: on a switch image the kernel carried 4,943 of 6,822 rows"`
		Limit     int      `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"How many to return"`
		Offset    int      `query:"offset" minimum:"0" doc:"How many to skip"`
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

		narrowed := finding.Filter{
			MinSeverity: input.Severity,
			Exploited:   input.Exploited,
			HasFix:      input.Fixable,
			Component:   input.Component,
			Exclude:     input.Exclude,
		}
		groups, total, err := finding.NewStore(in.DB.DB).Groups(ctx, subject, target.ID,
			input.Limit, input.Offset, narrowed)
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
				Exploited:  group.Exploited,
				Likelihood: float64(group.LikelihoodPPM) / 1_000_000,
				Score:      float64(group.ScoreCenti) / 100,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-finding-components", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/components",
		Summary: "List what is open, gathered by component",
		Description: "One row per component and version, with how many distinct issues are " +
			"open against it and how many places those sit at. The level above the findings " +
			"list: it answers where the weight is rather than what is wrong, which is the " +
			"question somebody asks before deciding what to read and what to put aside.\n\n" +
			"It is also how a person finds the one package worth hiding. On a switch " +
			"operating-system image the kernel carried 4,943 of 6,822 findings rows and the " +
			"next largest contributor carried 58 — a fact no list of issues makes visible, " +
			"because ordered by urgency it just looks like a long list.\n\n" +
			"Takes the same filters as the findings list, so the two agree about what is " +
			"being counted. Ordered by how many issues, not by urgency: ordering by urgency " +
			"would reproduce the findings list at worse resolution.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product   string   `path:"product"`
		Stream    string   `path:"stream"`
		Variant   string   `path:"variant"`
		Severity  string   `query:"severity" enum:"low,medium,high,critical" doc:"Keep only issues rated this badly or worse. 'low' excludes nothing, including issues carrying no rating"`
		Exploited bool     `query:"exploited" doc:"Keep only issues somebody is known to be exploiting"`
		Fixable   bool     `query:"fixable" doc:"Keep only issues where an upstream fixed version is known"`
		Exclude   []string `query:"exclude" doc:"Drop components of these names"`
		Limit     int      `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"How many to return"`
		Offset    int      `query:"offset" minimum:"0" doc:"How many to skip"`
	}) (*ComponentFindingsOutput, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot read findings")
		}

		names := catalog.NewStore(in.DB.DB)
		named, err := names.LocateVisible(ctx, subject, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
		if err != nil {
			out := &ComponentFindingsOutput{}
			out.Body.Items = []ComponentFindingBody{}
			return out, nil
		}

		narrowed := finding.Filter{
			MinSeverity: input.Severity,
			Exploited:   input.Exploited,
			HasFix:      input.Fixable,
			Exclude:     input.Exclude,
		}
		groups, total, err := finding.NewStore(in.DB.DB).ComponentGroups(ctx, subject, target.ID,
			input.Limit, input.Offset, narrowed)
		if err != nil {
			return nil, refused(in.Logger, err, "cannot read what is open")
		}

		out := &ComponentFindingsOutput{}
		out.Body.Total = total
		out.Body.Items = make([]ComponentFindingBody, 0, len(groups))
		for _, group := range groups {
			out.Body.Items = append(out.Body.Items, ComponentFindingBody{
				Component: group.Component, Version: group.Version, Upstream: group.Upstream,
				Issues: group.Issues, Places: group.Places, Exploited: group.Exploited,
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

// StepBody is one component on the way down to another.
type StepBody struct {
	Component string `json:"component"`
	Version   string `json:"version,omitempty"`
}

// SittingBody is one place a component occupies in this build.
type SittingBody struct {
	Place      string `json:"place" doc:"Name this when recording a decision about it"`
	Consumer   string `json:"consumer,omitempty" doc:"What pulls the component in here. Absent under the product itself"`
	Suppressed bool   `json:"suppressed,omitempty" doc:"The build has already argued this place away"`
	// Chain is display rather than identity. A decision is keyed on the direct
	// consumer and nothing else, which is what keeps one judgment from
	// multiplying by every route through the graph.
	Chain []StepBody `json:"chain,omitempty" doc:"The way down to here, the build first and this component last. Empty where the inventory left the component unplaced"`
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
			"decision about it.\n\n" +
			"**A component name is not unique within a build.** Where one ships at several " +
			"versions, `version` says which — without it, a name that matches more than one is " +
			"refused rather than guessed at.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability" doc:"The issue, by any name it is known under"`
		Component     string `path:"component" doc:"The component's name, as the findings list gives it"`
		Version       string `query:"version" doc:"Which version, where the build ships that name at more than one"`
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
		// Name and version together, because a name alone is not unique: a
		// real image ships three vendored versions of one library, and
		// resolving the name on its own answers about whichever was interned
		// first — for two of the three rows, an issue it does not carry.
		component, err := graph.NewStore(in.DB.DB).
			ComponentVersionAt(ctx, target.ID, input.Component, input.Version)
		if err != nil {
			return nil, ambiguousOrMissing(err)
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
		sitting := SittingBody{
			Place: place.PlaceIdentity, Consumer: place.Consumer, Suppressed: place.Suppressed,
		}
		for _, step := range place.Chain {
			sitting.Chain = append(sitting.Chain, StepBody{
				Component: step.Name, Version: step.Version,
			})
		}
		body.Places = append(body.Places, sitting)
	}
	return body
}
