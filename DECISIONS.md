# openpsirt — Decisions

## Contents

- [1. About this document](#1-about-this-document)
- [2. What we're building](#2-what-were-building)
- [3. Decisions](#3-decisions)
  - [3.1 Scope and delivery](#31-scope-and-delivery-scp)
  - [3.2 API and docs](#32-api-and-docs-api)
  - [3.3 Ingest](#33-ingest-ing)
  - [3.4 What we track](#34-what-we-track-mdl)
  - [3.5 State and history](#35-state-and-history-sta)
  - [3.6 Triage and approval](#36-triage-and-approval-tri)
  - [3.7 Carrying decisions between releases](#37-carrying-decisions-between-releases-rel)
  - [3.8 Remediation](#38-remediation-rem)
  - [3.9 Ranking](#39-ranking-rnk)
  - [3.10 Reporting](#310-reporting-rpt)
  - [3.11 Access and permissions](#311-access-and-permissions-acc)
  - [3.12 Notifications](#312-notifications-ntf)
  - [3.13 Database](#313-database-dat)
  - [3.14 CI gate](#314-ci-gate-cig)
  - [3.15 Interface](#315-interface-uix)
- [4. Still open](#4-still-open)
- [5. What we ingest](#5-what-we-ingest)
- [6. Constraints that shape the design](#6-constraints-that-shape-the-design)
- [7. Choices not yet made](#7-choices-not-yet-made)
- [8. Out of scope](#8-out-of-scope)
- [9. Rejected](#9-rejected)
- [10. Prior art](#10-prior-art)
- [11. How this document changed](#11-how-this-document-changed)

---

## 1. About this document

The running record of what is decided and what is not.

**Decision IDs** are grouped by area, three letters and a number:

| | | | |
|---|---|---|---|
| `SCP` scope and delivery | `API` API and docs | `ING` ingest | `MDL` what we track |
| `STA` state and history | `TRI` triage and approval | `REL` carrying decisions between releases | `REM` remediation |
| `RNK` ranking | `RPT` reporting | `ACC` access and permissions | `NTF` notifications |
| `DAT` database | `CIG` CI gate | `REJ` rejected (Section 9) | |

Conventions:

- IDs are permanent from here on. A superseded decision is marked, never reused.
- Nothing is decided without a reason next to it.
- Sections 5 and 10 are reference material, not decisions.

---

## 2. What we're building

A tool that takes in SBOMs already carrying vulnerability data, tracks what
changes release to release, and lets people triage what it finds.

| | |
|---|---|
| **Input** | SBOMs pushed by build pipelines, roughly nightly |
| **Users** | Staff, signed in with GitHub or Okta |
| **Output** | A triage queue, reports, and a record of what was decided and why |
| **Shipping as** | Open source, installed and run by others |

The hard parts, in order: the dependency graph, tracking change over time,
making triage decisions survive re-scans, and coping with the size range of the
inputs.

---

## 3. Decisions

### 3.1 Scope and delivery — `SCP`

| # | Decision | Why |
|---|---|---|
| SCP-01 | Primary input is machine ingest of SBOMs that already carry vulnerability data | Stated scope |
| SCP-02 | Phase 1 is public findings from scans. Phase 2 is private findings, manual entry, GitHub integration | Private work can be done in GitHub today. The visibility field and role split still ship in Phase 1 |
| SCP-03 | Open source, installed and run by others | Decides packaging, config, secrets, and makes maintenance work a feature rather than a runbook |
| SCP-04 | Apache 2.0 | The patent grant is what vendor legal teams look for. Matches the surrounding SBOM tooling |
| SCP-06 | **Shipped dependencies must be permissively licensed** — MIT, BSD, Apache 2.0. **MPL is acceptable**; its obligations attach to changes in its own files, which we would never make | Keeps the product freely usable by anyone who installs it. MPL matters practically: the only serious MySQL and MariaDB driver in Go is MPL-2.0, so excluding it would undo the four-engine decision |
| SCP-07 | **Build tooling is unrestricted.** `golangci-lint` is GPL-3.0 and that is fine — running a tool over our code affects its licence no more than compiling with GCC does | Without this distinction the licence check in CI either fails on day one or acquires an exception nobody can explain later |
| SCP-11 | **Ingesting static analysis and fuzzing findings is intended scope, not yet built.** They would be scoped to product, release and variant like anything else, but carry no SBOM relationship | Raised as a future need close enough to matter now. What it changes today is the shape of the finding model (MDL-14), nothing more |
| SCP-08 | **We publish a CycloneDX SBOM of ourselves**, generated at build time and attached to releases | A tool that consumes SBOMs and ships without one is hard to defend. CycloneDX because it is the format this project treats as authoritative on the way in (ING-01) |
| SCP-09 | The SBOM is generated from the **built binary's module graph**, not from source | It describes what actually ships rather than what the repository happens to contain |
| SCP-10 | Licence data in our own SBOM is **not** relied on for compliance | The generator does not populate it reliably. Licence compliance is gated separately and properly by the allowlist check (SCP-06) |
| SCP-05 | One variant is the normal case, and the interface hides the dimension when there is only one | Multi-variant products are the interesting case, not the common one |

### 3.2 API and docs — `API`

| # | Decision | Why |
|---|---|---|
| API-01 | Go | Chosen up front |
| API-14 | **`huma` on `chi`** for the HTTP layer | Huma generates the OpenAPI document from the route definitions, so API-04's "generated, never hand-written" comes free rather than needing a separate generator. Also what nearby engineers already use |
| API-15 | **mkdocs-material with `mike`** for the documentation site | `mike` does versioned docs with a `latest` alias natively, which is exactly API-11. Adds Python to CI, which is trivial |
| API-02 | REST and JSON, documented with OpenAPI | Machine clients plus a UI. GraphQL and gRPC add weight a simple tool doesn't need |
| API-03 | The web UI is a client of the same public API. No private endpoints | Keeps the API honest and stops the UI becoming privileged |
| API-04 | The OpenAPI spec is generated in CI, never hand-maintained | Lets us generate the TypeScript client from it |
| API-05 | **The application serves no documentation.** Documentation is published (API-10) | Keeps the application to one job, and leaves it with **no unauthenticated routes at all** — one authorization rule, no exceptions, nothing for a future mistake to hide behind |
| API-10 | **Documentation is built as a release asset and published to GitHub Pages** — the generated API reference alongside the hand-written install, configure and operate guides | Under SCP-03 documentation is part of the product, and it has to be readable by someone evaluating the project before they install anything. Built in CI from the same specification artifact the application serves, so the two cannot disagree |
| API-13 | If the documentation tooling produces a downloadable bundle as a by-product, ship it with the release. **Not a requirement, and nothing is built to achieve it** | Documentation is normally published separately, and the air-gapped argument does not really hold — someone writing an integration is reading a spec wherever they develop, not off the appliance. Worth taking only if it comes free from the tooling choice |
| ~~API-08, API-09, API-12~~ | **Superseded by API-05.** Had the application serving public documentation from its own unauthenticated route | Reversed the same day: documentation is published and shipped instead, which removes the only unauthenticated route rather than having to fence it off |
| API-11 | Published documentation is **versioned, with a "latest" alias** — not a single rolling copy | The failure to design against is someone reading current documentation while running an older release. Versioned sets make "which version is this describing" answerable rather than assumed |
| API-07 | The web UI is built into the same binary and served from the **same origin** as the API | One artifact to install (SCP-03), no cross-origin configuration, and session cookies work without the relaxed settings a separate origin would force. Restores a decision lost when this document was condensed |
| API-06 | The API is the contract. UI upload is a testing convenience on the same endpoint | Stated |

### 3.3 Ingest — `ING`

| # | Decision | Why |
|---|---|---|
| ING-01 | CycloneDX 1.6 is the primary format. SPDX 2.3 accepted but lossy | Both reference producers emit CycloneDX. SPDX 2.x cannot carry vulnerabilities at all |
| ING-02 | We take the already-filtered scanner output. Issues the build suppressed are not received or re-decided | If a carried patch fixed it and the build says so, we don't refute it. No build-side change needed |
| ING-03 | Ingest is asynchronous: accept, queue, parse, report status | A 46 MB file with 56,600 components cannot be parsed inside a request |
| ING-04 | Ingest is generator-agnostic — an adapter per producer, one shared internal model, a test fixture per producer | Two producers today, a dozen expected |
| ING-05 | Never trust identifiers a scan file supplies to be stable between builds or consistent between producers. Identity comes from content | The format doesn't guarantee it. An unstable key would wipe every triage decision nightly |
| ING-06 | Ingest is sized and tested against the largest input, not the average | One pipeline spans roughly a thousandfold in file size |
| ING-07 | **Nightly branch scans are deleted once ingest succeeds. Tagged-release SBOMs are retained** | Two different things: a nightly scan is superseded the next night and there are 10–50 a day; a tag is immutable, rare, and the thing we need to re-scan later (ING-20). Storage at tag volume is trivial. For deleted files we keep a hash, stamp records with the parser version, and re-upload from the build if a re-parse is needed |
| ING-20 | **The deployment re-scans retained release SBOMs with grype on a schedule**, for supported releases only | A tag is built once, so its vulnerability data freezes at build time while new CVEs keep being found against the same component versions. Without this the tool cannot answer what affects a shipped release today — the question most likely to be asked during an incident. Stops at end-of-life (MDL-11), which is what those dates are for |
| ING-21 | **The build's suppressions must be retained alongside the release SBOM and applied when we re-scan** | Otherwise re-scanning undoes ING-02: every vulnerability a carried patch already fixed comes back as a fresh finding, and on shipped releases that is a flood of false criticals — destroying the credibility of the alarm this whole mechanism exists to raise |
| ING-22 | Grype is an **external dependency of the deployment**: the binary plus its own vulnerability database, which it maintains. Air-gapped installs need its offline database import | Correcting an earlier overstatement — we are not building or shipping a vulnerability database. But it is a real packaging consideration against the single-artifact goal |
| ING-23 | We re-scan; we still do not discover. The component list always comes from the build | Narrows what ING-04 means rather than reversing it. We never work out what is in a product — only whether what the build told us has since become vulnerable |
| ING-08 | Sizing target: 10–50 scan files a night across a few product lines, heavily skewed toward SONiC | The basis for storage and hardware sizing. See [Section 5](#5-what-we-ingest) |
| ING-09 | Every artifact is marked customer-facing or internal, defaulting to customer-facing | Feeds ranking. Defaulting to customer-facing is the safe direction |
| ING-14 | **A scan is only accepted if it is newer than the current state for that target**, judged by the creation time the SBOM records. Anything older or equal is rejected with a reason | Uploads do not arrive in the order they were made — retries, slow transfers, queued jobs. Without this, a scan built yesterday can land after today's and silently replace the live picture: findings reappear, closures reopen, and there is no symptom anyone would notice |
| ING-19 | **Uploads are refused when the pending queue is deeper than a configured limit**, telling the caller to retry later. No per-request rate limit | Guards what is actually at risk — other products' scans stuck behind one runaway pipeline — rather than counting requests, which measures nothing useful when scan sizes vary a thousandfold. The common runaway is already cheap: an identical file returns success without work (ING-17), and anything not newer is rejected (ING-14) |
| ING-18 | **A scan that fails partway through parsing changes nothing.** All or nothing, with the failure and its reason reported back | Partial data is indistinguishable from a product that shrank: 40,000 components read out of 56,600 looks exactly like 16,600 being removed. Findings close, reports say the product got smaller, and nothing indicates a parse failure. Ingest needs a staging step or one large transaction to achieve this, which is the price |
| ING-16 | **A scan whose creation time is in the future is rejected**, allowing a few minutes for clock skew | Worse than a stray bad value: once current state holds a date years ahead, nothing legitimate can ever be newer, so every real scan after it is rejected. One bad clock would wedge that target permanently |
| ING-17 | **An identical file uploaded again reports success without reprocessing**, recognised by the hash we already keep | The ordinary case is a retry after a timeout that actually succeeded. Rejecting it turns a landed scan into a red build, and the usual response is retry logic that swallows errors — which then hides real failures too. A *different* file that is not newer is still rejected as out of order |
| ING-15 | A badly-set clock on a build machine will reject its own scans | Accepted cost of trusting the producer's timestamp. It fails loudly at the pipeline rather than corrupting data quietly, which is the right way round |
| ING-11 | **A scan naming an undeclared release or variant is rejected.** Releases and variants are declared before CI can push to them | A typo in a pipeline would otherwise create a stream that looks entirely genuine — its own findings, counts and reports — while the real release appears to have stopped being scanned. Two wrong answers from one keystroke, and neither is visible. Declaration makes that impossible |
| ING-12 | Declaration is available through the API as well as the interface, and the rejection names exactly what is missing | The step has to be scriptable into whatever cuts a branch, or it becomes the thing everyone forgets. A generic failure would leave whoever sees the red build guessing |
| ING-13 | Declaring a release is where its decision inheritance is chosen (REL-08) | Turns the step from pure overhead into the natural moment to pick what the new line starts from |
| ING-10 | Two small daily feeds are pulled: known-exploited vulnerabilities, and exploitation-likelihood scores. Both need an offline import path | Needed for ranking, not for finding issues |

### 3.4 What we track — `MDL`

| # | Decision | Why |
|---|---|---|
| MDL-01 | The tracked unit is (product, stream or tag, variant). Variants belong to a stream, not to the product | A variant added in a later release shouldn't retroactively exist in earlier ones. Releases and variants are **declared before use** (ING-11), not discovered from ingest — an earlier draft said the opposite and was corrected |
| MDL-11 | **A release carries an end-of-life date**, settable at the product level too. Not a flag — a date | A date answers "what goes out of support next quarter", which is a real planning question, and it takes effect on its own rather than waiting for someone to remember. Matches how lifecycle policies are actually published |
| MDL-12 | Past its end-of-life date, a release keeps all its findings and history. **Nothing is deleted or hidden** | You still need to answer what was in a shipped release years later. End-of-life changes what is expected of us, not what is true |
| MDL-13 | End-of-life is reversible | Extended support happens. Rare, but the alternative is recreating a release to undo a date |
| MDL-14 | **A finding has a kind.** The kind supplies identity and expiry; everything above that is shared | Vulnerabilities from SBOMs are not the only findings worth triaging — static analysis and fuzzing produce findings that need the same queue, the same outcomes and the same reporting, but have no dependency path at all. Recorded now because a schema that hard-codes SBOM-shaped identity into the finding cannot take a second kind without a rewrite. Same class of constraint as per-product roles and the visibility column |
| MDL-15 | **Per kind:** how a finding is identified across runs, and what lapses a decision about it. **Shared across kinds:** product, release and variant scoping, the four outcomes, the review queue and approvals, assignment, remediation targets, computed resolution, visibility, reporting, notifications and roles | Names the seam. The shared half is most of the system, which is why one tool covering both is worth doing at all |
| MDL-16 | Kinds without a dependency path do not get the tree views | The "why is this here" navigation answers a question that only exists for a component pulled in by something else |
| MDL-02 | A variant is any parallel build of the same source differing only in target — chip variant, **operating system**, CPU architecture, platform | Broader than the chip-variant example suggests |
| MDL-03 | The dependency graph is stored as nodes and edges, never flattened | The same package appears at many versions and many places. A flat list can't answer "why is this here" |
| MDL-04 | Upstream provenance is kept and shown | A shipped `frr_10.5.4-sonic-0` matches CVEs through its upstream identity. Drop it and findings become unexplainable |
| MDL-05 | A finding is a component **at a specific place** in a specific build. Twelve places means twelve findings | Different consumers use different features of the same component, so some are affected and some aren't |
| MDL-06 | A place is identified by the hashed chain of component **names** down to it, excluding the top level. No versions anywhere in the key | Versions in the key would lapse every decision nightly, since the top-level version changes every build |
| MDL-07 | The top-level component is excluded from identity **and** from expiry | Its version changes every build, and its name differs per variant — including it would break cross-variant grouping entirely |
| MDL-08 | Identity is structural. Expiry is version-based. Neither borrows from the other | Overlapping them is how an unrelated top-level bump invalidates a leaf decision |
| MDL-09 | Capture CWE classification where the data carries it | Cheap, and it groups findings by weakness type rather than only by package |
| MDL-10 | Known limitation: the same version built with different feature flags can use its dependencies differently, and nothing in the scan data reveals it | Documented rather than solved. Better named now than found in an audit |

### 3.5 State and history — `STA`

| # | Decision | Why |
|---|---|---|
| STA-01 | Change over time is a primary query, not an audit log | Stated: track how it changes for each release |
| STA-02 | Triage happens against **current state** — the most recent build — not against individual builds | There is one live picture per stream or tag |
| STA-03 | Findings open and close themselves as state changes, and closure records why | Without it the tool fills with dead findings nobody closes. **Closure reason is determined from the SBOM, not guessed — see STA-04** |
| STA-04 | A finding auto-closes only when the SBOM explains why. **Four explanations**: component absent → removed; upstream version changed → fixed by upgrading; downstream revision changed → a patch landed; **the pedigree records a patch that resolves this vulnerability → mitigated by a carried patch, with no version change at all** | The fourth is the important one and was missing from an earlier draft. Because we receive the already-filtered scanner output (ING-02), a vulnerability a carried patch fixes simply never arrives — from our side it vanishes with nothing in the version string to explain it. The explanation lives in the pedigree's patch records naming what each patch resolves. Note this differs from the expiry rule, which ignores patch revisions as too fine-grained: for *explaining a disappearance* the patch data is exactly the signal |
| STA-13 | Because the explanations above cover normal operation, the unexplained bucket should be **empty in a healthy system** | A component unchanged in every respect, with no patch claiming to resolve the issue, and its vulnerability simply gone — there is no innocent reading of that |
| STA-16 | **SONiC rarely changes package versions**, so the pedigree patch record is the primary explanation path for our largest producer, not a fallback | Explanations keyed on a changing version string will seldom fire there. If pedigree patch data is thin, nearly every SONiC fix lands in the unexplained bucket and the red flags become noise nobody reads |
| STA-15 | If a producer does not emit pedigree patch data, its patch-mitigated disappearances land in the unexplained bucket and are flagged | Fails safe rather than silent. The cost is noise from a producer that omits the data, which is the correct pressure — the alternative is silently accepting closures we cannot account for |
| STA-05 | **An unexplained disappearance is always flagged. No threshold, never suppressed, no matter how many occur** | There is no volume at which "we cannot account for this" stops mattering. Suppressing them in bulk would hide exactly the case worth seeing |
| STA-14 | Several unexplained disappearances in one scan **additionally** raise a scan-level alert | A convenience, not a gate: the individual flags are already raised either way. It just says the likely fault is one broken scan rather than many independent anomalies, so nobody chases each one separately |
| STA-06 | Three classes of table: current state (never purged), triage history (never purged), change events (partitioned and purged) | A finding open for years lives in current state, so dropping old partitions can't remove it |
| STA-07 | Current-state rows carry their own summary fields — first seen, opened at, last changed | Otherwise purging events silently breaks a finding that has been open for years |
| STA-08 | The stored format is built so an unchanged build writes nothing | Normalize before comparing, use short numeric keys, write only on real change |
| STA-09 | All triage history is append-only. Nothing is overwritten | Also serves as the audit record — one mechanism, not two |
| STA-10 | Observed state (from scans) and remediation state (declared by people) are separate fields, never merged | Marking something fixed doesn't make it absent from the next scan, and vice versa |

### 3.6 Triage and approval — `TRI`

| # | Decision | Why |
|---|---|---|
| TRI-01 | Dismissing something as not applicable needs an explanation and goes to a review queue for approval | Stated |
| TRI-02 | Four outcomes, not two: **affected** (goes to remediation), **not applicable** (doesn't affect us), **deferred** (affects us, lower priority until a date), **won't fix** (affects us, permanent) | Our vocabulary had nowhere to put "yes, but not now" — the most common real triage answer. Reporting under RPT-01 needs to tell "doesn't apply" from "not now" from "never" |
| TRI-03 | **Not applicable expires when the code changes** (TRI-10). **Deferred expires on a date** and returns to the queue flagged for re-evaluation. Won't fix never expires; reopening is manual | Different claims, so different mechanisms. A version bump doesn't change a judgement about priority, and a calendar doesn't change whether code is reachable. Neither is a rival to the other |
| TRI-04 | A deferred item exports as **affected**. Never as not-affected | The deferral is an internal scheduling decision. Publishing it as not-affected would tell the outside world we assessed something as harmless when we had only postponed it |
| TRI-05 | Deferring is gated like any other suppression, **above the threshold in TRI-17** | TRI-16: hiding risk needs approval, and a deferral hides a known-affecting issue from the working queue. Settles that outcomes beyond "not applicable" are gated; TRI-17 then exempts short ones |
| TRI-20 | A re-affirmation returns to **full approval** when the justification category has changed, or when the severity has increased since the approval | Both fire on something having actually changed. A different justification is a different claim nobody has reviewed, and it would otherwise inherit an approval granted for other reasons. A severity increase means the original judgement was made about a smaller problem |
| TRI-21 | A **count** of re-affirmations does not trigger full approval | Considered and left out. It would fire on nothing having changed, which is inconsistent with every other expiry rule here — those all key on an actual change rather than on elapsed time or repetition |
| TRI-17 | **A deferral shorter than a configured threshold needs no approval; at or above it, a second person must approve.** The threshold is set by an admin per deployment, shipping with a 30-day default that is a starting point rather than a fixed rule | A quick "not this sprint" is ordinary triage; "not this year" is a decision worth a second pair of eyes. Splits the two without gating the most routine action in the tool |
| TRI-18 | The threshold applies to **cumulative deferred time on a finding**, not to each deferral in isolation | Otherwise repeatedly deferring for just under the threshold stays unapproved forever. Under this rule four consecutive 29-day deferrals cross a 30-day threshold at the second one, and every one after it needs approval |
| TRI-19 | Repeat and long-running deferrals are reported | Approval alone never catches a chain of them — the fourth approver sees the same shrug as the first. A report showing "deferred four times, 14 months total" is what actually surfaces it |
| TRI-06 | "Not affected" requires a standard VEX justification category plus free text | That vocabulary already encodes this exact reasoning, and makes VEX export nearly free |
| TRI-07 | The proposer and approver must always be different people. No override | Standard control. Consequence: a one-person install can't approve, and must say so plainly |
| TRI-08 | A reviewer may approve any selection in one action | Volume makes it necessary. Control moves from restricting the selection to informing the reviewer |
| TRI-09 | Every row in a bulk list shows the issue and severity, component and version, **the path that pulls it in**, the justification and explanation, the proposer, and the build | The path is the critical one — without it an approver cannot judge the claim at all |
| TRI-10 | An approved dismissal holds until the software changes — the vulnerable component's version, or its direct consumer's | Re-review fires when the reasoning could have stopped being true, not on a clock |
| TRI-22 | **A carried patch does not lapse a decision.** For a producer that changes code by patching rather than bumping versions, expiry is effectively inert and a decision stands until someone revisits it or the finding closes on its own | Accepted deliberately. A patch is our own change to our own build, and a reachability claim usually rests on structure a patch is unlikely to alter. Consequence recorded plainly: for SONiC — our largest producer — decisions made once may never be automatically re-examined however much the code moves |
| TRI-23 | Decision age is shown wherever a decision appears, and old ones are surfaced in the dismissal reports | The compensating control for TRI-22, and it costs nothing. An eight-year-old judgement should look like one rather than reading the same as yesterday's |
| TRI-11 | Only the **upstream** version counts. `10.5.4` → `10.6` lapses a decision; `-sonic-0` → `-sonic-1` does not | Patch revisions are too fine-grained and move constantly |
| TRI-12 | A version change in the middle of a chain needs no rule of its own | It either changes the direct consumer below it, which lapses that decision, or it doesn't |
| TRI-13 | Decisions survive nightly re-scans | Without this the tool is unusable by the second week |
| TRI-14 | When a version change reopens a decision, the same person may re-affirm it with a fresh reason, no second approver | Two people already approved the claim; a bump is a prompt to re-check. Full approval on every bump would produce rubber-stamping |
| TRI-15 | Dismissals can be withdrawn or revoked at any stage, and a bulk approval can be undone as a batch | Append-only, so it reads as dismissed → approved → withdrawn |
| TRI-16 | **Hiding risk needs approval. Re-exposing risk does not** | The queue exists to stop risk being hidden unseen, not to obstruct putting it back on the table |

### 3.7 Carrying decisions between releases — `REL`

| # | Decision | Why |
|---|---|---|
| REL-01 | Findings identical across variants are shown and acted on as **one item**. Only genuine differences are broken out | Variants are mostly the same. One collapsed item plus a short exception list beats a long, mostly-checked list |
| REL-02 | Grouping is presentation only. One grouped action writes N individual records, and the group is derived, never stored | Keeps per-place records intact, and a variant that later diverges falls out of the group by itself |
| REL-03 | Exceptions are grouped by the layer they come from, not listed flat | "These forty are all distro base packages" is one judgement. Forty flat rows is forty rubber stamps |
| REL-04 | Every kind of variant follows the exceptions model | Even across operating systems, a product's application and language dependencies are shared; the distro layer is what differs |
| REL-05 | A decision is keyed on (product, place, component upstream version, consumer upstream version, justification) — **not on the release it was made in** | A decision is a claim about a code combination, not a release. Makes inheritance a lookup instead of a copy — no syncing, no drift |
| REL-06 | A new version of a product picks up matching decisions automatically | Nothing has changed that our own rules say should lapse, so nothing is asked |
| REL-09 | **A decision carries across variants when the dependency chain is identical, and only then.** Any difference anywhere in the chain produces a different key, so no carry happens | The chain identity *is* the test — nothing extra is needed, since a differing chain simply fails to match. The one case this cannot catch is two variants compiling an identical chain with different feature flags, which no SBOM reveals; accepted as the documented limitation in MDL-10 |
| REL-07 | Dismissing something offers matching findings on other branches, releases and tags as **checkboxes, one per match, unchecked** | Not all-or-nothing: a component may be used in a later release and not an earlier one |
| REL-08 | When creating a release or branch, optionally seed from a nominated prior release | Exact matches already apply. Version-moved matches arrive as pending items **carrying the prior reasoning**. Import carries reasoning forward, never conclusions |

### 3.8 Remediation — `REM`

| # | Decision | Why |
|---|---|---|
| REM-01 | Findings can be assigned to a person, and assignment notifies | Stated |
| REM-02 | Fixes are tracked through to completion — target release, state, due date, verification | Makes this the single place to answer what we're shipping fixes for |
| REM-03 | A fix declared for a release whose next scan still shows the issue is flagged as a missed target | The scan is independent evidence against the claim |
| REM-16 | **A release past end-of-life has no remediation targets**, so nothing on it is overdue | Otherwise the overdue figure and the escalation view fill permanently with releases nobody will ever fix, and both stop being read. Also makes REM-13 honest: on an end-of-life release, "no decision recorded" is the correct state rather than an oversight |
| REM-14 | **Due dates come from a policy set by severity. There are no per-item overrides** | An override and a deferral do the same job, and deferral already carries the reason, the approval threshold, the cumulative cap and the reporting that an override would need rebuilt from scratch. Without overrides the overdue figure stays honest: something past target shows as past target rather than having its goalpost quietly moved |
| REM-15 | Deferral is the only sanctioned way to move a date. **The deferral date becomes the effective target**, and deferred items are reported separately from plainly overdue ones | Settles how deferral and the SLA clock interact. Keeps a conscious, approved decision from being counted as a failure, while stopping deferral from being a way to make overdue work disappear from the numbers |
| REM-04 | Remediation targets by severity, with time remaining or overdue shown per item, an SLA compliance rate, and a dedicated escalation view for items long past target | Targets come from the policy in REM-14. The escalation view is what makes overdue work visible rather than merely recorded |
| REM-05 | Work-distribution views: a shared queue of unassigned items, a per-person "my items" view sorted by urgency, per-person workload, and visible ownership on in-progress items | We had assignment but no collaboration layer at all. Cheap on top of REM-01, and visible ownership is what stops two people fixing the same thing |
| REM-06 | Show fix status for the same issue across branches, derived **only from scans** — "gone from main, still present in 2.4 and 2.3." No commit or pull-request linkage | We have no commit tracking, so branch-by-branch backport tracking is not available to us. But the useful half needs none of it: a fix landing shows up as the issue disappearing from that stream. Arguably better evidence than a merged pull request, since it reports what shipped rather than what was intended |
| REM-07 | While triaging, a person selects **which streams and variants they are targeting** for the fix. Declared intent, not commits. The candidate list is the set of places the issue currently appears, which we already compute | Closes the loop REM-06 leaves open, with no commit tracking. Intent is declared, scans supply the evidence, and reconciliation (REM-03) then separates **targeted but still present** — a missed target — from **not targeted and still present**, which is open by choice and not a failure. Today those two look identical |
| REM-13 | A branch nobody selected stays open and is **marked as having no decision recorded**, distinct from one being actively fixed | Nobody is made to answer the same question six times, but "open because we chose not to" and "open because nobody thought about it" stop looking identical. Reports can separate them, which is the question that gets asked after something ships with a known problem in it |
| REM-08 | Remediation intent is a **set of targets**, not a single field on a finding | Follows from REM-07 and corrects STA-10's implied shape: a fix spans several tracked units, so remediation state is per target. Observed state stays per tracked unit as before |
| REM-09 | **Resolved is computed, not declared.** An issue is resolved only when every declared target no longer shows it. Progress is reportable at any point — "two of three targets clear, 2.3 outstanding" | Removes the gap between someone marking work done and the work actually being done. Nobody can close an issue while a target they committed to still carries it, and nothing has to be remembered or ticked. Supersedes the human-declared "fixed" state implied by REM-02 |
| REM-10 | The target set belongs to the **assigned issue** — one assignment, one owner, covering every target. Targets clear individually, and the owner's view shows which remain | Keeps ownership singular while the work spans branches. Consequence for the workload views (REM-05): an issue with three targets counts as **one** item of work, not three — otherwise the busiest-looking person is just whoever covers the most streams |
| REM-23 | **Advisory publication and disclosure timelines apply to private findings only** — vulnerabilities in our own product. Known CVEs in shipped third-party components are tracked and fixed, not published about | That is dependency hygiene, and a consumer can already see it from the SBOM. Issuing a vendor advisory for every upstream CVE in a dependency is not what an advisory is for. Recorded as a scope choice **for now**, not a permanent boundary — some vendors do publish on third-party components, and this can be revisited |
| REM-24 | Consequence: **Phase 1 publishes nothing.** Publication is entirely Phase 2, arriving with private findings | Falls out of REM-23, since Phase 1 is public findings only. Worth stating so nobody builds an output path Phase 1 never uses |
| REM-17 | **Advisory publication is a pluggable output with several adapters, not one integration.** Phase 2 | Publishing routes differ completely by product: an open-source project uses GitHub's advisory system, a commercial appliance vendor publishes on its own security page or mails customers. Building for one and bolting on the rest is the wrong order |
| REM-21 | **CSAF is the adapter that matters for commercial products** — a machine-readable advisory document the vendor publishes wherever they like | Nearly free from what we already hold: VEX is a CSAF profile, and dismissal reasoning was aligned to the VEX vocabulary in TRI-06 precisely so this stayed cheap. Also what vendors are increasingly expected to produce |
| REM-22 | **GitHub Security Advisories is one adapter, for products hosted there** — draft privately, request a CVE, publish, and use its temporary private fork for fix work under embargo | Covers advisory publication, coordinated disclosure and the private fix space in one. Does not apply to commercial products, which use their own repositories and their own disclosure route |
| REM-18 | **We own the triage record; the advisory platform owns the published advisory.** No attempt to keep both as the source of truth | The question that decides whether an integration works or rots. Ours is the decision history; theirs is the public artifact and the CVE |
| REM-19 | Publishing **aggregates many findings into one advisory** — a product and a version range, not a path | The one place the design collapses rather than separates. Everything else is deliberately granular, so the aggregation rule needs stating rather than assuming |
| REM-20 | Optional and configured per deployment, like any other hand-off | Not every operator is on GitHub, and none should need an account there to use the tool |
| REM-11 | Hand-off to an external tracker such as Jira **may** be in scope: an optional path chosen by configuration, off by default | Recorded now so the seams exist. Retrofitting them later is the expensive version |
| REM-12 | Hand-off is configured separately for public and private. **Private defaults to no hand-off** | An external tracker's permissions are outside our enforcement and our audit |

### 3.9 Ranking — `RNK`

| # | Decision | Why |
|---|---|---|
| RNK-01 | Ranking by score is an indexed query. Scores are normalized at ingest, never computed on read | Sorting tens of thousands of rows must hit an index |
| RNK-02 | An admin-configurable source preference order decides the ranking score; the first source that rated the issue wins. All scores are kept and shown | Sources disagree, and a source often hasn't rated a new issue yet |
| RNK-03 | Ranking combines four signals: severity, known-exploited status, exploitation likelihood, and where the component ships | Ranking must be explainable per item, or people stop trusting the order |

### 3.10 Reporting — `RPT`

| # | Decision | Why |
|---|---|---|
| RPT-01 | Dismissals are a reportable dataset in their own right | The core of the reporting role: everything we decided not to fix, and why |
| RPT-02 | A scan coverage view: what is being scanned, and when each artifact was last seen. Stale or missing artifacts are flagged | A product silently dropping out of scanning is invisible today, and it is the failure that quietly makes everything else wrong. STA-14 catches a *broken* scan; this catches one that stopped arriving |
| RPT-09 | **The trend axis follows what is being viewed.** A branch plots on calendar time, because it is scanned nightly and has continuous data. Tagged releases plot release over release, because each is one frozen point. Rates always plot on calendar | A branch and a tag are different shapes of thing: one moves daily, one never moves again. Releases months apart make a calendar count read as slow drift rather than the step change it was, while a branch on a release axis has only one point |
| RPT-12 | **A branch's current state is comparable against the last release cut from it** — "8 criticals now, v2.4.1 shipped with 4" | The pre-release readiness question, and the reason a branch trend is worth having at all: is what we are about to ship better or worse than what we last shipped. Answerable from nightly data already collected |
| RPT-10 | The core dashboard plots **new, resolved and open over time**, together | Separately they are three numbers; together they say whether the team is keeping pace. New consistently outrunning resolved means a growing backlog, and that should be visible without anyone doing the arithmetic |
| RPT-11 | Trends break down by severity | An open count that is flat while its critical share rises is getting worse, and a single line would hide it |
| RPT-05 | **A release-to-release comparison**: CVEs fixed, newly present, and still present between any two points — not only adjacent releases | The data is already in the change history, including why each one went (component removed, version upgraded, carried patch). Directly feeds release notes |
| RPT-06 | The comparison is **exportable in a form that can go straight into release notes**, and **defaults to public findings only** | Its destination is a public document. Defaulting to public-only means including a private, embargoed finding is a deliberate act rather than something someone pastes in without noticing |
| RPT-07 | Each fixed entry carries **why** it was fixed, not just that it was | "Fixed by upgrading to 2.4" and "fixed by a carried patch" are different things to a reader, and STA-04 already determines which |
| RPT-08 | This is a report the operator uses, not the tool publishing — REM-24 is unaffected | Worth stating so the two do not read as contradictory. We generate; a person decides what to do with it |
| RPT-04 | Releases past end-of-life are **excluded from scan-coverage alerts** and reported separately from live ones | A dead release not being scanned is expected, not a fault. Without this the coverage view — the thing that catches a product silently dropping out — fills with releases that stopped on purpose |
| RPT-03 | Remediation metrics: fix velocity, mean time to remediate by severity, aging buckets, and trend against the previous 30 days | Nearly free given REM-02 tracks fixes fully. RPT-01 covers what we dismissed; this covers what we fixed |

### 3.11 Access and permissions — `ACC`

| # | Decision | Why |
|---|---|---|
| ACC-01 | Federated sign-in, both GitHub and Okta. One provider interface: an OIDC adapter for Okta, an OAuth2 adapter for GitHub | GitHub is not an OIDC provider — it offers OAuth 2.0 only, with no ID token and no discovery |
| ACC-02 | Role-based access control | Stated |
| ACC-03 | "Public" and "private" mean **disclosed or not**, not who may read. Every request is authenticated | A mistake in the rules exposes data to a colleague, not the internet |
| ACC-04 | Every finding carries a visibility level, enforced in the data-access layer with a required subject context — never per handler | Per-handler checks leak the first time someone adds an endpoint. Unset visibility reads as private |
| ACC-05 | Baseline roles: admin, reporting, approver, public-triage, private-triage, public-read, private-read | Stated as a floor |
| ACC-06 | Reporting and approver are **capabilities**, not visibility grants. What they reach is bounded by the person's read roles | No role doubling, and no handing out private access by granting a capability |
| ACC-07 | Roles are granted per product. Admin stays global | Every permission check takes subject, capability and product. Retrofitting would mean re-auditing every query |
| ACC-08 | A product you hold no role on is invisible — not listed, not counted | Otherwise the product list itself leaks what exists |
| ACC-33 | **A person can mint a personal API token** for scripting. Expiry is mandatory, with a maximum an admin configures | Without it the API is unusable by people: anything the interface does not offer cannot be automated, and the usual result is somebody driving a browser session with a script or reusing a CI key for work it was never scoped for |
| ACC-34 | A personal token is a **live reference to its owner, never a snapshot**. It can be narrowed below what its owner can do but never exceeds it, its reach shrinks the moment their roles shrink, and it dies with their account | Means a role withdrawn by group membership (ACC-22) cuts the token at the same instant, with nothing extra to remember. A snapshot would quietly outlive the access it was granted from |
| ACC-35 | Both the owner and an admin can revoke a personal token, and last-used is visible | Stale tokens are otherwise only discovered when someone leaves and nobody knows what breaks if it is turned off |
| ACC-29 | **Configuration names one or more bootstrap admin identities.** They are granted admin at every startup, not just the first, and can then create users, assign roles, and configure group bindings | Solves the chicken-and-egg: nobody can be granted access until an admin exists. Applying it on every start rather than once makes it the documented **recovery path** too — lose admin, add yourself to the config, restart. For software someone else operates, a way back in matters more than a tidy one-shot bootstrap |
| ACC-43 | When an admin deactivates someone, or their last role is removed, **their assigned findings return to the unassigned queue** and their pending dismissals stay in the review queue | Otherwise the work falls into a blind spot: assigned, so not in the shared queue; assigned to someone gone, so in nobody's own list. Invisible to every view rather than visibly orphaned. A reviewer can still judge a pending dismissal on its merits, and the append-only history keeps who held each item |
| ACC-44 | **We cannot detect that someone has left.** Nothing tells us an identity provider account was disabled — we read membership only at sign-in (ACC-38), and someone who has left never signs in again. Deactivation is an administrator action, not something the tool discovers | Stated plainly because the alternative is assuming a cleanup happens that never does. Their work would otherwise sit assigned indefinitely |
| ACC-45 | Admins are **notified when someone has not signed in for a configured period and still holds assigned work** — "X has not signed in for two weeks and has 6 items assigned" | Targeted at the thing that matters. An idle account holding nothing is harmless; work stuck behind someone who is not here is the problem, and it is the prompt that makes an admin realise they left. Suggestive rather than proof — long leave looks the same — so it asks rather than acts |
| ACC-30 | **Never first-user-wins.** The first person to sign in gains nothing unless they were already named | A well-known way for self-hosted software to be taken over: whoever reaches the URL first is not necessarily the person who installed it |
| ACC-31 | A bootstrap admin is a **pre-authorization, not a bypass** — they still sign in normally through a configured provider | Consistent with ACC-21. Naming someone grants them a role; it does not let them in without authenticating |
| ACC-32 | Bootstrap admins work in **both role-assignment modes** (ACC-25) | In direct mode it is how the first admin appears. In group-bound mode ACC-28 already requires a group mapped to admin, but this stays available as the break-glass route when that mapping is wrong or the provider is unreachable |
| ACC-25 | **Role assignment is one mode for the whole deployment: group-bound or direct. Never both, no hybrid** | A hybrid needs a precedence rule for someone holding triager from a team and reader directly — forgettable, and a way for a stale direct grant to outlive removal from the team. One mode means one answer to "where did this person's access come from" |
| ACC-36 | Switching to group-bound mode **keeps individual assignments but marks them inactive**, and every view showing them states that plainly. Switching back restores them | Lets a deployment try group-bound mode without the trial being one-way — and people do switch back, usually on discovering their groups don't map to how the team actually divides work. Deleting would make reverting a reconstruction from memory |
| ACC-37 | An inactive assignment is **never counted as effective access** in any report or access review | The specific way dormant rows cause harm: a row saying someone is an admin, read by an auditor or by a query nobody thought about, when it grants nothing at all |
| ACC-26 | In group-bound mode an admin manages **explicit mappings of team or group to role, per product**. Per-user assignment is disabled while that mode is on | Security is then managed where the org already manages it. Leaving per-user editing enabled would let an admin make changes that silently reset on next sign-in |
| ACC-27 | In group-bound mode the **mapping is the pre-authorization**: a first-time user in a mapped group is admitted and their record created then. Someone in no mapped group gets the generic not-authorized message | Reconciles with ACC-21, which forbids admitting anyone not authorized in advance. Mapping the group *is* that advance authorization, so this is not auto-provisioning by another name — an unmapped person still gets nowhere |
| ACC-28 | In group-bound mode at least one group must map to admin, checked at startup | Otherwise a deployment can lock itself out of its own administration, and the only route back is editing the database by hand |
| ACC-22 | Where a role is bound to a provider group, **losing the group withdraws the role**. Group-derived roles are never sticky | A bound role is a statement about current membership. Leaving the group and keeping the access would make the binding decorative, and it is exactly the case an access review is meant to catch |
| ACC-23 | Group membership is a **snapshot taken at sign-in**, so withdrawal takes effect within a bounded window, not instantly. Sessions carry a maximum lifetime and roles are re-derived on each sign-in | Neither provider notifies us when someone leaves a group — Okta puts groups in the login claim, GitHub needs a call after sign-in, and both are point-in-time. Without a session lifetime, a signed-in user could keep a withdrawn role indefinitely. Settled by ACC-38: no scheduled re-check |
| ACC-38 | **Group membership is read at sign-in only.** No background polling. The window in which a withdrawn role still applies is the session lifetime, which an admin configures | The deliberate case — someone leaving, or access being pulled — is handled instantly by revoking the session (ACC-16), which is a better mechanism than any polling interval. What remains is quiet drift after a team move, and shortening session lifetime addresses that without one provider call per active user per cycle against rate-limited APIs, or roles flapping when the provider has a bad day |
| ACC-24 | GitHub group binding uses **organization teams**, read after sign-in with the `read:org` scope | GitHub has no group claim. Practical limits worth knowing: we inherit whatever team structure the org already has rather than defining our own, and an organization can restrict OAuth app access, in which case membership is invisible and binding silently yields nothing — which must fail closed, not open |
| ACC-21 | **No account is ever created automatically, on any sign-in path.** Access is granted in advance or not at all. Someone who authenticates successfully but has not been authorized gets a **generic not-authorized message** | Authenticating proves who someone is; it says nothing about whether they should be here. The message is deliberately the same whether the account is unknown, known but unauthorized, or authorized for nothing — telling an outsider which of those applies is free reconnaissance |
| ACC-19 | **Optional trusted-header sign-in**: a reverse proxy authenticates and passes the username on in a header, which the app accepts as identity. Header name configurable | Lets development and testing run with no OAuth provider at all — basic auth at the web server is enough. Also supports operators who already authenticate at their ingress, which is the pattern REJ-03 rules out only as the *sole* mechanism |
| ACC-39 | The trusted-header path also accepts **group membership**, binding to roles exactly as identity-provider groups do | Extends no new trust: anyone able to forge the group header could already forge the username header and claim to be an admin outright. Lets an operator run entirely behind their existing ingress authentication with no identity provider configured in the app, and makes all three sign-in paths equivalent rather than leaving one second-class |
| ACC-40 | **Header name and delimiter are both configurable**, shipping with presets for the common proxies. There is no standard to hard-code | Neither header is standardised — `X-Remote-User` is a convention descended from the CGI `REMOTE_USER` variable, not a specification. oauth2-proxy uses `X-Auth-Request-User` and `X-Auth-Request-Groups`; Authelia uses `Remote-User` and `Remote-Groups`. The separator for multiple groups is convention too, usually a comma |
| ACC-41 | Missing or empty groups means **no roles**, never unrestricted | The failure that would otherwise be silent and total |
| ACC-42 | Known limitation: proxies that deliver identity in a signed token rather than plain headers — Cloudflare Access, AWS load balancer OIDC — are not supported by this path | Reading a header cannot verify a signature. Such deployments use a configured identity provider instead. Documented so it is a known boundary rather than a support question |
| ACC-20 | Trusted-header sign-in is **off unless explicitly enabled** and honoured **only from configured trusted sources** | Trusting the header unconditionally would let anyone able to reach the app directly become any user, admin included — curling the container bypasses the proxy entirely. Two guardrails are enough: turning it on and naming a trusted source are both deliberate acts. It is never a fallback when other sign-in is unconfigured, since that is how it ends up live by accident |
| ACC-15 | **The browser never holds a provider token.** The backend runs the sign-in exchange server-side and issues its own session cookie, marked `HttpOnly` so scripts cannot read it | GitHub returns an opaque token our API cannot verify by itself, so a browser-held-token design would need a second validation path and a frontend handling two token shapes — against ACC-01's single provider interface. It also keeps a script injection from stealing a session, and makes revocation real |
| ACC-16 | Sessions are stored in the database, not in process memory, and can be revoked immediately | Several copies of the app may run, and a session must work whichever one answers. Deleting the row cuts access off at once — see the first lesson in Section 10 |
| ACC-17 | **Three subject types, one resolution step**: a person via session cookie, a CI service via API key, and a person's own API token (ACC-33). Every request resolves to a subject with capabilities and scope before any business logic runs | Nothing downstream needs to know which door a request came through, which is what ACC-04's data-layer enforcement already assumes |
| ACC-18 | State-changing requests carry cross-site request forgery protection; API-key requests are exempt | Browsers attach cookies automatically, so a hostile page could otherwise act as a signed-in user. Keys are never sent automatically, so the guard would be pointless there |
| ACC-10 | CI pipelines authenticate with **API keys** — a distinct subject type from users, ingest-only. A key can push scan files and do nothing else: no reading findings, no triage, no reporting | A pipeline cannot complete an interactive sign-in, and it has no business holding a human's permissions. Keeps ACC-04's visibility rules from ever being reachable by a build server |
| ACC-11 | **A key's scope is a set of optional constraints, not a path.** Product is always required; release and variant are independent and either, both, or neither may be pinned — so product, product + variant, product + release, or all three. The upload **always states its full target explicitly**; the key only authorizes it. Every constraint present on the key must match, and a mismatch is **rejected, never re-routed** | A key per product is expected to be the common case, and such a key cannot possibly imply the release or variant — so the upload has to say. Making that the rule at every scope means one code path instead of a special case, requests that are readable in logs without resolving what a key was scoped to, and no chance of re-scoping a key silently changing where its uploads land. Silent re-routing would file one product's data under another's name, which is worse than a failed build |
| ACC-14 | A key's variant constraint matches the variant **by name**, across whichever releases the upload names | Variants belong to a stream rather than to the product (MDL-01), so there is no single global variant to point at. Matching the name is what makes "only ever the broadcom builds, on any branch" expressible without contradicting that |
| ACC-12 | Keys are created, scoped, rotated and revoked by an admin through the application, with last-used shown. **Every ingest records which key submitted it** | Under SCP-03 an operator cannot be asked to edit config files to rotate a credential. Recording the key alongside the parser version (ING-07) makes "where did this data come from" answerable |
| ACC-13 | Several keys per product, typically one per pipeline | Revoking a compromised or retired key must not take every other pipeline down with it |
| ACC-46 | **A private finding carries a disclosure date, defaulting to 90 days from creation.** Configurable per deployment. Public findings have none — they are already disclosed | Matches common coordinated-disclosure practice, and gives the embargo an end an outsider could hold us to |
| ACC-47 | **Reaching the date does not disclose anything automatically.** It escalates: the finding is flagged, and admins and its owner are told | Publishing embargoed detail because a timer expired is the wrong default — if the fix is not ready, disclosing anyway is a decision a person makes. Automatic would eventually publish something nobody was ready for |
| ACC-48 | Extending a disclosure date **needs a reason and, past a configured threshold, approval** — the same shape as deferral | Extending keeps risk hidden longer, which TRI-16 says needs a second person. Without it the date is decoration and the indefinite secrecy the framework warns about arrives one quiet extension at a time |
| ACC-49 | Findings approaching disclosure are surfaced before the date, not on it | The date arriving is the last moment to act, not the first useful warning |
| ACC-09 | When a private issue is disclosed, the whole record goes public — comments, decisions, actors | History inherits the finding's visibility. Private issues carry a standing notice saying so |

### 3.12 Notifications — `NTF`

| # | Decision | Why |
|---|---|---|
| NTF-01 | Channels sit behind one interface. Email required, Slack and Teams as adapters. Delivery is queued and retried | A third channel should be an adapter, not a rewrite |
| NTF-02 | Immediate email only for explicit human actions: you were assigned something, or a dismissal needs your approval | The only category that reliably deserves an interruption |
| NTF-05 | **A rejected dismissal emails its proposer immediately. An approved one does not** | Rejection puts the finding straight back in their queue, so silence would leave it sitting while they believe it is handled. Approval is the expected outcome and needs no announcement |
| NTF-06 | Because silence covers both "approved" and "not yet reviewed", the proposer's own view lists their dismissals still awaiting review | The gap NTF-05 leaves, closed with a view rather than more email |
| NTF-03 | A daily digest carries everything else. Opt-in, off by default | Nobody is subscribed to noise they didn't ask for |
| NTF-11 | **A newly-critical vulnerability in a shipped release is an operational alert**, not a dashboard entry | The stated purpose: "this release has a critical, we need to cut a new one." Something a person is told, not something they find when they next go looking |
| NTF-08 | **An in-app notification area with an unread count**, showing operational alerts to admins | Works with no email configured at all, which matters under SCP-03 — a self-hosted operator who never set up SMTP would otherwise have every operational alert sent into a void. Also puts them in front of whoever is actually using the tool rather than relying on someone reading mail |
| NTF-09 | **Condition-based alerts clear themselves when the condition ends. Event-based ones are acknowledged** | An artifact that resumes being scanned should stop being an alert without anyone dismissing it. Otherwise the count fills with resolved problems and people stop reading it — which is the same failure the digest rules were written to avoid |
| NTF-10 | **Everyone gets the notification area, not just admins** — a triager sees newly assigned work, a proposer sees a rejected dismissal, an approver sees items waiting on them. Content differs by role; the mechanism does not | New work arriving is the thing a triager most wants to notice, and it is the same feature. Also gives the rejected-dismissal notice (NTF-05) somewhere to live for anyone who does not read email |
| NTF-07 | **Operational alerts are their own category, sent to admins, and are not subject to the opt-in digest** | Three already-decided alerts fit neither existing category: dormant users holding work (ACC-45), artifacts that stopped being scanned (RPT-02), and scans flagged as broken (STA-14). None is an explicit human action, so none is immediate; all would land in a digest that is off by default and be seen by nobody. They are about the tool's own health, and an operator who has not opted in is exactly the one who needs telling |
| NTF-04 | A new build notifies nobody | Nothing is auto-assigned, so a build produces only digest content. Revisit this first if auto-assignment is ever added |

### 3.13 Database — `DAT`

| # | Decision | Why |
|---|---|---|
| DAT-01 | MySQL, MariaDB and PostgreSQL in production, SQLite for development and testing | Operators install against what they already run and know how to back up. Treated as three distinct production engines, not two — see DAT-12 |
| DAT-02 | No engine-specific SQL in the core | See [Section 6](#6-constraints-that-shape-the-design) for what this rules out |
| DAT-14 | **`bun`** for database access | Four engines, no engine-specific SQL, plus recursive tree queries and interval queries at scale. Bun handles dialect differences without hiding the SQL — and the queries are the hard part here, so control matters more than convenience |
| DAT-15 | **`goose`** for migrations | Embeds in the binary, covers all four engines, and supports Go-based migrations so the per-engine partitioning DDL branches cleanly rather than living in a directory per dialect |
| DAT-16 | Drivers: **`pgx`** (PostgreSQL, MIT), **`go-sql-driver/mysql`** (MySQL and MariaDB, MPL), **`modernc.org/sqlite`** (pure Go, BSD) | The SQLite choice is deliberate: pure Go means no cgo, which keeps the single static binary |
| DAT-17 | **The job queue is hand-rolled** — a table plus a bounded worker pool | River is PostgreSQL-only and Asynq needs Redis, so neither fits four engines. Around 200 lines we control, using row-skipping locks on the production engines and a simpler path on SQLite, which is development-only |
| DAT-03 | `sqlc` is not a candidate | It generates per-dialect code from per-dialect SQL; three engines would mean three query sets |
| DAT-12 | **MySQL and MariaDB are two separate targets, both tested in CI.** Four tested engines in total with PostgreSQL and SQLite | They stopped being interchangeable years ago, and the divergence reaches things we use — JSON behaviour, sequences, and above all partitioning syntax and limits. Partitioning is engine-specific by design and drives data purging, so an untested difference there deletes the wrong rows rather than raising a visible error. One extra container in the matrix against a silent data-loss class of bug |
| DAT-13 | Version floors are declared per engine, not per family | MariaDB and MySQL version independently and share no numbering. Proposed: PostgreSQL 14+, MySQL 8.0+, MariaDB 10.6+ |
| DAT-04 | Minimum supported versions are declared, tested, and checked at startup | Refusing to start beats failing confusingly later |
| DAT-07 | **The application creates and migrates its own schema at startup, on by default** | Self-hosted operators (SCP-03) should not need a separate step or an external script, and it removes app-versus-schema version skew: deploying the new binary is the whole upgrade |
| DAT-08 | Auto-migration can be **switched off**, and a `migrate` subcommand runs it separately | Lets an operator run migrations under different credentials, at a chosen time, and see what will change first. The default stays the easy path; this is the escape hatch for people who want control — a real need at scale, and free to provide |
| DAT-09 | Only one instance migrates at a time, enforced by a lock | Several copies starting together would otherwise race. Locking differs per engine, so this is one of the few places the portable-SQL rule (DAT-02) genuinely leaks and needs per-engine code |
| DAT-10 | Migration runs before serving, and startup health checks allow for it taking a long time | A change across a large partitioned table can run for a while. A probe that kills the container mid-migration turns a slow upgrade into an outage, and then restarts the migration from the beginning |
| DAT-11 | The running application may hold read and write rights only; schema-change rights are needed solely while migrating | Under DAT-08 an operator can separate the two entirely. It matters more here than most places: this app ingests files from CI, so a compromise holding permanent `DROP` rights is a materially worse outcome |
| DAT-05 | High-volume tables are partitioned by time and purged by dropping partitions, never by large deletes | Both production engines make large deletes expensive. Partitioning lives in DDL, so queries stay portable |
| DAT-06 | Purging exports rows to a compressed file before dropping | Archive then drop, never drop alone |

### 3.14 CI gate — `CIG`

| # | Decision | Why |
|---|---|---|
| CIG-01 | Static analysis runs on every pull request and **blocks the merge** | An analyzer that reports and is ignored is decoration. Branch protection is the real deliverable |
| CIG-08 | **Branch protection is not enforced yet** — early development. The gate is built and runs; making it a required check comes later | Deliberate deferral, recorded so CIG-01 does not read as already done. Until protection is on, the gate reports rather than blocks — which is exactly the state CIG-01 warns about, so this needs revisiting before the project takes outside contributions |
| CIG-02 | One file defines every tool version, rule selection, and whether a rule blocks. Two scopes: gate and advisory | A rule change is a one-file diff |
| CIG-03 | Full gate from day one. No baseline differencing, no fingerprinting — none of it built | A new repository has no backlog to grandfather, which removes the most complex machinery such designs carry |
| CIG-04 | `golangci-lint`, pinned: govet, staticcheck, errcheck, ineffassign, unused, revive — errcheck excluding Close/Flush/Sync/Shutdown, revive with `exported` and `package-comments` off | Measured tuning: 122 raw findings became ~43 actionable, losing nothing from staticcheck, govet, unused or ineffassign |
| CIG-05 | Every CI finding is reproducible locally with one documented command | A gate you can't run before pushing teaches people to push and wait |
| CIG-06 | Suppressions are checked in, reviewable, and carry a reason | Same principle as TRI-01: hiding something needs a second pair of eyes |
| CIG-07 | `govulncheck`, dependency review, secret scanning and the licence check gate on the same terms. **No Semgrep, no CodeQL** | We'd be shipping a vulnerability tool; its own supply chain can't be unexamined. `gosec` inside the existing lint run is a possible partial substitute |

### 3.15 Interface — `UIX`

| # | Decision | Why |
|---|---|---|
| UIX-15 | **Responsive design is a requirement, not an enhancement.** Every screen works on a phone | Stated. Rules out any packaged data grid that owns its own markup — the findings table has to become something else on a narrow screen, not scroll sideways |
| UIX-16 | On narrow screens the findings list becomes **cards, not a horizontally-scrolling table** | A row carrying severity, component, version, both ends of the path, state and product does not shrink. Sideways scrolling on a phone is the failure mode this is avoiding |
| UIX-17 | Small screens are designed for **review and respond**, not bulk work — read a finding, approve or reject one, check what is assigned to you | Nobody bulk-triages 300 findings or explores a deep dependency tree on a phone. Designing as though they might makes it worse at both. Bulk actions and the tree remain available but are not what small screens are shaped around |
| UIX-18 | **React, Vite, TypeScript, Tailwind, TanStack Query** | The findings table is the app, and TanStack Table with virtualisation handles tens of thousands of rows. Headless, so UIX-16's cards-on-mobile is possible at all — a packaged grid owning its own markup could not do it |
| UIX-19 | The API client is **generated** from the OpenAPI document with `openapi-typescript` and `openapi-fetch` | Change a Go type, regenerate, and TypeScript fails at every call site. The whole point of API-04 |
| UIX-20 | **Recharts** for charts | Enough for the trend shapes in RPT-09 to RPT-12, and familiar nearby |
| UIX-01 | **The findings list shows one row per place**, matching the data exactly | What you act on is what you see, with nothing aggregated away. Consequence to design around: the same CVE can fill a screen, so the **path has to be the thing that visually distinguishes one row from its siblings** — twelve rows differing only in a column nobody reads is the failure mode |
| UIX-12 | A findings row shows **both ends of the chain** — the owning subproject and the immediate parent, middle collapsed. Full chain on expand or in the tree | Those two are what differ between sibling rows: the top says which part of the product this is, the bottom says what directly pulls it in and is therefore what a decision is about. The middle rarely distinguishes anything |
| UIX-14 | **Opening a finding shows the complete chain**, root to component, with the version at each step | The row shows two ends for scanning (UIX-12); the detail view shows all of it, because that is where someone actually judges whether the vulnerable code is reached. Versions along the chain matter too — they are what the expiry rules key on |
| UIX-13 | Two rows are only ambiguous when **both** ends match — the same subproject reaching the same component twice by different routes | Rare, and visible when it happens. Expansion or the tree resolves it |
| UIX-10 | **Saved filters are personal. Nothing is shared** | No ownership, no permissions, no arguing about whose filter is authoritative, and nobody hesitates to save something half-formed |
| UIX-11 | Filter state lives in the URL, so a link carries it | Covers the team case without shared configuration: "here is the filter I use" is a paste, not a stored object someone can edit under you. Works because scope is explicit rather than remembered, so a link means the same thing to whoever opens it |
| UIX-07 | **The findings list is scoped to one product.** You pick a product first, and everything below is bound to it | Matches how roles are granted, and every screen is unambiguous about what it is showing. Suits the common case of someone whose job is one product |
| UIX-08 | The home page panels still summarise **across** products — assigned work, approvals waiting, alerts | Where the portfolio view lives, so UIX-07 does not leave someone covering several products checking each in turn to find out whether anything needs them |
| UIX-09 | Reports are **not** bound by UIX-07 and may span products | Comparing products is often the point of a report, and a person only ever sees products they hold a role on regardless |
| UIX-05 | **One home page, assembled from what the person holds** — assigned work, items awaiting their approval, operational alerts, overview panels. Not a different landing page per role | People commonly hold several roles at once, so "land on the page for your role" needs a precedence rule that would be wrong for the most active users. One page composes instead of choosing, and degrades naturally for someone with a single role |
| UIX-06 | Panel order and what is left out need deliberate design | The risk in UIX-05 is a page trying to be everything and succeeding at nothing |
| UIX-02 | **The dependency tree is browsable.** Walk the SBOM to see where a component carrying a CVE actually sits | The answer to "why is this here", and it matters more given UIX-01: with a row per place, the path is what makes rows different, so it needs somewhere to be explored properly rather than squeezed into a column |
| UIX-03 | Navigation goes **both ways** — from a finding to its position in the tree, and from any node in the tree to the findings beneath it | Triage starts from a list and asks "where is this"; review starts from a subproject and asks "what is in here". Both are real, and a one-way link makes the second one manual |
| UIX-04 | A full tree render is not attempted. Expansion is **lazy, with a focused view around a selected component** | Tens of thousands of nodes will not draw, and would not be readable if they did |


---

## 4. Still open

None. Every question raised so far has been answered — see Section 11 for how
the significant ones were settled.

---

## 5. What we ingest

Reference material from the two producers we have: `sonic-net/sonic-buildimage#27455`
and `opencomputeproject/onie#1133`.

**Format**

- CycloneDX 1.6 is the source of truth in both. ONIE also emits SPDX 2.3; SONiC also emits an in-toto provenance file.
- Vulnerability data comes from grype.
- Suppressions are mined from patch headers and applied before the report is written — so we never see them (ING-02).
- Components come from many ecosystems: Debian packages, Python wheels, Rust/Go/npm lockfiles, container layers, vendor blobs, locally-built sources.
- `pedigree.ancestors[]` carries upstream identity for patched forks.
- The document is hierarchical: a root component, nested sub-components, and an edge list.
- The same package legitimately appears at many versions and many places, so package-plus-version is **not** a unique key.
- **These are two producers out of many expected.** Anything the format doesn't require, we don't assume (ING-05, ING-04).

**Size — this is a ceiling, not a baseline**

> Do not use these as typical values or as a multiplier. SONiC is by far the
> largest producer. ONIE and the others are far smaller.

| Metric | SONiC (upper bound) |
|---|---|
| Components per file | ~56,600 |
| CycloneDX file size | ~46 MB |
| SPDX file size | ~27 MB |

**Volume** (ING-08): 10–50 files per night across a few product lines, heavily
skewed toward SONiC. Realistic daily total is likely under 1 GB, not the 2.5 GB
a flat average would suggest.

---

## 6. Constraints that shape the design

**Running on three database engines** (DAT-02, DAT-01) rules out:

- PostgreSQL JSON operators and GIN indexes on JSON
- `RETURNING` — MySQL 8 has none
- PostgreSQL full-text search
- Arrays, partial indexes, `DISTINCT ON`

And requires care with:

| Trap | Detail |
|---|---|
| **Collation** | MySQL defaults to case-**insensitive**; PostgreSQL and SQLite are case-sensitive. Package names and CVE IDs would compare differently unless collation is pinned |
| Index key length | MySQL is tightest. Package identifiers get long — index a hash, not the raw string |
| Timestamps | Semantics differ. Store UTC, be explicit about types |
| Migrations | Must be portable, or maintained per engine |
| Testing | Every engine, in CI. SQLite-only tests catch none of the above |

Good news: `WITH RECURSIVE` works on all three, so graph traversal needs no
engine-specific path.

**Scale** (ING-08, ING-06):

- Storing every nightly scan in full would mean 200 million to 1 billion rows a year. Not viable — hence STA-08 and STA-06.
- Day-over-day change is small. Store change, not scans.
- A 46 MB file parsed naively can cost hundreds of MB of memory. Stream it, and cap concurrent ingests.
- One pipeline spans roughly a thousandfold in input size. Size limits from the largest artifact, and benchmark against a full-size SONiC file.

---

## 7. Choices not yet made

Engineering choices we'll make and record. Listed so nothing is invisible.

| Area | Options |
|---|---|
| Component library | shadcn/ui is the candidate — you own the source rather than tracking a dependency — but the tree and grid work may want more. Better decided against a real screen than in the abstract |
| Partitioning detail | Which column, what granularity, and how to handle retiring a whole product — which partitioning by time does not solve |

---

## 8. Out of scope

- **Generating SBOMs.** We ingest them.
- **Scanning for vulnerabilities.** They arrive already scanned.
- Manual vulnerability report intake from outside the organisation.
- Being a CVE Numbering Authority.
- Patch management and **deploying** fixes. Remediation is tracked (REM-02); nothing ships from here.
- Customer-facing status pages.
- Licence and compliance analysis of SBOM contents. Adjacent and likely to be asked for — note it, don't build it.
- Replacing an external issue tracker. Hand-off is optional and configured (REM-11).

---

## 9. Rejected

| # | Rejected | Why |
|---|---|---|
| REJ-01 | `sqlc` | Per-dialect codegen; three engines would need three query sets |
| REJ-02 | PostgreSQL-specific SQL features | Breaks DAT-02 |
| REJ-03 | oauth2-proxy at the ingress as the only sign-in | Works in Kubernetes, doesn't travel to self-hosted or local development |
| REJ-04 | Parsing on the request | 46 MB and 56,600 components |
| REJ-05 | SPDX 2.x as the primary format | Cannot carry vulnerability data at all |
| REJ-06 | Semgrep and CodeQL | On measured evidence elsewhere, they run and get ignored |
| REJ-08 | Backport tracking by linking pull requests tagged with a target branch | Assumes commit and PR linkage we do not have. Replaced by REM-06, which derives the same picture from scans |
| REJ-07 | Versions inside the path identity key | The top-level version changes every build, so every decision would lapse nightly |

---

## 10. Lessons carried in

Two things learned the hard way elsewhere, recorded because several decisions
here exist to avoid repeating them.

1. **Don't keep request state in process memory if you'll run more than one copy.**
   A service that held job status in package-level variables behind two replicas
   needed sticky sessions at the ingress to work at all, which then capped how
   far it could scale. ING-03 puts ingest status in the database for this reason.
2. **Generate the API client from a real specification.** Where none is
   published, someone downstream ends up reconstructing one by pattern-matching
   hand-written documentation — and it silently drifts. API-04 exists to prevent
   that.

---

## 11. How this document changed

All entries 2026-08-27. Only the corrections and reversals are listed — the
routine additions are visible in the decisions themselves.

| What changed | Why it matters |
|---|---|
| "SSO via OIDC with GitHub" corrected | GitHub is not an OIDC provider. Resolved as one provider interface with two adapters (ACC-01) |
| "Reporting" reinterpreted, then partly reversed | First read as analytics with no manual intake. Wrong — private findings are entered by hand, so manual entry exists in Phase 2 |
| Per-build triage abandoned | "Builds of the same release" was the wrong model. Only current state matters, and findings open and close themselves (STA-02, STA-03) |
| SONiC's measurements reframed | Presented as a baseline; they are a ceiling. Other producers are far smaller (ING-08) |
| "OS variants diverge broadly" corrected | They don't. Application and language dependencies are shared; only the distro layer differs (REL-04) |
| Versions removed from path identity | Would have lapsed every decision nightly, since the top-level version changes every build (MDL-06, R7) |
| Top level removed from identity **and** expiry | Caught a real defect: the root's name differs per variant, so including it would have silently broken all cross-variant grouping (MDL-07) |
| Decisions unbound from releases | Storing decisions per release would have needed copy-forward, which drifts. Keyed structurally instead, so inheritance is a lookup (REL-05) |
| `gate-new` dropped entirely | Its whole purpose is grandfathering a backlog. Starting from zero, we never build one (CIG-03) |
| No external feeds → two required | True for finding issues, false for ranking them (ING-10) |
| Questions rewritten | The first list mixed product decisions with engineering calls, in shorthand that made both unanswerable |
| Tree views restored | The dependency-tree and "why is this here" views were decided in the long version and lost when the document was condensed — the fourth such loss found. Recovered as UIX-02 to UIX-04 |
| App-served docs reversed | Briefly had the application serving public API documentation, which meant one unauthenticated route to fence off. Replaced by publishing plus a release artifact, leaving no unauthenticated routes at all (API-05) |
| Variants declared, not discovered | An early decision had variants appear automatically from whatever a scan named. Reversed once the typo case was considered: a misspelled release is indistinguishable from a real one (ING-11) |
| Renumbered to grouped prefixes | Was a flat `D1`–`D110` sequence in rough chronological order. Regrouped by area so the topic is visible in the ID. Remediation and reporting were split out of an overloaded triage section at the same time. Old `D` numbers are retired and do not map forward |
| Same-origin UI serving restored | Recorded as a leaning in the long version and dropped when the document was condensed. Recovered as API-07, and it is load-bearing for how sign-in works |
| Machine authentication restored | How CI authenticates was an open item in the long version and was dropped when the document was condensed. Recovered as ACC-10 to ACC-13 |
| An early domain-model sketch dropped | Superseded once variants and the tracked unit were settled, and kept only as a placeholder. Removed in the renumbering rather than carried forward as a dead ID |

**Recurring lesson:** every bug in the identity and expiry rules came from
letting one fact into two rules. Versions belong to expiry. The variant belongs
to the tracked unit. Neither belongs in the path key.
