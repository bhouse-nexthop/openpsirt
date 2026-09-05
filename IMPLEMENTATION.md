# Implementation plan

High-level staging for building OpenPSIRT. Detail lives in the design documents
each stage produces, not here.

> **This document is temporary and will be deleted.** Once the work has landed,
> everything durable lives in `DESIGN-*.md`. Nothing may reference this file or
> its stage numbers — not code, not comments, not commit messages, not the
> design documents. Once it is gone those references become dead pointers. See
> `AGENTS.md`.

> **"Stage" here, "Phase" in DECISIONS.md.** Phase 1 and Phase 2 are *product*
> scope — public findings first, private findings later (SCP-02). Stages are
> build order. They are not the same thing and do not line up.

## How the documents relate

| Document | Answers | Changes when |
|---|---|---|
| `DECISIONS.md` | **Why** something is the way it is | A decision is made or reversed |
| `DESIGN-*.md` | **How** it actually works — structures, flows, behavior | The implementation changes |
| Code | What runs | Continuously |

The chain matters for audits. Code points at a design document, a design
document points at decision IDs, a decision says why. If code does something no
design document describes, it is a **remnant** — either the design document is
out of date or the code should not be there. Either way it gets re-examined
rather than assumed.

So a design document is not a summary of decisions. It says what was built,
names the decisions it satisfies, and — importantly — records the things that
were *not* decided anywhere but had to be chosen while writing the code.

---

## Stage 0 — Skeleton, and the pipeline that validates it

Nothing that does anything. The point is that from the next commit onward,
every change is checked automatically.

- Repository layout, Go module, a binary that builds, starts, serves a health
  endpoint and reports its version
- CI: build, test, static analysis gate, `govulncheck`, dependency review,
  secret scanning, license check
- The test harness and CI matrix skeleton, running even with almost no tests
- Documentation building and publishing
- Container image and Helm chart, both verified in CI

**Proves it works:** a deliberately broken change is rejected by the gate; a
trivial correct one goes green end to end; the binary builds, runs and reports
its version; the documentation site publishes.

**Produces:** `DESIGN-build.md`, `DESIGN-packaging.md`

Why first: everything after this is validated by this. Building it later means
every earlier stage was checked by hand and has to be re-checked.

---

## Stage 1 — Database foundations

- The data-access layer across all four engines
- Migration runner, startup lock, `migrate` subcommand
- Portability test harness; the CI matrix filled in

**Proves it works:** the suite passes on SQLite, MySQL, MariaDB and PostgreSQL;
migrations run and roll back on each; the binary refuses to start against an
unsupported engine version.

**Produces:** `DESIGN-database.md`

Why here: the portability rule (DAT-02) and the partition-key constraint
(DAT-05) are both far cheaper to establish than to retrofit.

---

## Stage 2 — Model and ingest

The data model and everything that fills it.

- Products, streams, tags, variants, with declaration before use
- Graph storage — nodes, edges, validity intervals, path identity
- CycloneDX adapter behind a producer-agnostic interface
- Inventory and suppressions in one multipart upload
- **The scan step**: run the scanner over an ingested inventory, apply the build's suppressions, record the findings and the scan's provenance
- Streaming parse, asynchronous queue, ordering, duplicate handling, atomicity
- Current state and change events

The finding model carries a kind from the start, even with only one kind in it.
A second kind — static analysis, fuzzing — has no dependency path, and a schema
that assumes one cannot take it later without a rewrite.

**Proves it works:** a real full-size SBOM ingests within budget; re-ingesting
identical content writes nothing; an older scan is rejected; **re-ingesting the
same content with every producer identifier shuffled changes no stored
identity** — the test that proves ING-05 and MDL-06.

**Produces:** `DESIGN-data-model.md`, `DESIGN-ingest.md`, `DESIGN-findings.md`

---

## Stage 3 — Access

Before any interface, because retrofitting authorization is an audit of every
query.

- Sign-in: OIDC, GitHub OAuth2, trusted header
- Sessions, and one subject-resolution step for all credential types
- Roles per product, visibility enforced in the data-access layer
- API keys with scope constraints; bootstrap admin
- **Handing work to somebody else is its own right** (ACC-61), held alongside
  triage rather than instead of it, with taking unowned work and handing back
  your own left to any triager
- **A judgment that is not about a product asks for the role anywhere**
  (ACC-62), because a rating is a claim about an issue and there is no product
  to hold a role on
