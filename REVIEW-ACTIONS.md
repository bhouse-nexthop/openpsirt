# Review actions

An action list from the functionality and usability review of 2026-09-05,
made against `6a2cb8d` by reading the code and driving the demo rather than
the documents alone. **This document is temporary**, in the same sense as
`IMPLEMENTATION.md`: it is worked through and then deleted, and nothing may
reference it. Anything durable that comes out of an item goes to
`DECISIONS.md` or the `DESIGN-*.md` for its area as the item lands.

The full report, with the evidence, the requests and the screenshots behind
every item, is the artifact "OpenPSIRT Deep Dive":
https://claude.ai/code/artifact/380a4893-9941-4c57-be59-3133a072e76e

Identifiers (X, P, W, V, R, F) match the report so an item can be followed
back. Sizes: S is days, M a week or two, L more. Tick an item when it lands.

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

## Fix now

- [ ] **X1 · Assessments leak undisclosed recorded flaws.** Recording, agreeing,
  withdrawing and listing an assessment require that the subject may read a
  finding of that issue at its visibility, in some product; narrow the list
  the way the counts beside it already are. TRI-40 and ACC-62 assume an issue
  is public knowledge, which is false for identifiers this deployment mints
  (MDL-24). Regression: a public triager rates a private recorded flaw and is
  told no such issue is known; a public reader lists assessments and the row
  is absent. **S**
- [ ] **X2 · Hidden and absent answer differently.** One helper that resolves an
  issue *as this subject sees it*, used by every finding-shaped route, so an
  invisible issue answers exactly as an unknown one: finding detail, place
  decision, reach, disclosure, decide, fix targets (404 rather than an empty
  200), assignment (404 rather than a 204 that wrote nothing). Add a random
  suffix to minted identifiers. **S–M**
- [ ] **X3 · A person's assignments endpoint is a staff directory.** An unknown
  identity answers as a known one with nothing visible. Add a test that walks
  every route with an `{identity}` parameter and pins the invariant; this is
  the third instance of the shape ACC-56 closed. **S**
- [ ] **X5 · Triagers cannot assign or take work.** Both pickers read the
  administrator-only people list. Read the product's mentionable list instead,
  and add "Take this" on the finding, the row and the Unassigned batch bar. **S**
- [ ] **X4 · Searching a CVE returns nothing.** Match any identifier or alias,
  enable the box at every scope, and land on a cross-product answer (see V9). **M**
- [ ] **X6 · The comparison's Fixed column floats over Introduced.** The column
  class `col fixed` collides with Tailwind's `.fixed`. Rename it, and add a
  lint that rejects component class names that are Tailwind utilities. **S**
- [ ] **X7 · Newly-critical on a shipped release alerts nobody.** NTF-11 is
  decided and `DESIGN-notifications.md` describes it as behavior; no
  notification kind exists. A condition-based alert in the NTF-09 shape,
  raised when a run opens a critical or exploited finding on a tag stream,
  cleared when it closes or is decided. Fix the design document in the same
  change. **S**
- [ ] **X8 · Administrator is silently every role.** `Holds` returns true for
  admin; the demo's admin approved a decision and an extension with no product
  role, and an admin's personal token minted an admin person and a pipeline
  key. Record the choice either way; recommended: admin administers people,
  roles, credentials, settings and the catalog, and reads or triages only
  where granted, and administration requires a session. **S–M**
- [ ] **V3 · The decision form opens with a dismissal ready to submit.** Stop
  defaulting to not-applicable with a justification pre-filled; no outcome
  until one is chosen, or Affected. **S**
- [ ] **X9 · Documents describe things that do not run.** ING-01 says SPDX is
  accepted; it is refused. ING-28's producer vulnerability report is not read.
  `DESIGN-reporting.md` says RPT-03 is both not built and built, and its
  Satisfies line omits RPT-03 and REM-04 while the body claims them. **S**
- [ ] **X10 · Small defects.** `make demo-status` curls the per-build findings
  path that moved. Approving an already-approved extension is a 500, not a
  409. The queue card counts a claim's own build among "other builds". Triage
  mode's bar says "1–4" for five outcomes. A deferral's success banner calls
  it a dismissal. Home folds negligible and unknown into "low". Kernel rows at
  build scope say nothing pulls them in while carrying 45 places. Readiness
  needs a tag's parent, the demo's tags have none, and it cannot be set later.
  Clean builds vanish from the releases list and chart. The CSAF advisory is a
  409 on the demo until the publisher is set. Mentioning a non-reader is
  accepted server-side. Capability-only grants produce a person who sees
  nothing, with no warning. The carry-from picker sits on Inventories rather
  than where a line is created. **S**

