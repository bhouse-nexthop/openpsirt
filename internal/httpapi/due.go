package httpapi

import (
	"context"
	"math"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
)

// LateBody is a finding running out of time with nobody having decided.
type LateBody struct {
	Vulnerability string `json:"vulnerability"`
	Severity      string `json:"severity,omitempty"`
	Exploited     bool   `json:"exploited,omitempty"`
	Component     string `json:"component"`
	Product       string `json:"product"`
	Stream        string `json:"stream"`
	Variant       string `json:"variant"`
	Places        int    `json:"places" doc:"How many places in that build this sits at"`
	AssignedTo    string `json:"assigned_to,omitempty" doc:"Empty means nobody, or not everywhere the same person"`
	Due           string `json:"due" doc:"When it is due, as a date"`
	DaysLeft      int    `json:"days_left" doc:"Negative once it is overdue"`
}

func registerDue(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-running-out", Method: http.MethodGet, Path: "/v1/running-out",
		Summary: "List findings running out of time that nobody has decided about",
		Description: "Returns open findings whose deadline falls within `days`, where nobody has " +
			"recorded a decision, across every product you can see.\n\n" +
			"A deadline that has been answered is not on this list. A dismissal takes a finding " +
			"off the clock, because the claim is that it will not be fixed, and a deferral " +
			"replaces the deadline with its own date. What is left is time passing with nothing " +
			"said.\n\n" +
			"The window comes from how urgent a finding is. Being known-exploited has its own " +
			"and it is the shortest, whatever the severity says. Anything the reports did not " +
			"rate takes the medium window.\n\n" +
			"One row per issue at a component, however many places it sits at. `days_left` is " +
			"negative once something is overdue.",
		Tags: []string{"Findings"},
	}, func(ctx context.Context, input *struct {
		Days  int `query:"days" default:"14" minimum:"0" maximum:"365" doc:"How far ahead to look"`
		Limit int `query:"limit" default:"50" minimum:"1" maximum:"200"`
	}) (*listOutput[LateBody], error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}
		rows, err := finding.NewStore(in.DB.DB).RunningOut(ctx, subject,
			time.Duration(input.Days)*24*time.Hour, input.Limit)
		if err != nil {
			return nil, wentWrong(in.Logger, "what is running out of time could not be read", err)
		}

		owners := make([]int64, 0, len(rows))
		for _, row := range rows {
			if row.AssignedTo != nil {
				owners = append(owners, *row.AssignedTo)
			}
		}
		names, err := access.NewStore(in.DB.DB).Names(ctx, owners)
		if err != nil {
			return nil, wentWrong(in.Logger, "who is dealing with these could not be read", err)
		}

		now := time.Now().UTC()
		out := &listOutput[LateBody]{}
		out.Body.Items = make([]LateBody, 0, len(rows))
		for _, row := range rows {
			body := LateBody{
				Vulnerability: row.Vulnerability, Severity: row.Severity,
				Exploited: row.Exploited, Component: row.Component,
				Product: row.Product, Stream: row.Stream, Variant: row.Variant,
				Places: row.Places,
				Due:    row.Due.Format(time.DateOnly),
				// Rounded down, not toward zero. Truncation reports something
				// twelve hours overdue as having zero days left, which reads
				// as due today rather than as late.
				DaysLeft: int(math.Floor(row.Due.Sub(now).Hours() / 24)),
			}
			if row.AssignedTo != nil {
				body.AssignedTo = names[*row.AssignedTo]
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		return out, nil
	})
}

// dueWindows reads how long a finding may stay open, by how urgent it is.
//
// One reader for it, in the finding package, because ingest writes a deadline
// with these numbers and anything reading one has to agree about what they
// are (REM-26).
func dueWindows(ctx context.Context, in Ingest) (finding.Windows, error) {
	if in.DB == nil {
		return finding.DefaultWindows(), nil
	}
	return finding.LoadWindows(ctx, in.DB.DB)
}
