package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
	"github.com/bhouse-nexthop/openpsirt/internal/notify"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
	"github.com/bhouse-nexthop/openpsirt/internal/triage"
)

// DecisionBody is a claim about a finding.
type DecisionBody struct {
	ID int64 `json:"id,omitempty" doc:"What to name this decision in a later request"`
	// ClaimID is the action this row was written by. The review queue lists
	// claims and approval works on them; a decision is one row of one.
	ClaimID int64  `json:"claim_id,omitempty" doc:"The claim this decision is one row of: the action that wrote it, which is what the review queue lists and what is approved"`
	Outcome string `json:"outcome" enum:"affected,not-applicable,deferred,wont-fix" doc:"What was decided"`
	// Justification is required for not-applicable and meaningless elsewhere:
	// the claim that something does not affect us is which of the recognized
	// reasons applies.
	Justification string `json:"justification,omitempty" enum:"component_not_present,vulnerable_code_not_present,vulnerable_code_not_in_execute_path,vulnerable_code_cannot_be_controlled_by_adversary,inline_mitigations_already_exist" doc:"Why it does not apply. Required when it does not"`
	// Mitigation is the one claim here that rests on configuration rather
	// than on code, so it is the one thing nothing will notice going away.
	// Naming it does not fix that; it makes the claim checkable (TRI-39).
	Mitigation    string `json:"mitigation,omitempty" doc:"What actually stops it — the rule, the setting, the service that is not exposed. Required when the reason is that mitigations already exist, and refused with any other"`
	DeferredUntil string `json:"deferred_until,omitempty" doc:"When a deferral returns, as a date. Required for a deferral"`
	Reasoning     string `json:"reasoning" minLength:"1" doc:"Why, in markdown. Somebody else has to agree with this"`
	State         string `json:"state,omitempty" enum:"proposed,approved,withdrawn,lapsed" doc:"Where it has got to"`
	// NeedsApproval says whether this is waiting for a second person. A short
	// deferral is not.
	NeedsApproval bool `json:"needs_approval,omitempty" doc:"Whether a second person has to agree before it takes effect"`
	// Places is how many findings this one judgment covers. A kernel issue
	// reaches dozens of modules and the answer is usually the same for all of
	// them, so whoever is deciding is told the size of what they are deciding.
	Places int `json:"places,omitempty" doc:"How many findings this decision covers"`
	// Versions is how many distinct versions sit at this place. More than one
	// means a single decision cannot honestly cover all of them.
	Versions int `json:"versions,omitempty" doc:"How many versions of the component sit here. More than one needs care"`
	// SentBackAt is when an approver last asked for more before they would
	// agree. Reported, because otherwise the only trace of it is a comment,
	// and the author's own list cannot tell a claim waiting on somebody else
	// from one waiting on them.
	SentBackAt string `json:"sent_back_at,omitempty" doc:"When an approver last asked for more. Empty means nobody has"`
	// SelectedBy is how the set was narrowed, where this claim was one of many
	// recorded in a single action. Reported so that "how were these chosen"
	// has an answer months later.
	SelectedBy string `json:"selected_by,omitempty" doc:"How the set was narrowed, for a claim recorded as one of many. Never part of the claim itself"`
}

