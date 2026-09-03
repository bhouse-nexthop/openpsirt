# Interface

The web interface, how it is built, and how it reaches the server.

Satisfies UIX-01 to UIX-05, UIX-07, UIX-08, UIX-11, UIX-12, UIX-14, UIX-16,
UIX-18 to UIX-23, UIX-25 to UIX-27, UIX-30 to UIX-32, UIX-34 to UIX-51,
API-17, ACC-56 to ACC-59, the half of ING-41 that is shown rather than
collected, and the interface half of TRI-45 to TRI-47. What is not built is
named at the end rather than left to be found by clicking.

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

## The look, and the three of them

The tokens, the type scale, the shell and every component come from the
restyled mockup, taken as they were settled rather than approximated (UIX-50).
The mockup drew the interface inside a frame; here the frame is the page, so
its tokens sit on the root element and its grid on the application container.
That is the whole of the translation, and it is why a screen here can be put
beside its mockup and compared control for control.

**Three looks, one markup.** A look is a token set — colors, the two
typefaces, radii, shadows — and nothing else. The first is a dark rail over a
light surface; the second is all light and hairline, denser; the third is dark
throughout. The choice is made from the menu on the avatar, is kept in the
browser, and changes nothing anybody else sees, the same rule as saved filters.
Fonts are bundled, so nothing is fetched at run time.

**Severity never borrows the accent.** Each look has its own accent and the
same five-band severity scale beside it, with exploited above critical. A page
that paints "critical" in the brand color has nothing left that means "act on
this."

**Labels use the conventional word** (UIX-47): Reject, Trend, Assignments,
Unassigned, Justification, Path, EPSS, Locations, Users and roles, Lapsed
decisions, Submit. A caption on a screen is a sentence at most; the reasoning
behind a control lives in the decision record.

## The shell

A rail down the side carries the brand and the entries grouped by what they
span; a bar across the top carries what you are looking at, a way to find
things, a way to upload, what is waiting on you, and who you are.

The rail's three groups are the point. "Across products" holds home, the review
queue, what is unassigned and the assignments. The named build holds the
findings, the dependency tree, the inventories and the comparison — the
comparison belongs to the product rather than to one build, and is reached
here because this is where somebody already is. "Manage" holds the catalog,
the users and the settings; branches, tags and variants have entries of their
own, scoped to the picked product, rather than being reachable only through a
product's row.

**A build-only entry declines rather than opening on a scope that means
nothing.** With a product, a branch or a variant unpicked, the findings, tree
and inventories entries are disabled and say why (UIX-39); the picker itself
already refuses "all" on those screens.

**The search in the bar is the findings list's own search**, reached without
going there first: it submits to the list's query. Without a build it says so.
"/" focuses it, unless somebody is already typing.

**Upload is in the bar on every screen** (UIX-49), because the form picks its
own target; the inventories screen has it too, because that is where the
result appears.

**On a narrow screen the rail goes and a tab bar of three arrives** — home,
findings, queue — which is what somebody reviews and responds from on a phone
(UIX-17). The findings table becomes cards there (UIX-16).

## What you are looking at

**The picker narrows the whole interface, and every level offers "all"**
(UIX-38). Product, branch and variant are chosen once and every cross-product
screen answers for that selection. The levels are independent: a variant
belongs to its product, so "this product, every branch, this variant" is a real
question rather than a mistake. Choosing "all" for the product leaves the two
below it unselectable, because neither means anything without one.

Summarizing across every product is still there — it is what "all" selects.
It stopped being the only option, which is what it was while home always
answered across everything: once somebody has picked a product, a page counting
the others is answering a question nobody asked.

**A screen that needs a whole build cannot be given half a scope** (UIX-39).
Six of them exist for one build and no other — the findings list, a finding,
deciding a place, the dependency tree, deciding several together, and scans —
because there is no dependency graph across branches and a finding is a row in
one build's scan. On those screens the levels that would go to "all" are
disabled and say why. The alternative was to accept the partial scope and
navigate somewhere it makes sense, which turns a filter into a jump nobody
asked for: a control that declines is less surprising than one that relocates
you.

