# Implementation plan

High-level staging for building openpsirt. Detail lives in the design documents
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
| `DESIGN-*.md` | **How** it actually works — structures, flows, behaviour | The implementation changes |
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
  secret scanning, licence check
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

**Produces:** `DESIGN-data-model.md`, `DESIGN-ingest.md`

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
- Client-side syntax highlighting, loaded only where a code block appears
- Draft preservation across failure, session expiry and a closed tab

**Proves it works:** the findings list stays usable against a full-size product;
the tree opens lazily without attempting a full render; every screen works on a
phone; **the preview matches what is published**, because it is the same
renderer; **a submission refused by the server leaves the text untouched and
says which line to fix**, and a session that expires mid-write does not lose
what was written.

**Produces:** `DESIGN-interface.md`

---

## Stage 6 — Remediation, reporting, notifications

- Declared targets, computed resolution, reconciliation against scans
- Reports: dismissals, coverage, metrics, release comparison
- Trends on both axes
- Email, digest, operational alerts, in-app notifications

**Proves it works:** a release comparison matches a known pair of releases; a
declared fix that did not land shows as a missed target.

**Produces:** `DESIGN-remediation.md`, `DESIGN-reporting.md`,
`DESIGN-notifications.md`

---

## Stage 7 — Scan scheduling and lifecycle

- Scheduled re-scanning of everything tracked, against a moving vulnerability database
- Retention of release inventories and their suppressions
- End-of-life dates and everything they switch off

The scan itself lands in Stage 2, because under ING-20 the vulnerability data is
produced here rather than sent to us — so without it there is nothing to triage
in Stage 4. What remains here is the schedule and the lifecycle around it.

**Proves it works:** a CVE published after a release was built surfaces against
that release, and raises an alert rather than sitting in a report.

**Produces:** extends `DESIGN-ingest.md`

---

## Stage 8 — Private findings and attachments (product Phase 2)

- Manual entry, visibility handling, disclosure dates and escalation
- Advisory publication: CSAF, and GitHub Security Advisories where they apply
- Attachments: object store, authorised fetch, redaction (`ATT`)

Attachments land here rather than earlier because the access rule they need is
the same one private findings need, and building it twice is how the second one
ends up weaker. The reference format and the fetch path are settled in Stage 4
so the text written before then does not need rewriting.

**Proves it works:** a private finding's attachment cannot be fetched by
someone who cannot see the finding, and the bucket is not readable without
going through the application.

**Produces:** `DESIGN-disclosure.md`, `DESIGN-attachments.md`

---

## Not planned yet

| | |
|---|---|
| Component library | Decided against a real screen in the interface stage, not in the abstract |
| Partition column and granularity | Needs the schema in front of us — settled during the database stage |
| External tracker hand-off | Optional, and the seams are built rather than the integration |
| Markdown renderer and sanitiser | The requirements are settled (`SEC-11` to `SEC-17`); the library pair is picked when the code is written |