- **Every operation states what it asks of a caller** (API-22), as structured
  data on the API document and as a line in its description, both from one
  value — and `docs/reference/privileges.md` is generated from the same
  registrations rather than maintained beside them

**Proves it works:** the role × visibility × endpoint matrix, **including
counts, aggregates, search and exports** — the paths that leak when only row
reads are checked.

**Produces:** `DESIGN-access.md`

---

## Stage 4 — Triage

- Findings per place; the outcomes; VEX justification vocabulary. **Five
  outcomes, not four**: `already-fixed` (TRI-51) says whoever packages the
  component has published the fix, carries the version they published it in,
  and exports as VEX `fixed` — which the justifications cannot say, because
  every one of them is a claim about our build rather than about the
  packager's patch
- Decisions keyed structurally, carried forward, expiring on version change
- Review queue, approval, separation of duties, bulk action, withdrawal
- Duplicates across variants, branches and tags
- Markdown text: justification revisions, comments. Rendering moved to the
  browser (UIX-22); what the server keeps is the policy at submission

**Proves it works:** a decision survives a nightly re-ingest; lapses when the
component or its consumer changes upstream version; does **not** lapse on a
packaging revision; a bulk approval can be undone as a batch; **editing an
approved justification withdraws the approval and the approved revision is
still readable**; a comment added after approval leaves it alone; a corpus of
known cross-site-scripting payloads survives rendering with nothing executable
in the output.

**Produces:** `DESIGN-triage.md`

---

## Stage 5 — Interface

- Findings list, dependency tree, finding detail, review queue, home
- Generated API client; responsive layouts
- Markdown editor: toolbar, Write and Preview, mention autocomplete
- **Client-side markdown rendering and sanitizing**, one renderer for preview
  and published text alike (UIX-22)
- Client-side syntax highlighting, loaded only where a code block appears
- Draft preservation across failure, session expiry and a closed tab

**Proves it works:** the findings list stays usable against a full-size product;
the tree opens lazily without attempting a full render; every screen works on a
phone; **the preview matches what is published**, because it is the same
renderer — the same one, in the browser, rather than two agreeing by luck; **a
corpus of known cross-site-scripting payloads survives the client renderer with
nothing executable in the output**, which is the control that moved out of the
server when rendering did; **a submission refused by the server leaves the text untouched and
says which line to fix**, and a session that expires mid-write does not lose
what was written.

**Produces:** `DESIGN-interface.md`

---

## Stage 6 — Remediation, reporting, notifications

Part of this landed early, because the interface stage needed numbers to draw
and shaping them against a screen after the fact would have meant shaping them
twice. `DESIGN-reporting.md` records what exists: release comparison, trends,
deadlines and what a new line would inherit.

- **Declared targets, computed resolution and reconciliation against scans
  are built** — somebody says which releases a fix is meant to reach, and the
  next scan of each answers whether it arrived. Nothing records "done".
  `DESIGN-remediation.md` says what it deliberately does not do
- Reports: dismissals, coverage, metrics — **release comparison is built**
- **Trends on calendar time are built**, and so is **release readiness** — a
  branch beside the last release cut from it. Release over release as a trend
  axis is not
- **The in-app notification area is built**, with both lifetimes and two of
  its producers, and **mail now carries what leaves it**: the categories worth
  interrupting somebody for go at once, and a daily digest — off until a person
  asks for it — carries what nothing else told them. A message about a finding
  nobody has announced says only that there is something. Each person chooses
  their own; an operator sets `MAIL_FROM` and `MAIL_SERVER` or nothing leaves
  at all. Chat adapters and operational alerts are not built;
  `DESIGN-notifications.md` says what is told and what is not

**Proves it works:** a release comparison matches a known pair of releases; a
declared fix that did not land shows as a missed target.

**Produces:** `DESIGN-remediation.md`, `DESIGN-reporting.md`,
`DESIGN-notifications.md`

---

## Stage 7 — Scan scheduling and lifecycle

- **Scheduled re-scanning of everything tracked is built**, against a moving
  vulnerability database: one replica asks, on an interval that is a setting,
  for every build holding an inventory that has not been scanned within it.
  `DESIGN-ingest.md` records what it deliberately does not do
- **Retention of release inventories and their suppressions is built** — a
  tagged release keeps both, a branch build's contents are let go once they
  have been read, and the record of what arrived outlives them either way.
  `DESIGN-ingest.md` says what is kept and why
