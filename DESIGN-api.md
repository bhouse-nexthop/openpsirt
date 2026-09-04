# The HTTP API

The one surface. Everything a person or a pipeline can do here goes through it,
and the interface, when it exists, will be a client of it like anything else.

Satisfies API-01 to API-03, API-05 to API-07, API-14, API-16 to API-22, ACC-09,
SEC-01, SEC-02.

## One API, no private half

There are no endpoints reserved for our own interface. A private half is how an
API stops being honest: the endpoints a browser needs get built quickly and
loosely because "only our code calls them", and the public ones drift into
being the second-class copy. It also makes the interface privileged — able to
do things nobody else can — which is a security boundary nobody designed.

So the interface uploads a scan through the same endpoint a build pipeline uses.
That upload path is a convenience for testing, not a second contract.

## Generated, never written

The OpenAPI document is produced from the route definitions rather than
maintained beside them. A hand-kept document is wrong the first time somebody
is in a hurry, and it is wrong silently, because nothing compares it with the
code.

The published document is checked in and the build fails when the committed
copy has drifted from what the code generates. That is what makes "generated"
true rather than aspirational: without the check, regenerating is a step
somebody skips.

The application serves no documentation of its own. Documentation is published
separately, which keeps the application to one job and leaves it with **no
unauthenticated route that reads anything** — the two health endpoints answer
without a credential and say nothing but whether the process is up and whether
it can reach its database.

## Descriptions are reference material

A summary is an imperative verb and the thing it acts on, in the words the
domain uses. Somebody scanning thirty operations has to find theirs in a
second, and a paraphrase that avoids naming the thing reads as a riddle.

A description says what the operation does, what it takes, what comes back, and
what a caller must know that is not obvious — an upload that answers before it
has parsed anything, a field required only for one outcome, an approval that a
later edit withdraws. **The reasoning is not there.** Somebody making a request
work does not want the design argument standing between them and the request;
it lives here and in `DECISIONS.md`.

**The first sentence says what the operation does**, and a dozen of these did
not. They opened with the argument — "one mode for the whole deployment, a
hybrid would need a precedence rule" — and never said the endpoint lists which
mode is in force. The rule reads as though it were about tone; it is not. A
description that argues instead of describing leaves a reader who has never
seen this system with no idea what the request returns, which is the one thing
the reference is for. Where the reasoning genuinely changes how an endpoint is
used, it goes in a paragraph after that first sentence.

Every parameter carries its own description too. A required query parameter
documented as nothing is a parameter somebody guesses at.

## What the shapes mean

| | |
|---|---|
| **201** | Something now exists and its identifier is in the answer |
| **202** | Accepted and not yet done. Only an upload, which answers before it has been read |
| **204** | Done, with nothing worth saying |
| **404** | The thing is not there — *or* is not yours, which are deliberately the same answer |
| **409** | The request conflicts with the state of what it names — a scan older than the one already held, a role granted the wrong way for the mode this deployment is in, an approval by the person who made the claim |
| **422** | The request was understood and cannot be stored as written |

### Not found and not yours are one answer

A product somebody holds nothing on is invisible rather than merely unreadable:
not listed, not counted, and reported as not declared, which is the same answer
a name nobody ever declared gets.

Anything else is an oracle. If "you may not see that" and "that does not exist"
answer differently, somebody holding one product learns the name of every other
by guessing and watching which answer comes back. The same applies to a
decision identifier, a person, and a credential.

The sentences are kept in one place. There were six spellings of it, two of
which described the wrong thing — a missing person and a missing credential
both answered "not declared", which names neither.

## A refusal says where to look

A justification runs to dozens of lines. A refusal naming only a category
leaves somebody hunting for the offending line by eye, so each fault travels as
its own detail carrying the line and the text that caused it.

This is an API shape decision rather than a presentation one: an interface can
only point at the problem if the answer says where the problem is. It was
briefly flattened into one sentence, which reads fine to a person and leaves a
client with nothing to point at.

## Markdown, and only markdown

Text written by people comes back as markdown. It is what an integrating
application can most easily lay out, it reads as plain text as it stands so it
doubles as the plain form, and it does not assume a browser — which most
callers of an API-first tool are not.

There is no markup representation and no `html=true`. There was, and it existed
for two readers: our own interface, and an email's HTML part. The interface
renders in the browser, and an email never goes through the API — so the
parameter had no caller. It was removed while API-20 is not yet in force and
removing it costs nothing.

What this means for a caller: you receive the source, and rendering it is
yours. Sanitize what you render. The server has already refused what its policy
forbids at submission (SEC-15), so the text is known-good under the rules in
force when it was written — but rules written since are the renderer's to
apply. The rules themselves are in `DESIGN-text.md`.

## Sorting and filtering

**No caller chooses a column.** Every list has one order, decided by what the
list is for — the findings list opens on what is being exploited, the decisions
list on what was judged most recently — and filters are named fields with fixed
meanings, bound as parameters.

That is a stronger position than an allowlist and was taken deliberately. A
value in a query can be bound as a parameter; **a column name cannot be**, so
the moment a sort column arrives from a query string it has to become part of
the statement, and the allowlist guarding it is a thing that can be wrong. Not
having the feature is the version that cannot be wrong.

If a caller-chosen order is ever wanted, the allowlist comes with it, and this
paragraph is the reason it is not optional.

## The version in the path

`/v1` is the shape the API will have. Before the first release it is not a
compatibility promise, and a change to a shape is an edit rather than a second
version standing beside the first.

## Choices the decisions did not cover

- **Paging is `limit` and `offset`, with a total.** A cursor is better under
  concurrent writes and worse for the thing people actually do, which is jump
  to a page. The total is separate from the page because somebody deciding
  whether to start work needs to know how much there is, and that is not how
  much is on the screen.
- **A list answers with an object, not an array.** An array at the top level
  has nowhere to put the total, and adding one later changes the shape for
  every existing client.
- **Names in paths, identifiers in bodies.** A product, stream and variant are
  named because that is what somebody typing a request knows and what a
  pipeline has in its configuration. A decision is numbered because it has no
  name, and inventing one would be inventing something to get wrong.
- **Where a place appears in a path**, it is the identity the findings list
  gave out, not something a caller composes. A caller free to name a place
  would be choosing which decisions apply where.

## Every operation says what it asks of a caller

The rights an endpoint needs lived only in its handler, so answering "who may
call this" meant reading code — and an integrator could not answer it at all
(API-22).

Each operation now carries a structured statement: the **scope** it is held
against — the deployment, a product, yourself, or any credential this
deployment recognizes — the **roles** that satisfy it, any one of which is
enough, and a short **note** where a rule is not a role.

**The line in the description is rendered from that same value**, not written
beside it. Two hand-written copies disagree within a month, and a wrong
permission is worse than the silence it replaced: a missing answer is known to
be missing, and a wrong one is trusted.

**A gate refuses an operation that says neither.** The failure is silent
otherwise — an endpoint added without one is not broken, it is undocumented,
and nobody finds out until they need the answer.

**The privileges page is generated from the same registrations.** It groups the
operations by scope and lists the roles for each, above a table of what every
role allows. Nothing in the chain is maintained by hand between the code that
enforces a right and the page somebody reads before asking for one.

Two rules in it are not roles and cannot be granted: visibility narrows every
answer, so an endpoint somebody may call still shows only what they may see;
and the proposer of a claim may never approve it.