**A narrow screen keeps every entry behind a menu.** The tab bar carries the
three places somebody reviews and responds from (UIX-17), and a menu control
in the bar and a fourth tab open the whole rail as a panel over the page, so
the rest of the interface is one tap further rather than absent on a phone.

**The picker stays open until the last level is chosen.** Each pick applies
at once, because a partial scope is a real answer, but the panel closes only
on the variant, on Escape, or on a click elsewhere. Closing after every pick
made choosing a build three openings of the same panel, and the three columns
side by side are the point of drawing it as one panel.

**Changing scope keeps you where you are.** A build-scoped screen swaps its
build and stays the same screen, because changing what you are looking at is a
property of the screen rather than a journey to another one.

**A screen that names a build in its address is the authority for it.**
Everything else — home, the review queue, the product list — remembers the last
one instead, so walking away from a build and back does not lose it. That
memory belongs to the tab rather than to the browser: it is where somebody is
working right now, not a preference, and a second tab looking at another
product must not drag the first one with it. A browser that refuses to store it
still works and simply forgets.

**A narrowed screen says what it is counting.** A page answering for one
product that looks exactly like a page answering for all of them is how two
people quote different figures for the same question. That applies to the
panels within it as much as to the page: a chart the picker has narrowed and a
label reading "all products" state opposite things, and the label is the half a
reader believes.

**A selection the server would refuse is never sent.** A branch or a variant
with no product above it is refused rather than guessed at, so the levels that
cannot stand alone are dropped on the way out. Sending one would turn a
selection nobody can make in the interface into an error.

## Screens

**The product list is the first pick.** The findings list is scoped to one
product and everything below it is bound to that (UIX-07), so choosing one is a
screen rather than a dropdown that silently changes what a number means.

**A build that has gone quiet is named on the front page and on the scans
screen.** A build that stops being scanned reports no new findings and fails
nothing, so it looks healthier than one still being scanned — it is the one
failure that makes every other number wrong rather than merely incomplete. How
long counts as quiet is a setting; a build nothing has ever been filed against
is the same failure caught earlier and is measured from when it was declared.
Quiet builds are named one at a time rather than counted, because a number is
read past and a name is acted on.

**Home leads with the work and puts the shape of things underneath** (UIX-42) —
what is waiting for review, what is being worked on, what stopped applying,
then the trends, and the operational state at the foot. Somebody opening this
most days wants to know what to do next; the trends answer a question asked
occasionally, or asked by somebody about to report upward. The charts are also
the slowest part of the page, so the half that is wanted first is also the half
that arrives first.

**Home leads with four figures that follow the scope** (UIX-51): open at or
above the floor — the list's own total where a build is picked, the trend's
latest point where the scope is wider — known exploited, pending the reader's
approval, and overdue. Each names what it counts and opens the screen behind
it. Then the work panels, then the trends (UIX-42).

**The findings list has a triage mode** (UIX-43). Off by default and one
control away; in the URL, so a link carries it. The row under the cursor is
marked; Enter opens the decision form inside it — the same form the finding
screen carries, at full width, with the reasoning at a readable measure — and
j and k move, the digits pick the outcome, r or Ctrl+Enter submits and moves to
the next row, Escape closes. None of the keys fire while somebody is typing in
a field, and none fire while the review sheet is open. The form needs the
finding's places, which the list row does not carry, so they are read when the
row opens. Recording lands on the next row rather than on the finding: the
list is where the work is.

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

**Severity never borrows the accent color.** It has its own scale, and
exploited is a band of its own above critical — a page that paints "critical"
in the brand color has nothing left that means "act on this". Being exploited
outranks whatever the score says, which is the ordering everywhere else in this
tool.

**Searching is how somebody gets into a list of thousands**, so the findings
list has a search box and it is the server's like every other filter. It
matches anywhere in a component's name, ignoring capitals, and it is submitted
rather than sent per keystroke — each one is a query over every open finding in
the build, and a half-typed word is not a question worth asking.

**The filters that are not the common ones live behind a control.** Severity,
exploited and fix-available are what somebody uses constantly and stay one
click away; package kind, what holds a thing, and how far it has been decided
sit in a panel that opens. How many of those are on is written on the control
while it is shut, because a narrowed list that looks unnarrowed is how two
people read one screen and quote different numbers.

