package httpapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// ModeBody is where this deployment's roles come from.
type ModeBody struct {
	Mode string `json:"mode" enum:"direct,group-bound" doc:"Whether an administrator assigns roles or provider groups derive them"`
}

// BindingBody is a provider group bound to a role.
type BindingBody struct {
	Group string `json:"group" minLength:"1" maxLength:"191" doc:"The group as the provider names it — a team slug, or a claim value"`
	// Product is absent where the binding carries administration, which is
	// global rather than held against a product.
	Product string `json:"product,omitempty" doc:"The product the role is held against"`
	Role    string `json:"role" enum:"reporting,approver,public-read,private-read,public-triage,private-triage,admin" doc:"What membership of this group grants"`
}

func registerBindings(api huma.API, a Administering, settings func() *setting.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "get-role-mode", Method: http.MethodGet, Path: "/v1/roles/mode",
		Summary: "Where roles come from",
		Description: "One mode for the whole deployment. A hybrid would need a precedence rule " +
			"for somebody holding one role from a team and another directly, which is how a stale " +
			"assignment outlives somebody's removal from the team it was shadowing.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, _ *struct{}) (*struct{ Body ModeBody }, error) {
		if _, _, err := administerable(ctx, a); err != nil {
			return nil, err
		}
		store := settings()
		if store == nil {
			return nil, huma.Error500InternalServerError("this process cannot read settings")
		}
		stored, _, err := store.Get(ctx, setting.RoleMode)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot read where roles come from", err)
		}
		return &struct{ Body ModeBody }{Body: ModeBody{Mode: string(access.AsMode(stored))}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-role-mode", Method: http.MethodPut, Path: "/v1/roles/mode",
		Summary: "Change where roles come from",
		Description: "Turning group binding on sets assignments aside rather than deleting them, " +
			"and turning it off restores them — so trying it is not a one-way door. Refused if it " +
			"would leave nobody able to administer this deployment.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, in *struct{ Body ModeBody }) (*struct{ Body ModeBody }, error) {
		rights, _, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}
		store := settings()
		if store == nil {
			return nil, huma.Error500InternalServerError("this process cannot write settings")
		}

		wanted := access.AsMode(in.Body.Mode)
		if string(wanted) != in.Body.Mode {
			return nil, huma.Error422UnprocessableEntity("that is not a way for roles to be assigned")
		}

		// Asked before the switch rather than after. A deployment that has
		// locked itself out of its own administration has one route back —
		// editing the database by hand — and refusing the change is cheaper
		// than discovering that afterwards.
		can, err := rights.CanAdminister(ctx, wanted)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot tell who would administer", err)
		}
		if !can {
			return nil, huma.Error409Conflict(
				"nothing would administer this deployment in that mode: bind a group to admin, " +
					"or name somebody in configuration, before switching")
		}

		if err := rights.SwitchTo(ctx, wanted); err != nil {
			return nil, wentWrong(a.Logger, "cannot change where roles come from", err)
		}
		if err := store.Set(ctx, setting.RoleMode, string(wanted)); err != nil {
			return nil, wentWrong(a.Logger, "cannot record where roles come from", err)
		}
		return &struct{ Body ModeBody }{Body: ModeBody{Mode: string(wanted)}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-bindings", Method: http.MethodGet, Path: "/v1/roles/bindings",
		Summary:     "What each group grants",
		Description: "The mappings that admit people in group-bound mode. In that mode a mapping is the advance authorization: somebody arriving for the first time in a mapped group is admitted, and somebody in none is refused.",
		Tags:        []string{"Administration"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[BindingBody], error) {
		rights, _, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}

		bindings, err := rights.Bindings(ctx)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot list the group bindings", err)
		}
		named, err := productNames(ctx, a)
		if err != nil {
			return nil, err
		}

		out := &listOutput[BindingBody]{}
		out.Body.Items = make([]BindingBody, 0, len(bindings))
		for _, binding := range bindings {
			out.Body.Items = append(out.Body.Items, BindingBody{
				Group: binding.GroupName, Product: named[binding.ProductID], Role: string(binding.Role),
			})
		}

		administering, err := rights.AdminGroups(ctx)
		if err != nil {
			return nil, wentWrong(a.Logger, "cannot list which groups administer", err)
		}
		for _, group := range administering {
			out.Body.Items = append(out.Body.Items, BindingBody{Group: group, Role: adminRole})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "bind-group", Method: http.MethodPost, Path: "/v1/roles/bindings",
		Summary: "Bind a group to a role",
		Description: "Administration is bound without a product, because it is global rather than " +
			"held against one. Everything else names the product it applies to.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *struct{ Body BindingBody }) (*struct{ Body BindingBody }, error) {
		rights, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}

		if in.Body.Role == adminRole {
			if in.Body.Product != "" {
				return nil, huma.Error422UnprocessableEntity(
					"administration is not held against a product, so a group bound to it names none")
			}
			if err := rights.BindAdmin(ctx, in.Body.Group); err != nil {
				return nil, wentWrong(a.Logger, "cannot bind a group to administration", err)
			}
			return &struct{ Body BindingBody }{Body: in.Body}, nil
		}

		role := access.Role(in.Body.Role)
		if !role.Valid() {
			return nil, huma.Error422UnprocessableEntity("that is not a role")
		}
		product, err := names.ProductByName(ctx, in.Body.Product)
		if err != nil {
			return nil, huma.Error404NotFound("no product is declared by that name")
		}
		if err := rights.Bind(ctx, in.Body.Group, product.ID, role); err != nil {
			return nil, wentWrong(a.Logger, "cannot bind a group", err)
		}
		return &struct{ Body BindingBody }{Body: in.Body}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "unbind-group", Method: http.MethodDelete, Path: "/v1/roles/bindings",
		Summary: "Stop a group granting something",
		Description: "Takes effect at each member's next sign-in, because membership is read then " +
			"and never again. To cut somebody off now, end their sessions.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, in *struct {
		Group   string `query:"group" required:"true"`
		Product string `query:"product"`
		Role    string `query:"role" required:"true"`
	}) (*struct{}, error) {
		rights, names, err := administerable(ctx, a)
		if err != nil {
			return nil, err
		}

		if in.Role == adminRole {
			// Refused where it would leave nobody able to administer, for the
			// same reason the mode change is.
			if err := rights.UnbindAdmin(ctx, in.Group); err != nil {
				return nil, wentWrong(a.Logger, "cannot unbind a group from administration", err)
			}
			if err := stillAdministrable(ctx, rights, a, settings); err != nil {
				// Put back, so a refusal does not half-apply.
				if restored := rights.BindAdmin(ctx, in.Group); restored != nil {
					return nil, wentWrong(a.Logger, "cannot restore an administration binding", restored)
				}
				return nil, err
			}
			return &struct{}{}, nil
		}

		product, err := names.ProductByName(ctx, in.Product)
		if err != nil {
			return nil, huma.Error404NotFound("no product is declared by that name")
		}
		if err := rights.Unbind(ctx, in.Group, product.ID, access.Role(in.Role)); err != nil {
			return nil, wentWrong(a.Logger, "cannot unbind a group", err)
		}
		return &struct{}{}, nil
	})
}

