# Access

Who is asking, and what they may reach.

Satisfies ACC-01 to ACC-08, ACC-10 to ACC-42, ACC-44 to ACC-61, SEC-03,
SEC-07 to SEC-09, SEC-20, UIX-32's server half, and the half of ACC-43 that has
a trigger today. What is not built is named at the foot.

## Authenticating is not being authorized

Two things, kept apart deliberately. Authenticating establishes who somebody
is. It says nothing whatever about whether they should be here.

So **no path creates an account**. Access is granted in advance or not at all,
and the first person to reach a fresh deployment gains nothing by being first —
which is a well-known way for self-hosted software to be taken over by whoever
finds the URL, rather than by whoever installed it.

Somebody unknown, and somebody known but granted nothing, are refused with the
**same answer**. Distinguishing them tells an outsider whether a name is real,
which is reconnaissance we would be giving away for free.

## Public and private mean disclosed, not readable

Every request is authenticated either way. A mistake in these rules exposes
something to a colleague rather than to the internet, which is the difference
between an embarrassment and an incident.

Anything unrecognized reads as **not disclosed**. A column added later would
otherwise default every row that predates it to visible.

## Who may hand work to whom

**Deciding who deals with something is a different act from deciding what it
is**, so it asks for a different right (ACC-61). Taking work nobody owns, and
handing back your own, are part of triaging. Giving work to somebody else, or
taking what they are holding, is the assigner's.

The exception is what keeps the ordinary case working. Findings arriving under
an already-assigned component start unowned, so there is a constant stream of
work to pick up; if starting on something needed somebody else's attention
first, working the queue would be a full-time job for whoever held the right.

**Taking work off a colleague is the act the right names, and doing it to
yourself is still doing it.** A triager may take what nobody owns and may not
take what somebody has.

**Assigner is held alongside triage, not instead of it.** The role widens what
a triager may do with work; on its own it assigns nothing, because handing
around findings in a product you may not argue about is not a narrower version
of triaging it. Enforced twice — the endpoint refuses it, and the store refuses
it again — and `may_assign` in the session answer is the same conjunction, so
an interface does not draw a control that always fails.

**Moving everything one person holds stays administrative.** That is about a
person rather than a finding — somebody has left, or their last role on a
product was withdrawn — and it spans every product at once, which is not a
per-product judgment at all.

Who may *be* assigned is not a grant of its own: it is visibility. Somebody is
told about work only where they can see the product it is in, which is checked
where the telling happens.

## Work is assigned, and comes back when somebody goes

A finding can be given to a person. It is set for a whole group at once — one
issue in one component, however many places it sits at — because assigning one
place and not another is not something anybody means to do.

**Handing something back is the same operation as giving it out**, with nobody
as the recipient. Two paths for what is one act drift apart, and the one used
less often is the one that ends up wrong.

**Nobody is assigned is a state to be asked about**, not an absence. Work that
nobody owns is what falls between people, and it is invisible unless it can be
listed — so it is listed across every product somebody can see, since work
falling between people is exactly what hides when every screen shows one
product and nobody looks at the others.

**Assigning covers what is there now.** Findings arriving under the same
component tomorrow start unassigned and appear in that list. A standing rule —
this subtree belongs to that person — is a different feature and worth having,
but conflating it with assignment means neither behaves predictably (ACC-54).

### Somebody leaving

Nothing tells this software that somebody has gone. Membership is read at
sign-in, and a person who has left never signs in again, so it is an action an
administrator takes rather than something discovered.

Until it is taken, their work is in no list at all: not in the shared one
because it is assigned, and not in anybody's own because they are not here.
That is worse than visibly orphaned, which is why releasing it exists as its
own operation and why an administrator can see how much each person holds.

Two answers, because they are different questions. **Releasing** says nobody is
dealing with it and puts it back where it can be picked up — the honest answer
when who takes it on has not been decided. **Handing over** says who is dealing
with it now. Only an administrator does either; a person hands back their own by
assigning it to nobody.

### One trigger that does happen on its own

**Withdrawing somebody's last role on a product hands back what they were
dealing with there**, and only there — what they hold elsewhere is untouched,
because nothing about those products changed.

