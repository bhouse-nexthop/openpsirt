package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
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
	huma.Register(api, huma.Operation{
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
			"the pieces fit together rather than in one of them.\n\n" +
			"From here it behaves like any other finding: triaged, assigned, decided, on the " +
			"same clock and in the same reports. No scan will close it — a run is the " +
			"authority on what it found, and it found none of this.",
		Tags: []string{"Findings"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		Product string `path:"product"`
		Stream  string `path:"stream"`
		Variant string `path:"variant"`
		Body    struct {
			Summary   string `json:"summary" minLength:"1" doc:"What the flaw is, in your own words"`
			Severity  string `json:"severity" enum:"critical,high,medium,low,negligible,none" doc:"How bad it is"`
			Component string `json:"component,omitempty" doc:"What carries it. Omit for the build itself"`
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
			Summary: input.Body.Summary, Severity: input.Body.Severity,
			Disclosed: input.Body.Disclosed,
		})
		if err != nil {
			if errors.Is(err, finding.ErrNoSuchComponent) {
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

func registerDisclosure(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
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
	}, func(ctx context.Context, input *struct {
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