## The ten that change what the tool is

- [ ] **V1 · Disclosure in the interface.** A Disclosure card in the finding
  header (state, embargo ends, extend, advisory), a chip on the row, a
  Disclosing screen off the rail with extension approval on it, and the
  screen for which builds a recorded flaw affects. **L**
- [ ] **W1 · Fix bundles.** A By-fix view beside By issue and By component: one
  row per (component, version, fixed-in) with the issues it closes, the worst
  severity, the exploited count and one control for Affected plus a fix
  target. Also the release coordinator's list (R9). **M**
- [ ] **R1 · Export, and a paged Audit screen.** CSV and JSON on findings,
  decisions, audit, comparison and running-out, streamed through the same
  visibility rules with the floor stated (RPT-14, ACC-07). **S–M**
- [ ] **F1 · One signed outbound webhook.** Per notification kind and per
  operational condition, carrying the NTF-15-safe body; with a stored ticket
  URL on a claim or fix target as the one-way hand-off (REM-11). Covers Slack,
  Teams, Jira-by-automation and paging without an adapter each (NTF-01). **M**
- [ ] **F2 · Supplier and distribution VEX as statements.** An administrator
  uploads a VEX or CSAF-VEX document against a product; matching findings show
  the supplier's statement, filter on it, and offer a prefilled claim. Never
  applied silently (REJ-10, REL-03). 1,113 of 1,125 no-fix findings on the
  demo are distro packages. **M**
- [ ] **V2 · Deadline, owner and age on rows and headers.** Due, days left,
  assigned-to and opened-at on the row and evidence bodies; a Due column; owner
  and age chips in the finding header; a "due in 7 days" tile on Home, which
  today shows only what is already overdue. **M**
- [ ] **P1, P3 · Readers see the record; reporting becomes statistics without
  rows.** Decisions, revisions, approvals and comments readable at the
  finding's visibility; acting stays at triage or approver; the queue stays
  triager-only. Then `reporting` grants figures that name no issue over
  products the holder may not read, with undisclosed work handled by NTF-18's
  numbers-not-names rule or a public/private split. Closes the §7 question. **M**
- [ ] **R2, R6 · A disposition register per build, with as-of; an SLA
  compliance rate.** One row per issue × component in a named build with
  state, outcome, justification, proposer, approvers, dates, deadline and
  whether it was met; an `as_of` parameter on it and on the audit list. The
  rate REM-04 promises, split deferred-by-decision from plainly late
  (REM-15). No new storage. **M**
- [ ] **R8, R9 · Release notes and a Release screen.** Heading follows content;
  still-present is opt-in for the customer form; a lead line with both builds,
  the date and the scanner version; from and to versions on fixed entries;
  state, outcome and justification on still-present entries; a count of
  omitted undisclosed entries. A per-build plan inverting fix targets; an
  optional released-on date on a tag; a tag's parent settable afterwards; a
  publisher in the demo seed. **M**
- [ ] **W2 · Group the same issue across sibling components.** Rows grouped by
  issue where components share an upstream or source name, with one form; one
  decision is still written per component and place (REL-02). **M**

## Then

- [ ] **W3 · Sort, filters, page size, selection, saved filters.** Sort; EPSS
  range, age and opened-since, deadline and overdue, fix-state declined,
  differs-between-variants, CWE, named assignee, sent-back-to-me; page size;
  multi-select on the list; queue filters by product, kind, proposer, age and
  severity; personal saved filters with a link (UIX-10; the half of subtree
  ownership §7 says to try first). **M**
- [ ] **W4 · Size the bulk claim before submit.** Compute places × issues as the
  selection changes and say how many may be picked; select the first N shown;
  a second page into the same claim; exclude exploited and critical before
  claiming; Deferred and Already fixed as outcomes. **S/M**
- [ ] **W5, W6 · Fewer steps, and the proposer told.** One-step review when
  nothing is offered; last-used outcome and justification as the default;
  triage mode remembered per person; Ctrl+Enter advertised. Notify on
  approval, undo and lapse; a "Back to you" panel on home and a tab on the
  queue; staleness reminders as settings something reads. **S**
- [ ] **F3 · A recorded flaw gets its CVE, its reporter and its dates.** Aliases
  on an issue; a reporter block (name, contact, received, acknowledged,
  credit); a per-issue timeline from the rows that exist plus those two dates;
  generated-at, by and a hash per advisory so a later one carries a revision
  history. Completes the CSAF document and the ISO 29147 / CRA timeline. **S each**