- **End-of-life dates are built** — a date on a release or on its product,
  inherited rather than copied, reversible, and switching off two things:
  a deadline on anything the release holds, and reporting a build of it as
  having gone quiet. `DESIGN-data-model.md` records the shape

The scan itself lands in Stage 2, because under ING-20 the vulnerability data is
produced here rather than sent to us — so without it there is nothing to triage
in Stage 4. What remains here is the schedule and the lifecycle around it.

**Proves it works:** a CVE published after a release was built surfaces against
that release, and raises an alert rather than sitting in a report.

**Produces:** extends `DESIGN-ingest.md`

---

## Stage 8 — Private findings and attachments (product Phase 2)

- **Manual entry is built** — a flaw in what a build ships, recorded by hand,
  filed under an identifier this deployment mints, starting undisclosed and
  behaving like any other finding from there. Visibility handling shipped in
  Phase 1. **A disclosure date is built** — an embargo gets an end, and what
  is approaching one is a list, before the date rather than on it.
  **Extending a date is built** — with a reason always, a second person past
  a cumulative threshold, and nothing moving until that person agrees.
  **Telling somebody the date arrived is built** — administrators and whoever
  holds it, as a condition that clears when somebody answers it.
  **A screen records one**, and **a person can close one** (REM-28), which
  nothing else could: computed resolution needs evidence and no scan reports
  such a finding, so it stayed open forever
- Advisory publication: **the CSAF document is built** — generated from what is
  held and handed over, with nothing sent anywhere and nothing recording that
  it was (REM-18, REM-21, REM-23). **Every adapter that would send it is not**,
  nor is the VEX profile of the document, which needs the mapping from a
  decision to the releases it covers. `DESIGN-remediation.md` says which half
  is which. GitHub Security Advisories (REM-22) is not built
- Attachments: object store, authorized fetch, redaction (`ATT`). **The last
  large Phase 2 item**, and the one that needs a new dependency — an
  S3-compatible client, which SCP-06 says needs a permissive license

Attachments land here rather than earlier because the access rule they need is
the same one private findings need, and building it twice is how the second one
ends up weaker. The reference format and the fetch path are settled in Stage 4
so the text written before then does not need rewriting.

**Proves it works:** a private finding's attachment cannot be fetched by
someone who cannot see the finding, and the bucket is not readable without
going through the application.

**Produces:** `DESIGN-disclosure.md`, `DESIGN-attachments.md`

---

## Before the first release

Work that has to happen once, at the end, and would be wrong to do earlier.

- **Collapse the schema into a single initial migration** (DAT-29). Ten
  migrations describe the order things were thought of rather than anything an
  operator will run. They are kept until then because walking the chain is what
  catches an ordering mistake between two of them.
- **Start keeping schema and API compatibility** from that point (DAT-29,
  API-20). Until then a schema change is an edit and a database is recreated.

## What the reach and shortfall decisions cost

These were costed before they were built. What each actually took is recorded
here so the estimates can be read against the outcome.

### TRI-30, TRI-31 — stating and recording reach

**Built.** One column on the approval for the count as granted, not the three
first costed: the split between this build, the builds already matching and the
ones ticked deliberately is how the reach is *shown*, and what the record needs
is how much was agreed to.

The reach itself stays derived. Storing it as anything but a snapshot taken at
approval would create a second copy of the matching rules to keep in step.

### TRI-32 — one judgment across many issues

**Built**, and the schema estimate was wrong by one column: how the set was
narrowed is recorded with each claim, so "how were these chosen" has an answer
later. The rest was as costed — it writes the decisions the model already has,
and a batch name already existed for undoing a bulk approval as a unit.

Selecting the set is the work, and the honest version of it is narrow: filter
by component, then by whatever the person types. Anything cleverer — grouping
by weakness class, or by a subsystem read out of advisory text — is a **guess
presented as an aid**, and the rule is that it may narrow a list and may never
be the claim.

The bound is a setting rather than a constant, as costed. What it bounds turned
out to matter: the limit is checked against the findings the names resolve to,
not the number of names, because each name may sit at many places and all of
them are written.

### STA-18 — marking a bump that fell short

**Built**, and the question the estimate flagged answered the cheap way: a
version change is not an update in place. Component identity carries the
version, so a bump closes one finding and opens another and both versions are
in hand at the moment of the change. The new row records what it arrived from.

