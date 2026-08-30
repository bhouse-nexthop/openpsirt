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
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// DecisionBody is a claim about a finding.
type DecisionBody struct {
	ID      int64  `json:"id,omitempty" doc:"What to name this decision in a later request"`
	Outcome string `json:"outcome" enum:"affected,not-applicable,deferred,wont-fix" doc:"What was decided"`
	// Justification is required for not-applicable and meaningless elsewhere:
	// the claim that something does not affect us is which of the recognized
	// reasons applies.
	Justification string `json:"justification,omitempty" enum:"component_not_present,vulnerable_code_not_present,vulnerable_code_not_in_execute_path,vulnerable_code_cannot_be_controlled_by_adversary,inline_mitigations_already_exist" doc:"Why it does not apply. Required when it does not"`
	DeferredUntil string `json:"deferred_until,omitempty" doc:"When a deferral returns, as a date. Required for a deferral"`
	Reasoning     string `json:"reasoning" minLength:"1" doc:"Why, in markdown. Somebody else has to agree with this"`
	State         string `json:"state,omitempty" enum:"proposed,approved,withdrawn,lapsed" doc:"Where it has got to"`
	// NeedsApproval says whether this is waiting for a second person. A short
	// deferral is not.
	NeedsApproval bool `json:"needs_approval,omitempty" doc:"Whether a second person has to agree before it takes effect"`
}

// PlaceBody names what a decision is about.
//
// Stated by the caller and checked against what is stored, rather than taken
// on trust: it is assembled from a finding, and a caller that could name a
// place freely would be choosing which decisions apply where.
type PlaceBody struct {
	Product       string `json:"product" minLength:"1"`
	Vulnerability string `json:"vulnerability" minLength:"1" doc:"The issue, by any name it is known under"`
	Place         string `json:"place" minLength:"1" doc:"Which place in the build, as the findings list gives it"`
}

// WaitingBody is one row of the review queue.
type WaitingBody struct {
	Decision DecisionBody `json:"decision"`
	Place    PlaceBody    `json:"place"`
	// Everything an approver needs to judge without opening it. A list where
	// judging a row means opening it is a list that gets approved unread.
	Reasoning          string `json:"reasoning"`
	PreviouslyApproved bool   `json:"previously_approved,omitempty" doc:"This was agreed to before and came back"`
	DeferredDays       int    `json:"deferred_days,omitempty" doc:"How long this finding has been put off in total"`
	ProposedBy         string `json:"proposed_by"`
	AgeDays            int    `json:"age_days" doc:"How long the claim has stood. An old judgment should look like one"`
}

// QueueOutput is a page of the review queue, with how much is behind it.
type QueueOutput struct {
	Body struct {
		Items []WaitingBody `json:"items"`
		// Total is how much work there is, which is not how much is shown. A
		// reviewer deciding whether to start needs the first number.
		Total int `json:"total"`
	}
}

