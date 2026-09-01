# Interface

The web interface, how it is built, and how it reaches the server.

Satisfies UIX-01 to UIX-05, UIX-07, UIX-08, UIX-11, UIX-14, UIX-16, UIX-18 to
UIX-23, UIX-25 to UIX-27, UIX-30 to UIX-32, UIX-34 to UIX-37, API-17, ACC-56
to ACC-59. What is not built is named at the end rather than left to be found
by clicking.

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

**Filtering is the server's, not the browser's.** A list narrowed after it
arrives is narrowed within one page of it, so "hide the kernel" would hide the
kernel from the twenty rows already fetched and from nothing else. The filters
are query parameters, which also puts them in the URL where a link carries
them.

**The findings list has a second view: by component.** The default asks "what
is wrong", grouped by issue; this asks "what is wrong *with this thing*", which
is the question somebody upgrading a package has. Same data, different subject.

**A triage line is announced where it applies, never silently.** Where a
product or the deployment has said what is worth triaging (TRI-43), the list
says so and says how many it is not showing. A list that quietly omits things
is a list that lies about how much there is.

**The dependency tree is a tree, and its counts are cumulative.** Each row
carries what is open beneath it as well as on it, so a container reads as the
sum of what it holds rather than as zero — which is what it reported before,
and what made the screen useless for punching down. Those totals are worked out
when the tree is read rather than stored after a scan, because they are derived
from findings and findings move: something dismissed, something assigned, a
rating reconsidered. A stored total is right at the moment a scan ends and
drifts from the screen beside it thereafter.

Branches are ordered by name rather than by that total. Ordering by it was
tried and reverted: an edge means "contains *or* depends on" and the document
does not distinguish them, so forty kernel-module packages each depending on
the one kernel all reported its total and filled the first screen — putting the
containers out of sight again, which is the same fault arrived at from the
other side.

**The finding screen is where deciding happens**, not a screen that links to
deciding. It carries what the issue is, how bad, what upstream has done, where
it sits in the build, the evidence, and both the Decide and the Assess cards.
Deciding reuses the same outcome, editor and reach components the standalone
decision screen uses, rather than a second form that drifts from the first.

**Where it sits shows the chain, not the immediate parent.** The same parent
can be reached by several routes, and a screen naming only the nearest one
cannot tell them apart. Where nothing records what pulls a component in, it
says that — rather than claiming the product itself does, which was a
comfortable sentence and not a true one.

**"Who is working on what" is three tabs, and the third is somewhere else.**
Nobody-assigned already had its own entry in the rail, so the tab links across
rather than drawing the same list twice.

**Release comparison carries a chart across every build**, not only the two
being compared: the comparison answers what changed between two, and the chart
answers whether it is getting better or worse. It is bars rather than a line,
because these are separate builds and a line between two releases draws a trend
through a gap where nothing happened.

**A setting whose value is one of a few words is a select, not a text box.**
A free field invites a value the server then refuses — and for a switch it
invites "true", "yes" and "1", none of which are what it takes.

**The scans screen says what the numbers were measured against** — which
scanner, at which version, reading which vulnerability database. Without it a
build with nothing wrong and a build last measured against a months-old
database read identically.

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

## Writing, and what carries it

The editor is a formatting toolbar over a plain textarea with Write and Preview
tabs — not a rich-text editor. What is stored is markdown, and an editor that
hides that eventually disagrees with what gets published. Every toolbar control
is a button rather than a keyboard shortcut, because it has to work on a phone.

**Preview is the published rendering**, not a second one that resembles it.
Both go through the renderer above.

**Unsent text is kept and restored.** A draft is written to the browser as
somebody types and cleared only once the server has actually taken it — so a
refused submission, an expired session and a closed tab all leave the words
where they were. Losing what somebody wrote is what teaches people to write
less, and the reasoning is the part of a decision that matters most.

Restoring is deliberately narrow: only into an empty field, and only once. A
draft that overwrote something a caller supplied would lose the thing it exists
to protect.

Storage that a browser refuses is not a failure. The draft is a convenience;
the text in front of somebody is the real thing, so every read and write of it
tolerates being turned down.

## What the initial load carries

Screens are split by route. The findings list has to stay usable against a
full-size product and has no business downloading a charting library, and the
markdown renderer is only needed where somebody reads or writes a
justification.

Measured: one bundle of 820 KB became a 248 KB initial load, with the chart
(369 KB) and the renderer (146 KB) fetched only by the screens that use them.

## Mentions offer only people who can already see it

Autocomplete after an `@` offers the people who can read findings of that
visibility in that product, and nobody else.

An autocomplete listing everybody teaches somebody to name a colleague who then
cannot open what they were called to. On an undisclosed finding it is worse
than unhelpful: the mention itself says a finding exists, which is the
disclosure the visibility rule is there to prevent.

**Asking who may be mentioned on an undisclosed finding is itself a question
about undisclosed findings**, so somebody who cannot read them is answered as
though the product were not there — the same answer every other path gives.
Without that second half, the endpoint is a way to enumerate who holds private
access, which is a more useful thing to steal than the list it is attached to.

