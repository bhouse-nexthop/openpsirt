# Access

Who is asking, and what they may reach.

Satisfies ACC-02 to ACC-08, ACC-10 to ACC-14, ACC-19 to ACC-21, ACC-29 to
ACC-31, SEC-03.

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
Not listed and not counted. The list of products is itself a statement about
what an organization ships.

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

## Where each thing is decided

| Decided | Where | Why there |
|---|---|---|
| Who is asking | One middleware, before any route | A handler that forgets to ask is a handler answering for everybody, and the forgetting is invisible until somebody reads the one that did |
| Whether they are anybody at all | The same middleware | Refusing before the request is examined means an unrecognized caller does not learn whether their body was well-formed |
| Whether they may reach this product | The data layer, on the query | A check beside the query cannot be skipped by adding another endpoint |
| Whether they may do this at all | The handler | Declaring a product is administration whatever the query looks like |

**A pipeline is refused a read rather than shown an empty one.** "Here is
nothing" and "you cannot ask" are different statements, and the first invites a
caller to believe the list is empty.

**Every documented route is authenticated, including the one that reports the
running version.** Which build is here is small reconnaissance, but it is
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
| Granting a role somebody already holds succeeds | An administrator scripting grants should not have to check first, and the outcome is what they asked for either way |
