# Interface

The web interface, how it is built, and how it reaches the server.

Satisfies UIX-01, UIX-07, UIX-11, UIX-16, UIX-18 to UIX-20, UIX-22, UIX-34,
UIX-37, API-17, ACC-56 to ACC-58. What is built so far is the shell, sign-in,
the product list and the findings list; the rest of Stage 5 is named at the
end.

## One artifact

The built interface is embedded into the binary and served from it. A
deployment is one container, there is no second thing to deploy, and the
interface cannot be a version behind the API it is talking to — which is the
failure that makes a client generated from an OpenAPI document worth having in
the first place.

Nothing is fetched at run time. No font, no script, no stylesheet from
anywhere, so an air-gapped install is an ordinary install rather than a
configuration.

### The embed needs a directory that always exists

`//go:embed` fails to compile when its target is missing, and the built output
is not in the repository. So `internal/webui/dist` is tracked, empty, and the
frontend build fills it.

That took two rules to state rather than one. The repository ignores `dist/`
everywhere, and git does not descend into an excluded directory — so a
negation nested inside one never applies, and the placeholder has to be
un-ignored at the top level along with the directory holding it. The build
target removes the build output by name for the same reason: clearing the
directory wholesale would delete the tracked files that let a fresh clone
compile.

**A binary built without the interface serves the API alone.** The embed is
read for `index.html` and yields nothing without one, so a checkout with no
node toolchain still builds and runs. That is a supported way to build this,
not a broken one.

## What the server hands to a browser

Three kinds of path, and the difference matters:

| | |
|---|---|
| **A route the server has** | Answered by the server, credential required as always |
| **A file in the built output** | Served as itself, cached by its content-hashed name |
| **Anything else** | The page, which does its own routing |

A single-page application owns its routing, so a path this server has never
heard of is not a mistake — it is a route the page knows. Answering 404 would
make every deep link fail on reload while working perfectly when navigated to.

**Except under the API.** Anything beginning `/v1` is answered by the server
even when it has no such route, because handing a page back to a client
parsing JSON reports a mistyped endpoint as a parse failure — a long way from
the mistake that caused it.

### The page loads without a credential; nothing else changes

The sign-in screen *is* the page, so the page has to load for somebody holding
nothing. What is served is a compiled application and its assets, carrying no
data.

The rule is expressed as **"the router has no route for this"**, asked of the
router, rather than as "the path is outside `/v1`". The prefix version is the
tempting one and it is wrong: the framework registers routes of its own — the
API document and the schemas it references — and a prefix rule hands those to
anybody who asks. That had already happened once before this interface
existed, which is why the credential check is a deny-by-default list rather
than a guarded prefix.

**Names the server owns are reserved even when nothing is routed there.** The
framework's documentation route is disabled by configuration, so it is
unrouted — and unrouted is exactly what marks a path as the page's. Without a
reserved list the interface would have silently claimed `/docs`, and turning
that route back on later would have been shadowed. A test asserts that
mounting the interface opens nothing: the API document, the schema routes and
every `/v1` path still refuse a stranger.

## The client is generated, never written

The API client comes from the committed OpenAPI document. No path and no
response shape is hand-written, so an endpoint that changes shape is a compile
error rather than a screen rendering `undefined`.

`make web-api` regenerates it and fails if the result differs from what is
committed, the same way the OpenAPI document itself is checked against the
server. Two generated artifacts, each gated against drifting from its source.

It has already earned that: the generated types say a list may be `null`,
which the hand-written version of the same screen would have discovered in a
browser.

### Reaching the server

The session is a cookie the browser holds, and nothing in the application ever
sees it. What the application does hold is the cross-site-request token, which
is deliberately readable by script where the session cookie is not — echoing it
is what distinguishes a request our page made from one somebody else's page
caused.

Every unsafe request carries it, attached once as middleware rather than per
call. The one call somebody forgets is the one that breaks in production and
not in review.

## A screen knows what it may offer before it draws

`GET /v1/session/me` answers who is asking and what they may do in each product
they can reach.

**Capabilities, not roles.** A screen needs to know whether to offer an action;
answering that from a list of roles means every client re-implementing the
mapping from roles to capabilities, and the copy that drifts is the one
offering a button that leads to a refusal. Which roles produce which capability
is the server's rule and stays there.

A product somebody cannot reach is simply absent, which a screen treats as
not-there rather than as forbidden — the same answer the server gives.

### Signing in

`GET /v1/sign-in` lists the providers an operator configured, and is answered
without a credential because it is what somebody sees before they have one. It
is the only reading endpoint outside the probes that is. What it discloses is
names an operator chose and nothing about whether any account exists, which is
the disclosure that would matter — and a test asserts exactly that rather than
assuming it.

A 401 from `session/me` is an answer rather than a failure: it means nobody is
signed in, which is what a fresh browser looks like. The shell offers a way in
instead of showing somebody an error about their own not being signed in.

## Rendering happens here

Markdown is rendered and sanitized in the browser. The server returns markdown
and only markdown (`DESIGN-api.md`), so preview and published text are the same
renderer rather than two that agree by luck, and there is no round trip per
keystroke.

What the server keeps is the half a client cannot do: the policy at submission.
That split, and what it moves, is set out in `DESIGN-text.md`.

## Screens

**The product list is the first pick.** The findings list is scoped to one
product and everything below it is bound to that (UIX-07), so choosing one is a
screen rather than a dropdown that silently changes what a number means.

**The findings list is one row per issue-and-component**, not per place
(UIX-01). A real image produced 335,021 individual findings that collapse to
7,906 rows, so the grouping is not a nicety: ungrouped, it is six thousand
screens of rows differing in a column nobody reads. Each row says how many
places it covers, because one judgment covering sixty places is a different act
from one covering one (UIX-34).

Paging lives in the URL, so a link carries what somebody is looking at
(UIX-11).

**Narrow screens get cards, not a table that scrolls sideways** (UIX-16). The
same data, laid out for the device, rather than a desktop table a phone cannot
use.

**Severity never borrows the accent colour.** It has its own scale, and
exploited is a band of its own above critical — a page that paints "critical"
in the brand colour has nothing left that means "act on this". Being exploited
outranks whatever the score says, which is the ordering everywhere else in this
tool.

## Choices the decisions did not cover

**Colour and the brand mark resolve through tokens in one place.** How an
operator overrides them is deliberately unsettled — that gets decided against
real screens rather than in the abstract — but keeping the whole palette in one
stylesheet means the answer will be a stylesheet, whatever it turns out to be,
rather than a hunt through components.

**Dependencies are pinned exactly, not by range.** The Makefile already says
every tool is pinned because a range resolves at build time and CI stops being
reproducible; a caret in a manifest does exactly that. `npm ci` installs the
lockfile.

**A failure shows what the server said.** Inventing a friendlier sentence hides
the one the server wrote, which names the line to fix or which part of a
declaration is missing — and is the more useful of the two.

## Not built yet

Named so that what is missing is a plan rather than something rediscovered by
clicking: the dependency tree (UIX-02 to UIX-04), the finding detail (UIX-14),
the review queue, the home page assembled from what somebody holds (UIX-05,
UIX-08), and the markdown editor with its toolbar, preview and mention
autocomplete (UIX-21, UIX-23, UIX-25, UIX-30 to UIX-33).

Mentions need server work that does not exist: UIX-25 wants autocomplete
offering only people who can see the finding, and there is no mention handling
anywhere and no endpoint to ask. That lands with the editor.