That is the case where the software does know something has changed, so
waiting for an administrator to notice a second time would be waiting for
nothing. The other trigger ACC-43 names is deactivating an account, and there
is no such thing here: an account is recorded or it is not.

### Nobody learns who has an account by being refused

Every one of these resolves the person **after** deciding whether the caller
may act at all. Resolving first and refusing after answers "does this person
have an account here" for anybody signed in — a name nobody holds and a name
somebody holds come back differently, which is a directory of the organization
readable by every account. It is the same rule as a finding somebody may not
reach answering as one that is not there, applied to people.

## Roles are held against a product

| | |
|---|---|
| **Read and triage, per visibility** | `public-read`, `private-read`, `public-triage`, `private-triage` |
| **Capabilities** | `reporting`, `approver` |
| **Global** | `admin` |

Triage implies reading at the same visibility. Somebody who may decide about a
finding can necessarily see it, and a deployment forced to grant both would
eventually grant one and wonder why nothing worked.

**A capability grants no visibility.** What a reporter or an approver reaches
is bounded by what they may read — otherwise handing somebody the ability to
approve would quietly hand them everything there is to approve.

The other half of that has to be honored too: a capability has to actually
grant something. Approving a triage decision asked for the triage role, which
made the approver capability do nothing at all — somebody granted exactly the
right to approve could approve nothing, and the only people who could were the
ones who could also have proposed it. So an act asks for the right it is named
for, alongside the visibility it acts on.

## Judgments that are not about a product

A rating of an issue is a claim about the issue, true wherever it appears
(TRI-40) — so there is no product to hold a role on, and what stood in that gap
was "is signed in". That is not an authorization rule (ACC-62).

**These ask for the role anywhere.** Recording a rating and taking one back ask
for triage on some product; agreeing to a milder one asks for that or the
approver capability, as agreeing does everywhere else. A rating sets the
deadline and can push a finding below the line a product triages at all — in
every product at once — so an account granted reading on one product is not
somebody to trust with it. Enforced in the store, and the API document says the
scope is a role on any product rather than pretending it is per-product.

## What somebody is told asks for reading it

A notification names the issue, the component and the build, and it is stored as
written — there is no visibility filter downstream that could repair it. So the
check is at the **visibility of the finding**, not of the product (ACC-63):
being able to see that a product exists is not being able to read its embargoed
work. Assigning an undisclosed finding to somebody who holds only public
reading assigns it and tells them nothing.

**A product somebody holds nothing on is invisible**, not merely unreadable.
Not listed, not counted, and — this is the part that is easy to get wrong —
**reported as not declared**, which is the same answer a name nobody ever
declared gets.

Anything else is an oracle. If "you may not see that" and "that does not exist"
answer differently, somebody holding one product can learn the name of every
other by guessing and watching which guess answers differently. So the lookup
and the check happen together: resolving a name first and authorizing
afterwards leaks the difference however carefully the second half is written.

The same applies to a pipeline. A stolen build credential must not become a
reader of the shipping catalog, so a key sees the one product it may send to
and everything else reads as not declared.

Admin is a property of the person rather than a grant against a product,
because it is the one role that is global. Modeling it as a grant would mean a
row whose product is absent, and a uniqueness rule over a column that may be
absent behaves differently on each of the four engines.

## One resolution step, three sorts of asker

What a request may do is decided in one place, not in each handler that
remembers to ask.

| Asker | How | What it may do |
|---|---|---|
| A person | A session cookie, issued after signing in through a provider, or a username a trusted proxy asserts on every request | What their roles allow |
| A person's own script | A token they minted, which reaches no further than they do and may not mint another | A narrowed view of what they may do |
| A pipeline | An API key | Send scans, and read back what became of its own |

**A pipeline may send and nothing else** — no reading findings, no triage, no
reporting. A build server has no business holding a person's permissions, and
this is also what keeps the visibility rules out of its reach entirely rather
than relying on them being applied correctly to it.

### A key's scope is constraints, not a path

The product is always required. The release and the variant are independent,
and either, both or neither may be pinned. The upload always states its full
target explicitly; the key only authorizes it.

Every constraint present must match, and **a mismatch is refused rather than
redirected**. A key covering one release must not quietly accept a scan of
another, and since a product-wide key cannot possibly imply which release an
upload is for, the upload has to say — so it always says.

