package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// EmbargoedBody is one finding nobody has announced, and when that ends.
type EmbargoedBody struct {
	Vulnerability string `json:"vulnerability"`
	Summary       string `json:"summary,omitempty"`
	Component     string `json:"component"`
	Product       string `json:"product"`
	Stream        string `json:"stream"`
	Variant       string `json:"variant"`
	Severity      string `json:"severity,omitempty"`
	DiscloseAt    string `json:"disclose_at" doc:"When the embargo ends. Reaching it discloses nothing"`
	// Passed says the date has arrived. It is a date to answer rather than a
	// trigger, so this is a row somebody has to act on rather than a record of
	// something that happened.
	Passed bool `json:"passed" doc:"Whether the date has already arrived"`
	Places int  `json:"places" doc:"How many findings this covers"`
}

// EnteredBody is what came of recording a flaw.
type EnteredBody struct {
	// Identifier is what it is filed under here, minted because a flaw nobody
	// has announced has no CVE to file it under.
	Identifier string `json:"identifier" doc:"What this deployment filed it as, such as SONIC-2026-0001"`
	Component  string `json:"component" doc:"What in the build carries it"`
	Visibility string `json:"visibility" enum:"public,private" doc:"Whether it has been disclosed"`
	DueAt      string `json:"due_at,omitempty" doc:"When it has to be answered by"`
}

func registerEntry(api huma.API, in Ingest) {
	huma.Register(api, requiring(huma.Operation{
		OperationID: "record-finding", Method: http.MethodPost,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings",
		Summary: "Record a flaw in what this build ships",
		Description: "Records a vulnerability in your own product — one no scanner reported, " +
			"usually because nobody outside knows about it yet.\n\n" +
			"**It starts undisclosed.** That is the case this exists for, and defaulting the " +
			"other way would make the dangerous mistake the quiet one. Recording an " +
			"undisclosed finding needs the private triage role on the product; send " +
			"`disclosed` for one that is already public, which needs the ordinary one.\n\n" +
			"**It is filed under an identifier this deployment mints** — the product's name, " +
			"the year and a number, which is the shape a vendor advisory takes. When a CVE is " +
			"assigned later it becomes another name for the same issue and nothing about the " +
			"finding, the decisions or the approvals moves.\n\n" +
			"`component` names what in the build carries it, as the build calls it. Leave it " +
			"out for the build itself, which is the honest answer where the flaw is in how " +
			"the pieces fit together rather than in one of them. A name the build holds at " +
			"more than one version is refused with the choices rather than resolved to one " +
			"of them; send `version`, and `ecosystem` where two share a version.\n\n" +
			"From here it behaves like any other finding: triaged, assigned, decided, on the " +
			"same clock and in the same reports. No scan will close it — a run is the " +
			"authority on what it found, and it found none of this.",
		Tags: []string{"Findings"}, DefaultStatus: http.StatusCreated,
	}, perProduct, "private-triage where the finding is undisclosed.", triageRights()...), func(ctx context.Context, input *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
		Variant string `path:"variant"`
		Body    struct {
			Summary   string `json:"summary" minLength:"1" doc:"What the flaw is, in your own words"`
			Severity  string `json:"severity" enum:"critical,high,medium,low,negligible,none" doc:"How bad it is"`
			Component string `json:"component,omitempty" doc:"What carries it. Omit for the build itself"`
			Version   string `json:"version,omitempty" doc:"Which one, where the build holds that name at several versions"`
			Ecosystem string `json:"ecosystem,omitempty" doc:"Which one, where two share a name and a version"`
			Disclosed bool   `json:"disclosed,omitempty" doc:"Whether this is already public. Undisclosed by default"`
		}
	}) (*struct {
		Status int
		Body   EnteredBody
	}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot record findings")
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

		row, identifier, err := finding.NewStore(in.DB.DB).Enter(ctx, subject, finding.Entering{
			TargetID: target.ID, Component: input.Body.Component,
			Version: input.Body.Version, Ecosystem: input.Body.Ecosystem,
			Summary: input.Body.Summary, Severity: input.Body.Severity,
			Disclosed: input.Body.Disclosed,
		})
		if err != nil {
			// Each of these is the caller's to fix, and says which. Falling
			// through to the generic refusal would answer a name the build
			// holds twice, and a summary of nothing but spaces, with a 500
			// that says the request went wrong at our end.
			var several *graph.Ambiguous
			switch {
			case errors.As(err, &several):
				return nil, severalComponents(several, "version, and ecosystem where two share one")
			case errors.Is(err, finding.ErrNoSuchComponent):
				return nil, huma.Error404NotFound(err.Error())
			case errors.Is(err, finding.ErrNothingSaid):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			case errors.Is(err, finding.ErrNothingScanned):
				return nil, huma.Error404NotFound(err.Error())
			}
			return nil, refusedFinding(in, err)
		}

		component, err := finding.NewStore(in.DB.DB).ComponentName(ctx, row.ComponentID)
		if err != nil {
			return nil, wentWrong(in.Logger, "what carries it could not be read", err)
		}
		out := &struct {
			Status int
			Body   EnteredBody
		}{Status: http.StatusCreated}
		out.Body = EnteredBody{
			Identifier: identifier, Component: component,
			Visibility: string(row.Visibility),
		}
		if row.DueAt != nil {
			out.Body.DueAt = stamp(*row.DueAt)
		}
		return out, nil
	})
}

