# Access

Who is asking, and what they may reach.

Satisfies ACC-02 to ACC-08, ACC-10 to ACC-15, ACC-16 to ACC-21, ACC-22 to
ACC-32, ACC-36 to ACC-41, ACC-50, SEC-03.

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
because it is the one role that is global. Modelling it as a grant would mean a
row whose product is absent, and a uniqueness rule over a column that may be
absent behaves differently on each of the four engines.

## One resolution step, three sorts of asker

What a request may do is decided in one place, not in each handler that
remembers to ask.

| Asker | How | What it may do |
|---|---|---|
| A person | A trusted header today; a provider later | What their roles allow |
| A pipeline | A key it holds | Send scans. Nothing else |

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

## The way back in

Configuration names administrators, and they are granted at **every** start
rather than only the first. That makes it the recovery path as well as the
bootstrap: lose administrative access, add yourself, restart. For software
somebody else operates, a way back in matters more than a tidy one-shot.

It is a pre-authorization and not a bypass. Being named grants the role; it
does not admit anybody who has not authenticated.

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