### A key reads back its own receipts and nothing else

Sending is not quite everything a pipeline needs. An upload is answered before
its documents are read, so the acceptance says the file arrived and nothing
about whether it could be used. The party who can fix a producer emitting
unreadable files is the pipeline that ran it, and it would otherwise see a
green build every night.

So a key may ask what became of the uploads **it** sent: whether each parsed,
and why not where it did not. Narrowed by the credential recorded on the scan,
authorized by the same scope check that authorized sending, and reaching no
findings, no triage and no reporting.

The narrowing is what keeps this a receipt rather than a report about the
product. Several pipelines per product is the expected arrangement, so the
narrowing is applied in the query rather than to the page after it is read — a
count taken before filtering says how many builds somebody else runs, which is
the same leak in a smaller number.

The secret is generated here, never chosen, and **stored hashed and shown
once**. A credential store that can hand back what it holds gives up every
pipeline's key along with a copy of the database. It is not a password: there
is nothing to slow a guesser down when there is nothing worth guessing.

### The trusted header

A reverse proxy authenticates and passes the username on. That lets a
deployment run with no provider at all, and suits operators who already
authenticate at their ingress.

Two guardrails, both deliberate acts: naming the header, and naming the sources
it is honored from. Trusting it unconditionally would let anybody who can reach
the process directly be anybody at all, administrator included — reaching the
container bypasses the proxy entirely. **A half-configuration stops the
process**, because a header named with nothing to trust it from is either a
mistake or the first half of one, and it is never a fallback for sign-in that
was not configured.

## Signing in through a provider

Two adapters behind one interface. One speaks OpenID Connect, for an identity
provider. The other speaks plain OAuth 2.0, for a forge that is not an OpenID
Connect provider at all — it issues no identity token and publishes no
discovery document, so there is nothing for the first adapter to verify and the
account has to be asked about instead. Writing one adapter would have meant
pretending otherwise.

**The exchange happens here and the browser gets a session of ours.** A
provider's token is never handed to a page: one of them is opaque and this API
could not check it anyway, so a browser holding one would mean a second way to
authenticate, verified by a second path, readable by anything that got into the
page.

What has to survive the round trip to the provider — the value the provider
echoes back, the one tying its answer to this request, and the proof-key secret
— is left with the browser where no script can read it. The alternative is a
table of half-finished sign-ins, which has to be swept and which anybody can
fill by starting sign-ins they never come back from.

The value the provider echoes is compared **before** anything is exchanged. It
is what stops somebody handing a signed-in person a callback of their own
making and having the session come back as theirs. The proof key is sent as a
digest and kept as the secret it hashes, so an authorization code taken in
flight cannot be exchanged by whoever took it. And the identity token has to
carry the value tying it to this sign-in, or it belongs to a different one.

**An identity token that names no subject is refused.** The specification
requires one and the verification does not check, and without one there is
nothing stable to match on — which would quietly reduce that deployment to
matching by name, the thing the next section exists to replace.

Where a provider states an address as somebody's name, it is used only when the
provider also says it verified it. An unverified one is whatever the account
holder typed, and an authorization waiting under somebody's work address would
otherwise be redeemable by anybody willing to claim that address.

### Where a provider is reached, and where it is not

Discovery, the key fetches that follow it, and the calls made to a forge all go
through a client that will talk to the configured host and nowhere else, will
not follow a redirect, and will not connect to an address inside this network.

Those addresses come from configuration and from a document fetched over the
network — which is to say from outside. An unrestricted client pointed at a
discovery document is a request-forgery primitive: it fetches whatever the
document names, from inside the network, with whatever the network trusts this
process to reach.

**Pinning the fetch is not enough on its own.** A document fetched from the
right host can still name endpoints on another one, which would turn every
sign-in into a redirect of the issuer's choosing. So the endpoints a provider
publishes are checked against the issuer's own host when the adapter is built,
and a provider that would misdirect people stops the process instead of
misdirecting the first person to sign in.

The address check runs **after** the name is resolved and before the connection
is made. Checked earlier it would see a name rather than an address and refuse
everything; checked by resolving separately and then connecting, it would leave
a window in which the name resolves to something else the second time it is
asked.