Three of them are worth stating:

**Package kind** is read from the package identifier rather than stored beside
it — the identifier is what says it, and a second copy is a second thing to
keep true. It is also the closest the data comes to "userland and not the
rest": a kernel and its modules are Debian packages and a statically linked
service is Go, and somebody triaging one is usually not triaging the other.

**What holds it** is the consumer a place records, so asking what is inside a
container asks for places whose consumer is that container. What the build
holds directly is the other half of the same question and has no container to
name, so it is asked for separately rather than by typing something.

**How far it has been decided** had to be defined, because a group covers every
place an issue sits at and those places can be in different states. Undecided
means no place has a decision of any kind; waiting means a claim stands
proposed and nobody has agreed; agreed means every place is answered; lapsed
means a decision here stopped applying and nothing replaced it. Partly answered
is deliberately not one of them — the row already says "12 places · 3
answered", which is the same fact in a more useful form.

**A component's name is what you click.** In both views it narrows the list to
that component. It used to be plain text with a button beside it saying "only
this", which is one act named twice, and the thing somebody reaches for is the
name. Hiding sits on the findings row instead, because that is the list being
triaged and one package drowning it is what somebody is getting past.

**Filtering is the server's, not the browser's.** A list narrowed after it
arrives is narrowed within one page of it, so "hide the kernel" would hide the
kernel from the twenty rows already fetched and from nothing else. The filters
are query parameters, which also puts them in the URL where a link carries
them.

**A row says where the component sits, as both ends of the way down**
(UIX-12): the part of the product it belongs to, and what directly pulls it
in. Those two are what differ between sibling rows — the top says which part of
the product this is, the bottom is what a decision is about — and the steps
between them rarely distinguish anything, so they are counted rather than
named.

Both ends cost one recursive statement for the whole page, not a walk per
row: the database climbs from every consumer on the page to the root and
returns the nodes on the way, and the shortest way down to each is unwound
from those rows. A test pins the cost to the page size rather than to a
statement count somebody has to maintain. The first version read every edge
of the build into memory — 18,561 rows on a switch operating-system image,
a couple of megabytes allocated on the screen opened most — and walked them
in Go; it was measured at three to eighteen milliseconds, which was true and
scaled with the graph rather than with the page. Climbing is bounded by the
depth of the graph instead of its size, and the walk is bounded at
sixty-four steps so a document in a loop is answered rather than followed.

A row covers every place its component sits at, and those places can be reached
different ways. Where they are, the row says the pair it shows is one of
several rather than presenting one route as though it were the only one — a
reader deciding about "the" parent would otherwise be deciding about a parent
they were never shown. Where the inventory placed the component nowhere, it
says that, rather than naming the product itself.

**A row says what upstream has done, and how old the issue is** (UIX-41).
"Upstream declined" and "nobody has fixed this yet" were the same blank, and
they call for different responses. Age comes out of the year in the identifier
at no cost, which is why it does not wait on a disclosure date being ingested:
a fix that has not arrived in six years and one that has not arrived in six
weeks are not the same situation, and the row used to show them identically.

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

**The count is every open issue, answered or not.** A dismissal does not
subtract from it. That is the behavior the screen has always had rather than a
choice made for it, and it is written down because the two readings — "what is
open here" and "what is still to answer here" — are both reasonable and the
screen currently gives the first.

**Both numbers are distinct issues, per path.** A finding is one issue at one
place, and a library at thirty-six places with two issues is seventy-two rows
— which is what every parent it sat beneath used to read, where somebody who
drilled down one path is looking at one place and expects two. A node's own
count is now the distinct issues open against that component, which is the
same at every place it sits, and the cumulative count is the distinct issues
across it and everything under it, each component counted once however many
ways it is reached. The root counts the distinct issues open in the build.
One recursive statement for the row's whole set of children: the subtree
under each as a set of components, joined to the distinct (component, issue)
pairs open in the build and counted per child. Not a query per node, and no
longer every edge of the build read into memory and walked — 0.08 s for the
root's thirty children on the full-size image.