// adminRole is how administration is named in a binding. It is not a role held
// against a product, so it is not one of the roles.
const adminRole = "admin"

// stillAdministrable refuses a change that would leave nobody able to
// administer this deployment.
//
// Asked against the mode actually in force. Unbinding the last administrators'
// group matters while roles are derived from groups and does not while they
// are assigned, and refusing in both would make a deployment that has never
// turned group binding on unable to tidy up a mapping it is not using.
func stillAdministrable(ctx context.Context, rights *access.Store, a Administering, settings func() *setting.Store) error {
	mode := access.Direct
	if store := settings(); store != nil {
		stored, _, err := store.Get(ctx, setting.RoleMode)
		if err != nil {
			return wentWrong(a.Logger, "cannot read where roles come from", err)
		}
		mode = access.AsMode(stored)
	}

	can, err := rights.CanAdminister(ctx, mode)
	if err != nil {
		return wentWrong(a.Logger, "cannot tell who would administer", err)
	}
	if !can {
		return huma.Error409Conflict(
			"that was the last thing granting administration: bind another group to admin, " +
				"or name somebody in configuration, first")
	}
	return nil
}

// productNames maps product rows to the names bindings state them by.
func productNames(ctx context.Context, a Administering) (map[int64]string, error) {
	names := a.Catalog()
	products, err := names.Products(ctx, access.NewPerson(0, "", true, nil))
	if err != nil {
		return nil, wentWrong(a.Logger, "cannot read the products roles are held against", err)
	}
	named := map[int64]string{}
	for _, product := range products {
		named[product.ID] = product.Name
	}
	return named, nil
}