The address a provider sends somebody back to is stated in configuration rather
than taken from the request. Taking it from the request would make it whatever
a caller claimed the host was, and whether that can be exploited depends
entirely on how strictly a provider matches its registered addresses — which is
not ours to assume. A deployment that configured a provider without stating its
address does not start.

### A proxy in front instead

A reverse proxy authenticates and passes the username on, which lets a
deployment run with no provider at all and suits operators who already
authenticate at their ingress.

It has no stable identifier of its own — it asserts a username on every request
and there is nothing else to match on. That is a property of the arrangement
rather than a gap: the proxy is the authority there, and a deployment trusting
it has already accepted that what it says is who somebody is.

Known limitation: proxies that deliver identity in a signed token rather than
plain headers are not supported by this path, because reading a header cannot
verify a signature. Such deployments configure a provider instead.

## Sessions, and proving a write was meant

A session is stored, not held in a process, so it works whichever replica
answers and so deleting the row cuts access off at once — the mechanism relied
on when somebody leaves, because group membership is only re-read at the next
sign-in.

**A session holds no roles.** It establishes who is asking; what they may reach
is read at the moment they ask. A role withdrawn from somebody takes effect on
their next request rather than at their next sign-in.

The token is stored hashed for the same reason a key is, and the cookie
carrying it cannot be read by script. Every session has an end: the lifetime is
exactly the window in which somebody who moved out of a team still holds what
the team gave them.

### A browser's credential arrives whoever asked for the request

That is what makes forgery possible, and it is true of both browser paths — our
own cookie, and the proxy's. So a state-changing request has to show it came
from one of our own pages.

| Arriving by | Shown how |
|---|---|
| A session of ours | A value bound to that session, which our pages read and echo. A page from another origin cannot read it |
| A proxy's header | Where the request came from. There is no session to hold a value, and a browser will not let a page misstate its own origin |

Where the request came from is checked for **both**, because it costs nothing
and still holds when the echoed value has leaked. A request stating another
origin, or stating none, is not one of ours.

Requests carrying a key or a token are exempt. Nothing sends those
automatically, so there is no request somebody else can cause a pipeline to
make, and the guard would break every build while protecting nothing.

Safe methods are named as a list, so a method nobody thought of is guarded
rather than exempt by having been forgotten.

## A name authorizes; an identifier decides

An administrator grants access to a person they can name. A provider reports
two things about whoever signs in: a username, and its own identifier for them.
Only the second is stable.

So the two are used for different jobs. **The username is redeemed once** — the
first successful sign-in pins the provider's identifier to the authorization
that was waiting under that name. **From then on the identifier decides**, and
the username is followed as a label.

That closes two failures, both of which are silent.

A username moves. People rename themselves at work, and a forge login can be
renamed and the name then registered by somebody else — at which point matching
on the name hands one person's access to another. Under this shape the
newcomer's identifier does not match what was pinned, so they are refused,
while the original holder is still recognized under their new name.

And a username is only unique within the provider that issued it. A deployment
with two providers configured would otherwise treat `alice` at each as one
person, handing whoever can register that name at either provider whatever was
granted at the other. Every lookup is therefore scoped to the provider,
including the one on the identifier — providers do not coordinate, and plenty
of them issue small numbers.

Pinning at first use is what lets authorization stay in advance. An
administrator cannot know an identifier before somebody has ever arrived, so
the grant has to be expressed in the moving name and then anchored to something
that does not move.

**The trusted header has no identifier beyond the name it asserts.** That is a
property of the arrangement rather than a gap: the proxy is the authority
there, it says who somebody is on every request, and a deployment trusting it
has already accepted that.

## Roles come from one place, for the whole deployment

Either an administrator assigns them or provider groups derive them. Never
both.

A hybrid needs a precedence rule for somebody holding one role from a team and
another directly. That rule is forgettable, and it is how a stale direct grant
outlives somebody's removal from the team it was shadowing. One mode means one
answer to "where did this person's access come from".

### A derived role is a statement about current membership

Membership is read at sign-in and never again — no provider tells us when
somebody leaves a group, and polling every active user against a rate-limited
API to find out is worse than the drift it would close. So every derived grant
is **replaced wholesale at each sign-in** rather than merged: a group somebody
left takes its roles with it, without anything having to notice that they left.