func registerTriage(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-review-queue", Method: http.MethodGet, Path: "/v1/review-queue",
		Summary: "What is waiting to be agreed to",
		Description: "Claims somebody has made that need a second person. Each row carries what " +
			"an approver needs to judge it without opening it, because a list that has to be " +
			"opened row by row is a list that gets approved unread.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Limit  int `query:"limit" default:"50" minimum:"1" maximum:"200"`
		Offset int `query:"offset" minimum:"0"`
	}) (*QueueOutput, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}

		waiting, total, err := store.Queue(ctx, subject, input.Limit, input.Offset)
		if err != nil {
			return nil, wentWrong(in.Logger, "the review queue could not be read", err)
		}

		out := &QueueOutput{}
		out.Body.Items = make([]WaitingBody, 0, len(waiting))
		for _, row := range waiting {
			out.Body.Items = append(out.Body.Items, WaitingBody{
				Decision:           decisionBody(row.Decision),
				Reasoning:          row.Reasoning,
				PreviouslyApproved: row.PreviouslyApproved,
				DeferredDays:       int(row.DeferredSoFar.Hours() / 24),
				AgeDays:            int(store.Age(&row.Decision).Hours() / 24),
			})
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "approve-decision", Method: http.MethodPost, Path: "/v1/decisions/{id}/approval",
		Summary: "Agree to a claim",
		Description: "The person who made a claim may never be the one who agrees to it, with no " +
			"override — so a deployment with one person cannot approve anything, which is the " +
			"control working rather than a gap in it. An agreement names one revision of the " +
			"reasoning, so revising afterwards takes it back.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		Body struct {
			Batch string `json:"batch,omitempty" doc:"Name a batch to agree to many at once, so it can be undone as one"`
		}
	}) (*struct{}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		switch err := store.Approve(ctx, subject, input.ID, input.Body.Batch); {
		case errors.Is(err, triage.ErrSamePerson):
			return nil, huma.Error409Conflict(
				"the person who proposed a decision may not be the one who agrees to it")
		case errors.Is(err, triage.ErrNotTheirs):
			return nil, huma.Error404NotFound("no decision is recorded there")
		case err != nil:
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "revise-decision", Method: http.MethodPut, Path: "/v1/decisions/{id}/reasoning",
		Summary: "State the reasoning again",
		Description: "Keeps every revision and takes back any agreement standing on what it said " +
			"before — a second person agreed to particular words, and different words are a claim " +
			"nobody has agreed to. Needs no approval of its own: this puts risk back on the table " +
			"rather than taking it off.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		Body struct {
			Reasoning string `json:"reasoning" minLength:"1"`
		}
	}) (*struct{}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		if _, err := store.Revise(ctx, subject, input.ID, input.Body.Reasoning); err != nil {
			return nil, refusedDecision(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "withdraw-decision", Method: http.MethodDelete, Path: "/v1/decisions/{id}",
		Summary:       "Take a claim back",
		Description:   "Needs no approval. Putting risk back on the table never does.",
		Tags:          []string{"Triage"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID int64 `path:"id"`
	}) (*struct{}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		if err := store.Withdraw(ctx, subject, input.ID); err != nil {
			return nil, refusedDecision(err)
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "undo-batch", Method: http.MethodDelete, Path: "/v1/approval-batches/{batch}",
		Summary: "Undo everything one agreement covered",
		Description: "A reviewer may agree to a long selection in one action, so undoing has to be " +
			"available at the same size. The claims stand; it is the agreement that is taken back.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Batch string `path:"batch"`
	}) (*struct {
		Body struct {
			Undone int64 `json:"undone"`
		}
	}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		undone, err := store.UndoBatch(ctx, subject, input.Batch)
		if err != nil {
			return nil, wentWrong(in.Logger, "an agreement could not be undone", err)
		}
		out := &struct {
			Body struct {
				Undone int64 `json:"undone"`
			}
		}{}
		out.Body.Undone = undone
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "comment-on-decision", Method: http.MethodPost, Path: "/v1/decisions/{id}/comments",
		Summary: "Say something about a decision",
		Description: "A comment is not the reasoning and never disturbs an agreement. Annotating an " +
			"approved decision months later is ordinary.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		Body struct {
			Body string `json:"body" minLength:"1"`
		}
	}) (*struct {
		Body struct {
			ID int64 `json:"id"`
		}
	}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		comment, err := store.Say(ctx, subject, input.ID, input.Body.Body)
		if err != nil {
			return nil, refusedDecision(err)
		}
		out := &struct {
			Body struct {
				ID int64 `json:"id"`
			}
		}{}
		out.Body.ID = comment.ID
		return out, nil
	})
}