**The list a tree number opens is `beneath`**: every open finding at the
component or anywhere under it, by the same walk — a recursive statement the
list's own query narrows by, rather than a set of identifiers walked in Go
and bound back in, which under the root is the whole build. `under` stays
the direct consumer. The two do not always show the same figure and are not forced to:
the tree counts distinct issues and the list is one row per issue and
component, so a subtree holding one issue at two components is two rows in the
list and one in the tree. A name the build does not hold is refused rather than
answered with an empty list, since an empty list is also what a clean subtree
looks like.

**The finding screen is where deciding happens**, before and after (UIX-46).
Before: what the issue is, how bad, what upstream has done, where it sits, the
evidence, the assessment, and the decision form. After: the decision that
stands, in its state — pending, approved, lapsed — with outcome, justification,
scope and who agreed to which revision, the reasoning rendered, and the
actions that fit the state: revise the reasoning or withdraw while it is
pending or approved, reaffirm with a note or make a new decision once it has
lapsed. Under it, one activity timeline built from the claim's proposal,
revisions, approvals and comments with the earlier decisions at this location
below; the revision history, marking which revision each approval named; the
comments; and the decisions made here before — lapsed and withdrawn — with
their reasoning offered back as "reuse this reasoning". The revisions and
comments shown are the claim's representative decision's, which the finding
names; a claim's records share one reasoning, so any of them answers.

**Which locations a decision covers is a summary with an exception, not a
list of checkboxes** (UIX-44). The form says "all 62 locations"; "exclude
locations" opens the list grouped by what pulls the component in — the
consumer, which is the axis that decides applicability — with a checkbox per
group that reads as mixed when part of a group is out, one per location under
it, and a filter box when there are more than a dozen. What it reads back is
"59 of 62, three left open under X". A location left out stays open and asks
nothing further.

**Where a decision applies beyond this build is a guided review on submit**
(UIX-45). The reach of the covered places is merged — one request per place,
over a sample of eight, because the builds that match one place of a finding
match the rest — and the sheet opens on a summary: this build, the builds
covered automatically by name, the builds at other versions, and any not
offered. Each build at another version is then walked one at a time with its
version, how many locations it holds, and the reasoning beside it, applied or
skipped with a or s, back with the arrow. The last step lists what will be
written and is confirmed; only then is anything sent. The decision here is
recorded first, with the places narrowed where any were excluded; each build
applied is recorded after it, one at a time, as the same claim against that
build's finding, so a refusal on one is reported for that one and does not
decide the rest. Escape leaves the sheet from anywhere, including from a field.

**An approved claim at the same component and consumer is offered to a new
issue** (TRI-47). Where the server names one, the finding shows it with its
reasoning and "apply decision #N", which fills the form and records the new
claim as an extension; it still needs a second person.

**It says how many of its places have been decided** (UIX-40). Once a judgment
can cover a chosen subset of them, a finding half answered has to look
different from one nobody has touched. The count of what the build argued away
through its own VEX documents cannot stand in for this: that is a different
claim by a different author, and reading it as ours would credit somebody
else's reasoning to us.

**Where upstream currency is switched on, the finding says what upstream has
released and when** (ING-41). Two facts rather than a judgment about anybody's
project: the newest version the component has published, and the date it
shipped. Where an issue was named a clear year after the last release and is
still unfixed, the screen says that is why there is no fix — phrased as the
reason nothing has arrived, never as a claim that a project is abandoned, which
is not something this tool knows. It states the release date rather than a year
parsed back out of the identifier, and it needs a full year of silence: where a
stand-in is only precise to a year, comparing two year-numbers makes a
five-week gap look identical to a five-year one. Where the deployment has this
switched off, the panel is absent rather than empty.

**A link that names a component ambiguously offers the choices rather than
refusing.** A name and a version together are not unique — a source repository
and the package built from it can share both — so a finding reached by name
alone can match more than one component. The screen lists what the name could
mean, carrying the ecosystem wherever the choices disagree about it, so that
following one resolves rather than returning the same refusal. Two lists are
possible and they are not interchangeable: the components this issue is
actually open at, and every component of that name. Saying the first sentence
over the second list would tell somebody an issue affects a version it does
not, so each says what is true of it.