The window in which a withdrawn role still applies is therefore the session
lifetime. The deliberate case — somebody leaving, access being pulled — is
handled at once by ending their sessions, which is a better mechanism than any
polling interval.

Somebody who signs in belonging to nothing that is mapped holds nothing, and is
refused exactly as a stranger is. **Missing or unreadable membership yields no
roles, never unrestricted** — that is the failure which would otherwise be
silent and total.

### The mapping is the authorization

In group-bound mode somebody arriving for the first time in a mapped group is
admitted and recorded then. That does not contradict access being granted in
advance: an administrator made the mapping before anybody arrived, and the
mapping *is* that advance grant. What is never true on any path is somebody
being admitted because a provider vouched for them and nothing else.

### Switching modes is reversible

Turning group binding on marks what an administrator assigned **inactive rather
than deleting it**, and turning it off makes it active again. People do switch
back, usually on discovering that their groups do not map to how the team
actually divides work, and deleting would make that a reconstruction from
memory. An inactive row grants nothing and is never counted as access — not in
a query, not in a report, not in a review.

Grants derived from groups are cleared on the way out. They are a cache of what
a provider said at somebody's last sign-in, and keeping them once nothing
refreshes them would leave roles nobody assigned and nothing will ever
withdraw.

Because both can exist at once — an assignment set aside, and a live derived
grant for the same role on the same product — what makes a grant unique
includes where it came from. Keying without that forbids exactly the pair this
is built to keep.

### Granting a role and reading one are different shapes

A grant is written with two things: which product, and which role. It is read
back with two more the writer cannot decide — whether it is in force, and
whether an administrator assigned it or a group derived it (API-21).

Both matter to a reader and neither is a request. Whether a grant is in force
is the deployment's answer, and the case where it is "no" is exactly the one
above: an assignment set aside by a switch to group binding is kept, and grants
nothing while it sits there. Where it came from is the first question an access
review asks. A caller recording somebody has no way to know either, so neither
is asked for.

Recording somebody answers with what is now on record rather than with the
request that changed it — how they can arrive, and every role they hold, read
back. The distinction is not cosmetic: a request refused the right to grant a
role, because roles are derived from groups here, would otherwise be answered
with its own claim that the role was granted and in force.

### Somebody named in configuration keeps administration

Naming administrators in configuration applies at **every** startup, not the
first, which is what makes it the way back in rather than a setup step. It
survives re-derivation from groups: a sign-in that stripped it would take the
recovery path away at the moment it is needed, which is when the group mapping
is wrong.

It stays a pre-authorization and not a bypass — being named grants the role and
admits nobody who has not authenticated. Configuration is authoritative over
who is named, so somebody removed from it and restarted is no longer named,
while an administrator promoted from inside the application keeps that, because
it did not come from there.

**A deployment is not allowed to start unable to administer itself.** In
group-bound mode that means at least one group mapped to administration, or
somebody named in configuration. The only route back from locking yourself out
is editing the database by hand, and nobody discovers that at a good moment.

### The proxy can report membership too

Where a reverse proxy authenticates, it can pass group membership on in a
second header. This extends no trust that was not already extended: anybody
able to forge the group header could forge the username header and claim to be
an administrator outright.

Both the header name and the separator are configured, because neither is
standardized — one common proxy sends `X-Auth-Request-Groups`, another sends
`Remote-Groups`, and they do not agree on what separates the names. A header
nobody named yields nothing, which is the same answer an empty one gives.

## A person's own credential

Anything the interface does not offer cannot be automated, and the usual result
is somebody driving a browser session with a script or reusing a pipeline's key
for work it was never scoped for. So a person can mint a credential of their
own.

It is a **live reference to its owner, never a snapshot**. What it reaches is
read from what they hold at the moment it is used, so a role withdrawn from
them cuts the token at the same instant — including one withdrawn because a
group membership went away, which is the case with nothing else to notice it. A
snapshot would quietly outlive the access it was granted from.

**A token may not mint or withdraw another.** Minting resolves through the
owner, so a token that could mint would ask for a wider one and be given it —
which makes every limit on a token exactly one request deep, the lifetime
ceiling included, and an administrator's narrowed token a way to get an
un-narrowed one.