// FindingRefBody is what a decision is about, as the findings list would
// show it: the build to link to, the issue, the component and where it sits.
type FindingRefBody struct {
	Product       string  `json:"product" doc:"The build to link to, by product, branch or tag, and variant"`
	Stream        string  `json:"stream"`
	Variant       string  `json:"variant"`
	Vulnerability string  `json:"vulnerability" doc:"The issue, under the name it is most widely known by"`
	Component     string  `json:"component"`
	Version       string  `json:"version" doc:"The version that ships"`
	Severity      string  `json:"severity,omitempty" doc:"Our rating where one stands, else as published"`
	Score         float64 `json:"score,omitempty"`
	Exploited     bool    `json:"exploited,omitempty"`
	FixState      string  `json:"fix_state,omitempty" enum:"fixed,none,wont-fix"`
	FixedIn       string  `json:"fixed_in,omitempty"`
	Description   string  `json:"description,omitempty" doc:"The first four hundred characters of what the report says, as plain text"`
	Owner         string  `json:"owner,omitempty" doc:"The part of the product this belongs to"`
	Parent        string  `json:"parent,omitempty" doc:"What directly pulls it in, which is what the decision is about"`
	Places        int     `json:"places" doc:"How many places the issue sits at in that component in that build"`
	Decided       int     `json:"decided" doc:"How many of those this claim covers"`
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

// ClaimBody is one proposer's action: what the review queue lists and what an
// approver agrees to.
type ClaimBody struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind" enum:"finding,together,extension,returned" doc:"What sort of action it was: one judgment about a finding, one about many issues at a component, an approved claim carried to a new issue, or rows an approver set aside"`
	DerivedFrom int64  `json:"derived_from,omitempty" doc:"The claim this one came from, for an extension or a returned set"`
	ProposedBy  string `json:"proposed_by"`
	ProposedAt  string `json:"proposed_at" doc:"When the action was taken, as a date and time"`
	SelectedBy  string `json:"selected_by,omitempty" doc:"How a bulk set was narrowed. Never part of the claim itself"`
}

// WaitingBody is one entry of the review queue: one claim, with what an
// approver needs to judge it.
type WaitingBody struct {
	Claim ClaimBody `json:"claim"`
	// Decision is a representative row of the claim — the earliest — and
	// Place is where it sits. A claim over many issues has many; the counts
	// below say how many.
	Decision DecisionBody `json:"decision"`
	Place    PlaceBody    `json:"place"`
	// Everything an approver needs to judge without opening it. A list where
	// judging a row means opening it is a list that gets approved unread.
	Reasoning          string `json:"reasoning"`
	PreviouslyApproved bool   `json:"previously_approved,omitempty" doc:"This was agreed to before and came back"`
	DeferredDays       int    `json:"deferred_days,omitempty" doc:"How long this finding has been put off in total"`
	ProposedBy         string `json:"proposed_by"`
	AgeDays            int    `json:"age_days" doc:"How long the claim has stood. An old judgment should look like one"`
	Decisions          int    `json:"decisions" doc:"How many rows the claim wrote"`
	Issues             int    `json:"issues" doc:"How many distinct issues it covers"`
	Places             int    `json:"places" doc:"How many distinct places it covers"`
	// Builds is every build the claim's rows currently cover, by matching.
	Builds []string `json:"builds" doc:"Every build the claim currently covers, as stream and variant"`
	// Outliers is what in a bulk set does not look like the rest. Only for a
	// claim over many issues.
	Outliers *OutliersBody `json:"outliers,omitempty" doc:"For a claim over many issues: the rows that do not look like the rest, and how many there are"`
	// Finding is what the representative decision is about: the build to
	// link to, the issue, the component and where it sits (TRI-09). For a
	// claim over many issues it describes the component and the build, with
	// the representative issue.
	Finding *FindingRefBody `json:"finding,omitempty" doc:"What the representative decision is about — build, issue, component, where it sits — so the card can be judged without opening it. Absent where no open finding sits at its place"`
}

// OutliersBody is what an approver of a bulk claim checks instead of reading
// every row.
type OutliersBody struct {
	Exploited int           `json:"exploited" doc:"Issues in the set known to be exploited"`
	Severe    int           `json:"severe" doc:"Issues rated critical or high"`
	Fixable   int           `json:"fixable" doc:"Issues a fix is available for"`
	Unmatched int           `json:"unmatched" doc:"Issues whose description does not carry the term the set was narrowed by"`
	Rows      []OutlierBody `json:"rows" doc:"The issues that stood out, exploited first and then by severity, at most twenty"`
}

