package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

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
