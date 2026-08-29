package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
)

// Administering is what the endpoints for people and credentials need.
type Administering struct {
	Access  func() *access.Store
	Catalog func() *catalog.Store
	Logger  *slog.Logger
}

// PersonBody is somebody who has been granted access.
type PersonBody struct {
	Identity    string `json:"identity" minLength:"1" maxLength:"191" doc:"What a sign-in path calls them"`
	DisplayName string `json:"display_name,omitempty" doc:"What to show instead of the identity"`
	Admin       bool   `json:"admin,omitempty" doc:"Whether they administer this deployment"`
	// Holds is what they may do, listed as product and role.
	Holds []HeldBody `json:"holds,omitempty"`
}

// HeldBody is one role against one product.
type HeldBody struct {
	Product string `json:"product" minLength:"1" doc:"The product the role is held against"`
	Role    string `json:"role" enum:"reporting,approver,public-read,private-read,public-triage,private-triage" doc:"What they may do with it"`
}

// KeyBody is a pipeline credential, without its secret.
type KeyBody struct {
	Name    string `json:"name" minLength:"1" maxLength:"191" doc:"What this credential is for"`
	Product string `json:"product" minLength:"1" doc:"The product it may send scans for. Always required"`
	// Stream and Variant narrow it further. Either, both or neither may be
	// given: a key covering a whole product cannot imply which release an
	// upload is for, which is why an upload always states its own target.
	Stream  string `json:"stream,omitempty" doc:"Optionally, the one release it may send for"`
	Variant string `json:"variant,omitempty" doc:"Optionally, the one variant it may send for"`
	// Secret is returned when the credential is created, and never again.
	Secret     string `json:"secret,omitempty" doc:"Shown once, at creation. It is stored hashed and cannot be shown again"`
	LastUsedAt string `json:"last_used_at,omitempty" doc:"When it last sent something"`
	Revoked    bool   `json:"revoked,omitempty" doc:"Whether it has been withdrawn"`
}