What is being typed after an `@` is read from the text before the cursor rather
than tracked as state, so it stays right however somebody edits — pasting,
deleting, or clicking elsewhere in the line.

## Running it locally

    make demo                       # build, start, seed, and say where to go
    make demo DEMO_HOST=yourbox     # if you browse by something but localhost

`make demo` is the whole of it. It builds the interface and the binary, starts
both, declares somewhere to file scans against, sends the full-size fixture
from `internal/sbom/testdata`, and prints the address. `make demo-status` says
whether the scan has landed and how much it found; `make demo-down` stops it;
`make demo-reset` throws the database away.

Seeding is idempotent — declaring something that exists succeeds and changes
nothing, and a document already ingested is recognized rather than read again —
so it can be run repeatedly without tearing anything down.

Four things it settles, each of which is a wrong answer somebody would
otherwise arrive at by experiment:

**Signing in without an identity provider.** The server supports a trusted
header — a deployment behind a proxy that authenticates for it — and in
development the Vite dev server is that proxy. It injects the header only when
`OPENPSIRT_DEV_USER` is set, and that only means anything if the server was
started trusting the header from that address. Two deliberate settings, neither
of which a real deployment has, in a file that configures the dev server rather
than the thing that serves the built interface.

The header's identity is prefixed by the path it arrives on, so `X-User: dev`
becomes `proxy:dev` — and that is the name the administrator has to be
bootstrapped under.

**A write needs an `Origin` the server recognizes.** The trusted-header path
carries no session to hold an echoed token, so the forgery guard falls back to
where the request came from, and answers a state-changing request only when it
has been told which origins it serves. The browser's origin is the dev
server's, not the API's, so `OPENPSIRT_BASE_URL` names the former. Reads work
without this and writes return 401 — a confusing way to find out, which is why
`DEMO_HOST` sets all three places at once.

**The dev server refuses a Host it does not know.** That is protection against
a hostile page resolving a name to this machine, so browsing by anything but
localhost has to name the host — a hostname rather than a wildcard, so the
protection still means something.

**Findings need a scanner.** Without `grype` on the path the upload is
accepted, the graph is stored, and the scan step fails with a receipt saying
so. That is the honest outcome and it is what the scans list is for, but the
triage screens stay empty until one is installed.

### What it is not

A demonstration deployment, not a small production one. It serves plain HTTP,
trusts a header from loopback, and hands administration to whoever sets that
header. Every one of those is a hole; together they are a machine somebody can
click around on.

## What the order is, and why it has to show its working

The findings list is ordered by urgency: known-exploited, then whether the
build reaches customers, then severity, then likelihood. Every one of those is
on the row, and that is not decoration.

An order that sorts on something it does not show reads as no order at all. The
first version showed only the severity word, and the top of a real list came
out "high, high, medium, medium, medium, high, high, critical" — correct, and
indistinguishable from unsorted. The first five were known-exploited and
nothing said so.

Two things in particular have to be visible:

**Known-exploited**, as its own badge rather than by replacing the severity
word. Replacing it answers one question by destroying another: an exploited
medium is still a medium, and the reader needs both facts to see why it sits
above an unexploited high.

**The score, beside the word.** They come from different places and can tie
while the words differ — a 2003 issue scored 10.0 reads "high" under CVSS v2
and "critical" under v3. Two rows tied at 10.0 with different words look
mis-sorted until the number is there. Genuine disagreement between word and
number is rare, measured at 3 of 2,645; the vocabulary difference is not.

**Where we have rated something ourselves, that is what orders the list** —
and both ratings are shown (TRI-42). Ours is what ranks, because being able to
say a published rating is wrong is pointless if everything that sorts and
filters then ignores us. The world's stays beside it, because a rating of ours
standing where the world's goes reads as the world's, and the first person to
check against the public record finds a discrepancy nobody declared.

Rating something **worse** takes effect at once; rating it **milder** waits for
a second person. That is not evenness for its own sake — milder is the
direction that hides things, and it hides more than a position in a list.
Severity sets the deadline, so calling a high a low pushes its deadline out by
months, and where a product has said what is worth triaging at all, a downgrade
across that line takes the finding off the working list and off any clock
entirely.

## Not built yet

Named so that what is missing is a plan rather than something rediscovered by
clicking.

**Editing a comment after it is written.** The endpoint exists; nothing calls
it, so a typo stands.

**Undoing a bulk approval as a batch.** Each claim can be revised on its own,
which is the slow way to undo sixty.

**A mention links nobody.** The editor offers the right candidates and writes
`@name` into the text, but the renderer treats it as ordinary words. UIX-24
wants a mention and a finding reference to become links, and that is resolution
the server has to do, because it needs to know what the reader may see.
Notifying somebody who was mentioned is a notification (Stage 6) rather than a
screen.

**The carry-forward preview.** What a decision will cover when a build moves is
a hint sentence rather than the panel the mockup draws.

**"How the reasoning changed" and "what has been said" are only on the decision
screen**, and are not reachable from the finding that led to them.

What is *no longer* here is worth stating, because this section claimed it for
a while after it stopped being true: release comparison, people and access, and
the settings screen are all built, and the catalogue screens can declare as
well as list.