It can be narrowed below its owner and never above. Narrowing **intersects**:
a token pinned to a product its owner cannot read reaches nothing rather than
being granted it. Administration is dropped by narrowing entirely, because
administration is global and a token narrowed to one product that still
administered everything would not be narrowed at all.

**Expiry is not optional**, with a maximum an administrator sets. A credential
that never runs out is one nobody ever revokes, and the ones that matter are
discovered when somebody leaves and nobody knows what breaks if it is turned
off. Revoking marks rather than deletes, so what used it stays answerable after
it stops working.

### Every credential says which kind it is

Pipeline keys and personal tokens carry distinct fixed prefixes. Two things
follow.

Resolution **dispatches on the prefix** rather than trying each store in turn,
so a pipeline's key can never be looked up as somebody's personal token, and
the cost of presenting a wrong credential does not depend on which store
happened to be asked first.

And a credential that ends up somewhere public is recognizable as one. Secret
scanners match fixed prefixes; a bare run of base64 matches nothing.

## Where each thing is decided

| Decided | Where | Why there |
|---|---|---|
| Who is asking | One middleware, before any route | A handler that forgets to ask is a handler answering for everybody, and the forgetting is invisible until somebody reads the one that did |
| Whether they are anybody at all | The same middleware | Refusing before the request is examined means an unrecognized caller does not learn whether their body was well-formed |
| Whether they may reach this product | The data layer, on the query | A check beside the query cannot be skipped by adding another endpoint |
| Whether they may do this at all | The handler | Declaring a product is administration whatever the query looks like |
| Whether a header from an untrusted source was a mistake | Logged, never answered | The caller is told no more than anybody else. An operator who trusted an address in one family and is reached from the other has no other way to find out, because the request simply fails, correctly, forever |

**A pipeline is refused a read rather than shown an empty one**, receipts for
its own uploads excepted. "Here is
nothing" and "you cannot ask" are different statements, and the first invites a
caller to believe the list is empty.

**Everything except the probes is authenticated**, named as a list rather than
guarded by a path prefix. A prefix leaves everything outside it open by
default, and the framework registers routes of its own — the API document and
the schemas it references were served to anybody who asked, including the
running version that the endpoint reporting it is authenticated to withhold. A
list means adding a route never quietly adds an exception.

**Including the one that reports the running version.** Which build is here is small reconnaissance, but it is
reconnaissance: it says which published issues might apply. The probes stay
open, because a container probe cannot sign in and they report nothing beyond
whether this process can serve.

**Every response carries the browser headers that turn off what nothing here
needs** (SEC-20): no guessing at a content type, no page anywhere may frame
this one, referrers stay on this origin, and a content security policy permits
only what the bundled interface ships — its own scripts, styles and fonts,
`data:` images, and requests to itself. Inline styles are the one concession,
because the chart library writes them. Set before any handler, on the API's
answers as well as the page's, so a route added later cannot lack them and a
JSON body opened in a browser is still covered. It is the second line SEC-15
accepted behind the markdown sanitizer: a bug there lands in a page that can
run no inline script and load nothing from anywhere else.

A deployment that cannot tell who is asking serves nobody. Failing closed means
an unconfigured deployment is a service that is up and refusing, which is
visible, rather than one that is up and answering everybody, which is not.

## A query without a subject is a fault, not a denial

Reading who is asking from a request's context **fails** when nobody is
attached. It is a bug in this program: somewhere a query was written that does
not say who is asking.

The safe-looking alternative — treating absence as "nobody, so show nothing" —
hides that until somebody writes the query that treats absence as "everybody".
One of those is a blank screen and the other is a disclosure.

## Where a sign-in comes back to

A sign-in may carry the address it began at, so somebody whose session ended
halfway through writing lands back on the screen they were on rather than on
the home page. Losing the words is prevented in the browser; losing the *place*
is prevented here.

**The address never leaves this deployment.** It is kept in the same cookie
that already holds what a sign-in has to remember while the browser is away, so
nothing a provider echoes back can decide where somebody ends up.