func registerAdministration(api huma.API, a Administering) {
	huma.Register(api, huma.Operation{
		OperationID: "list-people", Method: http.MethodGet, Path: "/v1/people",
		Summary: "List who has been granted access",
		Description: "Everybody who may sign in, and what each of them holds. Nobody is here " +
			"because they authenticated: access is granted in advance or not at all.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[PersonBody], error) {
		store, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}
		people, held, err := store.People(ctx)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot list people", err)
		}

		products, err := names.Products(ctx, access.NewPerson(0, "", true, nil))
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot read the products roles are held against", err)
		}
		named := map[int64]string{}
		for _, product := range products {
			named[product.ID] = product.Name
		}

		out := &listOutput[PersonBody]{}
		out.Body.Items = make([]PersonBody, 0, len(people))
		for _, person := range people {
			body := PersonBody{
				Identity: person.Identity, DisplayName: person.DisplayName, Admin: person.IsAdmin,
			}
			for _, grant := range held[person.ID] {
				body.Holds = append(body.Holds, HeldBody{
					Product: named[grant.ProductID], Role: string(grant.Role),
				})
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "record-person", Method: http.MethodPost, Path: "/v1/people",
		Summary: "Grant somebody access",
		Description: "Records somebody so that they may sign in, and optionally what they hold. " +
			"Recording the same person again confirms them and adds any roles named.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body PersonBody
	}) (*declaredOutput[PersonBody], error) {
		store, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}

		_, lookupErr := store.ByIdentity(ctx, in.Body.Identity)
		created := lookupErr != nil

		person, err := store.Ensure(ctx, in.Body.Identity, in.Body.DisplayName, in.Body.Admin)
		if err != nil {
			return nil, huma.Error400BadRequest(err.Error())
		}

		for _, hold := range in.Body.Holds {
			product, err := names.ProductByName(ctx, hold.Product)
			if err != nil {
				return nil, huma.Error404NotFound(err.Error())
			}
			if err := store.GrantRole(ctx, person.ID, product.ID, access.Role(hold.Role)); err != nil {
				return nil, huma.Error400BadRequest(err.Error())
			}
		}
		return answer(created, in.Body), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "withdraw-role", Method: http.MethodDelete,
		Path:    "/v1/people/{identity}/roles/{product}/{role}",
		Summary: "Take a role away",
		Description: "A grant is a statement about now. Withdrawing one removes it rather than " +
			"marking it, because what somebody used to hold is answered by the record of what " +
			"they did.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		Identity string `path:"identity"`
		Product  string `path:"product"`
		Role     string `path:"role"`
	}) (*struct{}, error) {
		store, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}
		person, err := store.ByIdentity(ctx, in.Identity)
		if err != nil {
			return nil, huma.Error404NotFound("not declared")
		}
		product, err := names.ProductByName(ctx, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		if err := store.Withdraw(ctx, person.ID, product.ID, access.Role(in.Role)); err != nil {
			return nil, wentWrong(a.Logger, "cannot withdraw the role", err)
		}
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-keys", Method: http.MethodGet, Path: "/v1/keys",
		Summary: "List the pipeline credentials",
		Description: "Which credentials exist, what each may send, when it was last used and " +
			"whether it still works. The secrets are not here and cannot be: what is stored is a digest.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[KeyBody], error) {
		store, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}
		keys, err := store.Keys(ctx)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot list credentials", err)
		}

		out := &listOutput[KeyBody]{}
		out.Body.Items = make([]KeyBody, 0, len(keys))
		for _, key := range keys {
			body := KeyBody{Name: key.Name, Revoked: key.RevokedAt != nil}
			if product, err := names.ProductByID(ctx, key.ProductID); err == nil {
				body.Product = product.Name
			}
			if key.LastUsedAt != nil {
				body.LastUsedAt = key.LastUsedAt.UTC().Format(timeFormat)
			}
			out.Body.Items = append(out.Body.Items, body)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "create-key", Method: http.MethodPost, Path: "/v1/keys",
		Summary: "Issue a pipeline credential",
		Description: "Creates a credential a build may send scans with, and returns its secret. " +
			"The secret is shown once and stored hashed: a credential store that can hand back " +
			"what it holds gives up every pipeline's key with a copy of the database.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct {
		Body KeyBody
	}) (*declaredOutput[KeyBody], error) {
		store, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}

		product, err := names.ProductByName(ctx, in.Body.Product)
		if err != nil {
			return nil, huma.Error404NotFound(err.Error())
		}
		scope := access.Scope{ProductID: product.ID}

		if in.Body.Stream != "" {
			stream, err := names.StreamByName(ctx, product.ID, in.Body.Stream)
			if err != nil {
				return nil, huma.Error404NotFound(err.Error())
			}
			scope.StreamID = &stream.ID
		}
		if in.Body.Variant != "" {
			variant, err := names.VariantByName(ctx, product.ID, in.Body.Variant)
			if err != nil {
				return nil, huma.Error404NotFound(err.Error())
			}
			scope.VariantID = &variant.ID
		}

		key, secret, err := store.NewKey(ctx, in.Body.Name, scope)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot issue a credential", err)
		}
		return answer(true, KeyBody{
			Name: key.Name, Product: in.Body.Product, Stream: in.Body.Stream,
			Variant: in.Body.Variant, Secret: secret,
		}), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "revoke-key", Method: http.MethodDelete, Path: "/v1/keys/{name}",
		Summary: "Withdraw a pipeline credential",
		Description: "Stops it working, without removing the record of what it sent. Revoking one " +
			"credential leaves every other pipeline running.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		store, _, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}
		keys, err := store.Keys(ctx)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot read the credentials", err)
		}
		for _, key := range keys {
			if key.Name != in.Name || key.RevokedAt != nil {
				continue
			}
			if err := store.Revoke(ctx, key.ID); err != nil {
				return nil, wentWrong(a.Logger, "cannot withdraw the credential", err)
			}
			return nil, nil
		}
		return nil, huma.Error404NotFound("not declared")
	})
}

// timeFormat is how a moment is reported.
const timeFormat = "2006-01-02T15:04:05Z"

// administerable refuses anybody who is not an administrator, and hands back
// what the endpoint needs.
//
// Managing who may do what is the one thing that must never be reachable by a
// role granted on a product: somebody who may triage a product must not be
// able to grant themselves more of it.
func administerable(ctx context.Context, a Administering) (*access.Store, *catalog.Store, error) {
	if err := administrating(ctx); err != nil {
		return nil, nil, err
	}
	if a.Access == nil || a.Catalog == nil {
		return nil, nil, huma.Error500InternalServerError("this process cannot administer anything")
	}
	store, names := a.Access(), a.Catalog()
	if store == nil || names == nil {
		return nil, nil, huma.Error500InternalServerError("this process cannot administer anything")
	}
	return store, names, nil
}
