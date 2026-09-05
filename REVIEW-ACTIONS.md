# Review actions

An action list from the functionality and usability review of 2026-09-05,
made against `6a2cb8d` by reading the code and driving the demo rather than
the documents alone. **This document is temporary**, in the same sense as
`IMPLEMENTATION.md`: it is worked through and then deleted, and nothing may
reference it. Anything durable that comes out of an item goes to
`DECISIONS.md` or the `DESIGN-*.md` for its area as the item lands.

It is written to be worked from without anything else to hand: every item
carries the evidence behind it (the path, the request, the count), the
suggested change, and where it is not obvious, the regression test that would
pin it. The narrative version, with screenshots, is the artifact "OpenPSIRT
Deep Dive": https://claude.ai/code/artifact/380a4893-9941-4c57-be59-3133a072e76e

Identifiers (X, P, W, V, R, F) match that report. Sizes: S is days, M a week
or two, L more. Tick an item when it lands.

## How the review was made

Five reviews ran in parallel, one per slice: comparative scope, views and
hierarchy, reports by persona, workflows and automation, and permissions with
a live probe. Each cited the document, the code path, and the request or
screenshot behind every claim; the items here were re-checked against the
code and the demo before inclusion. Nothing in the repository was changed by
the review.

The demo's shape, which the counts below refer to: SONiC with one branch and
two variants is **7,612 pieces of work** (issue × component, across both
variants) and 5,797 distinct issues; the kernel package alone carries 5,088
issues at 45 places each (228,960 findings); OpenPSIRT with one branch, two
tags and two variants has 28 open. Identities: `dev` is the bootstrap
administrator through the proxy on :8080, `ana` (:8081) and `ben` (:8082)
hold public-read, public-triage and approver on both products. The container
trusts `X-User` from its own subnet, which includes the host, so any existing
person can be acted as directly:

    curl -s -H 'X-User: ana' -H 'Host: localhost:8080' http://172.31.71.2:8080/v1/session/me
    # writes add: -H 'Origin: http://localhost:8080'

## What the review found, in six lines

- **One leak and two oracles.** Anybody with triage on any product can rate an
  undisclosed recorded flaw by name and be handed its severity, and every
  credential then sees that row. Not-found messages distinguish a hidden issue
  from an absent one, and minted identifiers are a counter. A read endpoint
  confirms whether an identity has an account.
- **"Where is CVE-X in anything we ship" cannot be asked.** The search box
  matches component names only, is disabled outside one build, and no endpoint
  spans products.
- **Phase 2 exists only in the API.** Disclosure state, the embargo date,
  extensions, the disclosing list and the advisory are referenced nowhere in
  the interface.
- **Nothing leaves the application except mail, and nothing exports.** No
  webhook, no tracker hand-off, no CSV or JSON, no VEX per build. The
  newly-critical-on-a-shipped-release alert (NTF-11) is decided and not built.
- **Volume is handled one row at a time.** 5,047 fixable rows on the demo
  collapse into 271 upstream bumps; 37% of rows are the same CVE at a sibling
  package; each is its own page. Triagers cannot assign work, even to
  themselves.
- **The record works for triagers only.** Readers see findings but no
  decisions; the audit list caps at 500 with no paging; administrative changes
  leave no trail; the release note lists open items under a heading that says
  fixes.

---

## Fix now

- [x] **X1 · Assessments leak undisclosed recorded flaws.** *Leak, demonstrated.* **S**

  **Done.** Recorded as TRI-53. Recording, agreeing to, taking back and listing
  a claim now all ask whether the subject may read a finding of that issue
  somewhere; an issue sitting at no build here is exempt, which is what keeps
  the forward-looking half of TRI-40. A refusal is spelled as an unused name
  and an unreadable claim is absent from the list, so neither the issue name
  nor the claim identifier can be walked.

  **A second leak was found beside it and fixed in the same change**: the
  counts on a waiting claim used `onlyVisible`, whose own comment warns that
  the visibility half alone "admits every disclosed finding in the deployment,
  including in products the asker holds nothing on". So an approver holding one
  product was told how many findings the issue had elsewhere and how many
  products those were. Now `onlyReadable`, and pinned — the test read 2
  products before the fix and 1 after.

  All four controls were verified by breaking them and watching the named test
  fail.

  Evidence, all reproduced:

      # rev-pubtri: public-triage on sonic. GET on the finding → 404 "no open finding is recorded there"
      POST /v1/issues/SONIC-2026-0003/assessment   {"severity":"low","reasoning":"probe"}
      → 201 {"id":1,"vulnerability":"SONIC-2026-0003","severity":"low","published":"critical",...}

      # rev-other: private-triage on openpsirt ONLY, holds nothing on sonic
      POST /v1/issues/SONIC-2026-0004/assessment → 201 {... "published":"high" ...}
      POST /v1/issues/SONIC-2026-9999/assessment → 404 "no issue is known by that name"

      # rev-public: public-read only
      GET /v1/assessments → 200 [... {"vulnerability":"SONIC-2026-0003","published":"critical","reasoning":"probe",...}]

  `published` is the severity recorded on the embargoed flaw. The store
  checks `subject.HoldsAnywhere(PublicTriage, PrivateTriage)` in
  `internal/finding/assessment.go` (around line 78) and never asks whether
  the subject may read any finding of that issue. The list handler in
  `internal/httpapi/assessment.go` (around line 148) is registered
  `anySubject` with the description "Every claim, whoever asks: a rating is
  about an issue, not a product". The `open` / `in_products` counts on the
  row *are* narrowed (absent for rev-public), which shows the narrowing
  exists and was applied to the counts but not the row.

  Why: TRI-40 and ACC-62 treat an issue as public knowledge, true for a CVE
  and false for an identifier this deployment minted (MDL-24, MDL-27). It
  also lets an outsider to the product push an embargoed finding below the
  triage floor of a product they hold nothing on, the worry in ACC-62
  inverted.

  Change: recording, agreeing to, withdrawing and *listing* an assessment
  require that the subject may read at least one finding of that issue at
  its visibility, in some product; `readableFindings` already expresses the
  predicate. For listing, narrow rows the way the counts are narrowed.

  Regression: a public triager rates a recorded private flaw → "no issue is
  known by that name"; a public reader lists assessments → the row is
  absent. Runs on SQLite and PostgreSQL (DAT-12).