// OutlierBody is one issue in a bulk claim that does not look like the rest.
type OutlierBody struct {
	DecisionID    int64    `json:"decision_id" doc:"A row of the claim about this issue, to set aside when approving"`
	Vulnerability string   `json:"vulnerability"`
	Severity      string   `json:"severity,omitempty"`
	Exploited     bool     `json:"exploited,omitempty"`
	FixedIn       string   `json:"fixed_in,omitempty"`
	Description   string   `json:"description,omitempty" doc:"The first two hundred characters of what the report says"`
	Why           []string `json:"why" doc:"Which of the four signals made it stand out"`
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
		Summary: "List claims awaiting approval",
		Description: "Returns the claims waiting for a second person, newest first, limited to " +
			"what you may approve every row of. One entry is one claim — one proposer's action, " +
			"however many decisions it wrote — with a representative decision and place, how " +
			"many rows, issues and places it covers, and every build it currently reaches.\n\n" +
			"Each entry carries the full reasoning, whether it was previously approved and came " +
			"back, how long the finding has been deferred in total, and how old the claim is. A " +
			"claim over many issues also carries `outliers`: the rows that do not look like the " +
			"rest, which is what to read instead of all of them.\n\n" +
			"Approve, send back or set rows aside with `POST /v1/claims/{id}/approval` and " +
			"`POST /v1/claims/{id}/send-back`.",
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

		// Which finding each row is about, and who made the claim, resolved
		// here rather than left as identifiers. A queue row saying product 4,
		// issue 91 is a row an approver has to make two more requests to
		// understand, fifty times a page.
		decisions := make([]triage.Decision, 0, len(waiting))
		for _, row := range waiting {
			decisions = append(decisions, row.Decision)
		}
		named, err := describeDecisions(ctx, in, store, decisions, nil)
		if err != nil {
			return nil, wentWrong(in.Logger, "the review queue could not be read", err)
		}

		out := &QueueOutput{}
		out.Body.Items = make([]WaitingBody, 0, len(waiting))
		for i, row := range waiting {
			entry := WaitingBody{
				Claim:              claimBody(row.Claim, named[i].ProposedBy),
				Decision:           decisionBody(row.Decision),
				Place:              named[i].Place,
				Reasoning:          row.Reasoning,
				PreviouslyApproved: row.PreviouslyApproved,
				DeferredDays:       int(row.DeferredSoFar.Hours() / 24),
				ProposedBy:         named[i].ProposedBy,
				AgeDays:            int(store.Age(&row.Decision).Hours() / 24),
				Decisions:          row.Decisions,
				Issues:             row.Issues,
				Places:             row.Places,
				Builds:             row.Builds,
			}
			if entry.Builds == nil {
				entry.Builds = []string{}
			}
			if row.Outliers != nil {
				entry.Outliers = outliersBody(*row.Outliers)
			}
			entry.Finding = named[i].Finding
			out.Body.Items = append(out.Body.Items, entry)
		}
		out.Body.Total = total
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "approve-decision", Method: http.MethodPost, Path: "/v1/decisions/{id}/approval",
		Summary: "Approve a triage decision",
		Description: "Approves a proposed decision. Returns 409 if you proposed it yourself — the " +
			"proposer and approver must always be different people, and there is no override.\n\n" +
			"An approval is recorded against the specific revision of the justification that " +
			"exists now. Editing the justification afterwards withdraws the approval and returns " +
			"the decision to this queue.\n\n" +
			"Pass `batch` to approve many decisions under one name, so they can be undone " +
			"together with `DELETE /v1/approval-batches/{batch}`.",
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
			return nil, noSuchDecision()
		case err != nil:
			return nil, huma.Error422UnprocessableEntity(err.Error())
		}
		return &struct{}{}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "revise-decision", Method: http.MethodPut, Path: "/v1/decisions/{id}/reasoning",
		Summary: "Update a decision's justification",
		Description: "Replaces the justification text with a new revision. Earlier revisions are " +
			"kept and remain readable.\n\n" +
			"**This withdraws any existing approval** and returns the decision to the review " +
			"queue, marked as previously approved. Requires no approval of its own.\n\n" +
			"The text is markdown and is validated before it is stored; a 422 names the line and " +
			"the offending text.",
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
		Summary: "Withdraw a triage decision",
		Description: "Withdraws the decision so it no longer applies to any finding. The record " +
			"is kept — a withdrawn decision reads as proposed, approved, then withdrawn. " +
			"Requires no approval.",
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
		Summary: "Undo a bulk approval",
		Description: "Withdraws every approval recorded under this batch name, returning those " +
			"decisions to the review queue. The decisions themselves stand — only the approvals " +
			"are undone. Returns how many were affected.\n\n" +
			"Only decisions on products you may triage are touched.",
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
		Summary: "Add a comment to a decision",
		Description: "Adds a markdown comment to a decision. Comments are separate from the " +
			"justification and never affect an approval, so an approved decision can be annotated " +
			"at any time.\n\n" +
			"A comment may later be edited by its author only, and editing overwrites it rather " +
			"than keeping revisions.",
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
		Summary: "Record a triage decision for a finding",
		Description: "Records how a finding was triaged: `affected`, `not-applicable`, `deferred` " +
			"or `wont-fix`.\n\n" +
			"`not-applicable` requires a `justification` from the standard VEX vocabulary. " +
			"`deferred` requires `deferred_until` as a date.\n\n" +
			"The decision applies to every build running the same component and consumer upstream " +
			"versions, including future releases — it is matched by code, not copied between " +
			"releases. It stops applying automatically when either upstream version changes.\n\n" +
			"The response says whether a second person must approve it. Most outcomes require " +
			"approval; a deferral shorter than the configured threshold does not.\n\n" +
			"It also says how many findings this one judgment covers, and how many distinct " +
			"versions of the component sit at this place — more than one means a single " +
			"decision cannot honestly cover all of them.",
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
			Mitigation:    input.Body.Mitigation,
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
		threshold, err := deferralThreshold(ctx, in)
		if err != nil {
			return nil, wentWrong(in.Logger, "cannot tell whether that needs agreement", err)
		}
		needs, err := store.NeedsApproval(ctx, proposal, threshold)
		if err != nil {
			return nil, wentWrong(in.Logger, "cannot tell whether that needs agreement", err)
		}
		// Recorded on the claim, not merely reported back. A claim that says
		// it is waiting for somebody and is stored as needing nobody takes
		// effect the moment it is made, and the answer telling the caller it
		// was waiting is the only trace of a control that did not run.
		proposal.NeedsApproval = needs

		decision, err := store.Propose(ctx, subject, proposal)
		if err != nil {
			return nil, refusedDecision(err)
		}

		body := decisionBody(*decision)
		body.Reasoning = input.Body.Reasoning
		body.NeedsApproval = needs
		// How much this one judgment covers, so nobody discovers afterwards
		// that they answered for sixty-two modules or for two versions of the
		// same package.
		body.Places, body.Versions = at.Places, at.Versions()
		return &struct{ Body DecisionBody }{Body: body}, nil
	})
}

