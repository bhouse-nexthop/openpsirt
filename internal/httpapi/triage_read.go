package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// DecisionDetail is a decision with everything needed to understand it without
// asking again: where it applies, what it says, and how far it has got.
type DecisionDetail struct {
	Decision  DecisionBody `json:"decision"`
	Place     PlaceBody    `json:"place"`
	Reasoning string       `json:"reasoning" doc:"The justification as it currently stands, in markdown"`
	// ReasoningHTML is present only when html=true was asked for. Markdown is
	// what every consumer gets by default; HTML assumes a browser, and most
	// callers of this are not one.
	ReasoningHTML string `json:"reasoning_html,omitempty" doc:"The same text as sanitized HTML. Only when html=true"`
	ProposedBy    string `json:"proposed_by"`
	ProposedAt    string `json:"proposed_at" doc:"When the claim was made, as a date and time"`
	AgeDays       int    `json:"age_days" doc:"How long the claim has stood. An old judgment should look like one"`
}

// RevisionBody is one statement of a justification.
type RevisionBody struct {
	ID        int64  `json:"id" doc:"What an approval names when it says which words were agreed to"`
	Ordinal   int64  `json:"ordinal" doc:"Which revision this is, counting from one"`
	Body      string `json:"body" doc:"The justification text, in markdown"`
	BodyHTML  string `json:"body_html,omitempty" doc:"The same text as sanitized HTML. Only when html=true"`
	WrittenBy string `json:"written_by"`
	WrittenAt string `json:"written_at"`
}

// ApprovalBody is one person agreeing to one revision of a justification.
type ApprovalBody struct {
	ID          int64  `json:"id"`
	RevisionID  int64  `json:"revision_id" doc:"Which revision of the justification was agreed to"`
	ApprovedBy  string `json:"approved_by"`
	ApprovedAt  string `json:"approved_at"`
	WithdrawnAt string `json:"withdrawn_at,omitempty" doc:"When this approval was taken back, if it was"`
	Batch       string `json:"batch,omitempty" doc:"The batch it was approved under, if it was a bulk approval"`
}

// CommentBody is one remark on a decision.
type CommentBody struct {
	ID        int64  `json:"id"`
	Body      string `json:"body" doc:"The comment text, in markdown"`
	BodyHTML  string `json:"body_html,omitempty" doc:"The same text as sanitized HTML. Only when html=true"`
	WrittenBy string `json:"written_by"`
	WrittenAt string `json:"written_at"`
	EditedAt  string `json:"edited_at,omitempty" doc:"When the author last changed it, if they did"`
}