**It is a path here or it is discarded**, and this is the whole of the defense.
A sign-in that sends a browser wherever a parameter says makes this
deployment's own domain vouch for somebody else's page, which is the classic
bug in exactly this flow. So an address is kept only when it starts with a
single `/`; does not start with `//` or `/\`, which browsers read as
protocol-relative and would send the browser to another host; and parses with
no scheme and no host of its own. Anything else becomes the home page — which
is where a sign-in landed unconditionally before this existed.

**Checked on the way in and again on the way out.** The cookie is the browser's
own, so somebody may edit it. A person redirecting themselves gains nothing,
but an address that left here is an address this deployment sent, and that is
what an open redirect is.

**Discarded rather than refused.** Turning a bad address into a failed sign-in
would punish the person for a link somebody else wrote.

## The way back in

Configuration names administrators, and they are granted at **every** start
rather than only the first. That makes it the recovery path as well as the
bootstrap: lose administrative access, add yourself, restart. For software
somebody else operates, a way back in matters more than a tidy one-shot.

It is a pre-authorization and not a bypass. Being named grants the role; it
does not admit anybody who has not authenticated.

## A secret never reaches a log

Connection strings are redacted and credentials and tokens are never written at
any level (SEC-08). Not a level to be turned down in production: a token at
debug is a token in whatever collects the logs, read by everybody who can read
those and kept for as long as they are kept, which is usually longer than the
token's own life.

The one that actually happens is a database URL with a password in it, printed
once at startup by something helpful. It is redacted where it is formatted, not
where it is logged, so a second caller that logs the same value cannot
reintroduce it.

The rule is written down rather than left as a habit because the failure is
invisible: nothing breaks, no test fails, and the leak lives in a system nobody
thinks of as holding secrets.

## Disclosure

Public and private mean disclosed and not disclosed, so an undisclosed finding
is one somebody intends to disclose eventually. All of this is built, and
arrived with private findings — a flaw in what a build ships, recorded by hand.

**A private finding carries a disclosure date**, from a setting that defaults
to ninety days after it was recorded (ACC-46). A public finding has none:
it is already disclosed, and a date on it would be a deadline for something
that has already happened. Ninety days is what coordinated disclosure practice
converges on, and the point of having one at all is that it gives the embargo
an end somebody outside could hold us to.

**Reaching the date discloses nothing** (ACC-47). It escalates. Publishing
embargoed detail because a timer expired is the wrong default in both
directions — if the fix is not ready, disclosing anyway is a decision a person
makes, and automatic publication eventually publishes something nobody was
ready for. So the date is answered rather than acted on: what has reached it
appears on the list below, marked as passed, and stays there until somebody
decides something.

**What is approaching disclosure is surfaced before the date** (ACC-49), on a
list ordered soonest first. The date arriving is the last moment to act on it
rather than the first useful warning, and a list that only ever showed what was
already past would be a list of decisions somebody has already failed to make.

**That list is itself a disclosure.** Every row on it is a finding nobody has
announced, so a product somebody may not read undisclosed work in contributes
nothing to their copy of it — not a row and not a count, because a count says
as much as a row. What that costs them is a shorter list; what the alternative
costs is the thing the whole public and private split exists to prevent.

**Extending the date needs a reason, and past a threshold that is a setting —
thirty days by default — a second person** (ACC-48). The same shape as a
deferral, because it is the same act:
keeping risk hidden for longer. Without that the date is decoration, and the
indefinite secrecy the disclosure frameworks warn about arrives one quiet
extension at a time.

**A reason is required always**, however short the extension. One with no
reason is a record saying somebody moved it and nothing else, which is the
state keeping the history exists to prevent.

**The threshold is measured against everything the embargo has already been
moved by**, not against the request in hand. Measured per request the exception
swallows the rule three weeks at a time — the same reasoning the deferral
threshold is written with, and the same arithmetic. Only extensions that took
effect count toward it: asking for a long one and being refused moved nothing,
and letting a refusal push every later request over the line would charge
somebody for something that never happened.

**An extension that needs agreement moves nothing until it has it.** A request
waiting for a second person changes nothing about the finding it is about,
which is already true of a decision waiting for one. An embargo that ran on
while somebody thought about it would be the extension taking effect on one
person's say-so with a queue entry as decoration.

**The person who asked may not be the one who agrees.** That is the control the
threshold exists to reach.

**A date only ever moves later.** Bringing one forward is disclosing sooner,
which is a different act and not this one.

**Every request is kept, granted or not**, oldest first, with why and by whom.
One extension is a judgment and six is a policy nobody wrote down, and the
difference is invisible if each replaces the last. A request nobody agreed to
is part of the record too: what was asked for is part of how long this stayed
hidden.

**The date arriving tells somebody** (ACC-47). Administrators, and whoever
holds the finding where they may still read undisclosed work in that product —
the second only where they may, for the same reason the list itself is a
disclosure. It is a condition rather than an event: it stands while the date is
past and nothing has been decided, and it clears when somebody answers it,
which is the same shape as reporting a build that stopped being scanned. A
finding that becomes disclosed leaves it by ceasing to be private.

## Where somebody is reached outside the application

**A person's record carries a mail address, and it is optional** (ACC-60).
Somebody without one is told nothing outside the application and keeps the area
inside it, which is the same shape as a deployment that never configured mail
at all.

It has two sources and one field. An administrator sets it with the rest of the
record, and a sign-in provider fills in one nobody set. Which of the two it came
from is kept, exactly as it is kept for an administrator a group granted — so a
provider may refresh what a provider gave, and may never overwrite what
somebody here decided. Written the other way round, an administrator correcting
a wrong address would watch the next sign-in put it back.

Neither source alone is enough, which is why there are two. A provider covers
the ordinary case for nothing and covers nothing at all on the trusted-header
path, which is a supported deployment and the one the demo uses; it also has
nothing to offer until a first sign-in, which is exactly the person ACC-45 is
about. Recording every address by hand covers everybody and asks somebody to
type what the provider already knows.

**A provider's address is taken only where the provider says it checked it.**
That is the caution the username fallback already takes and it matters more
here: an address nobody checked is whatever the account holder typed, and mail
sent to it is mail sent wherever they said — which, for an alert about a
finding nobody has announced, is the disclosure the alert exists to avoid
(NTF-15). The identity a provider hands over says both things separately, so an
adapter that cannot check says so rather than handing over an address and
leaving the question open. For one forge that means asking for the addresses it
has confirmed rather than reading the one on the public profile, which is
whatever somebody chose to show and often nothing.

**Failing to record an address does not fail a sign-in.** Arriving is what was
asked for; where to reach somebody later is not part of it, and refusing the
first because the second did not work would lock people out over a column.

## Somebody who has stopped coming in

**An administrator is told when somebody has not signed in for a configured
period and still holds assigned work** (ACC-45) — "X has not signed in for two
weeks and has six items assigned". Not built: assignment exists and so does
handing back what an absent person holds, and what is missing is the prompt.

That is the honest shape of the gap. Nothing tells this software somebody has
left (ACC-44), membership is only read at sign-in, and a person who has gone
never signs in again — so the software cannot discover it and can only notice
the absence and say so. Until it does, work assigned to somebody who stopped
coming in is in no list at all: not in the shared one because it is assigned,
and not in theirs because they are not here.

## Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| Triage implies reading at the same visibility | Nobody decides about what they cannot see, and requiring both grants means one eventually gets forgotten |
| A key is honored from anywhere | It holds a credential rather than being vouched for by position. Where it connects from says nothing about whether it is genuine |
| The stored key digest is compared again in constant time | Finding a row by digest is not by itself a statement that two secrets match |
| A person holding triage may send a scan | Somebody re-uploading a build by hand is doing triage work. It is not administration, and it is not something a reader should be able to do |
| A pipeline sees the product it may send to | It may send there, so knowing it exists is already implied. Pretending otherwise would make an upload to its own product indistinguishable from one to a product that is not there, which is the difference somebody fixing a misconfigured pipeline needs |
| A fault is logged rather than described | The framework serializes an error passed alongside the message, so handing it one hands the caller the query text and, for a connection failure, the address and user it tried |
| Naming every address as a trusted source is refused | It reaches the same place as naming none, through the setting that is supposed to be the guard |
| Granting a role somebody already holds succeeds | An administrator scripting grants should not have to check first, and the outcome is what they asked for either way |
