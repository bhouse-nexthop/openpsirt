package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// SettingBody is one thing a deployment has decided for everybody in it.
type SettingBody struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Default bool   `json:"default,omitempty" doc:"Nobody has set this; the shipped value is in use"`
	Means   string `json:"means" doc:"What it decides"`
}

// settable is every setting an operator may change, with what it decides.
//
// A list rather than anything the store will accept, so that adding a setting
// is a deliberate act and a typo in a name is refused instead of quietly
// creating a setting nothing reads.
var settable = []struct {
	name  string
	means string
}{
	{setting.DueExploited, "How long a known-exploited finding may stay open. Its own window, and the shortest: severity is how bad a flaw is, being exploited is a fact about the world"},
	{setting.DueCritical, "How long a critical may stay open"},
	{setting.DueHigh, "How long a high may stay open"},
	{setting.DueMedium, "How long a medium may stay open"},
	{setting.DueLow, "How long a low may stay open"},
	{setting.DeferralThreshold, "How long something may be put off before a second person has to agree. Measured against everything the finding has already been put off for, not against the postponement being asked for"},
	{setting.SessionLifetime, "How long a sign-in lasts"},
	{setting.MaxTokenLifetime, "The longest a personal token may be valid for"},
}

func registerSettings(api huma.API, in Ingest) {
	huma.Register(api, huma.Operation{
		OperationID: "list-settings", Method: http.MethodGet, Path: "/v1/settings",
		Summary: "List what this deployment has decided",
		Description: "Returns every setting an operator may change, its value, and what it " +
			"decides. `default` means nobody has set it and the shipped value is in use.\n\n" +
			"The shipped numbers are a starting point rather than a recommendation. What a " +
			"deployment can hold to is a question about that deployment, and a deadline nobody " +
			"agreed to produces an estate that is permanently late and a signal everybody " +
			"ignores.",
		Tags: []string{"Administration"},
	}, func(ctx context.Context, _ *struct{}) (*listOutput[SettingBody], error) {
		if err := administrating(ctx); err != nil {
			return nil, err
		}
		settings := setting.NewStore(in.DB.DB)
		out := &listOutput[SettingBody]{}
		out.Body.Items = make([]SettingBody, 0, len(settable))
		for _, each := range settable {
			value, set, err := settings.Get(ctx, each.name)
			if err != nil {
				return nil, wentWrong(in.Logger, "the settings could not be read", err)
			}
			if !set {
				value = shipped(each.name)
			}
			out.Body.Items = append(out.Body.Items, SettingBody{
				Name: each.name, Value: value, Default: !set, Means: each.means,
			})
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "set-setting", Method: http.MethodPut, Path: "/v1/settings/{name}",
		Summary: "Change something for this deployment",
		Description: "Sets one value for everybody here. Durations are written the way Go writes " +
			"them — `72h`, `30m` — and a value that cannot be read is refused rather than " +
			"stored, since a setting nothing can parse is a policy silently reverting to the " +
			"shipped one.\n\n" +
			"Only the settings this deployment recognizes may be set. A name it does not know is " +
			"refused, because storing it would create something nothing ever reads.",
		Tags: []string{"Administration"}, DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
		Body struct {
			Value string `json:"value" minLength:"1"`
		}
	}) (*struct{}, error) {
		if err := administrating(ctx); err != nil {
			return nil, err
		}
		known := false
		for _, each := range settable {
			if each.name == input.Name {
				known = true
				break
			}
		}
		if !known {
			return nil, huma.Error404NotFound("this deployment has no setting by that name")
		}
		// Checked before it is stored. A duration nothing can read would leave
		// every caller falling back to the shipped value, which is a policy
		// that quietly stopped applying.
		if _, err := time.ParseDuration(input.Body.Value); err != nil {
			return nil, huma.Error422UnprocessableEntity(
				fmt.Sprintf("%q is not a length of time — write it as 72h, 30m or 45s",
					input.Body.Value))
		}
		if err := setting.NewStore(in.DB.DB).Set(ctx, input.Name, input.Body.Value); err != nil {
			return nil, wentWrong(in.Logger, "that setting could not be recorded", err)
		}
		return &struct{}{}, nil
	})
}

// shipped is the value in force when nobody has set one.
func shipped(name string) string {
	switch name {
	case setting.DueExploited:
		return "72h"
	case setting.DueCritical:
		return "168h"
	case setting.DueHigh:
		return "720h"
	case setting.DueMedium:
		return "2160h"
	case setting.DueLow:
		return "4320h"
	case setting.DeferralThreshold:
		return "720h"
	case setting.SessionLifetime:
		return "12h"
	case setting.MaxTokenLifetime:
		return "2160h"
	}
	return ""
}