**Where it sits shows the chain, not the immediate parent.** The same parent
can be reached by several routes, and a screen naming only the nearest one
cannot tell them apart.

**Three situations, and they are not one.** A component with a named consumer
is drawn under it. A component the build contains directly has no consumer and
a chain of two — the build, then the component — and that chain *is* the answer
to what pulls it in. A component the inventory placed nowhere has no chain at
all, and only that one is told "nothing recorded what pulls this in".

The middle case was folded into the last, so a package the image demonstrably
contains was reported as having no recorded consumer — on the same screen that
drew the chain underneath it. Claiming the product itself pulls something in
was the earlier error and is still wrong where nothing was recorded; saying
nothing was recorded where the build itself is the answer is the same mistake
pointing the other way.

**The review queue is one card per claim** (TRI-45): one proposer's action,
however many decisions it wrote. The card carries the reasoning as it stands,
how many records the claim wrote, how many locations and builds it reaches,
whether it was approved before and came back, and how long the finding has
been put off. Approving and rejecting work on the claim, and rejecting needs a
reason. Selecting several and naming a batch approves several claims together,
so they can be undone together.

**A bulk claim shows its outliers** (TRI-46): how many in the set are known
exploited, critical or high, have a fix available, or do not match the
narrowing, with the rows that stood out listed. Any of those can be set aside;
the button then reads "approve N, reject M", the rest is approved as one, and
the ones set aside return to the proposer as a claim of their own with the
reason. An extension (TRI-47) says which claim it rests on.

**Lapsed decisions and deferrals that ran out sit underneath.** They do not
need a second person, since two people already agreed; they need a fresh
reason. The row carries the decision and not the build it was made in, so
reaffirming happens on the finding, where its locations are, and the card
links to the decision.

**Adding to the catalog is an action, not a form above the table** (UIX-48).
Products, branches and tags, variants and users each carry an "add" control in
the header and a floating action, and both open a drawer with the form; the
table is what the screen is about.

**An inventory can be uploaded from the interface** (UIX-49), from the bar and
from the inventories screen. The drawer takes the target, prefilled from the
scope and refused by the server if undeclared, one CycloneDX inventory, and any
number of OpenVEX suppression documents — exactly the two parts the endpoint
takes. It posts the same multipart request a pipeline sends, then opens the
inventories screen, where the receipt shows "queued" until the run says what
it changed. The receipts list re-reads itself while it is open, because that is
when something is moving.

**The screen that lists receipts is called Inventories.** A scan is what the
deployment does to an inventory after it arrives; what a person uploads, and
what the list is of, is inventories.

**The catalog says what each entry holds.** Products carry how many branches,
tags and variants they have, what is open against them and when they were last
scanned; branches and tags carry what they came from, what is open and when
they were last scanned; variants carry whether they ship to customers and what
is open. A list of names alone makes somebody open every row to find out
whether anything is behind it, which is the question the list exists to answer.
Every count of what is open is issues at components, the way the findings list
counts, so a catalog row and the list it opens agree.

**A product row also says what it triages from**, and an administrator changes
it there. Everybody sees it because it explains a number — a product whose open
count is smaller than somebody expects is usually a product with a line — and
"deployment's" is shown rather than the deployment's current word, because
following it and stating it are different things (`DESIGN-triage.md`). Only an
administrator gets the control, since hiding findings is what every other part
of this gates.

**The inventories list says what each run changed**, not only that it finished.
Opened and closed are counted as issues at components like everything else. A
run covers a build rather than an upload, so where several uploads are answered
by one run the numbers sit on the newest of them and the rest are blank rather
than repeating one fact three times.

**What is waiting on you sits in the bar, with a count.** Everyone has one, and
what appears in it differs by what they hold rather than by which feature they
were given. A condition — something that is true until it stops, like a build
that is not being scanned — is marked apart from an event, because
acknowledging one hides it rather than resolving it. `DESIGN-notifications.md`
says what is told and when.

**Assignments is two tabs: what is due soon and undecided, and who holds
what.** Unassigned work is its own screen with its own rail entry, and a row
nobody holds says "unassigned" in muted text rather than drawing nobody as a
person with an avatar — a name in that column is a person to go and ask, and
"nobody" is not.