// ResolvedBody is what came of saying a flaw is fixed.
type ResolvedBody struct {
	Closed int    `json:"closed" doc:"How many locations of the issue in this build were closed"`
	At     string `json:"at" doc:"When it was closed"`
}

func registerResolution(api huma.API, in Ingest) {
	huma.Register(api, requiring(huma.Operation{
		OperationID: "resolve-finding", Method: http.MethodPost,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/findings/{vulnerability}/resolve",
		Summary: "Close a recorded flaw as fixed in this build",
		Description: "Closes a flaw somebody recorded here, in one build, because it has been " +
			"fixed there. Every location of the issue in that build is closed together.\n\n" +
			"**Only a flaw somebody recorded.** Everywhere else, resolution is computed from " +
			"scans rather than declared, which is what stops a fix being reported that shipped " +
			"in nobody's release. A flaw recorded by hand is the one case with no such " +
			"evidence and no prospect of any — no scan reports it — so a person closes it or " +
			"nothing does. An issue a scanner found is refused.\n\n" +
			"**A reason is required.** A closure with no reason is a record saying somebody " +
			"closed it and nothing else.\n\n" +
			"**Nothing reopens one.** Closing is a considered act, and this is the way it is " +
			"undone: it is not.",
		Tags: []string{"Findings"},
	}, perProduct, "", triageRights()...), func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability"`
		Body          struct {
			Because string `json:"because" minLength:"1" doc:"What fixed it"`
		}
	}) (*struct{ Body ResolvedBody }, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot close findings")
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
		issueID, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, input.Vulnerability)
		if err != nil {
			return nil, noSuchIssue()
		}

		done, err := finding.NewStore(in.DB.DB).Resolve(ctx, subject,
			target.ID, issueID, input.Body.Because)
		if err != nil {
			switch {
			case errors.Is(err, finding.ErrNotOursToClose):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			case errors.Is(err, finding.ErrNoReason):
				return nil, huma.Error422UnprocessableEntity(err.Error())
			case errors.Is(err, finding.ErrNothingOpenThere):
				return nil, noSuchFinding()
			case errors.Is(err, access.ErrDenied):
				return nil, noSuchFinding()
			}
			return nil, wentWrong(in.Logger, "that could not be closed", err)
		}
		return &struct{ Body ResolvedBody }{Body: ResolvedBody{
			Closed: done.Closed, At: stamp(done.At),
		}}, nil
	})
}

func registerDisclosure(api huma.API, in Ingest) {
	huma.Register(api, requiring(huma.Operation{
		OperationID: "list-approaching-disclosure", Method: http.MethodGet,
		Path:    "/v1/disclosing",
		Summary: "List what is approaching disclosure",
		Description: "Returns findings nobody has announced whose embargo is running out, " +
			"soonest first, and the ones whose date has already arrived.\n\n" +
			"**Before the date, not on it.** The date arriving is the last moment to act on " +
			"something rather than the first useful warning, and a list that only ever showed " +
			"what was already past would be a list of decisions somebody has already failed to " +
			"make.\n\n" +
			"**Nothing here discloses anything.** Reaching the date escalates: the row appears " +
			"and the people who can act on it are told. Publishing embargoed detail because a " +
			"timer expired is the wrong default — if the fix is not ready, disclosing anyway is " +
			"a decision a person makes.\n\n" +
			"Every row is undisclosed by definition, so this list is a disclosure in its own " +
			"right: a product you may not read undisclosed work in contributes nothing to it, " +
			"not even a count.",
		Tags: []string{"Findings"},
	}, perProduct, "Only where you may read undisclosed work.", privateRights()...), func(ctx context.Context, input *struct {
		ScopeQuery
		Within int `query:"within" default:"30" minimum:"1" maximum:"365" doc:"How many days ahead to look"`
		Limit  int `query:"limit" default:"100" minimum:"1" maximum:"500"`
	}) (*listOutput[EmbargoedBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot read findings")
		}
		scope, err := scoped(ctx, in, subject, input.ScopeQuery)
		if err != nil {
			return nil, err
		}

		store := finding.NewStore(in.DB.DB)
		rows, err := store.Disclosing(ctx, subject, scope,
			time.Duration(input.Within)*24*time.Hour, input.Limit)
		if err != nil {
			return nil, wentWrong(in.Logger, "what is approaching disclosure could not be read", err)
		}

		now := time.Now().UTC()
		out := &listOutput[EmbargoedBody]{}
		out.Body.Items = make([]EmbargoedBody, 0, len(rows))
		for _, row := range rows {
			out.Body.Items = append(out.Body.Items, EmbargoedBody{
				Vulnerability: row.Vulnerability, Summary: row.Summary,
				Component: row.Component, Product: row.Product,
				Stream: row.Stream, Variant: row.Variant, Severity: row.Severity,
				DiscloseAt: stamp(row.DiscloseAt), Passed: row.Passed(now),
				Places: row.Places,
			})
		}
		return out, nil
	})
}