- [ ] **X2 · "Hidden" and "absent" answer differently, and minted identifiers are a counter.** *Leak, demonstrated.* **S–M**

  As `rev-public` (identical for `rev-pubtri` and a public person's token):

      GET .../findings/SONIC-2026-0003/components/sonic-broadcom → 404 "no open finding is recorded there"
      GET .../findings/SONIC-2026-0005/components/sonic-broadcom → 404 "no issue is known by that name"  (before 0005 existed)
      GET /v1/products/sonic/issues/SONIC-2026-0001/disclosure  → 404 "no open finding is recorded there"
      GET /v1/products/sonic/issues/SONIC-2026-0007/disclosure  → 404 "no issue is known by that name"
      GET .../findings/SONIC-2026-0003/components/sonic-broadcom/fix-targets → 200 {"items":[],"declared":0,...}
      GET .../findings/SONIC-2026-9999/components/sonic-broadcom/fix-targets → 404
      POST .../findings/SONIC-2026-0003/.../decision (public triager)       → 404 "no open finding is recorded there"
      PUT  .../findings/SONIC-2026-0003/.../assignment (public triager)     → 204  (RowsAffected 0, nothing written)
      PUT  .../findings/SONIC-2026-9999/.../assignment                      → 404 "no issue is known by that name"

  The issue name is resolved (`noSuchIssue`, `internal/httpapi/absent.go`
  around line 45) before the finding's visibility is checked
  (`noSuchFinding`, line 41), so the two sentinels differ. Identifiers are
  minted sequentially per product and year (MDL-24), so a public reader can
  walk `SONIC-2026-0001…` and read off how many undisclosed flaws exist and
  when the counter moved. This is ACC-56's shape ("a name nobody holds and a
  name somebody holds answer identically") applied to issues; it was avoided
  for decisions (`/decisions/61` and `/decisions/999999` answer alike) and
  attachments (both 403).

  Change: one helper, "resolve this issue *as seen by this subject*", used by
  every finding-shaped route: finding detail, place decision, reach,
  disclosure list and extend, decide, fix targets (404 rather than an empty
  200), assignment (404 rather than 204). And mint identifiers that are not a
  counter (random suffix), which removes the "watch the counter" channel
  without touching handlers. Both is best.

- [ ] **X3 · `GET /v1/people/{identity}/assignments` is a staff directory.** *Leak, demonstrated.* **S**

      GET /v1/people/proxy:rev-public/assignments   X-User: rev-report → 200 {"items":[],"total":0}
      GET /v1/people/proxy:nobody-xyz/assignments   X-User: rev-report → 404 "nobody here is called that"

  Any credential, including one holding no product at all (`rev-report`, a
  narrowed admin token), learns whether an identity has an account.
  `internal/httpapi/assignment.go` (around lines 255–262) resolves the name
  with `ByIdentity` before any check; ACC-56 was applied to the assignment
  writes and to hand-back but not to this read. The 2026-09-03 note found the
  same shape on comments; this is the third instance.

  Change: an unknown identity answers exactly as a known one with nothing
  visible, `200 {"items":[],"total":0}`. Add one test that walks every route
  with an `{identity}` parameter and asserts the invariant, rather than a
  fourth point fix.

- [ ] **X5 · Triagers cannot assign or take work, including for themselves.** **S, high impact**

  The holder select on the finding (`web/src/screens/Finding.tsx` around
  lines 1622–1660) and "Assign to…" on Unassigned (`Unassigned.tsx` around
  lines 105–120) both `GET /v1/people`, which is administrator-only
  (`docs/reference/privileges.md`). As ana both selects contain only
  "Nobody" / "Assign to…" (holder options `['Nobody']`). The API allows
  taking unowned work without `assigner` (the assignment endpoint's own
  description says so); the interface offers no way to do it.

  Change: read `GET /v1/products/{product}/mentionable`, which already
  returns the people who may see the product and is callable by a triager.
  Add a one-click **"Take this"** on the finding, the list row and the
  Unassigned batch bar; self-assignment is the common case. Also: there is no
  "assign everything under this container" or "assign this page"; the
  Unassigned batch bar helps but needs a working picker first.

- [ ] **X4 · Searching a CVE returns nothing.** **M, highest workflow impact**

  The top bar is labelled "Find a component or an issue…" but submits
  `findings?q=` (`web/src/app/Shell.tsx` around lines 270–306), and `q` is
  documented in `docs/reference/openapi.yaml` as "keep only components whose
  name contains this". Typing `CVE-2026-9079` at build scope returns 0 rows;
  at product or "all" scope the box is disabled outright
  (`disabled={!build}`), though the findings list has worked at product scope
  since UIX-53. No endpoint answers "where is this CVE in anything I may
  see", the most common question a PSIRT is asked when an advisory lands.

  Change: `q` (or a new `issue=`) matches any identifier or alias; the box is
  enabled at every scope; the answer is cross-product (see V9 for the view
  and the endpoint it needs).

- [ ] **X6 · The comparison's Fixed column floats over Introduced.** *Rendering bug.* **S**

  `web/src/screens/Compare.tsx` renders `<Column kind="fixed" …>` and the
  column's class is `` `col ${kind}` `` (line ~325), so the class is `col
  fixed`; the built stylesheet (`web/dist/assets/*.css`) carries Tailwind
  v4's `.fixed{position:fixed}`, and the column leaves the grid. The same
  trap as the recorded `ring` / `block` / `grid` collisions; `fixed` was
  missed.

  Change: rename the modifier (`was-fixed`), and add a lint that rejects
  component class names that are Tailwind utilities, so this stops recurring.

- [ ] **X7 · A newly-critical or newly-exploited vulnerability on a shipped release alerts nobody.** *Decided, documented, not built.* **S**

  NTF-11 is decided; `DESIGN-notifications.md` (around line 82) describes it
  as behavior ("operational alerts are their own category: they go to
  administrators and are outside the opt-in digest"). `notify.Kind` in
  `internal/notify/notify.go` (lines 50–73) has six values: `assigned`,
  `mentioned`, `sent-back`, `build-quiet`, `holding-absent`,
  `disclosure-due`. Nothing computes it; `internal/notify/watch.go` does not
  look for it. The scheduled rescan (`internal/scanner/schedule.go`) is the
  path that would discover it, and today it is learned when somebody opens
  the list.

  Change: a condition-based operational alert (NTF-09 shape) raised when a
  run opens a finding at critical or exploited on a *tag* stream, cleared
  when it closes or is decided; the digest already has the data shape
  (NTF-18). Fix the design document in the same change, which currently
  describes something that does not run.

- [ ] **X8 · Administrator is silently every role, and an admin's token can mint credentials.** *Model gap, unrecorded.* **S–M**

      POST /v1/decisions/61/approval             X-User: dev (admin, no product roles) → 204
      POST /v1/disclosure-extensions/1/approval  X-User: dev                            → 204
      POST /v1/people  Authorization: Bearer <rev-admin2's token>  {"identity":"proxy:rev-made-by-token","admin":true} → 201
      POST /v1/keys    Authorization: Bearer <same token>                                                             → 201

  `Subject.Holds` in `internal/access/access.go` (around lines 161–176)
  returns true for admin on every role and product, so an administrator
  proposes, approves, reads every embargo and re-rates severities.
  `privileges.md` says holding every role does not amount to admin and never
  says admin amounts to every role; ACC-05/ACC-07 say admin is global and
  nothing says it implies triage and approval. A personal token carrying
  admin can create an *admin person* and a pipeline key, credentials that
  outlive the token, so "a token cannot mint another" (ACC-33/34,
  `internal/access/token.go` around line 97) holds for tokens and not for
  what an admin token can mint instead.

  Change: record the choice either way. Recommended: admin administers
  people, roles, credentials, settings and the catalog, and reads or triages
  only where granted (a bootstrap admin grants themselves roles like anybody
  else); administration requires a session, never a token (`Delegated()`
  already exists to check). The change is one `if s.Admin { return true }`;
  the cost is the demo and bootstrap, which rely on it. This is also what
  makes a `private-read` auditor meaningful: today no reader sees everything
  without also being able to change everything.

- [ ] **V3 · The decision form opens with a dismissal ready to submit.** **S**

  `web/src/ui/Decide.tsx` line 74: `useState(prefill?.outcome ??
  "not-applicable")`, and the justification select lands on
  `vulnerable_code_not_in_execute_path` (lines 74–78). Every finding opens
  with a dismissal one click away, against the owner's direction not to
  encourage them.

  Change: no outcome selected and Submit disabled until one is chosen, or
  default to "Affected". In triage mode, default to the last-used pair (W5).

- [ ] **X9 · Documents describe things that do not run.** **S**

  - ING-01 says "SPDX 2.3 accepted but lossy" and the README's table implies
    it; the only SPDX in code is `internal/sbom/cyclonedx_test.go:362`
    asserting `{"bomFormat":"SPDX"}` is *refused*. Mark the clause superseded
    or write a lossy adapter to the same internal model.
  - ING-28's "secondary path" (a producer-supplied vulnerability report) is
    not built: `cyclonedx.go` never reads a `vulnerabilities[]` array.
    Working notes record this; `DECISIONS.md` still reads as if it exists.
    Put "not built" beside it in `DESIGN-ingest.md`.
  - `DESIGN-reporting.md` line 7 says RPT-03 is not built; line 405 says
    "Remediation metrics … (RPT-03). Built". Its Satisfies line omits RPT-03
    and REM-04 while the body claims both. REM-04's "SLA compliance rate" is
    not computed anywhere (`grep -ri compliance` hits only the decision
    text), and `DESIGN-remediation.md:7` says REM-04 is satisfied by an
    escalation view that never mentions a rate. Settle both.
  - NTF-01's row still says "Slack and Teams as adapters" without
    qualification; only `DESIGN-notifications.md` "What is not built" says
    they are not.
  - The gate checks that decisions have documents, never that documents are
    true; a person has to.

- [ ] **X10 · Small defects found on the way.** **S each**

  - `make demo-status` prints "open 404 page not found" per build: the
    target (Makefile around line 747) curls
    `/v1/products/<product>/streams/<s>/variants/<v>/findings`, the per-build
    path UIX-53 moved to `/v1/products/<product>/findings?stream=&variant=`.
  - Approving an already-approved disclosure extension: `POST
    /v1/disclosure-extensions/1/approval` by a second approver → 500 "that
    could not be recorded" (log: "that extension has already been agreed
    to"). Should be 409 like the self-approval case.
  - The queue card says "2 other builds: master · broadcom, master ·
    mellanox" for a claim on broadcom: `buildsCovered` in
    `internal/triage/queue.go` counts the claim's own build. It also carries
    only the consumer name (`docker-bmp-watchdog`), not "the two ends of the
    way down" TRI-09 asks for.
  - Triage mode's bar reads "1–4 outcome" (`web/src/screens/Findings.tsx`
    around lines 626–633) but `5` = Already fixed exists.
  - The deferral confirmation says "The dismissal takes effect once a second
    person approves"; a deferral is not a dismissal (TRI-04).
  - Home's severity tile shows low 1,415 by folding `negligible` (236) and
    `unknown` (1,141) into it, while `/v1/trend` reports them separately.
  - Kernel rows at build scope show "nothing records what pulls this in"
    while holding 45 places whose consumers are `host-image` and the
    `opennsl-modules` packages: the row's "way down" is empty when a place's
    chain has depth 0 (`finding.Ends`).
  - Readiness (`internal/finding/readiness.go` around lines 96–105) requires
    `st.parent_id = branch`. The demo's tags have no parent, so Home reads
    "nothing has been released from this branch and scanned here" for
    openpsirt main/container although v1.0 and v1.1 are scanned. The Streams
    screen offers "Cut from" but nothing says readiness depends on it, and a
    tag declared without one cannot be corrected (no PUT). Both demo tags
    show "Cut from —".
  - `Releases` (`internal/finding/releases.go` around lines 56–88) selects
    from open findings, so a build with zero open has no row: openpsirt's
    `binary` variant (scanned, 0 open) is absent from `/releases` and from
    the "Open findings by release" bars, reading as unscanned. Drive it from
    `target` with a finished run and left-join the counts.
  - `GET .../issues/OPENPSIRT-2026-0001/advisory` → 409 "set
    OPENPSIRT_PUBLISHER_NAME and OPENPSIRT_PUBLISHER_NAMESPACE" on the demo
    (REM-20 is deployment configuration), so the CSAF path is undemonstrable
    out of the box. Set a publisher in `make demo`.
  - Mentioning somebody who cannot read the finding: `POST
    /v1/decisions/61/comments` by private-triage with `@proxy:rev-public` →
    201; rev-public's notifications stay empty (ACC-63 holds). ACC-59 says
    mentioning is "refused while composing"; only the candidate list
    enforces it. Refuse the write, or return the dropped mentions so the
    interface can say so.
  - Capability-only grants: `approver`-only and `reporting`-only resolve to
    `reach: []`, every product "not declared"; `POST /v1/people` and the
    People screen accept the grant silently. Warn on grant; show "sees
    nothing" beside such a person.
  - "What this line would inherit" (REL-07) lives at the bottom of the
    Inventories screen of the *new* line (`Inventories.tsx` around line
    173), where nobody making a branch looks. Surface it from Branches and
    tags when a branch is created, and on home as "v1.1 inherits nothing
    yet".
  - The openpsirt container's inventory list shows twelve identical
    "Completed · — · —" rows with an empty Serial column; "7587" on the sonic
    row lacks a separator.

---

## The ten that change what the tool is

- [ ] **V1 · Disclosure in the interface.** **L**

  `EvidenceBody` and the list's `FindingBody` (openapi.yaml, `FindingBody`
  around line 1636, `EvidenceBody` around 1421) carry no `disclosed`,
  `visibility` or `disclose_at`; `web/src/screens/Finding.tsx` and
  `Findings.tsx` have zero references to disclosure (one comment at
  `Findings.tsx:17`). The undisclosed recorded flaw `OPENPSIRT-2026-0001`
  renders "Unrated · Undecided · 1 location". Referenced nowhere under
  `web/src` (grep, excluding the generated schema): `GET /v1/disclosing`,
  `GET/POST …/issues/{v}/disclosure`, `POST
  /v1/disclosure-extensions/{id}/approval`, `PUT …/issues/{v}/builds`
  (already in the working notes), `GET …/issues/{v}/advisory`. So: no
  embargo date on the finding, no way to move it, no approval of an
  extension, no list of what is approaching disclosure, no advisory preview,
  and somebody with private-read cannot tell which rows they must not talk
  about.

  Change: a **Disclosure card** in the finding header row (state chip ·
  embargo ends · extend… · advisory ↗ · which builds it affects); a chip on
  the list row from a `disclosed` field on `FindingBody`; a **Disclosing**
  screen off the rail from `/v1/disclosing`, soonest first, past due on top,
  with extension approval on it.

- [ ] **W1 · Fix bundles: a "By fix" view with one act for Affected plus a fix target.** **M, high impact**

  `GET /v1/products/sonic/findings?fixable=true` → total 5,047. Grouped by
  (component, version, `fixed_in`) they are **271** distinct upstream bumps;
  one kernel bump to `6.12.85-1` closes 917 rows. A fix target today is one
  checkbox per release on one finding ("Fix this in master broadcom",
  verified via `fix-targets`: `declared: 1, state: fixing`), keyed per issue
  × component, so declaring that bump means 917 checkboxes on 917 pages.
  "Affected" as an outcome writes a decision that needs no approval and
  "goes to remediation", yet the fix target is a separate control further up
  the page, and the Together claim (`Together.tsx`) cannot carry one.

  Change: a third view beside By issue and By component: one row per bump
  with the issues it closes, the worst severity, the exploited count, and one
  control for "Affected → fix in: [releases]". Compatible with the accuracy
  constraint: Affected hides nothing, and a fix target is intent the scan
  verifies (REM-06). It is also the release coordinator's list (R9), and it
  removes the N separate "Affected, fix in X" cards an approver sees today
  only because there is no bundle.

- [ ] **R1 · Export of any filtered list; a paged Audit screen.** **S–M**

  No `text/csv` or `encoding/csv` anywhere in `internal/`; findings `limit`
  max 200 (`internal/httpapi/findings.go` around line 157), audit max 500
  (`audit.go` around line 77). `web/src/screens/Audit.tsx:65` asks `limit:
  500` with no paging and line ~206 says "Narrow the dates to print the
  rest"; Print is the only take-away. A year with 600 dismissals cannot be
  produced as one document; "all open criticals on the shipped release, as a
  spreadsheet" is 39 paged calls. `DESIGN-api.md`, `DESIGN-interface.md` and
  `docs/` do not mention export; ACC-07 already names exports as something
  visibility covers.

  Change: `format=csv` and JSON on findings, decisions, audit, comparison and
  running-out, streamed through the same visibility rules, with the floor
  stated in a header row (RPT-14); paging on the Audit screen; a download
  beside Print on Audit, Findings and Compare.

- [ ] **F1 · One signed outbound webhook, and a stored ticket URL.** **M**

  grep finds no webhook, Slack or Teams code; NTF-01 and REM-11/12 record
  the seams (`DESIGN-remediation.md` "No hand-off to an external tracker").
  Nothing leaves the application except SMTP. Every comparable tool
  integrates with a tracker and a chat channel; without one, fix targets are
  wishes and approvals are found by polling the queue.

  Change: one signed HTTP POST per notification kind and per operational
  condition, configured per deployment, carrying the NTF-15-safe body; that
  one mechanism gives Slack, Teams, Jira-by-automation and paging without an
  adapter each. Beside it, a stored link field (ticket URL) on a claim or fix
  target, and later a one-way "open a ticket" button (REM-11).

- [ ] **F2 · Supplier and distribution VEX applied as statements.** **M**

  `internal/sbom/openvex.go` reads statements (status, justification,
  impact/action statement, products by purl) only from the documents a build
  uploads with its inventory (`internal/httpapi/scans.go`, `internal/ingest`);
  `internal/sbom/suppression.go` (lines 19–26) honours `not_affected`,
  `fixed`, `under_investigation`. There is no administrator path and no
  per-product statement store, and no consumption of distribution VEX or OSV
  beyond what the scanner folds into `fix_state` (which does carry Debian's
  "declined", shown in the list). On the demo, 5,088 issues sit at one kernel
  package and 1,113 of 1,125 no-fix findings are distro packages (REJ-13's
  measurement); Debian publishes machine-readable "not affected / no-dsa"
  judgments for exactly these, and each is retyped as a TRI-32 claim a second
  person must read.

  Change: an administrator uploads a VEX or CSAF-VEX document against a
  product; matching findings show the supplier's statement on the row and
  the finding, filter on it ("distro says not-affected"), and offer a
  prefilled not-applicable claim; a person still submits and a second person
  still approves. **Never applied silently**: that would be a third party's
  claim standing as ours (REL-03 keeps them apart; REJ-10 stays intact).

- [ ] **V2 · Deadline, owner and age on rows and headers; "due soon" on Home.** **M**

  Settings define five remediation windows and Home has an Overdue tile, but
  no row and no finding header shows "due 2026-09-08 · 2 days".
  `/v1/running-out` returns `due`, `days_left`, `assigned_to` (openapi.yaml
  around lines 2297–2335); the findings row body has none of them,
  `Finding.tsx` mentions "deadline" only in prose (lines ~274, ~1196), and
  the detail body has `assigned_to` but the row does not. Neither body
  exposes `opened_at` although the finding row stores it (DESIGN-findings
  "A finding carries when it opened"); the only age shown is "N years old"
  from the CVE year (`Findings.tsx:15–25`), which is the age of the CVE.
  Home shows only what is already overdue, never what is due this week. The
  finding header line is "sonic · master · broadcom · 44 locations · 0 of 44
  decided": no due date, no owner, no disclosure.

  Change: `due`, `days_left`, `assigned_to`, `opened_at` on the row and
  evidence bodies; a Due column coloured past due; owner and age chips in the
  header beside severity ("open 12d"); a "due in 7 days" tile on Home and a
  Deadlines section on Reports; a named-assignee column.

- [ ] **P1, P3 · Readers see the record; `reporting` becomes statistics without rows.** **M**

  P1: `readable()` in `internal/triage/decision.go` (around line 716) admits
  only somebody who may *decide or approve* at that visibility. `rev-privread`
  (private-read on sonic) reads finding 0003, its attachment and its
  extension history, but `GET /v1/decisions/61` → 404, `standing: []` on the
  finding, `/v1/audit` total 0, `/v1/review-queue` total 0; `rev-public` sees
  0 of 91 sonic decisions. Documented in `DESIGN-triage.md` ("a reader who
  holds neither right sees a decision as one that is not there") and
  internally consistent, but a reader sees "deferred" and cannot see why or
  by whom; the screen called **The record** is empty for anyone who is not a
  triager; ACC-09 ("when disclosed the whole record goes public") is true
  only for people who could already see it. Every compared tool lets a Reader
  see the analysis and its history.

  Change: decisions, revisions, approvals and comments readable at the
  *finding's* visibility (`readableFindings` semantics); acting on them stays
  at triage/approver; the queue can stay triager-only. One predicate swap
  plus the matrix tests it changes.

  P3 (DECISIONS §7 asks): `access.Reporting` has one non-test use,
  `may_report` in `/v1/session/me` (`whoami.go` around line 128), read by
  nothing in `web/src` beyond the type; every report endpoint is
  `anySubject` and narrowed. Recommended: **aggregates without rows**. A
  subject holding `reporting` but no read role gets counts, trends,
  remediation metrics, the SLA rate, throughput and release bars for the
  product, figures that name no issue, component or justification; every
  list-shaped report (audit, running-out, repeat deferrals, comparison, the
  disposition register) stays gated by read visibility. Close the one
  explicit leak, an aggregate over *undisclosed* work, by splitting the role
  public/private like the read roles, or by folding undisclosed counts into
  NTF-18's "numbers, not names" rule, which has already judged a count
  acceptable for private work. With P1 in place, readers also get the
  record, which is the auditor's role. Do not add a check to the report
  endpoints before this is decided.

- [ ] **R2, R6 · A disposition register per build, with `as_of`; an SLA compliance rate.** **M, no new storage**

  R2: `/v1/audit` lists what was *decided*. The auditor's first question is
  the complement: for release X, every known vulnerability with its
  disposition *including "nobody has decided"*, who decided, who approved,
  when, the justification, and whether the deadline was met. Today that is
  the findings list with `stream=<tag>` and `state=undecided|waiting|agreed|lapsed`
  run four times and joined by hand, and the row carries none of the who or
  when. Every count is evaluated against today's vulnerability data and
  today's decisions (RPT-09 says so); "what was known and decided on the day
  v1.0 was cut" has `opened_at`, `closed_at`, `proposed_at`, `ended_at` and
  approval timestamps stored and no query. Change: one row per issue ×
  component in a named build with state, outcome, justification, proposer,
  approvers, dates, deadline and met; an `as_of` parameter on it and on the
  audit list; exportable (R1). Optionally a frozen "release attestation"
  written when a tag's first scan finishes.

  R6: closed rows keep `due_at` (only *open* rows lose a deadline at
  end-of-life or below the floor, `internal/finding/due.go` around lines
  394–398), so the inputs exist. Change: per window and severity, closed
  within deadline / closed total, open past deadline / open total; split
  deferred-by-decision from plainly late (REM-15); say it in
  `DESIGN-reporting.md`. If "what was the deadline *then*" must be exact, a
  deadline history is the one thing to store.

- [ ] **R8, R9 · Release notes that a coordinator can use; a Release screen with the plan.** **M**

  R8, measured on the demo (v1.0 → main container, `GET
  …/comparison/notes`): output begins `## Security fixes in main container`
  followed only by `### Still present` with 24 CVEs (`internal/finding/notes.go`
  around lines 33–58). The title uses internal stream/variant names.
  `ChangedBody` has vulnerability, component, severity, because: a fixed
  entry says "the component was upgraded" and not *to what*, though the later
  build's version is in the row the comparison reads (`compare.go` around
  lines 75–88 groups on `c.name`, dropping `c.version`). Still-present
  entries carry no disposition: the comparison does not join decisions, so
  the note cannot say "not affected: vulnerable code not present" for what
  was argued, nor separate REM-13's "open by choice" from "open by neglect".
  A note whose build fixes an embargoed flaw does not say it omitted one
  (public-only default RPT-06 is right; the count is missing).

  Change: heading follows content ("Security changes between v1.0 and
  v1.1"); "Still present" opt-in for the customer form; a lead line with both
  builds, the date and the scanner/DB version; `from_version` / `to_version`
  / `fixed_in` on fixed entries ("upgraded 3.5.6 → 3.5.7");
  `still_present[].state/outcome/justification` and a "Known issues" section
  printing the standing justification; a count of omitted undisclosed
  entries in the response and a line on the screen.

  R9: fix targets exist only per finding (`GET …/fix-targets`; `FixingIn` in
  `fix.go` around line 117 answers per issue). "Everything declared for
  v1.2, and which of it has landed (missed / fixing / clear)", the report
  REM-03/REM-09 were written for, has no endpoint and no screen. `stream`
  has `created_at` only (`00002_catalog.go` around lines 55–67);
  `trend/releases` labels its axis with `cut` = declaration time, and both
  demo tags read `2026-09-05T15:28:52Z`. A coordinator's bundle is four
  places today: notes, VEX (absent, R5), CSAF documents for own flaws, plan
  status (absent). Change: `GET …/variants/{v}/plan` inverting fix targets
  per build; an optional `released_on` on a tag, falling back to the first
  scan's `built_at`; a tag's parent settable after the fact; a **Release**
  screen per tag gathering notes, plan, VEX and advisories; a publisher in
  the demo seed.

- [ ] **W2 · Group the same issue across sibling components.** **M**

  2,831 of 7,612 rows (37%) are the same CVE at another binary package of
  the same source: 1,016 CVEs at 2–14 components. Examples: curl /
  libcurl4t64 / libcurl3t64; vim at five packages; `CVE-2025-68121` at
  `stdlib` go1.25.6, go1.24.9 and go1.25.3 under three Go binaries;
  `golang.org/x/crypto` at four versions. Each is a separate decision; the
  guided review said "Other versions · 0" because the reach is keyed on
  *place* (component under the same consumer), correct per TRI-30, so three
  full decisions for one CVE in one product.

  Change: group rows by issue where components share an upstream or source
  name (the model already holds `UpstreamName`, `cyclonedx.go` around line
  306): "CVE-2026-9079 · 3 packages of curl · 61 locations" with one form.
  Presentation only (REL-02): one action still writes one decision per
  component and per place.

---

## Then

- [ ] **W3 · Sort, the missing filters, page size, selection, saved filters.** **M**

  Server filters on `GET /v1/products/{product}/findings`
  (`internal/httpapi/findings.go` around lines 140–215): stream, variant,
  severity (floor), exploited, fixable, below_floor, component, q (name
  substring), ecosystem, under, under_build, beneath, state, outcome,
  assigned (me/somebody/nobody), reassessed, unconfirmed, exclude, limit,
  offset. The interface exposes all of them (`Findings.tsx` around lines
  90–123) and filter state rides in the URL (UIX-11). Missing, in the order a
  triager reaches for them:

  | Filter / control | Status | Note |
  |---|---|---|
  | Issue identifier (CVE/GHSA/alias) | absent | X4 |
  | Sort | absent; order is urgency only (`read.go` around lines 152–157), no `sort` parameter | sorting by fix version, EPSS, age, locations, component is how people build a batch |
  | EPSS range / KEV only | `exploited=yes` exists; no EPSS threshold | Home already reasons about EPSS |
  | Age / opened since / new since my last visit | absent | `finding.opened_at` exists; `workSince` exists in `assign.go` around line 489; no list filter reads either |
  | Deadline / overdue / due within N days | absent in the list | Home has an Overdue tile and `/v1/running-out` exists; the list cannot show the same rows |
  | Fix state = none / wont-fix (upstream declined) | `fixable` only | "declined" is shown in the Fixed-in column but cannot be filtered: the rows that need a *decision* rather than a bump |
  | Differs between variants (`builds=1`) | absent | the 27 real variant differences on the demo (all avahi on mellanox) cannot be found; UIX-53's own measurement motivates this |
  | Weakness (CWE) | stored, shown on the finding, not filterable | `DESIGN-findings.md` argues for it |
  | Assigned to a named person | only me/somebody/nobody | the Work screen has it; the list does not |
  | Sent back to me | State column only | W6 |
  | Page size / column choice | fixed at 50 / fixed columns (`Paged.tsx`) | 7,612 rows is 153 pages |
  | Saved filters (UIX-10) | decided, not built (no code in `web/src`) | the cheap half of subtree ownership that §7 says to try first |
  | Multi-select in the list | absent; there is a cursor but no selection | Unassigned has selection, only for assigning |

  Review queue: `GET /v1/review-queue` takes only `mine`, `limit`, `offset`.
  No product, kind (finding / together / extension / returned), proposer,
  age, outcome or severity filter, and no sort; an approver on several
  products reads one interleaved list. Add those, and group cards that share
  outcome + justification + component.

- [ ] **W4 · Size the bulk claim before submit; let it span pages; more outcomes.** **S/M**

  Measured: open the component's bulk page (reachable only from the
  by-component row or by URL), narrow `driver`, "Select all 500 shown", type
  a reasoning, Submit → refused in 859 ms: "that is 22000 findings and the
  limit here is 2000". The cap is counted in findings and the screen counts
  in issues: a kernel issue sits at 45 places, so the default cap of 2,000
  is ~44 issues per action; 805 "driver" candidates become 18 hand-narrowed
  claims, each a separate card with its own outlier table. TRI-35 is right
  that the bound is on rows written; the screen lets somebody discover it
  after typing. The candidate list has no severity, fix-version, EPSS or
  exploited sort or filter (only a description `contains` term). The outcome
  select offers only Not applicable / Won't fix / Affected (`Together.tsx`
  `<Claim>`): no Deferred (a kernel bump scheduled for the next release is
  the common bulk deferral) and no Already fixed (TRI-51 is precisely a
  per-source-package bulk claim). Select-all is page-bound at 500 of 805.

  Change: compute `places × issues` as the selection changes and say "you
  may pick about 44 of these"; offer "select the first N shown"; allow a
  second page into the same claim; chips to hide exploited and critical
  before claiming (what the approver's TRI-46 outlier table will flag
  anyway); add Deferred and Already fixed; consider a per-product cap.

- [ ] **W5, W6 · Fewer steps per decision; the proposer told; staleness reminders.** **S**

  Measured cost of one decision from the list: ana filter (1), row (2),
  outcome (3), justification (4), reasoning (5), Submit (6), "Review and
  submit" (7), "Confirm and submit" (8); ben: queue (1), Approve (2). In
  triage mode: `j`, `Enter`, `2`, type, `Esc` (to leave the editor), `r`,
  `Enter`, `Enter`, and the next row opens itself with the "Submitted"
  banner. The two-step review sheet (`web/src/ui/Review.tsx` always starts at
  step 0 and needs a second Enter) runs even when "Other versions · 0"; at
  ~150 decisions a day that is ~300 clicks. Triage mode is off by default and
  its state lives only in `?mode=triage`; UIX-43 says "one control away", but
  for people who do this all day it should be remembered per person like the
  look (UIX-50). Defaults are fixed constants (`Decide.tsx:74–78`) rather than
  last-used. `Ctrl+Enter` works from inside the editor and is not advertised.

  Reject → revise works (ben: queue, Reject, reason, Reject; ana: bell,
  notice link, Revise, text, Save revision; the claim leaves the queue and
  returns after the revision). But a sent-back claim is invisible on ana's
  home and on the queue's "Mine, pending" tab (`Queue.tsx` `mine` shows
  waiting claims only); only the bell (kind `sent-back`) and the list's State
  column ("Rejected") show it. Nobody is told when their claim is approved,
  undone or lapses; the proposer finds out by re-opening the finding.

  Change: one confirm when nothing is offered (or the Submit button carries
  "Submit · 30 locations, 1 matching build"); remember triage mode per
  person; last-used outcome and justification within a session; advertise
  Ctrl+Enter; notify on approval, undo and lapse; a "Back to you · N" panel
  on home and a tab on the queue; staleness reminders as settings something
  reads: claim waiting more than N days (approver), deferral ending in 7
  days (proposer), sent-back untouched for N days.

- [ ] **W7, V4, V5, V7 · The finding page, the list, and the decision page for people not in triage mode.** **M**

  Finding page order: description, six of thirty dependency chains (each
  starting "sonic-broadcom › host-image › …", ~500 px), holder, fix targets,
  assessment, similar decisions, *then* Decision at ~1,550 px on a page that
  runs to 2,000–2,800 px; on a 1,000 px viewport the primary action is three
  screens down. The pending case already shows the standing decision card
  first. The justification select shows bare tokens
  (`vulnerable_code_not_in_execute_path`) while `Together.tsx` uses labels.
  After submitting, the page offers "Go to the review queue" but not "next
  finding"; there is no next/previous. The deferral form's hint is static
  ("Under the deferral threshold this stands on its own; over it, a second
  person") and does not say which side of the line the chosen date falls
  on, though `NeedsApproval` (`internal/triage/queue.go` around line 667)
  already computes it; the queue card afterwards correctly says "Put off 116
  days in total".

  List: columns are Severity, Issue, Component, Build, EPSS, Fixed in,
  Locations, State; no title or one-line summary (the `FindingBody` carries
  no description; the detail body does), so a triager sees "CVE-2026-74280 ·
  linux-image" fifty times and the arrow preview costs a click per row.
  Component names wrap, "hide" and the version add a line, "· one of 2
  builds" adds another: a row is ~90 px and 50 rows are ~5,000 px
  (`findings-product.png` was 4,971 px tall); triage mode's j/k walks the
  same tall rows. Two words for no severity: `GO-2026-5932` shows "Unknown",
  `OPENPSIRT-2026-0001` shows "Unrated".

  Counts in three units unlabelled: rail "Unassigned 7,644" (findings, all
  products) beside Home "5,803 open issues" (distinct vulnerabilities); under
  build scope rail "Unassigned 7,616" beside "Findings 7,589"; tree root
  5,771 (distinct issues beneath a path); Products 7,616; Reports "Appeared
  5,803". The vocabulary rule is right and only Home's tile explains it.

  Decision page (`/decisions/:id`): outcome chip, reasoning, approvals,
  comments. Missing: scope (the queue card and the standing card both have
  it), justification code, revision history (queried at `Decision.tsx` around
  line 235, nothing rendered), activity timeline, and any approve/reject
  control (ben sees no action; approval lives only on the queue).

  Change: two columns on the finding: evidence left (description, match,
  path collapsed to the distinct consumers with one expandable chain), a
  sticky action column right (standing decision or the form, assignment,
  deadline, disclosure); labels with a one-line meaning per justification;
  next and previous within the current filter; one form for Affected plus fix
  target; the deferral form says "116 days · needs a second person" as the
  date changes. List: a summary line under the identifier (first sentence or
  CVE title), component truncated with a tooltip, "one of 2 builds" behind
  the number, a compact density forced in triage mode, one word for no
  severity. Units on every count; rail badge and page title agree. Decision
  page parity with the standing card, with the approver's action on it.

- [ ] **W8 · Decide with reach in one request.** **M**

  Other builds are applied sequentially, one POST each, with partial success
  reported per build (`Decide.tsx` around lines 196–236), and the reach shows
  a sample (`REACH_SAMPLE = 8`, line ~52). Fine at 2 builds; at 10 branches
  × 3 variants the confirm step is 30 round trips. Change: a single
  decide-with-reach endpoint so the act is one transaction (DAT-30) and one
  answer.

- [ ] **F3 · A recorded flaw gets its CVE, its reporter, its dates, and its advisory a revision record.** **S each**

  The record body (`internal/httpapi/enter.go` around lines 77–89) carries
  summary, severity, vector, weaknesses, component, disclosed; no identifier
  field. `internal/advisory/advisory.go` (around line 304) anticipates "a CVE
  assigned later is another name for the same issue" and no endpoint records
  one, so the CSAF document ships `ids` without `cve`. No reporter, no
  received-at, no acknowledged-at, no credit line, so `acknowledgments` is
  empty and the ISO 29147 / CRA timeline (received → acknowledged → triaged →
  fixed → disclosed; CRA Art. 14's 24h/72h/14d) cannot be evidenced; the
  90-day clock starts from when somebody typed the flaw in. `advisory.go`
  (around lines 121–130) emits `tracking.status` but REM-18 stores nothing
  about issuance, so a second advisory cannot carry `revision_history` or
  increment `version`, which CSAF validators check.

  Change: `PUT /issues/{id}/aliases` recording a CVE or GHSA against a minted
  identifier, shown on the finding and used by the advisory; a reporter
  block (name, contact, received, acknowledged, credit) on a recorded flaw;
  a per-issue timeline view assembled from the rows that exist plus those two
  dates; record generated-at, by and a hash per advisory (a fact about a
  moment, which AGENTS.md allows). A public intake form stays out of scope
  (§8); this is the inside half.

- [ ] **R3 · An administration audit trail; comment history.** **M, new table**

  Good: a withdrawn decision is a state, not a delete (`triage.go` around
  lines 308–311); reasoning revisions are kept and an edit withdraws approval
  (TRI-24/25); withdrawn approvals stay with `withdrawn_at`; attachments
  redact to a tombstone; `two_people` is computed from the record
  (`Judged.BySomebodyElse()`, `triage/audit.go` around lines 64–72) rather
  than trusted. Gaps: comment edits overwrite with only `edited_at`
  (`internal/triage/comment.go` around lines 32–35: "what it said before is
  not" kept); role grants carry `revoked_at` but no `granted_by`
  (`00010_access.go`); settings have `updated_at` and no history or actor
  (`00001_settings.go` around lines 31–35), so "who changed the critical
  window from 7 to 30 days, and when", which moves every open deadline
  (REM-26), is unrecoverable; the same for end-of-life dates and the triage
  floor, which silently take findings off the clock (REM-16, REM-27). SEC-08
  is about secrets in logs, not an admin trail; no decision covers one.

  Change: an `administration_event` table (actor, action, before, after, at)
  written by the settings, role, end-of-life, floor, key and token handlers,
  readable to administrators from the Audit screen; comment history kept
  like reasoning revisions.

- [ ] **R4 · Provenance per run; the Audit screen's dropped fields and missing filters.** **S**

  `scan_run` stores `scanner_version` and `database_version` per run
  (`migrations/00009_finding.go` around lines 117–119), but
  `ReceiptsOutputBody` returns one `measured_against` for the newest
  finished run and per-receipt items do not carry them; the finding row does
  not name the run that opened or closed it. "Which grype and which DB
  produced the finding you dismissed on 3 March" is unanswerable. The Audit
  screen drops `fixed_version` (`internal/httpapi/audit.go` around line 44;
  no reference in `Audit.tsx`), the one field that checks an already-fixed
  claim; its `OUTCOMES` filter omits `already-fixed` though the API enum has
  it (TRI-51); rows carry `id` and are not linked to the decision or the
  finding. `/v1/audit` cannot filter on `two_people=false` (the exception
  report an auditor wants), nor by proposer, approver, issue or component.
  Reports shows `time_to_fix` sorted alphabetically (critical, high, low,
  medium); `RepeatBody.last_until` is returned and not shown.

  Change: scanner/DB version on each receipt item and on the finding ("first
  reported by run N, scanner X, DB Y"); the four audit filters; show
  `fixed_version`, add `already-fixed`, link rows; severity order on
  time-to-fix; show `last_until`.

- [ ] **R7 · The manager's figures, none of which need new storage.** **S each**

  What exists: backlog trend by week and severity (Home); fixed/appeared,
  time-to-fix by severity, aging buckets (Reports); overdue count; per-person
  held and overdue (Assignments); repeat deferrals; queue and unassigned
  counts. What does not, with the data it would read:

  - Breakdown by product and branch on one screen (the store already groups
    by product in `Releases`): open, critical, exploited, overdue, undecided,
    waiting approval, fixed-in-window.
  - Exploited-and-open, and undecided-critical, as tiles linking to the
    filtered list (`exploited=true` is 3 on the demo and appears nowhere as a
    figure).
  - Time-to-decide (`opened_at → first proposed_at`) and approval latency
    (`proposed_at → approved_at`), averages and p90 per severity; the queue's
    `DecisionDetail.age_days` exists per item and is never aggregated.
  - Throughput per person: proposed / approved / sent back / withdrawn within
    the window, narrowed like Assignments (a count is a disclosure).
  - Aging buckets × severity band, and a second cut by state (undecided /
    waiting / deferred); `BucketBody` has one count today.
  - A "postponed" count with the nearest date beside overdue (REM-15's
    deferred-by-decision vs plainly late).
  - `opened_after`, `closed_after`, `proposed_after` filters on findings and
    decisions, and a weekly "what changed" section on Reports (the trend
    gives counts, not contents; the digest NTF-16 is per person, not this).
  - Windows past 90 days on the Reports screen; the API allows 366.

- [ ] **R5 · A VEX per build from standing decisions.** **M; already recorded and deferred**

  `advisory.ErrNotOurs` refuses any scanner-reported issue (REM-23), and the
  document is not the VEX profile (`DESIGN-remediation.md` "What is not
  built"; `DESIGN-triage.md` "there is no export yet"). But VEX is precisely
  the document for third-party components ("we ship openssl 3.5.6, CVE-X,
  not_affected, vulnerable_code_not_present"); every `not-applicable`
  decision already uses the VEX justification vocabulary (TRI-06); TRI-04
  already says how a deferral exports (as affected); and the tool *ingests*
  OpenVEX from builds without being able to emit the equivalent for a
  release. Customers running their own scanners on a shipped image ask
  "which of these are you not affected by" more than they ask for an
  advisory per own flaw.

  Change: treat REM-23 as being about *advisories*; generate CycloneDX VEX
  or OpenVEX per (product, stream, variant) from approved `not-applicable`
  and `already-fixed` claims, public-only by default like the comparison
  (RPT-06); CSAF-VEX later.

- [ ] **V6 · Screens the API already serves.** **M**

  - **Product overview.** `App.tsx` (around lines 71–74) routes `/products`,
    `/products/:product/streams`, `/products/:product/variants`, nothing at
    `/products/:product`. The Products table is an admin control surface
    (inline `<select>` for triage floor, inline `<input type=date>` for end
    of support, no label, no save affordance). "How is SONiC doing", open by
    severity per build, overdue, pending approvals, last scan per build,
    quiet builds, EOL, who holds what, is one call each (`releases`,
    `running-out`, `review-queue`, `scanning`, `assignments`) and no page.
  - **Scan run detail.** Inventory rows (Received, Built, State, Opened,
    Closed, Sent, Placed, Serial) are not clickable. What a run opened and
    closed by severity, parse warnings, the components not placed ("6,815 /
    6,854" with 39 unexplained on the demo), scanner and DB version for
    *that* run: the data exists per run (ING) and has nowhere to go.
  - **Per-person page.** `GET /v1/people/{identity}/assignments` exists;
    Assignments shows a summary table only (`Work.tsx` around lines
    188–191).
  - **Disclosing list and advisory preview** (V1).
  - **Personal tokens, the daily digest, the version.** `POST/GET/DELETE
    /v1/tokens`, `PUT /v1/session/me/digest`, `GET /v1/version` are referenced
    nowhere under `web/src`. A person cannot mint a token for scripts or turn
    the digest on from the interface (the README says it is "off until asked
    for"); no version is shown anywhere. Add all three to the "You" menu.
  - **Batch undo.** The queue says "undo reverts them all"; `DELETE
    /v1/approval-batches/{batch}` is unused.
  - **Flaws recorded here.** The Record screen has no link to what was
    recorded before; the findings list filtered by `matched=recorded` would
    do.

- [ ] **V8 · Smaller hierarchy items.** **S each**

  - Queue card: nothing says why the claim might be wrong. No reachability
    or path, no previous decisions on the issue elsewhere, no "N other
    findings on this component are undecided"; the detail body already has
    `similar` and `previous`.
  - By-component view ranks by issue count with no severity split:
    `vim-common` with 44 issues outranks a component with 3 criticals. Add a
    worst-severity chip and a severity strip per row, sortable.
  - Tree node panel: "5,650 open issues in everything under it by this path ·
    0 at this component here" with a link; add a severity strip per node and
    the component's own open issues inline (top 5, link to the rest).
  - Reports cards link nowhere: "What was argued away" is one sentence with
    no link to the record filtered to dismissals; "What is aging" rows are
    not links. Every figure should open the list it counts.
  - The "Open findings by release" chart has no legend and no totals;
    `/v1/products/{p}/releases` returns `by_severity` per build.
  - Settings cards beyond the three hand-titled groups (`Settings.tsx` around
    lines 57–84) are titled by the key's last segment: "Every", "Max size",
    "Quota", "Absent after", with bytes shown raw (26214400). Group them
    (Scanning, Attachments, People) and format sizes.
  - Dates in four formats: "2026-09-05 11:32", "2026-09-05T06:27:00Z"
    (scanner data on Inventories), "0d", "today", "2 years old". One
    absolute and one relative form.
  - Trend copy on a one-week-old deployment: "Backlog growing: new exceeded
    resolved in 1 of 12 weeks; open up 5,803 across the range" and "Critical
    went 0 → 389". Suppress until N weeks of data exist.
  - Record-a-flaw's description placeholder ("The management socket answers a
    request before anyone has authenticated.") reads as content.
  - Unassigned is the only cross-product list and has the fewest columns
    (Severity, Issue, Component, Build, Locations): bring it to parity, or
    make Findings work at scope "all" (V9) and retire it as a filter.
  - Notifications: after propose/approve neither ana nor ben had anything
    in `/v1/notifications`; the queue is the mechanism, but a person not
    looking at the queue gets nothing in-app (see W6).

- [ ] **V9 · A vulnerability-centric, cross-product view.** **L, backend and screen**

  The rail's Findings, Dependencies and Inventories are greyed out at scope
  "all" (`Shell.tsx` `needs`); Unassigned and the overdue tile are the only
  cross-product views; findings are per product in the API and
  `/v1/assessments` carries ratings only. "Which products ship openssl
  3.0.x" is answerable per product (`/v1/products/{p}/findings/components`)
  and not across. PSIRT work starts from a CVE as often as from a product.

  Change: a cross-product findings endpoint and a page keyed on an issue:
  every product, build and place it sits at, with the standing decision in
  each; X4 lands here.

- [ ] **F4 · Run deltas on the pipeline receipt.** **S**

  `DESIGN-access.md`: "A key reads back its own receipts and nothing else";
  receipts say parsed / not parsed. Per-run deltas are computed
  (`DESIGN-ingest.md` "What each run changed"). A build that introduced a
  known-exploited critical is green in CI. Change: opened and closed counts
  by severity and exploited on the receipt, a count about the key's own
  upload so ACC-50's narrowing holds; the smallest possible CI gate.

- [ ] **F5 · Retained SBOM download; labels; air-gap documentation; SSVC words.** **S each**

  - ING-07 retains tag documents; no operation returns one. "Send me the SBOM
    you scanned for v2.4" is answered from the build system. Add `GET
    …/scans/{id}/document` for retained tags.
  - No tags or labels on findings, claims or issues (grep). Teams tag
    "customer-escalated", "release-blocker", "waiting-on-vendor". A free-text
    label set on a claim or issue, filterable.
  - ING-22 calls air-gapped scanner database import a hard requirement;
    `docs/configuration.md` "Scanning" offers only `OPENPSIRT_SCANNER_PATH`;
    the image sets `GRYPE_DB_CACHE_DIR`. A paragraph, or a `make` target that
    produces the offline bundle the chart mounts.
  - No SSVC anywhere. The RNK-03 signals (exploited → reaches customers →
    severity → likelihood, packed by RNK-06) are nearly SSVC's deployer
    inputs. Label the bands Act / Attend / Track on the finding and in
    reports; optionally "automatable" and "mission impact" as per-product
    settings. Ordering only; changes no count (REJ-10).

- [ ] **Automation as evidence, never as a decision.** **S → L**

  Judged against "no reduction in accuracy, do not encourage dismissals".
  What exists and was verified working: carry-forward by lookup (REL-05/06;
  mellanox reached automatically every time), cross-line carry with reasoning
  (REL-07/08), Together claims with the narrowing recorded (TRI-32/34) and
  outliers for the approver (TRI-46), extension of an approved claim to a new
  issue at the same component (TRI-47: "Apply decision #30 to this issue"
  appeared on the next openssl finding after ben approved), previous and
  similar decisions offered back, urgency packing (RNK-06) with the reason
  shown, triage floor per product (TRI-43), end-of-life switching clocks off,
  rescans on a schedule, build suppressions from OpenVEX kept as their own
  layer (REL-03), lapse marking in one statement, Already fixed for distro
  backports (TRI-51), batch approval under a name with batch undo.

  | Opportunity | Compatible? | Shape here | Size |
  |---|---|---|---|
  | Distribution / supplier VEX | yes, as evidence with prefill | F2 | M |
  | Suggested outcome from history across products | yes | similar decisions are offered only at the same places in the same product; offer "decided in product X as not-applicable, same justification" as a prefill; reasoning travels, conclusions do not (REL-08) | S |
  | Rules that prefill claims | only as a draft generator | a saved predicate over the list filters, run after each scan, prefilling outcome, justification and reasoning for a claim a *person* proposes. One review argued a rule may itself propose a pending claim marked "proposed by rule X" with TRI-46 outliers; the other that TRI-07 needs two people and a rule as proposer leaves one, the rubber stamp TRI-45 was written against. Take the narrower form | M |
  | Kernel config awareness | yes, as evidence | accept the build's `.config` as an artifact; mark kernel issues whose subsystem/driver path is not built; offer "narrow to not-built" in Together with the config named as the narrowing (TRI-34); the SBOM cannot say this, a config can | L |
  | Reachability for Go binaries | yes, as evidence | govulncheck-style symbol reachability answers most `stdlib` / `x/crypto` rows; prefilled `vulnerable_code_not_in_execute_path`; out of proportion for the deb estate | L |
  | SSVC overlay | yes, ordering only | F5 | S |
  | Batch approval of similar claims | exists; missing the finding of them | queue filters and grouping (W3) | S |
  | Staleness reminders | yes | W6 | S |
  | Auto-accept low with no fix in a distro package | **no** | hides risk without a person; the honest versions exist (TRI-43, TRI-51); REJ-13 measured that "distro-maintained with no fix" is 1,113 of 1,125 no-fix rows, i.e. not a signal | — |
  | Assignment rules, round-robin, subtree ownership | already recorded (§7, ACC-54) | blocked in practice by X5; then saved filters (W3) | — |

- [ ] **P2 · Refuse assigning work to somebody who cannot read it.** **S**

  Admin assigned SONIC-2026-0003 to `rev-public` → 204; `GET /v1/assignments`
  (admin) says rev-public holds 1 open; rev-public's own list and
  notifications are empty (ACC-63, correct). `DESIGN-access.md` documents
  this ("assigns it and tells them nothing"); the effect is work that appears
  held in every administrative view and is in nobody's list, the blind spot
  ACC-43 is written to prevent. `WhoCanRead` already answers who may; refuse
  with a sentence to the assigner that does not reveal the visibility to a
  public triager.

- [ ] **P4 · Per-case need-to-know on undisclosed findings.** **L**

  Everybody holding `private-triage` on SONiC sees every embargoed SONiC
  flaw. Coordinated disclosure practice (FIRST PSIRT framework, ISO/IEC
  29147/30111) is built on case-level lists: three named people know about
  CVE-X before disclosure, not everyone with private access to the product;
  a contractor with private triage on one component sees the pre-disclosure
  RCE in another. The data model can carry it (a finding has an issue and a
  product; an allowlist table keyed on (product, vulnerability) intersected
  with `Reads(Private)`), and the query helpers (`narrowedBy`,
  `readableFindings`) are centralized enough that it is one more predicate
  rather than a re-audit. This is the feature that would cover most of what
  people ask multi-tenancy for, so REJ-09 stands.

---

## Appendix A · The permissions model as implemented, and what held

Sources: `DESIGN-access.md`, ACC-01..63, `docs/reference/privileges.md`,
`internal/access/access.go` (110–300), `internal/triage/decision.go`
(716–818), `internal/finding/assign.go` (40–80), `internal/finding/assessment.go`
(78, 406), `internal/attach/store.go` (52–95), `internal/httpapi/rights.go`.

| Dimension | Values | Where enforced |
|---|---|---|
| Product | one grant row per (person, product, role) | `Subject.grants` |
| Visibility | `public` (disclosed) / `private` (undisclosed); unknown reads as private | `Subject.Reads`, `access.Visible`, per-query `WHERE visibility IN (...)` |
| Read vs act | `public-read`, `private-read`, `public-triage`, `private-triage` (triage implies read at that visibility) | store and handler |
| Capabilities | `approver`, `assigner`, `reporting`: grant no visibility | handler (`requiring(...)`) and store |
| Global | `admin`: `Holds()` true for every role on every product | everywhere (X8) |
| Credentials | session or trusted header (person); personal token (person, delegated, optionally narrowed to one product, admin dropped when narrowed); pipeline key (send + own receipts) | `resolve.go`, `token.go` (141–162) |
| Mode | `direct` grants or group-bound; never both | `binding.go` |

Visibility is applied in the query by subject on: findings, decisions,
queue, audit, unassigned, holdings, trend, remediation, readiness, releases,
comparison, tree counts, attachments, disclosure lists,
notifications-at-write-time, mentionable. No store read path serving a
caller takes no subject; the subject-free ones (`Recompute`, `Sweep`,
`Lapse`, scan `Apply`) are jobs.

**Comparison with other models.** Dependency-Track: global permissions
(`VIEW_PORTFOLIO`, `VULNERABILITY_ANALYSIS`, `POLICY_MANAGEMENT`, …) on teams,
opt-in portfolio ACL, API keys carrying a team's full permissions; OpenPSIRT
has the disclosed/undisclosed split, approver ≠ proposer, ingest-only keys
with pinned scope, enumeration-resistant answers, and per-product ACL on by
default; it lacks team as a first-class grantee outside bound mode.
DefectDojo: global roles plus per-Product-Type / per-Product membership
(Reader, Writer, Maintainer, Owner, API Importer), staff superuser; OpenPSIRT
has visibility of *individual findings* within a product, assign separated
from triage, second-person approval; it lacks an **Owner** (product-level
administration without deployment admin) and per-product delegation of user
management. GitLab: project/group roles, no embargo concept; OpenPSIRT lacks
group inheritance of roles. Jira-style issue security levels and PSIRT
trackers: per-case lists (P4). Also absent and judged not needed now:
stream/variant scope for people (keys have it, ACC-11; assignment covers the
product, a deliberate simplification), time-bound grants (group-bound mode
covers it), attachment visibility separate from the finding (a file follows
its issue, `attach/store.go` 81–95, the right default), comment-only or
external-reporter roles (§8).

**Probe matrix**, 13 identities × 68 endpoints, 884 rows. Identities: `dev`
(admin, header), `rev-admin2` (admin, tokens), `rev-private`
(private-triage on sonic), `rev-privread` (private-read), `rev-public`
(public-read), `rev-pubtri` (public-triage + approver), `rev-report`
(reporting only), `rev-approver` (approver only), `rev-other`
(private-triage on openpsirt only), `tok-public`, `tok-admin-narrow` (admin
token narrowed to sonic), `tok-admin-wide`, `key` (pipeline key for sonic),
`anon`. Planted: `SONIC-2026-0003` (recorded flaw, critical, two builds,
summary containing `ZETA-7731`), `SONIC-2026-0004` (openssl, high), decision
61 on 0003 (reasoning `ZETA-7733`, approved by `dev`), comments 1–2, one
attachment (`REVIEW-SECRET-evidence.txt`), disclosure extension 1 (reason
`ZETA-7735`, past threshold, approved by `dev`), assignments 0003 →
`rev-public` and 0004 → `rev-privread`.

| Identity | Findings total (sonic) | Sees 0003 / decision 61 / attachment / extension | Aggregates narrowed |
|---|---|---|---|
| dev, tok-admin-wide | 7,619 | yes / yes / yes / yes | n/a |
| rev-private | 7,619 | yes / yes / yes / yes | as designed |
| rev-privread | 7,619 | finding yes; **decision 404, standing `[]`**; attachment yes; extension yes | yes |
| rev-public, tok-public, rev-pubtri | 7,612 | 404 / 404 / 403 / 404 | yes: trend 5,798 vs 5,802 open; readiness 7,588 vs 7,592; tree root `findings` 1 vs 4, `beneath` 5,770 vs 5,774; **but X1, X2, X3** |
| rev-report, rev-approver | product "not declared" | nothing | a signed-in person with an empty world |
| rev-other | product "not declared" | nothing on sonic, **but X1** | — |
| tok-admin-narrow | product "not declared" | nothing | admin dropped on narrowing; reaches nothing at all |
| key | 401 everywhere except own receipts | — | other product "not declared" (ACC-53 holds) |
| anon | 401 everywhere | — | as designed |

Writes by `rev-pubtri` against the undisclosed flaw (decision, comment,
approval, send-back, reword, withdraw, claim approval, extension, extension
approval, resolve, fix-targets, attachment): all refused as "not there".
Self-approval by `rev-private` on decision 61, claim 3 and extension 1: 409
with a clear sentence each. Token minting through a token: 403. A public
triager cannot record an undisclosed flaw (404) and can record a disclosed
one (201, `SONIC-2026-0005`). Attachment fetch: 403 for both an existing
private token and a nonexistent one; no signed URL outlives a revocation
(the token is authorized on every fetch, `attach/read.go` 25–88).
Mentionable at `visibility=private` answers "no product is declared" to a
public reader. A token narrowed to a product the owner holds nothing on is
refused at minting. Person creation with an unknown role, and any request
without a credential, are refused. **None of this needs re-checking.**

## Appendix B · Reporting inventory, and what the screens drop

| Surface | Endpoint | Screen | Answers |
|---|---|---|---|
| Trend | `GET /v1/trend` (weeks, scope) | Home | new / resolved / open per week, open by severity |
| Release-over-release | `GET /v1/trend/releases` | Home (product with 2+ tags in scope) | open now per tagged release |
| Open per build | `GET /v1/products/{p}/releases` | Compare bars | open + by severity per build |
| Comparison | `GET /v1/products/{p}/comparison` | Compare | fixed / newly present / still present, with closure reason and `arrived_from` |
| Release notes | `GET …/comparison/notes` (`text/markdown`) | Compare "Write release notes" | the comparison as markdown |
| Readiness | `GET …/variants/{v}/readiness` | Home | branch now vs last tag cut from it |
| Running out | `GET /v1/running-out` | Home Overdue tile, Findings | one row per issue at a component with `due`, `days_left`, `assigned_to` |
| Remediation metrics | `GET /v1/remediation` (days, scope) | Reports "Keeping pace" | fixed, opened, time_to_fix by severity, aging buckets |
| Repeat deferrals | `GET /v1/deferrals/repeated` | Reports | places deferred ≥2 times, times, total_days, standing |
| The record | `GET /v1/audit` (product, outcome, state, from, to, ≤500) | Audit (printable) | every judgment with proposer, approvers, withdrawn approvals, `two_people`, reasoning, justification, mitigation, `fixed_version` |
| Decisions | `GET /v1/decisions`, `/{id}`, `/revisions`, `/approvals`, `/comments` | Decision | one judgment in full |
| Who holds what | `GET /v1/assignments`, `/v1/people/{id}/assignments` | Assignments | per-person count and overdue count |
| Embargoes | `GET /v1/disclosing` | none | undisclosed flaws and their dates |
| Advisory | `GET …/issues/{v}/advisory` | none | CSAF 2.0 for a recorded flaw only (REM-23) |
| Receipts | `GET …/variants/{v}/scans` | Inventories | per upload: hash, size, components placed; `measured_against` for the latest run only |
| Fix targets | `GET …/fix-targets` | Finding | per finding: the six states per build |

Everything is computed on request (RPT-13) and narrowed by the subject.
Questions the store could answer with no new storage and no endpoint asks:
SLA compliance rate, time-to-decide, approval latency, per-person
throughput, point-in-time ("as of") state of a build, a per-build
remediation plan. The `finding` row carries `opened_at`, `closed_at`,
`closed_because`, `due_at` (kept on close), `assigned_at`, `assigned_to`;
`vulnerability` carries `first_seen_at`; `decision` carries `proposed_at`,
`proposed_by`, `ended_at`, `state`; `approval` carries `approved_by`,
`approved_at`, `withdrawn_at`.

## Appendix C · Capability classes against comparable tools

Compared with OWASP Dependency-Track (DT), DefectDojo (DD), GitLab/GitHub
vulnerability management, Snyk/Trivy/Grype dashboards, GUAC, VulnerableCode,
CSAF tooling (Secvisogram, csaf-poc), and the frameworks a PSIRT is measured
against (FIRST PSIRT Services Framework, ISO/IEC 29147 and 30111, CISA VEX
and SSVC, NIST SSDF, EU CRA). "Missed?" is for a few-person team handling
thousands of findings.

| Capability | Others | OpenPSIRT | Missed? | Smallest useful version |
|---|---|---|---|---|
| SBOM ingest (CycloneDX) | DT, GUAC, GitLab | built | — | — |
| SPDX ingest | DT, GUAC | absent, claimed by ING-01 | low now; rises when a supplier hands over SPDX | X9 |
| VEX from the build | DT | built (OpenVEX, three statuses) | — | — |
| VEX from upstream vendors and distros | DT per project, CSAF aggregators, GUAC | absent | **yes** | F2 |
| Producer vulnerability report | everyone accepts scanner output | absent (ING-28 says it exists) | medium: a customer with a mandated scanner cannot feed it in | read `vulnerabilities[]` into the finding model tagged with provenance (ING-29 already specifies the tag) |
| Static analysis / fuzzing (SARIF) | DD (150+ parsers), GitLab, GitHub | absent, already recorded (SCP-11, ING-24..27, MDL-14) | medium: a fuzzer finding is a recorded flaw typed by hand | a SARIF adapter into the recorded-flaw path, identity from rule + location |
| Third-party report intake | FIRST, ISO 29147 require a receipt channel; DD has none | out of scope (§8) | the inside half is missed | F3's reporter block; no public form |
| CVE request / alias later | GHSA, Secvisogram | out of scope (CNA); attaching a CVE later is absent | **yes**: every disclosed flaw eventually has a CVE | F3 |
| CVSS, EPSS, KEV | DT, GitLab, Snyk, Trivy | built (from Grype, `grype.go` 119–125, 216–217; packed by RNK-06) | — | — |
| Own assessment vs published | DD severity override, DT analysis | built and stronger (TRI-40/41/42) | — | — |
| SSVC | CISA, DT via policy | absent | medium | F5 |
| Reachability | Snyk, GUAC | absent; REJ-06 rejects the analysers that would feed it | low for distro packages, medium for Go | automation table |
| Severity floor | GitLab policies | built (TRI-43) | — | — |
| Policy engine / auto-dismissal | DT policy engine, GitLab policies, Trivy Rego, Snyk ignore | absent by design (REJ-10, REJ-14, TRI-07) | yes, in a bounded form | rules that prefill (automation table) |
| Suggested reasoning from prior decisions | DD dedup, DT analysis reuse | partial: lapsed reasoning offered back; TRI-47 at the same component | medium | automation table |
| Dedup across products | DD | built in the useful sense (UIX-53); cross-product deliberately not (MDL-05) | — | — |
| Tags / labels | DT, DD, GitLab | absent | medium | F5 |
| Saved filters | DD, GitLab | absent; §7 names it | yes | W3 |
| Standing subtree ownership | DT owners, DD leads | absent, already recorded (§7) | yes at scale | saved filter first |
| Tracker hand-off | DT, DD bidirectional, GitLab | absent, already recorded (REM-11/12) | **yes** | F1 |
| Webhooks | DT, DD, GitLab | absent | yes | F1 |
| Chat adapters | DT | absent, already recorded (NTF-01) | yes | F1 |
| Fix PRs / auto-bump | Snyk, Dependabot | out of scope | no | — |
| CI gate | DT policy → CI, Trivy, GitLab widgets | absent, structurally (a key reads only its own receipts) | medium | F4 |
| CSV / JSON export | DT, DD, GitLab | absent | **yes** | R1 |
| VEX/VDR export per build | DT, GitLab | absent, already recorded and deferred | yes for a vendor | R5 |
| CSAF advisory | Secvisogram, csaf-poc | partial: generated, no CVE, no acknowledgments, no revision tracking, never published | medium | F3 |
| Portfolio dashboard | DT, DD | built (home) | — | — |
| MTTR / SLA metrics | DD | partial: 30-day fixed/opened/aging; no compliance rate | medium | R6, R7 |
| Audit trail export | DD, GitLab audit events | partial: judgments only, no admin events | yes | R1, R3 |
| SBOM download for a build | DT | absent for retained tags | medium | F5 |
| Embargo with end, extension control, approaching list | 29147, FIRST | built and above the norm (ACC-46..49) | — | — |
| Reporter / acknowledgment / coordination timeline | 29147 §6, CRA Art. 14 | partial: recorded-at, embargo, decisions, close stored; not assembled; received/acknowledged absent | yes | F3 |
| Advisory revision tracking | CSAF `tracking` | partial | medium once anything is published | F3 |
| Multi-scanner | DT | Grype only, behind a one-implementation interface (`scanner.go:17`) | low; a Trivy implementation would be the first test of ING-20's comparability rule | later |
| Multi-tenancy | DT, DD | rejected (REJ-09) | no | P4 |
| Air-gapped scanner DB | DT mirror | claimed (ING-22), undocumented | medium for appliance vendors | F5 |
| License / component policy | DT | out of scope | no | — |

## Already recorded, and the review's view

| Item | Where | View |
|---|---|---|
| Standing ownership of a subtree; saved filter first | §7, ACC-54 | Agree; blocked in practice by X5, then W3 |
| What the reporting role is for | §7 | P3 above |
| Notifications not reconciled after revocation | working notes | Clear on the ACC-43 trigger, keyed on what the notification concerns; not re-tested |
| Multi-tenancy rejected | REJ-09 | Agree; P4 covers most of the demand |
| No hiding by anything other than a decision; no per-container bulk | REJ-10, REJ-14 | Agree; every automation above prefills and never decides |
| Abandonment as a triage signal rejected | REJ-13 | Agree; its measurement is what makes F2 the right lever |
| Third-party intake out of scope | §8 | Agree on the form; disagree on recording the reporter (F3) |
| VEX profile and advisory adapters deferred | REM-17, REM-22 | R5 argues the per-build VEX is worth more than the adapters |
| Tracker hand-off; Slack and Teams | REM-11, NTF-01 | F1 |
| Enumeration oracle on comments (2026-09-03) | working notes | Fixed there; X2 and X3 are the same shape on other routes, which argues for the route-walking test rather than another point fix |
| CycloneDX 1.7 fixture | DESIGN-ingest | Still open |

## Residue in the demo

The permissions probe left people `rev-public`, `rev-pubtri`,
`rev-privread`, `rev-private`, `rev-private2`, `rev-report`, `rev-approver`,
`rev-assign`, `rev-other`, `rev-admin2`, `rev-made-by-token` (admin);
pipeline keys `rev-pipeline`, `rev-key-by-token`; four personal tokens;
recorded flaws `SONIC-2026-0003`, `-0004` (undisclosed) and `-0005`
(disclosed); decision 61 (approved), comments 1–2, attachment `2799b011…`,
extension 1 (approved), assessments 1–3 (proposed), assignments to
`rev-public` and `rev-privread`. The workflow review left claims 1–4 and a
fix target on `CVE-2026-18798` made by ana and ben. `make demo-reset`
removes all of it.
