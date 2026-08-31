package httpapi

import (
	"context"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
)

// CanBody is one product somebody can reach, and what they may do in it.
//
// Named for the question it answers rather than for "reach", which already
// means something else here — how far a single judgment travels (TRI-30).
//
// What they *may do*, not which roles they hold. A screen needs to know
// whether to offer an action, and answering that from a role list means every
// client re-implementing the mapping from roles to capabilities — which is the
// server's rule, and the copy that drifts is the one that offers a button
// leading to a refusal.
type CanBody struct {
	Product   string `json:"product"`
	Name      string `json:"name" doc:"What to show for it"`
	MaySee    bool   `json:"may_see" doc:"Read findings that have been disclosed"`
	SeesAll   bool   `json:"sees_all" doc:"Read findings nobody has disclosed yet"`
	MayTriage bool   `json:"may_triage" doc:"Argue about a finding"`
	MayHide   bool   `json:"may_hide" doc:"Argue about a finding nobody has disclosed"`
	MayAgree  bool   `json:"may_agree" doc:"Agree to somebody else's claim"`
	MayReport bool   `json:"may_report" doc:"Read the reports"`
}

// WhoBody is the caller, as the caller.
type WhoBody struct {
	Identity string    `json:"identity" doc:"What this deployment calls them"`
	Name     string    `json:"name" doc:"What to show instead of the identity, where one is recorded"`
	Admin    bool      `json:"admin" doc:"Administers this deployment"`
	Kind     string    `json:"kind" enum:"person,key" doc:"A person who signed in, or a credential"`
	Reach    []CanBody `json:"reach" doc:"The products they can reach, and what they may do in each"`
}

func registerWhoAmI(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "get-current-subject", Method: http.MethodGet, Path: "/v1/session/me",
		Summary: "Describe whoever is asking",
		Description: "Returns the caller, the products they can reach, and what they may do in " +
			"each one.\n\n" +
			"It answers what a screen has to know before it draws: whether to offer an action " +
			"at all. Without it a client either hides nothing and lets people find the refusal, " +
			"or re-implements the mapping from roles to capabilities and drifts from the one " +
			"the server enforces.\n\n" +
			"Capabilities, not roles, for that reason. Which roles produce which capability is " +
			"the server's rule and stays there.",
		Tags: []string{"Access"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body WhoBody }, error) {
		subject, err := reading(ctx)
		if err != nil {
			return nil, err
		}

		body := WhoBody{
			Identity: subject.Identity, Admin: subject.Admin,
			Kind: string(subject.Kind), Reach: []CanBody{},
		}

		if in.DB == nil {
			return &struct{ Body WhoBody }{Body: body}, nil
		}

		// The display name, where one is recorded. Falls back to the identity
		// rather than to nothing, so a header never renders blank.
		if subject.Kind == access.Person {
			named, err := access.NewStore(in.DB.DB).Names(ctx, []int64{subject.ID})
			if err != nil {
				return nil, wentWrong(in.Logger, "who you are could not be read", err)
			}
			body.Name = named[subject.ID]
		}
		if body.Name == "" {
			body.Name = subject.Identity
		}

		// Named, not numbered. Every other endpoint takes a product by name,
		// so returning identifiers here would make this the one answer a
		// client has to translate before it can use it.
		products, err := catalog.NewStore(in.DB.DB).Products(ctx, subject)
		if err != nil {
			return nil, wentWrong(in.Logger, "what you can reach could not be read", err)
		}
		for _, product := range products {
			body.Reach = append(body.Reach, CanBody{
				Product: product.Name, Name: product.DisplayName,
				MaySee:  subject.Reads(access.Public, product.ID),
				SeesAll: subject.Reads(access.Private, product.ID),
				MayTriage: subject.Holds(access.PublicTriage, product.ID) ||
					subject.Holds(access.PrivateTriage, product.ID),
				MayHide:   subject.Holds(access.PrivateTriage, product.ID),
				MayAgree:  subject.Holds(access.Approver, product.ID),
				MayReport: subject.Holds(access.Reporting, product.ID),
			})
		}
		sort.Slice(body.Reach, func(i, j int) bool {
			return body.Reach[i].Product < body.Reach[j].Product
		})
		return &struct{ Body WhoBody }{Body: body}, nil
	})
}