func registerProposing(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "decide", Method: http.MethodPost,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/decision",
		Summary: "Decide about a finding",
		Description: "One judgment about one issue in one component, at one place. It applies " +
			"wherever the same code combination appears — including in later releases, which pick " +
			"it up by looking it up rather than by copying it — and it stops applying when the " +
			"code it was a claim about changes.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability" doc:"The issue, by any name it is known under"`
		Place         string `path:"place" doc:"Which place, as the findings list gives it"`
		Body          DecisionBody
	}) (*struct{ Body DecisionBody }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}

		at, err := decidingAbout(ctx, in, subject, input.Product, input.Stream, input.Variant,
			input.Vulnerability, input.Place)
		if err != nil {
			return nil, err
		}

		proposal := triage.Proposal{
			Place: triage.Place{
				ProductID: at.ProductID, VulnerabilityID: at.VulnerabilityID,
				PlaceIdentity: at.PlaceIdentity, Visibility: at.Visibility,
				ComponentUpstream: at.ComponentUpstream, ConsumerUpstream: at.ConsumerUpstream,
			},
			Outcome:       triage.Outcome(input.Body.Outcome),
			Justification: triage.Justification(input.Body.Justification),
			Reasoning:     input.Body.Reasoning,
			By:            subject.ID,
			SeverityCenti: at.SeverityCenti,
		}
		if input.Body.DeferredUntil != "" {
			until, err := time.Parse(time.DateOnly, input.Body.DeferredUntil)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity("a deferral returns on a date, written as YYYY-MM-DD")
			}
			proposal.DeferredUntil = &until
		}

		// Asked before the claim is recorded, so the answer can say whether it
		// is waiting for anybody. A short deferral is ordinary triage and
		// takes effect at once.
		needs, err := store.NeedsApproval(ctx, proposal, deferralThreshold(ctx, in))
		if err != nil {
			return nil, wentWrong(in.Logger, "cannot tell whether that needs agreement", err)
		}

		decision, err := store.Propose(ctx, subject, proposal)
		if err != nil {
			return nil, refusedDecision(err)
		}

		body := decisionBody(*decision)
		body.Reasoning = input.Body.Reasoning
		body.NeedsApproval = needs
		return &struct{ Body DecisionBody }{Body: body}, nil
	})
}

// decidingAbout resolves the names in a path to the place a decision is made
// about, authorized on the way.
func decidingAbout(ctx context.Context, in Ingest, subject access.Subject,
	product, stream, variant, vulnerability, place string) (*finding.Deciding, error) {
	names := catalog.NewStore(in.DB.DB)
	named, err := names.LocateVisible(ctx, subject, product, stream, variant)
	if err != nil {
		return nil, huma.Error404NotFound(err.Error())
	}
	target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
	if err != nil {
		return nil, huma.Error404NotFound("nothing has been scanned there")
	}

	issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, vulnerability)
	if err != nil {
		return nil, huma.Error404NotFound("no issue is known by that name")
	}

	at, err := finding.NewStore(in.DB.DB).PlaceFor(ctx, subject, target.ID, issue, place)
	if err != nil {
		return nil, huma.Error404NotFound("no open finding is recorded there")
	}
	return at, nil
}

// triaging resolves who is asking and the store they act through.
func triaging(ctx context.Context, in Ingest) (access.Subject, *triage.Store, error) {
	subject, err := reading(ctx)
	if err != nil {
		return access.Subject{}, nil, err
	}
	if in.DB == nil {
		return access.Subject{}, nil, huma.Error500InternalServerError("this process cannot record decisions")
	}
	return subject, triage.NewStore(in.DB.DB), nil
}

// refused turns a store's refusal into an answer.
//
// A decision somebody may not reach answers as one that is not there, so that
// guessing identifiers says nothing. Everything a caller could have got right
// is reported, including where in their text to look.
func refusedDecision(err error) error {
	switch {
	case errors.Is(err, triage.ErrNotTheirs):
		return huma.Error404NotFound("no decision is recorded there")
	case errors.Is(err, triage.ErrSamePerson):
		return huma.Error409Conflict("the person who proposed a decision may not agree to it")
	}
	var faults markdown.Faults
	if errors.As(err, &faults) {
		return huma.Error422UnprocessableEntity(faults.Error())
	}
	return huma.Error422UnprocessableEntity(err.Error())
}

// decisionBody renders a decision as the API states it.
func decisionBody(d triage.Decision) DecisionBody {
	body := DecisionBody{
		ID: d.ID, Outcome: string(d.Outcome), State: string(d.State),
	}
	if d.Justification != nil {
		body.Justification = *d.Justification
	}
	if d.DeferredUntil != nil {
		body.DeferredUntil = d.DeferredUntil.Format(time.DateOnly)
	}
	return body
}

// deferralThreshold is how long a deferral may run before a second person has
// to agree, as this deployment has it set.
func deferralThreshold(ctx context.Context, in Ingest) time.Duration {
	const shipped = 30 * 24 * time.Hour
	if in.DB == nil {
		return shipped
	}
	threshold, err := setting.NewStore(in.DB.DB).Duration(ctx, setting.DeferralThreshold, shipped)
	if err != nil {
		return shipped
	}
	return threshold
}