**Release comparison carries a chart across every build**, not only the two
being compared: the comparison answers what changed between two, and the chart
answers whether it is getting better or worse. It is bars rather than a line,
because these are separate builds and a line between two releases draws a trend
through a gap where nothing happened.

**A setting whose value is one of a few words is a select, not a text box.**
A free field invites a value the server then refuses — and for a switch it
invites "true", "yes" and "1", none of which are what it takes.

**The inventories screen says what the numbers were measured against** — which
scanner, at which version, reading which vulnerability database. Without it a
build with nothing wrong and a build last measured against a months-old
database read identically.

## Choices the decisions did not cover

**Color and the brand mark resolve through tokens in one place.** How an
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

    make demo                       # build the image, start it, seed it, say where to go
    make demo DEMO_HOST=yourbox     # if you browse by something but localhost

**The only thing it needs on the machine is docker.** Testing a change is
ordinary development rather than something done once before a release, so
standing an instance up has to be something anybody can do anywhere — a demo
that needs a page of prerequisites is one that gets run by the person who wrote
it and nobody else.

It builds the image from the working tree, so what comes up is the change being
tested rather than something published earlier. The interface and the binary
are built *inside* the image: the Go build embeds the interface, and the
directory it embeds is git-ignored, so an image built from a clean checkout
would otherwise carry no interface at all — which is what was happening.

`make demo-status` says whether the scan has landed and how much it found;
`make demo-down` stops it; `make demo-reset` throws the database away and keeps
the scanner's vulnerability database, which is a gigabyte and is not what
anybody is resetting.

Everything it writes lives in a git-ignored directory in the tree, so deleting
the checkout deletes the state. A command run from a checkout that writes to
somebody's home directory is a surprise.

Seeding is idempotent — declaring something that exists succeeds and changes
nothing, and a document already ingested is recognized rather than read again —
so it can be run repeatedly without tearing anything down.

### It is the real thing behind a real proxy

The application is authenticated by a trusted header: a deployment puts it
behind something that has already identified the caller and states who they are
(ACC-19, ACC-20). The demo runs exactly that — the image, with a small proxy in
front of it adding the header — rather than a development server standing in
for one. Two consequences worth stating:

**No mode in the application trusts anybody.** The alternative was a
development switch that assumes an identity, and that is a hole nobody should
ship even switched off by default. The proxy is where this belongs, because it
is where it belongs in a real deployment too.

