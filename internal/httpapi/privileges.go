package httpapi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Privileges renders the page saying who may call what.
//
// Built from the operations the server registered rather than maintained
// beside them, so the chain runs from the code that enforces a right to the
// page somebody reads before asking for one, with nothing in between that has
// to be remembered.
func Privileges(api huma.API) string {
	type row struct{ method, path, needs, note string }
	byScope := map[string][]row{}

	spec := api.OpenAPI()
	for path, item := range spec.Paths {
		for method, op := range map[string]*huma.Operation{
			"GET": item.Get, "PUT": item.Put, "POST": item.Post,
			"DELETE": item.Delete, "PATCH": item.Patch,
		} {
			if op == nil || op.OperationID == "" {
				continue
			}
			asks, ok := op.Extensions[requiresExtension].(requires)
			if !ok {
				continue
			}
			needs := strings.Join(asks.AnyOf, ", ")
			if needs == "" {
				needs = "—"
			}
			byScope[asks.Scope] = append(byScope[asks.Scope],
				row{method, path, needs, asks.Note})
		}
	}

	var out strings.Builder
	out.WriteString(privilegesHeader)
	for _, scope := range []string{deploymentWide, perProduct, anyProduct, ownSubject, anySubject} {
		rows := byScope[scope]
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].path != rows[j].path {
				return rows[i].path < rows[j].path
			}
			return rows[i].method < rows[j].method
		})
		fmt.Fprintf(&out, "\n## %s\n\n%s\n\n", scopeTitles[scope], scopeBlurbs[scope])
		out.WriteString("| | Endpoint | Roles | |\n|---|---|---|---|\n")
		for _, r := range rows {
			fmt.Fprintf(&out, "| %s | `%s` | %s | %s |\n", r.method, r.path, r.needs, r.note)
		}
	}
	return out.String()
}

var scopeTitles = map[string]string{
	deploymentWide: "Administrator",
	perProduct:     "A role on the product",
	anyProduct:     "A role on any product",
	ownSubject:     "Your own",
	anySubject:     "Any credential",
}

var scopeBlurbs = map[string]string{
	deploymentWide: "Held for the deployment rather than per product. Holding every " +
		"product role there is does not amount to any of it.",
	perProduct: "Granted per product. Any one of the roles listed is enough — a dash " +
		"means any role on that product will do — and what you may see narrows what the " +
		"answer contains.",
	anyProduct: "Not about a product at all: a rating is a claim about an issue, true " +
		"wherever it appears, so there is no product to hold a role on. The role is asked " +
		"for anywhere instead, because being signed in is not a role.",
	ownSubject: "About whoever is asking. No role is needed and none helps.",
	anySubject: "Any credential this deployment recognizes. The answer is narrowed to what " +
		"you may see, so two people calling the same endpoint get different answers " +
		"rather than one of them getting an error.",
}

const privilegesHeader = `# Privileges

Who may call what. **Generated from the API document**, which is generated from
the server — so this cannot describe a rule the code does not enforce, or miss
an endpoint it serves.

Access is granted in advance or not at all: no account is created by signing
in. A role is held per product except where noted, and being able to *see*
something is never the same as being able to change it.

## The roles

| Role | What it allows |
|---|---|
| ` + "`public-read`" + ` | Read findings that have been disclosed |
| ` + "`private-read`" + ` | Read findings nobody has announced |
| ` + "`public-triage`" + ` | Argue about disclosed findings: decide, revise, withdraw, comment. Take work nobody owns, and hand back your own |
| ` + "`private-triage`" + ` | The same for findings nobody has announced, including recording one and moving its disclosure date |
| ` + "`approver`" + ` | Agree to somebody else's claim. A triager may also approve, and the proposer may never approve their own |
| ` + "`assigner`" + ` | Give work to somebody else, or take what they are holding |
| ` + "`reporting`" + ` | Read the reports for a product |
| administrator | The deployment: people, roles, credentials, settings, and the catalog |

Two rules are not roles and cannot be granted. **Visibility narrows every
answer**, so an endpoint you may call still shows only what you may see. And
**the proposer of a claim may never approve it**, whoever they are.
`