// FindingDecisionBody is one judgment about a finding, and which of its places
// it covers.
type FindingDecisionBody struct {
	Outcome       string `json:"outcome" enum:"affected,not-applicable,deferred,wont-fix"`
	Justification string `json:"justification,omitempty" enum:"component_not_present,vulnerable_code_not_present,vulnerable_code_not_in_execute_path,vulnerable_code_cannot_be_controlled_by_adversary,inline_mitigations_already_exist" doc:"Why it does not apply. Required when it does not"`
	Mitigation    string `json:"mitigation,omitempty" doc:"What actually stops it — the rule, the setting, the service that is not exposed. Required when the reason is that mitigations already exist, and refused with any other"`
	DeferredUntil string `json:"deferred_until,omitempty" doc:"Required when it is deferred. A date, as 2026-03-31"`
	Reasoning     string `json:"reasoning" minLength:"1" doc:"Why this holds"`
	// Places is the deliberate narrowing. Absent means every place, which is
	// the default TRI-29 asks for.
	Places []string `json:"places,omitempty" doc:"Which places this covers, as the finding names them. Omit for all of them"`
	// Extends names an approved claim this one carries to a new issue. The
	// outcome and justification have to be the source's, and the places have
	// to be ones the source sits at.
	Extends int64 `json:"extends,omitempty" doc:"An approved claim at the same component and consumer to carry to this issue. The outcome and justification must match it; the new claim is recorded as an extension of it and still needs a second person"`
}

