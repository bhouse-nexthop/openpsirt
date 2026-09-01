package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// PointBody is what was true at one moment.
type PointBody struct {
	At         string         `json:"at" doc:"The end of this step, as a date"`
	Open       int            `json:"open"`
	Opened     int            `json:"opened" doc:"Findings that appeared during this step"`
	Resolved   int            `json:"resolved" doc:"Findings that went away during this step"`
	BySeverity map[string]int `json:"by_severity"`
}

// ChangedBody is one issue that differs between two builds.
type ChangedBody struct {
	Vulnerability string `json:"vulnerability"`
	Component     string `json:"component"`
	Severity      string `json:"severity,omitempty"`
	Because       string `json:"because,omitempty" enum:"removed,upgraded,revised,superseded,unexplained" doc:"Why it went. Only on fixed entries"`
	ArrivedFrom   string `json:"arrived_from,omitempty" doc:"The version this was bumped from since the earlier build. Only on still-present entries, where it means the bump did not reach the fix"`
}

func registerReports(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "get-trend", Method: http.MethodGet, Path: "/v1/trend",
		Summary: "Show new, resolved and open over time",
		Description: "Returns the three counts per step, with open split by severity, across " +
			"every product you can see.\n\n" +
			"Three series rather than one, because separately they are three numbers and " +
			"together they say whether the team is keeping pace: new consistently outrunning " +
			"resolved is a growing backlog.\n\n" +
			"Split by severity because a total that barely moves while its critical share rises " +
			"is getting worse, and a single line hides exactly that.\n\n" +
			"Worked out when it is asked for. Nothing is precomputed or refreshed on a schedule " +
			"until a measurement says it has to be.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		ScopeQuery
		Weeks int `query:"weeks" default:"12" minimum:"1" maximum:"104"`
	}) (*listOutput[PointBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		const week = 7 * 24 * time.Hour
		since := time.Now().UTC().Add(-time.Duration(input.Weeks) * week)

		scope, err := scoped(ctx, in, subject, input.ScopeQuery)
		if err != nil {
			return nil, err
		}
		points, err := finding.NewStore(in.DB.DB).Trend(ctx, subject, scope, since, week, input.Weeks)
		if err != nil {
			return nil, wentWrong(in.Logger, "the trend could not be worked out", err)
		}
		out := &listOutput[PointBody]{}
		out.Body.Items = make([]PointBody, 0, len(points))
		for _, p := range points {
			out.Body.Items = append(out.Body.Items, PointBody{
				At: p.At.Format(time.DateOnly), Open: p.Open,
				Opened: p.Opened, Resolved: p.Resolved, BySeverity: p.BySeverity,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "compare-releases", Method: http.MethodGet,
		Path:    "/v1/products/{product}/comparison",
		Summary: "Compare two builds",
		Description: "Returns what was fixed, what is newly present, and what is still there " +
			"between two builds of one product.\n\n" +
			"Between **any** two, not only adjacent ones: what a release note has to answer is " +
			"usually about the last release a customer has, which is rarely the previous one.\n\n" +
			"Each fixed entry says why it went, because \"fixed by upgrading\" and \"fixed by a " +
			"carried patch\" are different sentences to a reader. `superseded` is the one to " +
			"read carefully — it means the version moved and the issue came with it, so it was " +
			"not fixed at all.\n\n" +
			"A still-present entry carrying `arrived_from` is the same failure seen from the " +
			"other side: somebody moved that version since the earlier build and the issue came " +
			"with it, so the bump did not reach the fix.\n\n" +
			"**Public findings only unless you ask otherwise.** Its destination is usually a " +
			"public document, so including something undisclosed should be deliberate rather " +
			"than something pasted in without noticing.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Product        string `path:"product"`
		From           string `query:"from" required:"true" doc:"The earlier build's stream — a branch or a tag"`
		FromVariant    string `query:"from_variant" required:"true" doc:"The earlier build's variant"`
		To             string `query:"to" required:"true" doc:"The later build's stream"`
		ToVariant      string `query:"to_variant" required:"true" doc:"The later build's variant"`
		IncludePrivate bool   `query:"include_undisclosed" doc:"Include findings nobody has disclosed"`
	}) (*struct {
		Body struct {
			Fixed []ChangedBody `json:"fixed"`
			Newly []ChangedBody `json:"newly_present"`
			Still []ChangedBody `json:"still_present"`
		}
	}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		names := catalog.NewStore(in.DB.DB)
		locate := func(stream, variant string) (int64, error) {
			named, err := names.LocateVisible(ctx, subject, input.Product, stream, variant)
			if err != nil {
				return 0, noSuchProduct()
			}
			target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
			if err != nil {
				return 0, nothingScannedThere()
			}
			return target.ID, nil
		}
		from, err := locate(input.From, input.FromVariant)
		if err != nil {
			return nil, err
		}
		to, err := locate(input.To, input.ToVariant)
		if err != nil {
			return nil, err
		}

		comparison, err := finding.NewStore(in.DB.DB).Compare(ctx, subject, from, to,
			input.IncludePrivate)
		if err != nil {
			return nil, refusedFinding(in, err)
		}

		out := &struct {
			Body struct {
				Fixed []ChangedBody `json:"fixed"`
				Newly []ChangedBody `json:"newly_present"`
				Still []ChangedBody `json:"still_present"`
			}
		}{}
		out.Body.Fixed = changed(comparison.Fixed, true, false)
		out.Body.Newly = changed(comparison.Newly, false, false)
		// Only the still-present column says what a place was bumped from. On
		// a fixed entry the closure already says what happened, and on a new
		// one there was nothing to bump.
		out.Body.Still = changed(comparison.Still, false, true)
		return out, nil
	})
}

func changed(rows []finding.Changed, why, bumped bool) []ChangedBody {
	out := make([]ChangedBody, 0, len(rows))
	for _, row := range rows {
		body := ChangedBody{
			Vulnerability: row.Vulnerability, Component: row.Component, Severity: row.Severity,
		}
		if why {
			body.Because = string(row.Because)
		}
		if bumped {
			body.ArrivedFrom = row.ArrivedFrom
		}
		out = append(out, body)
	}
	return out
}

// InheritedBody is one claim a new line could take on.
type InheritedBody struct {
	Decision      int64  `json:"decision"`
	Vulnerability string `json:"vulnerability"`
	Component     string `json:"component"`
	Outcome       string `json:"outcome"`
	Was           string `json:"was" doc:"The version the claim was made against"`
	Now           string `json:"now" doc:"What the new line has"`
	Reasoning     string `json:"reasoning" doc:"The old words, to start from rather than start without"`
	DeferredDays  int    `json:"deferred_days,omitempty" doc:"How long this has already been put off, across every line it has been carried through"`
}

// CarriedBody is what a new line would inherit.
type CarriedBody struct {
	Applying  int             `json:"applying" doc:"Reach it by matching. Nothing to choose"`
	Moved     []InheritedBody `json:"moved" doc:"The version differs, so each needs a fresh answer"`
	Postponed []InheritedBody `json:"postponed" doc:"Deferrals, offered separately and never carried by default"`
	Absent    int             `json:"absent" doc:"Cover nothing in the new line"`
}

func registerCarry(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "preview-carried-decisions", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/carried",
		Summary: "Show what triage a new line would inherit",
		Description: "Returns what an existing line's decisions would mean for this one, without " +
			"changing anything. Ask before creating a line, because the answer is what somebody " +
			"is agreeing to — a carry that happens silently is one nobody reviews.\n\n" +
			"Four groups, because they need four different things:\n\n" +
			"`applying` reach this line by matching. Nothing to choose; a decision is a claim " +
			"about a combination of code rather than about a release.\n\n" +
			"`moved` held a claim at a version this line does not have. Each would come across " +
			"as a **proposal carrying the old reasoning** — never as a decision, because the " +
			"version moved and the old conclusion is not a conclusion about the new code. " +
			"Making somebody start from a blank page, having thrown away what was written last " +
			"time, is how a tool teaches people to stop writing reasoning at all.\n\n" +
			"`postponed` were deferrals. \"Not this sprint\" was about that sprint, so carrying " +
			"one silently gives a new line expiry dates nobody chose. Each says how long it has " +
			"already been put off across every line it has come through, because that total is " +
			"what carrying it again agrees to.\n\n" +
			"`absent` cover nothing here and are left behind.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Product     string `path:"product"`
		Stream      string `path:"stream"`
		Variant     string `path:"variant"`
		From        string `query:"from" required:"true" doc:"The stream to inherit from"`
		FromVariant string `query:"from_variant" required:"true" doc:"That stream's variant"`
	}) (*struct{ Body CarriedBody }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		_, to, err := browsing(ctx, in, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, err
		}
		_, from, err := browsing(ctx, in, input.Product, input.From, input.FromVariant)
		if err != nil {
			return nil, err
		}

		carried, err := store.WouldCarry(ctx, subject, from, to)
		if err != nil {
			return nil, refusedDecision(err)
		}
		body := CarriedBody{
			Applying: carried.Applying, Absent: carried.Absent,
			Moved:     inherited(carried.Moved),
			Postponed: inherited(carried.Postponed),
		}
		return &struct{ Body CarriedBody }{Body: body}, nil
	})
}

func inherited(rows []triage.Inherited) []InheritedBody {
	out := make([]InheritedBody, 0, len(rows))
	for _, row := range rows {
		out = append(out, InheritedBody{
			Decision: row.DecisionID, Vulnerability: row.Vulnerability,
			Component: row.Component, Outcome: string(row.Outcome),
			Was: row.Was, Now: row.Now, Reasoning: row.Reasoning,
			DeferredDays: row.DeferredDays,
		})
	}
	return out
}
