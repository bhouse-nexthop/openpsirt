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

**Proves it works:** the role × visibility × endpoint matrix, **including
counts, aggregates, search and exports** — the paths that leak when only row
reads are checked.

**Produces:** `DESIGN-access.md`

---

## Stage 4 — Triage

- Findings per place; the four outcomes; VEX justification vocabulary
- Decisions keyed structurally, carried forward, expiring on version change
- Review queue, approval, separation of duties, bulk action, withdrawal
- Duplicates across variants, branches and tags
- Markdown text: justification revisions, comments, server-side rendering

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
- Email, digest, operational alerts — **the in-app notification area is
  built**, with both lifetimes and two of its producers; see
  `DESIGN-notifications.md` for what is told and what is not yet

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
  **Telling somebody the date arrived is not**
- Advisory publication: CSAF, and GitHub Security Advisories where they apply
- Attachments: object store, authorized fetch, redaction (`ATT`)

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

## Decided, and waiting for the stage that carries it

Recorded here so a decision that exists and is not implemented is a scheduled
thing rather than a gap somebody rediscovers by auditing.

| Decision | Waits for | Why not now |
|---|---|---|
| ACC-45 | Somebody being told | Assignment exists now, and so does releasing what an absent person holds. What is missing is the prompt — noticing that somebody has not signed in for a while *and* holds work — which is a notification rather than a screen |
| ACC-43, second half | Deactivating an account | The half with a trigger today is built: withdrawing somebody's last role on a product hands back what they were dealing with there. The other half needs a way to deactivate somebody, which does not exist — an account is recorded or it is not |
| ACC-44 | Nothing — it is a statement | That we cannot detect somebody has left is recorded so nobody assumes a cleanup happens that never does |
| MDL-16 | The tree views | Interface |
| TRI-04, TRI-09, TRI-19 | Export, the bulk list, and reporting | Each belongs to a stage of its own |
| REL-07, ING-13 | The interface | What a new line would inherit can be asked for; ticking the ones to carry is a screen |
| ACC-46 to ACC-49 | Private findings | Whole feature, with disclosure dates and the escalation around them |
| ING-24 to ING-27, SCP-11 | Analyzer findings | Intended scope, not built. The finding model carries a kind from the start so a second kind needs no rewrite, which is the part that had to be got right early |
| REL-01, REL-03, REL-04, SCP-05 | The interface | All are about how findings and exceptions are presented and acted on together. Deciding them against a real screen rather than in the abstract is the point |
| UIX-24, linking a mention or a finding reference | Resolution the server has to do | The editor offers the right people to mention and writes the text; what nothing does is turn `@name` or a finding reference into a link. That is resolution rather than formatting — it needs to know what the reader may see — so it belongs on the server beside the policy, not in the renderer |
| The server-side renderer (`internal/markdown.Render`) | An email's HTML part | The interface renders in the browser (UIX-22), so the renderer's only remaining reader is a digest email, which has no client to render for it. It is kept rather than deleted and rebuilt: it carries the sanitizer and a corpus of cross-site-scripting payloads, and a security control rebuilt from memory comes back weaker. Its tests are what keep it exercised until Stage 6 wires it up |
| MDL-10 | Nothing — it is a limitation | The same version built with different feature flags can use its dependencies differently. Recorded so nobody assumes the graph says more than it does |

## Not planned yet

| | |
|---|---|
| Component library | Decided against a real screen in the interface stage, not in the abstract |
| Partition column and granularity | Needs the schema in front of us — settled during the database stage |
| External tracker hand-off | Optional, and the seams are built rather than the integration |
| Markdown renderer and sanitizer | The requirements are settled (`SEC-11` to `SEC-17`); the library pair is picked when the code is written |