// ExtensionBody is one time somebody moved the end of an embargo.
type ExtensionBody struct {
	ID            int64  `json:"id"`
	Was           string `json:"was" doc:"Where the embargo ended before"`
	Until         string `json:"until" doc:"Where it was asked to end"`
	Reason        string `json:"reason"`
	AskedBy       string `json:"asked_by"`
	AskedAt       string `json:"asked_at"`
	NeedsApproval bool   `json:"needs_approval" doc:"Whether a second person had to agree"`
	ApprovedBy    string `json:"approved_by,omitempty"`
	ApprovedAt    string `json:"approved_at,omitempty"`
	// InForce says the date follows this one. An extension waiting for
	// agreement has moved nothing.
	InForce bool `json:"in_force"`
}

func registerExtensions(api huma.API, in Ingest) {
	const path = "/v1/products/{product}/issues/{vulnerability}/disclosure"

	huma.Register(api, requiring(huma.Operation{
		OperationID: "extend-disclosure", Method: http.MethodPost, Path: path,
		Summary: "Ask to move a disclosure date later",
		Description: "Moves the end of an embargo, across every undisclosed finding of this " +
			"issue in this product.\n\n" +
			"**A reason is required, always**, however short the extension. One with no reason " +
			"is a record saying somebody moved it and nothing else.\n\n" +
			"**Past a threshold it needs a second person**, and the threshold is measured " +
			"against everything this embargo has already been moved by rather than against " +
			"this request alone — measured per request, the exception swallows the rule three " +
			"weeks at a time. It is the same act a deferral is, and the same shape.\n\n" +
			"**An extension that needs agreement moves nothing until it has it.** The request " +
			"is on record either way; `in_force` says whether the date follows it.\n\n" +
			"A date only ever moves later. Bringing one forward is disclosing sooner, which is " +
			"a different act.",
		Tags: []string{"Findings"}, DefaultStatus: http.StatusCreated,
	}, perProduct, "A second person agrees past the threshold.", []access.Role{access.PrivateTriage}...), func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Vulnerability string `path:"vulnerability"`
		Body          struct {
			Until  string `json:"until" doc:"Where the embargo should end, as a date"`
			Reason string `json:"reason" minLength:"1" doc:"Why it is being extended"`
		}
	}) (*struct {
		Status int
		Body   ExtensionBody
	}, error) {
		subject, store, product, issue, err := embargoAt(ctx, in, input.Product, input.Vulnerability)
		if err != nil {
			return nil, err
		}
		until, err := time.Parse(time.DateOnly, input.Body.Until)
		if err != nil {
			return nil, huma.Error422UnprocessableEntity(
				"until has to be a date, as 2026-12-31")
		}

		asked, err := store.Extend(ctx, subject, product, issue, until, input.Body.Reason)
		if err != nil {
			if errors.Is(err, finding.ErrNotEmbargoed) {
				return nil, noSuchFinding()
			}
			return nil, refusedFinding(in, err)
		}
		body, err := extensionBody(ctx, in, []finding.Extension{*asked})
		if err != nil {
			return nil, wentWrong(in.Logger, "the extension could not be read back", err)
		}
		return &struct {
			Status int
			Body   ExtensionBody
		}{Status: http.StatusCreated, Body: body[0]}, nil
	})

	huma.Register(api, requiring(huma.Operation{
		OperationID: "list-disclosure-extensions", Method: http.MethodGet, Path: path,
		Summary: "List how an embargo has been moved",
		Description: "Every time this embargo was moved, oldest first, with why and by whom.\n\n" +
			"Kept in full and never overwritten. One extension is a judgment and six is a " +
			"policy nobody wrote down, and the difference is invisible if each replaces the " +
			"last. A request still waiting for agreement is here too: what was asked for is " +
			"part of how long this stayed hidden, whether or not it was granted.",
		Tags: []string{"Findings"},
	}, perProduct, "Only where you may read undisclosed work.", privateRights()...), func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Vulnerability string `path:"vulnerability"`
	}) (*listOutput[ExtensionBody], error) {
		subject, store, product, issue, err := embargoAt(ctx, in, input.Product, input.Vulnerability)
		if err != nil {
			return nil, err
		}
		rows, err := store.Extensions(ctx, subject, product, issue)
		if err != nil {
			return nil, refusedFinding(in, err)
		}
		items, err := extensionBody(ctx, in, rows)
		if err != nil {
			return nil, wentWrong(in.Logger, "the extensions could not be read", err)
		}
		out := &listOutput[ExtensionBody]{}
		out.Body.Items = items
		return out, nil
	})

	huma.Register(api, requiring(huma.Operation{
		OperationID: "agree-to-extension", Method: http.MethodPost,
		Path:    "/v1/disclosure-extensions/{id}/approval",
		Summary: "Agree to moving a disclosure date",
		Description: "Records a second person agreeing, and moves the date.\n\n" +
			"The person who asked may not be the one who agrees. That is the control the " +
			"threshold exists to reach, and an extension somebody approved for themselves is " +
			"the same as one nobody approved.",
		Tags: []string{"Findings"}, DefaultStatus: http.StatusNoContent,
	}, perProduct, "Not the person who asked for it.", []access.Role{access.PrivateTriage}...), func(ctx context.Context, input *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		if in.DB == nil {
			return nil, huma.Error500InternalServerError("this process cannot read findings")
		}
		err = finding.NewStore(in.DB.DB).AgreeToExtension(ctx, subject, input.ID)
		switch {
		case errors.Is(err, finding.ErrNotEmbargoed):
			return nil, noSuchFinding()
		case errors.Is(err, finding.ErrSamePerson):
			return nil, huma.Error409Conflict(
				"the person who asked to move a date may not be the one who agrees to it")
		case err != nil:
			return nil, refusedFinding(in, err)
		}
		return &struct{}{}, nil
	})
}