func registerTriageReading(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-decisions", Method: http.MethodGet, Path: "/v1/decisions",
		Summary: "List triage decisions",
		Description: "Returns triage decisions on products you may triage, newest first, with " +
			"the justification text for each.\n\n" +
			"Filter by `outcome` to list dismissals (`not-applicable`, `wont-fix`) or " +
			"postponements (`deferred`), by `state` to separate what is approved from what is " +
			"still waiting or has been withdrawn, and by `product` to limit to one product.\n\n" +
			"Set `expired=true` to list deferrals whose date has passed — the findings that have " +
			"come back and need judging again.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Product string `query:"product" doc:"Limit to one product, by name"`
		Outcome string `query:"outcome" enum:"affected,not-applicable,deferred,wont-fix" doc:"Limit to one outcome"`
		State   string `query:"state" enum:"proposed,approved,withdrawn,lapsed" doc:"Limit to one state"`
		Expired bool   `query:"expired" doc:"Only deferrals whose date has passed"`
		HTML    bool   `query:"html" doc:"Also return each justification as sanitized HTML"`
		Limit   int    `query:"limit" default:"50" minimum:"1" maximum:"200"`
		Offset  int    `query:"offset" minimum:"0"`
	}) (*DecisionsOutput, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}

		filter := triage.Filter{
			Outcome: triage.Outcome(input.Outcome),
			State:   triage.State(input.State),
			Expired: input.Expired,
		}
		if input.Product != "" {
			product, err := catalog.NewStore(in.DB.DB).ProductByName(ctx, input.Product)
			if err != nil {
				return nil, noSuchProduct()
			}
			filter.ProductID = product.ID
		}

		decisions, reasoning, total, err := store.List(ctx, subject, filter, input.Limit, input.Offset)
		if err != nil {
			return nil, wentWrong(in.Logger, "what was decided could not be read", err)
		}
		details, err := describeDecisions(ctx, in, store, decisions, reasoning)
		if err != nil {
			return nil, wentWrong(in.Logger, "what was decided could not be read", err)
		}
		for i := range details {
			details[i].ReasoningHTML = asHTML(ctx, in, input.HTML, details[i].Reasoning)
		}

		out := &DecisionsOutput{}
		out.Body.Items = details
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-decision", Method: http.MethodGet, Path: "/v1/decisions/{id}",
		Summary: "Get a triage decision",
		Description: "Returns one decision with the justification as it currently stands, where " +
			"it applies, who proposed it and how long it has stood.\n\n" +
			"For the earlier justifications see `GET /v1/decisions/{id}/revisions`, and for who " +
			"agreed to which of them see `GET /v1/decisions/{id}/approvals`.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		HTML bool  `query:"html" doc:"Also return the justification as sanitized HTML"`
	}) (*struct{ Body DecisionDetail }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		decision, reasoning, err := store.Read(ctx, subject, input.ID)
		if err != nil {
			return nil, refusedDecision(err)
		}
		details, err := describeDecisions(ctx, in, store, []triage.Decision{*decision},
			map[int64]string{decision.ID: reasoning})
		if err != nil || len(details) == 0 {
			return nil, wentWrong(in.Logger, "that decision could not be read", err)
		}
		details[0].ReasoningHTML = asHTML(ctx, in, input.HTML, details[0].Reasoning)
		return &struct{ Body DecisionDetail }{Body: details[0]}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-decision-revisions", Method: http.MethodGet,
		Path:    "/v1/decisions/{id}/revisions",
		Summary: "List a decision's justification history",
		Description: "Returns every revision of the justification, oldest first, with who wrote " +
			"each and when.\n\n" +
			"An approval names the specific revision that was agreed to, so this is how to read " +
			"what an approver actually saw rather than what the text says now.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		HTML bool  `query:"html" doc:"Also return each justification as sanitized HTML"`
	}) (*listOutput[RevisionBody], error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		revisions, err := store.Revisions(ctx, subject, input.ID)
		if err != nil {
			return nil, refusedDecision(err)
		}

		authors := make([]int64, 0, len(revisions))
		for _, revision := range revisions {
			authors = append(authors, revision.WrittenBy)
		}
		names, err := access.NewStore(in.DB.DB).Names(ctx, authors)
		if err != nil {
			return nil, wentWrong(in.Logger, "the reasoning could not be read", err)
		}

		out := &listOutput[RevisionBody]{}
		out.Body.Items = make([]RevisionBody, 0, len(revisions))
		for _, revision := range revisions {
			out.Body.Items = append(out.Body.Items, RevisionBody{
				ID: revision.ID, Ordinal: revision.Ordinal, Body: revision.Body,
				BodyHTML:  asHTML(ctx, in, input.HTML, revision.Body),
				WrittenBy: names[revision.WrittenBy],
				WrittenAt: revision.WrittenAt.Format(time.RFC3339),
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-decision-approvals", Method: http.MethodGet,
		Path:    "/v1/decisions/{id}/approvals",
		Summary: "List who approved a decision",
		Description: "Returns every approval recorded against this decision, including ones later " +
			"withdrawn, each naming the revision of the justification it was given for.\n\n" +
			"A withdrawn approval is kept rather than deleted: who agreed to what, and when it " +
			"stopped counting, is part of the record.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		ID int64 `path:"id"`
	}) (*listOutput[ApprovalBody], error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		approvals, err := store.Approvals(ctx, subject, input.ID)
		if err != nil {
			return nil, refusedDecision(err)
		}

		approvers := make([]int64, 0, len(approvals))
		for _, approval := range approvals {
			approvers = append(approvers, approval.ApprovedBy)
		}
		names, err := access.NewStore(in.DB.DB).Names(ctx, approvers)
		if err != nil {
			return nil, wentWrong(in.Logger, "who agreed could not be read", err)
		}

		out := &listOutput[ApprovalBody]{}
		out.Body.Items = make([]ApprovalBody, 0, len(approvals))
		for _, approval := range approvals {
			body := ApprovalBody{
				ID: approval.ID, RevisionID: approval.RevisionID,
				ApprovedBy: names[approval.ApprovedBy],
				ApprovedAt: approval.ApprovedAt.Format(time.RFC3339),
			}
			if approval.WithdrawnAt != nil {
				body.WithdrawnAt = approval.WithdrawnAt.Format(time.RFC3339)
			}
			if approval.Batch != nil {
				body.Batch = *approval.Batch
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-decision-comments", Method: http.MethodGet,
		Path:    "/v1/decisions/{id}/comments",
		Summary: "List comments on a decision",
		Description: "Returns the comments on a decision, oldest first, with who wrote each and " +
			"when. A comment that has been edited also carries when it was last changed.\n\n" +
			"Comments are separate from the justification and never affect an approval.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		HTML bool  `query:"html" doc:"Also return each comment as sanitized HTML"`
	}) (*listOutput[CommentBody], error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		comments, err := store.Discussion(ctx, subject, input.ID)
		if err != nil {
			return nil, refusedDecision(err)
		}

		authors := make([]int64, 0, len(comments))
		for _, comment := range comments {
			authors = append(authors, comment.WrittenBy)
		}
		names, err := access.NewStore(in.DB.DB).Names(ctx, authors)
		if err != nil {
			return nil, wentWrong(in.Logger, "the discussion could not be read", err)
		}

		out := &listOutput[CommentBody]{}
		out.Body.Items = make([]CommentBody, 0, len(comments))
		for _, comment := range comments {
			body := CommentBody{
				ID: comment.ID, Body: comment.Body,
				BodyHTML:  asHTML(ctx, in, input.HTML, comment.Body),
				WrittenBy: names[comment.WrittenBy],
				WrittenAt: comment.WrittenAt.Format(time.RFC3339),
			}
			if comment.EditedAt != nil {
				body.EditedAt = comment.EditedAt.Format(time.RFC3339)
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "edit-comment", Method: http.MethodPut, Path: "/v1/comments/{id}",
		Summary: "Edit a comment",
		Description: "Replaces the text of a comment. Only its author may do this, and the new " +
			"text overwrites the old rather than being kept as a revision — a comment is a " +
			"remark, not a justification.\n\n" +
			"The text is markdown and is validated before it is stored; a 422 names the line and " +
			"the offending text.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		Body struct {
			Body string `json:"body" minLength:"1"`
		}
	}) (*struct{}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		if err := store.Reword(ctx, subject, input.ID, input.Body.Body); err != nil {
			return nil, refusedDecision(err)
		}
		return &struct{}{}, nil
	})
}

// DecisionsOutput is a page of decisions, with how many there are behind it.
type DecisionsOutput struct {
	Body struct {
		Items []DecisionDetail `json:"items"`
		Total int              `json:"total"`
	}
}

// describeDecisions fills in the names a decision refers to by identifier.
//
// A row saying product 4, issue 91 is a row somebody has to make two more
// requests to understand, and the lists this feeds are exactly where that
// happens fifty times.
func describeDecisions(ctx context.Context, in Ingest, store *triage.Store,
	decisions []triage.Decision, reasoning map[int64]string) ([]DecisionDetail, error) {

	people := make([]int64, 0, len(decisions))
	products := make([]int64, 0, len(decisions))
	issues := make([]int64, 0, len(decisions))
	for _, decision := range decisions {
		people = append(people, decision.ProposedBy)
		products = append(products, decision.ProductID)
		issues = append(issues, decision.VulnerabilityID)
	}

	names, err := access.NewStore(in.DB.DB).Names(ctx, people)
	if err != nil {
		return nil, err
	}
	productNames, err := catalog.NewStore(in.DB.DB).ProductNames(ctx, products)
	if err != nil {
		return nil, err
	}
	issueNames, err := finding.NewVulnerabilities(in.DB.DB).NamesByID(ctx, issues)
	if err != nil {
		return nil, err
	}

	details := make([]DecisionDetail, 0, len(decisions))
	for _, decision := range decisions {
		details = append(details, DecisionDetail{
			Decision: decisionBody(decision),
			Place: PlaceBody{
				Product:       productNames[decision.ProductID],
				Vulnerability: issueNames[decision.VulnerabilityID],
				Place:         decision.PlaceIdentity,
			},
			Reasoning:  reasoning[decision.ID],
			ProposedBy: names[decision.ProposedBy],
			ProposedAt: decision.ProposedAt.Format(time.RFC3339),
			AgeDays:    int(store.Age(&decision).Hours() / 24),
		})
	}
	return details, nil
}

// StandingBody is what a place currently has decided about it, and what it had
// before.
type StandingBody struct {
	// Standing is the decision suppressing this finding, absent where nothing
	// is. Absent is the ordinary answer for a finding nobody has judged.
	Standing *DecisionDetail `json:"standing,omitempty"`
	// Previously is what was decided here before, newest first: claims that
	// were withdrawn, and claims the code moved out from under. It is what
	// makes re-deciding a re-reading rather than a blank page.
	Previously []DecisionDetail `json:"previously"`
}

func registerPlaceDecisions(api huma.API, in Ingest) {
	const at = "/v1/products/{product}/streams/{stream}/variants/{variant}" +
		"/findings/{vulnerability}/places/{place}/decision"

	huma.Register(api, huma.Operation{
		OperationID: "get-finding-decision", Method: http.MethodGet, Path: at,
		Summary: "Get what has been decided about a finding",
		Description: "Returns the decision currently suppressing this finding, if any, together " +
			"with everything decided here before — withdrawn claims, and claims that stopped " +
			"applying when an upstream version changed.\n\n" +
			"`standing` is absent when nothing has been decided, or when a claim is still waiting " +
			"for approval: a claim nobody has agreed to suppresses nothing.\n\n" +
			"Read `previously` before deciding again. A claim that lapsed on a version bump is " +
			"usually still the right answer, and re-affirming it is a different request from " +
			"making a new one.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability" doc:"The issue, by any name it is known under"`
		Place         string `path:"place" doc:"Which place, as the findings list gives it"`
	}) (*struct{ Body StandingBody }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		where, err := decidingAbout(ctx, in, subject, input.Product, input.Stream, input.Variant,
			input.Vulnerability, input.Place)
		if err != nil {
			return nil, err
		}
		place := triage.Place{
			ProductID: where.ProductID, VulnerabilityID: where.VulnerabilityID,
			PlaceIdentity: where.PlaceIdentity, Visibility: where.Visibility,
			ComponentUpstream: where.ComponentUpstream, ConsumerUpstream: where.ConsumerUpstream,
		}

		standing, err := store.Applying(ctx, place)
		if err != nil {
			return nil, wentWrong(in.Logger, "what applies here could not be read", err)
		}
		previously, err := store.PreviouslyAt(ctx, subject, place)
		if err != nil {
			return nil, wentWrong(in.Logger, "what was decided here could not be read", err)
		}

		all := previously
		if standing != nil {
			all = append([]triage.Decision{*standing}, previously...)
		}
		reasoning, err := store.ReasoningFor(ctx, all)
		if err != nil {
			return nil, wentWrong(in.Logger, "the reasoning could not be read", err)
		}
		details, err := describeDecisions(ctx, in, store, all, reasoning)
		if err != nil {
			return nil, wentWrong(in.Logger, "what was decided here could not be read", err)
		}

		body := StandingBody{Previously: []DecisionDetail{}}
		if standing != nil {
			body.Standing = &details[0]
			details = details[1:]
		}
		body.Previously = append(body.Previously, details...)
		return &struct{ Body StandingBody }{Body: body}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "reaffirm-decision", Method: http.MethodPost, Path: at + "/reaffirmation",
		Summary: "Re-affirm a decision after an upstream version changed",
		Description: "Re-makes a decision that stopped applying because an upstream version " +
			"moved, at the versions this finding has now. `previous` is the decision being " +
			"re-made, from `previously` in `GET .../decision`.\n\n" +
			"Only the person who made the original may do this, and it normally needs no second " +
			"approver: two people already agreed to the claim, and a version bump is a prompt to " +
			"re-check rather than a new claim.\n\n" +
			"It does need approval again if the justification differs from the original, or if " +
			"the vulnerability's severity has risen since — both mean this is not the claim that " +
			"was agreed to. The response says which happened.\n\n" +
			"`reasoning` is required. \"Still true\" with nothing behind it is what a " +
			"re-affirmation becomes when it is made too easy.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability"`
		Place         string `path:"place"`
		Body          struct {
			Previous  int64  `json:"previous" doc:"The decision being re-made"`
			Reasoning string `json:"reasoning" minLength:"1" doc:"Why it still holds, in markdown"`
		}
	}) (*struct{ Body DecisionBody }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		where, err := decidingAbout(ctx, in, subject, input.Product, input.Stream, input.Variant,
			input.Vulnerability, input.Place)
		if err != nil {
			return nil, err
		}

		made, err := store.Reaffirm(ctx, subject, triage.Reaffirmation{
			PreviousID: input.Body.Previous,
			Place: triage.Place{
				ProductID: where.ProductID, VulnerabilityID: where.VulnerabilityID,
				PlaceIdentity: where.PlaceIdentity, Visibility: where.Visibility,
				ComponentUpstream: where.ComponentUpstream, ConsumerUpstream: where.ConsumerUpstream,
			},
			Reasoning: input.Body.Reasoning,
			By:        subject.ID,
		}, where.SeverityCenti)
		if err != nil {
			return nil, refusedDecision(err)
		}

		body := decisionBody(*made)
		body.Reasoning = input.Body.Reasoning
		body.NeedsApproval = made.NeedsApproval
		return &struct{ Body DecisionBody }{Body: body}, nil
	})
}

// asHTML renders stored markdown for a caller that asked for it.
//
// Markdown is what every consumer gets by default, because it is what an
// integrating application can most easily lay out and it reads as plain text
// as it stands. HTML assumes a browser, and in an API-first tool most callers
// are not one — so it is available on request and never the default.
//
// Rendered on the way out rather than stored, every time. A sanitizer rule
// written next year then applies to text written last year, which it could not
// if markup had been stored when the text arrived.
//
// A rendering that fails yields nothing rather than failing the request. The
// source is in the same answer and is the authoritative form; refusing to
// return a decision because one field could not be turned into markup would
// make a presentation problem into an outage.
func asHTML(ctx context.Context, in Ingest, want bool, source string) string {
	if !want || source == "" {
		return ""
	}
	markup, err := markdown.Render(ctx, source)
	if err != nil {
		in.Logger.Warn("could not render stored text as markup", "error", err)
		return ""
	}
	return markup
}