// DecidedBody is what one judgment about a finding recorded.
type DecidedBody struct {
	ClaimID       int64   `json:"claim_id" doc:"The claim this action made, which is what the review queue lists and what is approved"`
	Recorded      int     `json:"recorded" doc:"How many places it was written against"`
	Covered       int     `json:"covered" doc:"How many findings those places hold"`
	Left          int     `json:"left" doc:"Places of this finding left open, because they were not named"`
	NeedsApproval bool    `json:"needs_approval" doc:"Whether a second person has to agree"`
	IDs           []int64 `json:"ids"`
}

func registerFindingDecision(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "decide-finding", Method: http.MethodPost,
		Path: "/v1/products/{product}/streams/{stream}/variants/{variant}" +
			"/findings/{vulnerability}/components/{component}/decision",
		Summary: "Record one judgment about a finding, covering its places",
		Description: "Records the same claim against every place this issue occupies in this " +
			"component. Naming `places` narrows it; leaving it out covers all of them, which " +
			"is the default a judgment should have — a kernel flaw reaches sixty modules and " +
			"the answer is almost always the same for all of them, so asking sixty times " +
			"guarantees somebody stops reading.\n\n" +
			"A place left out stays **open**. Nothing is recorded against it and nothing is " +
			"asked about it: a component used unsafely in one consumer and not another is " +
			"exactly what per-place findings exist to capture, and demanding a justification " +
			"for the places you did not answer is the tool arguing with a judgment it asked " +
			"for.\n\n" +
			"One action still writes one record per place, each keyed and expiring on its own " +
			"(REL-02), so a place that later diverges is not silently covered by a decision " +
			"nobody made about it.\n\n" +
			"The ordinary approval rules apply however many places this reaches. Always " +
			"needing a second person is about a claim covering **several issues** nobody read " +
			"one by one; this is one claim about one issue (TRI-38).\n\n" +
			"Pass `extends` to carry an approved claim to this issue: the source must be " +
			"approved, sit at the same component under the same consumer, and the outcome and " +
			"justification must match it. The new claim is recorded as an extension of it and " +
			"still waits for a second person. `similar` on `GET .../findings/{vulnerability}/" +
			"components/{component}` lists the claims that qualify.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability" doc:"The issue, by any name it is known under"`
		Component     string `path:"component" doc:"The component, as the findings list gives it"`
		Version       string `query:"version" doc:"Which version, where the build ships more than one under that name"`
		Body          FindingDecisionBody
	}) (*struct{ Body DecidedBody }, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}

		target, issue, component, err := findingAbout(ctx, in, subject,
			input.Product, input.Stream, input.Variant,
			input.Vulnerability, input.Component, input.Version)
		if err != nil {
			return nil, err
		}
		all, err := finding.NewStore(in.DB.DB).PlacesFor(ctx, subject, target, issue, component)
		if err != nil || len(all) == 0 {
			return nil, noSuchFinding()
		}

		// Narrowed to what was named, and a name nothing matches is refused
		// rather than ignored: somebody who meant to cover six places and
		// mistyped one should not quietly cover five.
		places := all
		if len(input.Body.Places) > 0 {
			wanted := map[string]bool{}
			for _, name := range input.Body.Places {
				wanted[name] = true
			}
			places = make([]finding.Deciding, 0, len(wanted))
			for _, place := range all {
				if wanted[place.PlaceIdentity] {
					places = append(places, place)
					delete(wanted, place.PlaceIdentity)
				}
			}
			if len(wanted) > 0 {
				return nil, huma.Error422UnprocessableEntity(
					"this finding does not sit at every place you named")
			}
		}

		var until *time.Time
		if input.Body.DeferredUntil != "" {
			when, err := time.Parse(time.DateOnly, input.Body.DeferredUntil)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(
					"a deferral returns on a date, written as YYYY-MM-DD")
			}
			until = &when
		}

		threshold, err := deferralThreshold(ctx, in)
		if err != nil {
			return nil, wentWrong(in.Logger, "cannot tell whether that needs agreement", err)
		}

		out := &struct{ Body DecidedBody }{}
		proposals := make([]triage.Proposal, 0, len(places))
		for _, place := range places {
			proposal := triage.Proposal{
				Place: triage.Place{
					ProductID: place.ProductID, VulnerabilityID: place.VulnerabilityID,
					PlaceIdentity: place.PlaceIdentity, Visibility: place.Visibility,
					ComponentUpstream: place.ComponentUpstream,
					ConsumerUpstream:  place.ConsumerUpstream,
				},
				Outcome:       triage.Outcome(input.Body.Outcome),
				Justification: triage.Justification(input.Body.Justification),
				Mitigation:    input.Body.Mitigation,
				Reasoning:     input.Body.Reasoning,
				By:            subject.ID,
				SeverityCenti: place.SeverityCenti,
				DeferredUntil: until,
			}
			// Asked per place rather than once for the set. The threshold
			// reads the claim, and two places of one finding can differ in
			// what they carry — one answer for all of them would report a
			// control that did not run on some.
			needs, err := store.NeedsApproval(ctx, proposal, threshold)
			if err != nil {
				return nil, wentWrong(in.Logger, "cannot tell whether that needs agreement", err)
			}
			proposal.NeedsApproval = needs
			out.Body.NeedsApproval = out.Body.NeedsApproval || needs
			out.Body.Covered += place.Places
			proposals = append(proposals, proposal)
		}

		// One action, one transaction. Half the places written and the rest
		// abandoned leaves a finding neither answered nor open, with nothing
		// saying which places were which.
		var recorded []*triage.Decision
		if input.Body.Extends != 0 {
			recorded, err = store.Extend(ctx, subject, input.Body.Extends, proposals)
		} else {
			recorded, err = store.ProposeMany(ctx, subject, proposals)
		}
		if err != nil {
			return nil, refusedDecision(err)
		}
		for _, decision := range recorded {
			out.Body.IDs = append(out.Body.IDs, decision.ID)
			out.Body.ClaimID = decision.ClaimID
		}
		out.Body.Recorded = len(recorded)
		out.Body.Left = len(all) - out.Body.Recorded
		return out, nil
	})
}

// findingAbout resolves the names in a path to one finding: the build, the
// issue and the component it is open against, authorized on the way.
//
// Name and version together, because a name alone is not unique — a real image
// ships three vendored versions of one library, and resolving the name on its
// own answers about whichever was interned first.
func findingAbout(ctx context.Context, in Ingest, subject access.Subject,
	product, stream, variant, vulnerability, component, version string) (int64, int64, int64, error) {

	names := catalog.NewStore(in.DB.DB)
	named, err := names.LocateVisible(ctx, subject, product, stream, variant)
	if err != nil {
		return 0, 0, 0, noSuchProduct()
	}
	target, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
	if err != nil {
		return 0, 0, 0, nothingScannedThere()
	}
	issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, vulnerability)
	if err != nil {
		return 0, 0, 0, noSuchIssue()
	}
	at, err := graph.NewStore(in.DB.DB).ComponentVersionAt(ctx, target.ID, component, version)
	if err != nil {
		return 0, 0, 0, ambiguousOrMissing(err)
	}
	return target.ID, issue, at, nil
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
		return nil, nothingScannedThere()
	}

	issue, err := finding.NewVulnerabilities(in.DB.DB).ByName(ctx, vulnerability)
	if err != nil {
		return nil, noSuchIssue()
	}

	at, err := finding.NewStore(in.DB.DB).PlaceFor(ctx, subject, target.ID, issue, place)
	if err != nil {
		return nil, noSuchFinding()
	}
	return at, nil
}

// ReachBody is how far a judgment made here would travel.
type ReachBody struct {
	Here      int         `json:"here" doc:"Places in this build the judgment covers"`
	Automatic []MatchBody `json:"automatic" doc:"Other builds it reaches by matching. Nothing to agree to"`
	Differing []MatchBody `json:"differing" doc:"Same issue, another version. Each is a separate judgment"`
}

func reachBody(r finding.Reach) ReachBody {
	body := ReachBody{
		Here:      r.Here,
		Automatic: make([]MatchBody, 0, len(r.Automatic)),
		Differing: make([]MatchBody, 0, len(r.Differing)),
	}
	for _, m := range r.Automatic {
		body.Automatic = append(body.Automatic, MatchBody{
			Stream: m.Stream, Variant: m.Variant, Version: m.ComponentUpstream, Places: m.Places,
		})
	}
	for _, m := range r.Differing {
		body.Differing = append(body.Differing, MatchBody{
			Stream: m.Stream, Variant: m.Variant, Version: m.ComponentUpstream, Places: m.Places,
		})
	}
	return body
}

// MatchBody is the same issue at the same place in another build.
type MatchBody struct {
	Stream  string `json:"stream"`
	Variant string `json:"variant"`
	// Version is what that build has, and why this is a separate question.
	// Where it matched, the decision already reaches there and nobody is
	// asked.
	Version string `json:"version,omitempty" doc:"The upstream version that build has, which differs from this one"`
	Places  int    `json:"places" doc:"How many places it sits at there"`
}

func registerElsewhere(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "get-decision-reach", Method: http.MethodGet,
		Path:    "/v1/products/{product}/streams/{stream}/variants/{variant}/findings/{vulnerability}/places/{place}/reach",
		Summary: "Show how far a decision here would reach",
		Description: "Returns the three parts of what a judgment made here covers.\n\n" +
			"`here` is how many places in this build. `automatic` are other builds it reaches " +
			"without anybody doing anything, because their upstream versions and chains already " +
			"match — a decision is a claim about a combination of code, not about a release. " +
			"`differing` hold the same issue at the same place at another version, so each is a " +
			"separate judgment.\n\n" +
			"Only `differing` is a choice. The first two follow from the matching rules and are " +
			"there to be told, not agreed to — and showing them as one number is how a decision " +
			"comes to reach builds the person making it never knew about.",
		Tags: []string{"Triage"},
	}, func(ctx context.Context, input *struct {
		Product       string `path:"product"`
		Stream        string `path:"stream"`
		Variant       string `path:"variant"`
		Vulnerability string `path:"vulnerability"`
		Place         string `path:"place"`
	}) (*struct{ Body ReachBody }, error) {
		subject, _, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		at, err := decidingAbout(ctx, in, subject, input.Product, input.Stream, input.Variant,
			input.Vulnerability, input.Place)
		if err != nil {
			return nil, err
		}

		names := catalog.NewStore(in.DB.DB)
		named, err := names.LocateVisible(ctx, subject, input.Product, input.Stream, input.Variant)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		here, err := names.ExistingTarget(ctx, named.StreamID, named.VariantID)
		if err != nil {
			return nil, nothingScannedThere()
		}

		reach, err := finding.NewStore(in.DB.DB).Reaching(ctx, subject, *at, here.ID)
		if err != nil {
			return nil, wentWrong(in.Logger, "cannot look for the same issue elsewhere", err)
		}
		return &struct{ Body ReachBody }{Body: reachBody(reach)}, nil
	})
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
		return noSuchDecision()
	case errors.Is(err, triage.ErrSamePerson):
		return huma.Error409Conflict("the person who proposed a decision may not agree to it")
	}
	var faults markdown.Faults
	if errors.As(err, &faults) {
		return refusedText(faults)
	}
	return huma.Error422UnprocessableEntity(err.Error())
}

// refusedText answers a refused piece of writing with where to look.
//
// Each fault travels as its own detail, carrying the line and the text that
// caused it. Flattening them into one sentence is what this used to do, and it
// leaves an interface with nothing to point at: "remote images are not
// allowed" against a forty-line justification means somebody hunting for it by
// eye, which is the whole reason positions are gathered in the first place.
func refusedText(faults markdown.Faults) error {
	details := make([]error, 0, len(faults))
	for _, fault := range faults {
		details = append(details, &huma.ErrorDetail{
			Message: fault.Reason,
			// Where in the submitted text, not where in the request body. A
			// client is pointing a cursor at a line somebody typed.
			Location: fmt.Sprintf("line %d", fault.Line),
			Value:    fault.Offending,
		})
	}
	return huma.Error422UnprocessableEntity(
		"that text cannot be stored as written", details...)
}

// decisionBody renders a decision as the API states it.
func decisionBody(d triage.Decision) DecisionBody {
	body := DecisionBody{
		ID: d.ID, ClaimID: d.ClaimID, Outcome: string(d.Outcome), State: string(d.State),
	}
	if d.Mitigation != nil {
		body.Mitigation = *d.Mitigation
	}
	if d.Justification != nil {
		body.Justification = *d.Justification
	}
	if d.DeferredUntil != nil {
		body.DeferredUntil = d.DeferredUntil.Format(time.DateOnly)
	}
	if d.SentBackAt != nil {
		body.SentBackAt = d.SentBackAt.UTC().Format(time.RFC3339)
	}
	if d.SelectedBy != nil {
		body.SelectedBy = *d.SelectedBy
	}
	return body
}

// deferralThreshold is how long a deferral may run before a second person has
// to agree, as this deployment has it set.
//
// A failure to read it is reported rather than answered with the shipped
// default. This decides which deferrals need a second person, so quietly
// substituting a different threshold substitutes a different control — and a
// deployment that had tightened it would find it loosened at exactly the
// moment its database was in trouble, with nothing saying so. A setting nobody
// has changed is a different matter, and answers with the default.
func deferralThreshold(ctx context.Context, in Ingest) (time.Duration, error) {
	const shipped = 30 * 24 * time.Hour
	if in.DB == nil {
		return shipped, nil
	}
	return setting.NewStore(in.DB.DB).Duration(ctx, setting.DeferralThreshold, shipped)
}

func registerSendBack(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "send-decision-back", Method: http.MethodPost,
		Path:    "/v1/decisions/{id}/send-back",
		Summary: "Send a decision back for more",
		Description: "Asks the author for more before agreeing. The claim leaves the review " +
			"queue and comes back when they revise it.\n\n" +
			"`because` is required and is recorded as a comment, because that is what it is — " +
			"the author needs the words, and a reason kept anywhere else is one nobody reads. " +
			"Sending something back without saying what is missing is a round trip nobody learns " +
			"from.\n\n" +
			"Needs no approval of its own: it puts risk back on the table rather than taking it " +
			"off. You cannot send back a claim whose current words are your own — that is yours " +
			"to revise.",
		Tags: []string{"Triage"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		ID   int64 `path:"id"`
		Body struct {
			Because string `json:"because" minLength:"1" doc:"What needs to change, in markdown"`
		}
	}) (*struct{}, error) {
		subject, store, err := triaging(ctx, in)
		if err != nil {
			return nil, err
		}
		author, err := store.SendBack(ctx, subject, input.ID, input.Body.Because)
		if err != nil {
			return nil, refusedDecision(err)
		}

		// A rejected dismissal goes straight back into its author's queue, so
		// silence would leave it sitting there while they wait to hear
		// (NTF-05). Logged rather than returned on failure: it has been sent
		// back either way, and answering with an error invites a second one.
		if err := notify.NewStore(in.DB.DB).Tell(ctx, notify.Telling{
			PersonID: author, Kind: notify.SentBack,
			Body: "A claim of yours was sent back: " + input.Body.Because,
			Link: "/decisions/" + strconv.FormatInt(input.ID, 10),
		}); err != nil && in.Logger != nil {
			in.Logger.Error("could not say that a claim was sent back",
				"error", err, "decision", input.ID)
		}
		return &struct{}{}, nil
	})
}