It surfaces on the finding and in a release comparison's still-present column.
Not yet in the review queue, which lists decisions rather than findings.

It stayed an inequality. **Do not let it become version ordering by accident** —
that is a different project, with a per-ecosystem comparator and a corpus to
test it against, and it buys a sharper sentence rather than a new signal.

### REM-28 — closing what nothing else can close

**Built**, and it was not costed at all because nobody had noticed the gap.
It was found by writing a test for the advisory's `fixed` list and discovering
that list could never be filled: one path closes a finding, it passes over
anything a person recorded, and nothing else closed one. So a flaw recorded by
hand opened and stayed open forever, invisibly — every screen looked right and
the wrongness was only that it never left.

What it took was wider than the feature: closure had to stop being readable
only as "a run did this". `closed_at` on the row, the run or the person beside
it as provenance, and every index that carried the run to answer "is this open"
moved to the moment. That is the same change `opened_at` had already been
given, for the same reason plus one — spelled the old way, the column saying a
finding is over could only be filled in by something that never looks at it.

The lesson worth keeping: **the second half of a symmetry is worth doing when
the first half is done.** The opening side was fixed alone, and the closing
side sat there with the identical latent defect — the trend still reached the
run for the closing moment — until a feature needed it.

## Decided, and waiting for the stage that carries it

Recorded here so a decision that exists and is not implemented is a scheduled
thing rather than a gap somebody rediscovers by auditing.

| Decision | Waits for | Why not now |
|---|---|---|
| ACC-43, second half | Deactivating an account | The half with a trigger today is built: withdrawing somebody's last role on a product hands back what they were dealing with there. The other half needs a way to deactivate somebody, which does not exist — an account is recorded or it is not |
| ACC-44 | Nothing — it is a statement | That we cannot detect somebody has left is recorded so nobody assumes a cleanup happens that never does |
| TRI-04, TRI-19 | Export and reporting | Each belongs to a stage of its own. TRI-09 was the third of these and is built: a queue entry says what it is about |
| REL-07 | A screen | ING-13 was the other half and is built: what a new line would inherit can be asked for. What is missing is the form — the reach is computed and shown, and ticking the ones to carry is not |
| ING-24 to ING-27, SCP-11 | Analyzer findings | Intended scope, not built. The finding model carries a kind from the start so a second kind needs no rewrite, which is the part that had to be got right early |
| The internal half of UIX-24 | A route that means "this issue, wherever we have it" | Identifiers link out to the records that define them. A reference to a finding held here cannot link yet because an identifier alone does not name one — a finding is an issue at a place in a build — so what is missing is the address rather than the link |
| The server-side renderer (`internal/markdown.Render`) | Nothing, now — a decision to take | Stage 6 landed and did not wire it: mail carries the markdown as its text part (NTF-14), which reads fine unrendered, so the HTML part it was being kept for was never built. It is still kept rather than deleted, because it carries the sanitizer and a corpus of cross-site-scripting payloads and a security control rebuilt from memory comes back weaker — but "waiting for Stage 6" is no longer true, and the honest state is that it has no consumer and its tests are what exercise it |
| RPT-01, and what the `reporting` role is for | A discussion, not a stage | The role gates nothing: every report endpoint asks for a credential and narrows by what the caller may see. The likely shape is a minimal statistics role — counts and trends without reading the findings behind them — which cuts across a capability granting no visibility, since an aggregate over work somebody may not read is still an answer about it. `DECISIONS.md` §7 holds it; adding a check to the report endpoints before that discussion would settle it by accident |
| Clearing what somebody was told when their role goes | A decision about the record | Reading a notification is narrowed by person and nothing else, so a withdrawn private-triage leaves a list naming undisclosed findings. ACC-43 hands their work back on the same trigger. Whether revocation clears those rows or merely stops serving them is the open part; `DESIGN-notifications.md` states the gap |
| MDL-10 | Nothing — it is a limitation | The same version built with different feature flags can use its dependencies differently. Recorded so nobody assumes the graph says more than it does |

## Not planned yet

| | |
|---|---|
| Component library | The stage this waited for has passed and the interface was built without one, from the mockup's own tokens and hand-written components. So the choice was made by building rather than by deciding, and `DECISIONS.md` §7 still lists it as open — which is the honest state, not a settled one |
| Partition column and granularity | Needs the schema in front of us — settled during the database stage |
| External tracker hand-off | Optional, and the seams are built rather than the integration |
