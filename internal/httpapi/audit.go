package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// AgreedBody is one person agreeing to one judgment, and when.
type AgreedBody struct {
	By string `json:"by" doc:"Their sign-in identity"`
	At string `json:"at" doc:"When they agreed"`
	// WithdrawnAt is part of the record rather than a reason to leave the
	// agreement out: what somebody agreed to and then stopped agreeing to is
	// what an audit is looking for.
	WithdrawnAt string `json:"withdrawn_at,omitempty" doc:"When the agreement was taken back, by the approver or by somebody editing the words it was given for"`
}

// JudgedBody is one judgment as an auditor reads it.
type JudgedBody struct {
	ID      int64  `json:"id"`
	Issue   string `json:"issue" doc:"The vulnerability, under the name it is filed here"`
	Product string `json:"product"`
	// What it was about. Named from a finding at the place, in any state — a
	// judgment about something since fixed or removed is exactly what an audit
	// asks for, so it is named rather than left blank.
	Component string `json:"component"`
	Version   string `json:"version,omitempty"`
	Consumer  string `json:"consumer,omitempty" doc:"What pulls the component in. Absent where the build holds it directly"`

	Outcome       string `json:"outcome" enum:"affected,not-applicable,deferred,wont-fix,already-fixed"`
	Justification string `json:"justification,omitempty" doc:"The recognized reason it does not apply"`
	Mitigation    string `json:"mitigation,omitempty" doc:"What stops it, where the reason is that a control already does. Nothing here notices that control being removed, so this is the record somebody checks"`
	DeferredUntil string `json:"deferred_until,omitempty"`
	FixedVersion  string `json:"fixed_version,omitempty" doc:"The package version the claim says the fix arrived in, where it claims one has. What somebody auditing an already-fixed claim checks against the packager's own record"`
	Reasoning     string `json:"reasoning" doc:"The words the standing agreement was given for. Editing them withdraws the agreement, so this and what was agreed to cannot drift apart"`

	State    string `json:"state" enum:"proposed,approved,withdrawn,lapsed"`
	Standing bool   `json:"standing" doc:"Whether it applies now. A judgment can be approved and no longer standing — the code moved out from under it"`

	ProposedBy string       `json:"proposed_by"`
	ProposedAt string       `json:"proposed_at"`
	EndedAt    string       `json:"ended_at,omitempty" doc:"When it stopped applying — withdrawn, or lapsed because the code moved"`
	Approvals  []AgreedBody `json:"approvals"`
	// TwoPeople is the separation-of-duties control stated as a fact about
	// this record rather than as a rule that exists. Read from the names: a
	// report that said the rule was satisfied because the rule exists would be
	// reporting on itself.
	TwoPeople bool `json:"two_people" doc:"Whether somebody other than the proposer has a standing agreement on it"`
}

func registerAudit(api huma.API, in Ingest) {
	huma.Register(api, requiring(huma.Operation{
		OperationID: "list-audit", Method: http.MethodGet, Path: "/v1/audit",
		Summary: "List judgments with who made them and who agreed",
		Description: "Every judgment recorded in a period, newest first, with what it was " +
			"about, the reasoning it rests on, who proposed it and when, and who agreed and " +
			"when — including agreements later taken back.\n\n" +
			"The period is the date a judgment was **proposed**, not approved: a judgment " +
			"belongs to when it was argued, and dating it by its agreement would move it out " +
			"of that period whenever an approval came late, which is the ordinary case.\n\n" +
			"Narrowed by what you may see, like every other list here. Nothing about this view " +
			"is exempt from the visibility rules — a report showing more than the screens it " +
			"summarizes would be a way around them.",
		Tags: []string{"Reports"},
	}, anySubject, "Answers only what you may see."), func(ctx context.Context, input *struct {
		Product string `query:"product" doc:"Limit to one product, by name"`
		Outcome string `query:"outcome" enum:"affected,not-applicable,deferred,wont-fix,already-fixed" doc:"Limit to one kind of judgment"`
		State   string `query:"state" enum:"proposed,approved,withdrawn,lapsed" doc:"Limit to one state"`
		From    string `query:"from" doc:"Only judgments proposed on or after this date, as YYYY-MM-DD"`
		To      string `query:"to" doc:"Only judgments proposed before this date, as YYYY-MM-DD"`
		Limit   int    `query:"limit" default:"100" minimum:"1" maximum:"500"`
		Offset  int    `query:"offset" minimum:"0"`
	}) (*struct {
		Body struct {
			Items []JudgedBody `json:"items"`
			Total int          `json:"total"`
		}
	}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		filter := triage.Filter{
			Outcome: triage.Outcome(input.Outcome),
			State:   triage.State(input.State),
		}
		if input.Product != "" {
			named, err := catalog.NewStore(in.DB.DB).VisibleProduct(ctx, subject, input.Product)
			if err != nil {
				return nil, noSuchProduct()
			}
			filter.ProductID = named.ID
		}
		from, err := aDate(input.From)
		if err != nil {
			return nil, err
		}
		to, err := aDate(input.To)
		if err != nil {
			return nil, err
		}
		var since, until time.Time
		if from != nil {
			since = *from
		}
		if to != nil {
			until = *to
		}

		rows, total, err := store.Audit(ctx, subject, filter, since, until, input.Limit, input.Offset)
		if err != nil {
			return nil, wentWrong(in.Logger, "the record could not be read", err)
		}
		out := &struct {
			Body struct {
				Items []JudgedBody `json:"items"`
				Total int          `json:"total"`
			}
		}{}
		out.Body.Items = make([]JudgedBody, 0, len(rows))
		for _, row := range rows {
			body := JudgedBody{
				ID: row.ID, Issue: row.Issue, Product: row.Product,
				Component: row.Component, Version: row.Version, Consumer: row.Consumer,
				Outcome: string(row.Outcome), Reasoning: row.Reasoning,
				State: string(row.State), Standing: row.Standing(),
				ProposedBy: row.ProposedByName, ProposedAt: stamp(row.ProposedAt),
				TwoPeople: row.BySomebodyElse(),
				Approvals: make([]AgreedBody, 0, len(row.Approvals)),
			}
			if row.Justification != nil {
				body.Justification = *row.Justification
			}
			if row.Mitigation != nil {
				body.Mitigation = *row.Mitigation
			}
			if row.DeferredUntil != nil {
				body.DeferredUntil = row.DeferredUntil.UTC().Format(time.DateOnly)
			}
			if row.FixedVersion != nil {
				body.FixedVersion = *row.FixedVersion
			}
			if row.EndedAt != nil {
				body.EndedAt = stamp(*row.EndedAt)
			}
			for _, agreed := range row.Approvals {
				one := AgreedBody{By: agreed.By, At: stamp(agreed.At)}
				if agreed.WithdrawnAt != nil {
					one.WithdrawnAt = stamp(*agreed.WithdrawnAt)
				}
				body.Approvals = append(body.Approvals, one)
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		out.Body.Total = total
		return out, nil
	})
}