**The header's identity is prefixed by the path it arrives on**, so `X-User:
dev` becomes `proxy:dev` — and that is the name the administrator has to be
bootstrapped under.

**A write needs an `Origin` the server recognizes.** The trusted-header path
carries no session to hold an echoed token, so the forgery guard falls back to
where the request came from and answers a state-changing request only when it
has been told which origins it serves. Reads work without this and writes
return 401 — a confusing way to find out, which is why `DEMO_HOST` sets every
place that needs it at once.

**The scanner is in the image**, so findings appear rather than the triage
screens sitting empty. Its vulnerability database is fetched once into the
git-ignored directory and kept between runs, because a demo that downloads a
gigabyte every time is one people stop running. On a machine that reaches the
network through a proxy, the demo passes that through — a container inherits
nothing of the shell that started it.

### The developer loop is a different command

    make dev                        # this machine's binary, and the interface's dev server

`make dev` is for editing the interface and watching it reload. It needs Go,
node and a scanner installed here, and it serves the interface from the dev
server rather than from the binary — **so it does not exercise the artifact
that ships**, which is the whole point of the embed (UIX-37). It is the faster
loop and the less true one, and the names now say which is which.

### What it is not

A demonstration deployment, not a small production one. It serves plain HTTP
and hands administration to whoever the proxy in front of it says they are.
Both are holes; together they are a machine somebody can click around on.

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

## What the interface does not have tests for

Worth stating rather than leaving to be discovered, because a green build says
more than it means here.

`make check` type-checks every screen against a client generated from the API
document, so a screen cannot disagree with the shape the server sends and a
drifted endpoint is a compile error rather than a blank panel. That is real
coverage and it is most of what the frontend needs.

What it is not is a test of what a screen *says*. Two pieces are pulled out and
tested on their own — what a notification is called, and the count on the
control that opens it — because saying the wrong thing there is a defect rather
than a matter of taste. Everything else is checked by a person looking at it.

The honest summary: several thousand lines of interface, two test files. Where
a screen computes something rather than draws it, that computation should come
out into a function beside them.

## Where this diverges from the mockup

The restyled mockup is the reference, and each screen has been put beside it
and compared control for control. What differs is listed so that it is chosen
rather than inherited.

**The mockup's home carries a known-exploited tile at every scope; here it
appears only with a build picked.** Nothing counts exploited findings across
products, and a tile that guessed would be worse than one that is absent.

**The mockup's finding sample carries builds already past the fix, shown and
not offered.** The reach endpoint does not say whether a build's version sits
past the fixing version — there is no version ordering here (STA-18) — so
every build at another version is offered and the "not offered" card is empty.

**The mockup reaffirms a lapsed decision inline on the queue card.** The queue
row does not carry the build, and reaffirming is a claim about one place in
one build, so it happens on the finding.

**The mockup's variants and branches screens carry a product select of their
own.** Here the scope picker is that control, and the screens follow it.

**The queue card's reach row names the builds a claim covers from the claim's
own answer**, where the mockup drew "matching automatically" and "ticked
deliberately" apart. The record does not keep which builds were reached by
lookup and which by an applied decision, and the number an approval keeps is
one (TRI-31).

**Release readiness on home is still the marker the mockup carries.**
Comparing a branch against the last release cut from it is not built.

**The users table grants roles inline.** The mockup's columns are user,
identity, last sign-in, roles and assigned; here the last two give way to a
grant control on the row, because granting is what an administrator opens the
screen to do, and last sign-in and assigned work are read from the person's
own row rather than listed for everybody.

**The inventories table omits the product, branch and variant columns.** The
screen is scoped to one build, so those three are the scope bar rather than a
column repeated on every row; what it adds is when the producer says the build
was made, which the mockup did not have.

**Two words, used the way the record uses them: an issue and a finding.** An
issue is the vulnerability itself, one identity across its aliases (MDL-19);
a finding is an issue at a place. Home's open figure, the trend and the
severity ring count issues — the number people expect on a front page and
report upward — and are labelled "issues". The findings list and the rail's
counts are one row per issue and component, so the same issue at three
versions of one library is three findings there, and the tile's caption says
so. "CVE" is not used as the label, because not every issue carries one.

**The trend is drawn by hand rather than by the charting library.** Open
runs to thousands and a week's new or resolved to tens, so on the library's one
shared scale the two lines the chart exists for flattened into the baseline
and vanished. The mockup's form — open as an area in its own band with the
endpoint named, new against resolved as paired bars beneath on their own
scale, one x axis — has no expression in the library short of two charts
pretending to be one, so it is a small SVG of its own. The severity split and
the ring stay with the library.

**Settings show the duration the server stores, with a reading beside it.**
The mockup writes "3 days" in the field; the server takes and returns its own
duration syntax, so what is typed is what is stored and the reading — "= 3
days" — sits beside it rather than in it.

## Not built yet

Named so that what is missing is a plan rather than something rediscovered by
clicking.

**Editing a comment after it is written.** The endpoint exists; nothing calls
it, so a typo stands.

**A mention links nobody.** The editor offers the right candidates and writes
`@name` into the text, but the renderer treats it as ordinary words. UIX-24
wants a mention and a finding reference to become links, and that is resolution
the server has to do, because it needs to know what the reader may see.

**The carry-forward preview.** What a decision will cover when a build moves is
a hint sentence rather than the panel the mockup draws.

**Arriving at the tree from a finding opens at the root, not at the
component.** The mockup opens on the component with the path above it already
expanded, which means walking upward a step at a time before anything can be
drawn.

**Release readiness.** Comparing a branch's current state against the last
release cut from it (RPT-12) has no endpoint; the home panel says so.

**A claim scoped to a consumer subtree, and ownership by subtree.** Both were
proposed in the workflow review and neither is decided (DECISIONS.md Section 4).