// embargoAt resolves a product and an issue for the disclosure endpoints.
func embargoAt(ctx context.Context, in Ingest, productName, issueName string) (
	access.Subject, *finding.Store, int64, int64, error) {

	subject, err := reading(ctx)
	if err != nil {
		return subject, nil, 0, 0, err
	}
	if in.DB == nil {
		return subject, nil, 0, 0,
			huma.Error500InternalServerError("this process cannot read findings")
	}
	product, err := catalog.NewStore(in.DB.DB).VisibleProduct(ctx, subject, productName)
	if err != nil {
		return subject, nil, 0, 0, noSuchProduct()
	}
	issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, issueName)
	if err != nil {
		return subject, nil, 0, 0, noSuchIssue()
	}
	return subject, finding.NewStore(in.DB.DB), product.ID, issue, nil
}

// extensionBody names the people an extension record refers to by identifier.
func extensionBody(ctx context.Context, in Ingest, rows []finding.Extension) ([]ExtensionBody, error) {
	people := make([]int64, 0, len(rows)*2)
	for _, row := range rows {
		people = append(people, row.AskedBy)
		if row.ApprovedBy != nil {
			people = append(people, *row.ApprovedBy)
		}
	}
	names, err := access.NewStore(in.DB.DB).Names(ctx, people)
	if err != nil {
		return nil, err
	}
	out := make([]ExtensionBody, 0, len(rows))
	for _, row := range rows {
		body := ExtensionBody{
			ID: row.ID, Was: stamp(row.Was), Until: stamp(row.Until),
			Reason: row.Reason, AskedBy: names[row.AskedBy],
			AskedAt: stamp(row.AskedAt), NeedsApproval: row.NeedsApproval,
			InForce: row.InForce(),
		}
		if row.ApprovedBy != nil {
			body.ApprovedBy = names[*row.ApprovedBy]
		}
		if row.ApprovedAt != nil {
			body.ApprovedAt = stamp(*row.ApprovedAt)
		}
		out = append(out, body)
	}
	return out, nil
}