- [ ] **R3 · Administrative audit trail.** An event table (actor, action,
  before, after, at) written by the settings, role, end-of-life, floor, key
  and token handlers, readable from Audit; comment history kept like
  reasoning revisions. **M**
- [ ] **R7 · Manager figures.** Breakdown by product and branch; exploited-open
  and undecided-critical tiles; time-to-decide and approval latency;
  throughput per person; aging by severity and state; deferred separated from
  late; opened-after, closed-after and proposed-after filters; windows past 90
  days. **S each**
- [ ] **R5 · VEX per build from standing decisions.** CycloneDX VEX or OpenVEX
  per (product, stream, variant) from approved not-applicable and
  already-fixed claims, public-only by default. Already recorded and deferred;
  worth more to auditors and customers than the advisory adapters. **M**
- [ ] **R4 · Provenance per run, and the Audit screen's dropped fields.**
  Scanner and database version per receipt item and on the finding; Audit
  shows `fixed_version`, offers already-fixed, links rows, and filters on
  two-people, proposer, approver, issue and component. **S**
- [ ] **V6 · Screens the API already serves.** A product overview page; a scan
  run detail from an inventory row; a per-person page; personal tokens, the
  digest and the version in the You menu; batch undo where the queue promises
  it. **M**
- [ ] **W7, V4, V5, V7 · The finding page and the list for people not in triage
  mode.** Decision beside the description in a sticky column, path collapsed,
  next and previous, labels with a meaning per justification, one form for
  Affected plus fix target. Units on every count. A summary line per row and
  a compact density forced in triage mode. Decision page parity with the
  standing card, with the approver's action on it. **M**
- [ ] **V8 · Smaller hierarchy items.** Queue card says why a claim might be
  wrong; by-component ranks by worst severity; one word for no severity; the
  path lists distinct consumers; tree node has a severity strip; report
  figures link to the list they count; per-release chart legend and totals;
  settings grouped and sizes formatted; one date format; trend copy suppressed
  on a young deployment; record-a-flaw placeholder and a link to what was
  recorded. **S each**
- [ ] **W8 · Decide with reach in one request.** One transaction (DAT-30) and
  one answer instead of a POST per build. **M**
- [ ] **F4 · Run deltas on the pipeline receipt.** Opened and closed counts by
  severity and exploited, about the key's own upload (ACC-50), so a pipeline
  can gate on it. **S**
- [ ] **F5 · Retained SBOM download, labels, air-gap documentation.** An
  operation returning a retained tag's document (ING-07); free-text labels on
  a claim or issue; the offline scanner database path ING-22 asserts. **S**
- [ ] **Automation as evidence, never as a decision.** Suggested outcome from
  history across products (reasoning travels, conclusions do not, REL-08);
  SSVC words on the RNK-06 bands; rule-prefilled claims that a person still
  proposes; kernel config as a build artifact that narrows a Together claim
  (TRI-34); Go symbol reachability for the Go product. **S → L**
- [ ] **P2 · Refuse assigning work to somebody who cannot read it.** **S**
- [ ] **P4 · Per-case need-to-know on undisclosed findings.** An allowlist keyed
  on (product, vulnerability) intersected with the private-read predicate;
  covers most of what multi-tenancy (REJ-09) is asked for. **L**

## Already recorded, and the review's view

| Item | Where | View |
|---|---|---|
| Standing ownership of a subtree; saved filter first | §7, ACC-54 | Agree; blocked in practice by X5, then W3 |
| What the reporting role is for | §7 | P3 above |
| Notifications not reconciled after revocation | working notes | Clear on the ACC-43 trigger, keyed on what the notification concerns |
| Multi-tenancy rejected | REJ-09 | Agree; P4 covers most of the demand |
| No hiding by anything other than a decision; no per-container bulk | REJ-10, REJ-14 | Agree; every automation above prefills and never decides |
| Abandonment as a triage signal rejected | REJ-13 | Agree; its measurement is what makes F2 the right lever |
| Third-party intake out of scope | §8 | Agree on the form; disagree on recording the reporter (F3) |
| VEX profile and advisory adapters deferred | REM-17, REM-22 | R5 argues the per-build VEX is worth more than the adapters |
| Tracker hand-off; Slack and Teams | REM-11, NTF-01 | F1 |
| CycloneDX 1.7 fixture | DESIGN-ingest | Still open |

## Residue in the demo

The permissions probe left people named `rev-*`, recorded flaws
`SONIC-2026-0003` to `0005`, decision 61, two comments, an attachment,
disclosure extension 1, three assessments, two pipeline keys, four personal
tokens and two assignments; the workflow review left claims 1 to 4 and a fix
target on `CVE-2026-18798` made by ana and ben. `make demo-reset` removes all
of it.
