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

## Stage 0 — Foundations

Nothing product-specific. Get the shape right before there is anything to
migrate.

- Build, CI gate, dependency and licence checks
- Configuration, logging, health endpoints, version reporting
- Database layer across all four engines, migration runner, startup lock
- Portability test harness and the CI matrix

**Proves it works:** the suite passes on SQLite, MySQL, MariaDB and PostgreSQL;
migrations run and roll back on each; the binary refuses to start against an
unsupported engine version.

**Produces:** `DESIGN-database.md`

Why first: the portability rule (DAT-02) and the partition-key constraint
(DAT-05) are both far cheaper to establish than to retrofit.

---

## Stage 1 — Model and ingest

The data model and everything that fills it.

- Products, streams, tags, variants, with declaration before use
- Graph storage — nodes, edges, validity intervals, path identity
- CycloneDX adapter behind a producer-agnostic interface
- Streaming parse, asynchronous queue, ordering, duplicate handling, atomicity
- Current state and change events

**Proves it works:** a real full-size SBOM ingests within budget; re-ingesting
identical content writes nothing; an older scan is rejected; **re-ingesting the
same content with every producer identifier shuffled changes no stored
identity** — the test that proves ING-05 and MDL-06.

**Produces:** `DESIGN-data-model.md`, `DESIGN-ingest.md`

---

## Stage 2 — Access

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

## Stage 3 — Triage

- Findings per place; the four outcomes; VEX justification vocabulary
- Decisions keyed structurally, carried forward, expiring on version change
- Review queue, approval, separation of duties, bulk action, withdrawal
- Duplicates across variants, branches and tags

**Proves it works:** a decision survives a nightly re-ingest; lapses when the
component or its consumer changes upstream version; does **not** lapse on a
packaging revision; a bulk approval can be undone as a batch.

**Produces:** `DESIGN-triage.md`

---

## Stage 4 — Interface

- Findings list, dependency tree, finding detail, review queue, home
- Generated API client; responsive layouts

**Proves it works:** the findings list stays usable against a full-size product;
the tree opens lazily without attempting a full render; every screen works on a
phone.

**Produces:** `DESIGN-interface.md`

---

## Stage 5 — Remediation, reporting, notifications

- Declared targets, computed resolution, reconciliation against scans
- Reports: dismissals, coverage, metrics, release comparison
- Trends on both axes
- Email, digest, operational alerts, in-app notifications

**Proves it works:** a release comparison matches a known pair of releases; a
declared fix that did not land shows as a missed target.

**Produces:** `DESIGN-remediation.md`, `DESIGN-reporting.md`,
`DESIGN-notifications.md`

---

## Stage 6 — Shipped-release rescanning and lifecycle

- Retain release SBOMs and their suppressions; scheduled rescanning
- End-of-life dates and everything they switch off

**Proves it works:** a CVE published after a release was built surfaces against
that release, and raises an alert rather than sitting in a report.

**Produces:** extends `DESIGN-ingest.md`

---

## Stage 7 — Private findings (product Phase 2)

- Manual entry, visibility handling, disclosure dates and escalation
- Advisory publication: CSAF, and GitHub Security Advisories where they apply

**Produces:** `DESIGN-disclosure.md`

---

## Not planned yet

| | |
|---|---|
| Component library | Decided against a real screen in Stage 4, not in the abstract |
| Partition column and granularity | Needs the schema in front of us — settled during Stage 0 |
| External tracker hand-off | Optional, and the seams are built rather than the integration |
