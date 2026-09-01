# Working notes

Where the interface work is, what is decided, and what is still open. **This
document is temporary**, like `IMPLEMENTATION.md`: it holds the state of one
stretch of work so it survives a break. Anything durable belongs in
`DECISIONS.md` or a `DESIGN-*.md` and should be moved there rather than left
here.

## Where to pick up

**Everything decided so far is built except one half.** The state of it, in the
order it would be picked up:

| | |
|---|---|
| **Done** | The rebuilt SBOM is in as the fixture, the demo is reseeded from it, and `ING-36`/`ING-37` carry their new numbers |
| **Built** | `ING-41`, end to end: the `upstream.currency` setting (off by default), the pass that fills the columns, a third worker beside the reader and the runner, and the finding screen showing both what upstream has released and why there is no fix |
| **Not started** | `DESIGN-interface.md` is behind the code and is the thing most likely to be lost, because this document is temporary and it is not |

**How `ING-41` ended up built**, since the shape is not obvious from the
decision:

1. `upstream.currency`, off by default, and the fourth kind of setting — a
   switch. The value is read **each cycle** rather than at startup, so turning
   it *off* takes effect without a redeploy, which matters more than turning it
   on does.
2. A pass over components whose answer is missing or older than a day, 200 at a
   time with a quarter-second between requests. These are free public services
   and a tool that walks them as fast as it can is the reason they end up
   needing a rate limit. A first run drains over hours; nothing needs it sooner.
3. Distribution packages are skipped, because the distribution is the
   maintainer and its release date says nothing about the software inside.
4. **A question we cannot answer is still recorded as asked.** A private module
   and a vendored fork both look like a package the index has never heard of,
   and without writing the time of asking they would be asked about again
   tomorrow and every day after, forever. A request that *failed* records
   nothing, so it is retried — the two are different and are stored
   differently.
5. The no-fix signal compares the year in the identifier against the newest
   release, which is what `ING-41` says and why `REJ-11` is not needed. It is
   deliberately phrased as *why there is no fix* rather than as a claim that a
   project is abandoned — nothing here knows that, and `REJ-13` rejected
   saying so.

**Two things settled on 2026-09-01, both found by re-reading rather than by a
test failing.** Neither would have shown up as a bug.

*A clear year of silence, not merely an earlier year.* The first version asked
whether the newest release was from an earlier calendar year than the
identifier — so an issue named in January 2026 against a release the previous
December got "waiting for a fix is unlikely to end", on a five-week gap. The
code and the comment above it disagreed, and the comment was right. Now it
takes a full year. **The general shape: where a stand-in is only precise to a
year, comparing two year-numbers makes a five-week gap look identical to a
five-year one.** The screen states the release date rather than a year parsed
out of the identifier, which was also brittle for identifiers that carry no
year.

*A package the index has never heard of is left for a month, not a day.* Every
other answer goes stale in a day. This one almost never changes — a private
module, an internal fork, something vendored from a git URL — and on a real
image a large share of components are exactly that, so the daily rule meant
thousands of permanently unanswerable questions at free services run by other
people. A month still notices the rare package that does get published, and
still recovers from an index that was having a bad day in a way that looked
like "never heard of it". The two are told apart by whether a version was
stored, which the data already recorded.

**Four things learned by asking the real indexes**, which are in the code and
worth not rediscovering: crates.io refuses a request that does not identify
itself; the Go proxy escapes an uppercase letter as `!` and its lowercase; PyPI
dates a release by its files rather than by itself; and npm dates each version
only in its full document, which for `@types/node` is eleven megabytes — a four
megabyte read bound was silently truncating it into invalid JSON.


The interface is built out and running. `make demo` brings it up; the details
and the two settings people get wrong are in `DESIGN-interface.md` under
"Running it locally".

**Every screen the mockup has now exists.** People and access, release
comparison and "who is working on what" were built in this stretch, and the
catalogue screens can declare as well as list. What is left is one structural
divergence and two missing numbers — see "Audit: the mockup's screens against
what is built" below, which is the place to start rather than
`DESIGN-interface.md`.

**Researched: what a slow query does to the rest of the process.** Measured
against the real database with the driver this build uses, so two of the three
questions are answered and the third is narrowed.

*Cancellation works.* `modernc.org/sqlite` honours the context: a query
cancelled mid-flight stopped dead — process CPU frozen at the tick it was
cancelled on and unchanged six seconds later. So the theory that a cancelled
walk keeps computing is **wrong**, and nothing needs fixing there.

*One slow query blocks every request in the process.* `internal/database/
pool.go:68` sets `MaxOpenConns(1)` on SQLite, for a good reason — SQLite has
one writer and more connections add contention rather than concurrency. The
consequence is that the pool is also the queue: with a slow query running,
`SELECT 1` beside it waited the full three seconds it was given and never got
a connection. That, and not cancellation, is why one broken screen stopped
`/v1/products` from answering. It is a property of the engine rather than a
defect, and it is worth knowing that on SQLite **any** slow statement is a
whole-process outage. The other three engines get 25 connections and do not
have it.

*Still unexplained: the three cores.* The wedged process was burning about
three full cores an hour and a half after the request that started it was
cancelled, and neither finding above accounts for that — one connection runs
one query, and cancellation stops it. The likeliest remaining candidate is the
garbage collector, which is parallel, working over result sets that a browser
tab kept re-requesting. To settle it: reproduce with a deliberately slow
statement, leave the tab open, and watch per-thread CPU rather than the
process total.

*Fixed: `SIGTERM` did not stop it and `SIGKILL` was needed.* The cause was not
where I first guessed. The HTTP server already had a fifteen-second grace; what
had none was `workers.Wait()`, the wait for the background readers. Those wait
on the same single SQLite connection the runaway requests were holding, so a
worker could not get a connection, could not notice it had been asked to stop,
and the wait had no end. It now has the same bound as the server's, and says so
in the log when it gives up. Measured after: `SIGTERM` stops it in **0.20 s**.

**The mockup is the approved design** and is the reference for anything
visual. It is published at
`https://claude.ai/code/artifact/fe3e1df3-fa9d-4d12-9545-b31d9366d078`. The
palette, type scale and shell in `web/src/index.css` were taken from it
verbatim rather than approximated — that was the correction after a first
attempt built from the decision text alone and came out looking nothing like
it.

## Decided during this stretch

Each of these is recorded in `DECISIONS.md`, except the last, which changed
nothing here and is recorded in the upstream commit instead; they are listed
here only so the reasoning is findable while the work is fresh.

| | |
|---|---|
| **RNK-06 amended twice** | Likelihood used to outrank severity. Measured wrong: a 2004 negligible with no score outranked all 379 criticals on a likelihood of 0.80. Multiplying the two was tried next and reversed on the same evidence — 95% of 5,661 open issues sit between 0.001 and 0.01 likelihood, so multiplying amplifies noise inside a spike. **Severity now leads; likelihood orders what is equally severe** |
| **The trend counts issues, not places** | It counted finding rows, which are per place, and reported 441,108 open where 5,661 issues were open — a 78× inflation measuring how much the graph shares rather than how much there is to answer |
| **A component is addressed by name and version** | A name is not unique in a build. Resolving by name alone answered about whichever was interned first, so two of three rows for one library said "no such finding". An ambiguous name with no version is now a 409 that says to give one |
| **ACC-57 to ACC-59** | Capabilities per product in one answer; the sign-in providers are public; mention candidates are only people who can already read the finding |
| **UIX-37** | The interface is embedded in the binary; a path the router has no route for belongs to the page |
| **One package arriving as two components was not ours to decide** | It read as a modelling question about purl namespaces and was none of the five options weighed here. The producer was emitting two records for one artifact — see "The split, and the reload it needs" below |

## Decided on 2026-09-01, and built

Talked through one at a time and written into `DECISIONS.md`. Listed here only
so the queue is visible while the work is fresh; the reasoning is there, not
here.

| | |
|---|---|
| **`TRI-37`, `TRI-38`, `UIX-40`** | ~~Built.~~ The Decide card is on the finding, covering every place by default with them listed and untickable. Its own endpoint, not the bulk one, so the ordinary approval rules still apply. One action is one transaction: thirty places took 6.09 s written one at a time and 0.10 s together, and a failure halfway no longer leaves a finding half answered |
| **`UIX-38`, `UIX-39`** | ~~Built.~~ The picker narrows the whole interface, `running-out`, `trend` and `unassigned` take a scope, and each narrowed screen says what it is counting. A branch or variant with no product is refused rather than guessed at |
| **`REM-26`** | ~~Built.~~ A deadline is computed at ingest and stored, and recomputed when the policy changes. Eight seconds became 1.35 s. The rewrite itself is sliced by identifier range and runs off the request: as one statement it took nineteen seconds and, on SQLite's single connection, that is the whole process answering nothing — the outage this document already diagnosed once. Sliced, other requests stay under a second while it runs. It is **not** on the job queue, so a restart mid-rewrite leaves some findings on the old deadline until the next scan or the next edit |
| **`UIX-41`** | ~~Built.~~ A findings row says what upstream has done — "declined" and "none yet" were the same blank — and how old the issue is, from the year in its identifier |

**All four are built.** Five more were decided on 2026-09-01 and are not:

| | |
|---|---|
| **`TRI-39`** | ~~Built.~~ Dismissing something because a mitigation stops it requires naming the mitigation, on both decision forms. The tool still will not notice its removal — that gap is accepted, not closed |
| **`TRI-40` to `TRI-42`** | ~~Built.~~ Verified end to end on the demo build: a high rated critical takes effect at once and moves the order; rated low it waits, moves nothing, and once a second person agrees it drops the order and takes all 86 of that issue's findings off the clock entirely — which is the danger that made approval the answer. The published rating is never overwritten |
| **`TRI-43`, `TRI-44`, `REM-27`** | ~~Built.~~ What a product considers worth triaging, with what it hides counted and named. Nothing below it is on a clock, and nothing known to be exploited is ever below it |
| **`ING-41`** | Whether a dependency has a newer version, and when that version shipped, read from the public index for its ecosystem. The one thing here that reaches the network, so it is off unless a deployment turns it on and it fails quietly |
| **`UIX-42`** | ~~Already true.~~ Home leads with the work and puts the trends underneath, and always did — the audit finding that prompted this was a misreading |

The largest of those is the assessment: it is a second kind of judgment and
wants the same care the first one got.

Also worth doing while in there: **check what the scanner knows about *why*
there is no fix.** `none` currently conflates "the distro considered it and
shrugged" with "nobody has fixed this yet" — 1,125 findings, of which 246 are
older than 2023 and so are plainly not the second. `wont-fix` already separates
427 of them, so the scanner has some of the nuance; whether it has more is
unchecked.

## Open, and needing a decision

Nothing here now. The two that were open — scoping home, and a disclosure date
— were settled on 2026-09-01 and are recorded as `UIX-38`, `UIX-39` and
`REJ-11`. What is open now lives in `DECISIONS.md` Section 4, where it belongs.

## Issues seen in the running interface

Reported from clicking around the demo. Each one is checked against the mockup
and against `DECISIONS.md` before it is written down, because the interface was
built once already from decision text alone and came out wrong.

**Fixed — "Where it sits" showed one flat row, not the chain.** `UIX-14` says it plainly:
*opening a finding shows the complete chain, root to component, with the version
at each step*. The mockup draws exactly that — `sonic-broadcom 202411.0`, then
`docker-sonic-mgmt-framework 1.0.0` indented under it, then `openssh` indented
under that, and the whole chain repeated for the second route through
`docker-platform-monitor`. What `web/src/screens/Finding.tsx:150` builds is one
row per place holding the direct consumer's **name only**: no root, no
intermediate steps, no versions. `UIX-12` is unimplemented the same way — the
findings row is meant to show both ends of the chain and shows only the
immediate parent (`web/src/screens/Findings.tsx:324`).

**Still true — "the product itself" is most of what you see, and it is not a
fallback.** It
renders whenever `finding.consumer_id` is null. In the demo database that is
**284 of the 450 components** that carry findings — so on a component picked at
random it is the likely answer, which is what makes it read as the screen giving
up. It is not wrong: the SBOM's edges are mostly *containment* (image → container
→ package), so a host package genuinely has the image as its only parent, and
4,818 of 8,374 components in the fixture have exactly that and nothing else.
Naming something more useful there needs richer edges from the producer, not a
change here. The phrase is also borrowed from the wrong screen — in the mockup
it belongs to the tree's "What pulls this in" panel, for a node with no parents.

**Waiting on the rebuilt SBOM — two places can render as the same row.** 207,606 findings have two places
whose consumer name is identical. It is not version drift — no consumer name in
the database exists at two versions — it is the duplicate-component bug below:
`opennsl-modules` interned twice from one artifact, so one place is listed twice
with nothing to tell the two apart. Most of this should dissolve when the
rebuilt SBOM lands. What will not dissolve is that the row has no ancestry to
distinguish it by, which is the first item.

**Fixed — "What was decided →" was the wrong affordance on that panel.**
The mockup's single **"Show where this sits in the build →"** is there and
opens the tree. The per-place link stayed only because deciding lived on its
own screen and this panel was the only route to it; the Decide card is now on
the finding itself, so the panel is an orientation link again and links to a
claim only where one stands.

**"Dependencies" in the left rail never loads, and wedges the whole server.**
Not a UI bug: `GET .../variants/{variant}/components` never returns.
`graph.Store.Roots` (`internal/graph/snapshot.go:301`) asks for the root's
direct children and hangs two correlated subqueries off every row — how many
findings are open against it, and whether anything is under it. The root has
**5,270 direct children** in the demo build, so both run 5,270 times.

*The findings count.* `finding` carries five indexes and **none of them
contains `component_id`**, so SQLite picks `finding_urgency_idx (target_id,
closed_run_id)`, which matches all 441,108 open findings for the target, and
filters each one. Measured: **637 ms per row, so about 56 minutes** for that
column alone. Adding `finding (target_id, component_id, closed_run_id)` takes it
to 0.067 ms — the whole column in 0.35 s.

*The children count.* Its subquery drives off `graph_edge` by `(target_id,
closed_scan_id)`, which is every one of the 19,192 edges, per row. An index on
`graph_node (component_id)` was tried and made it **worse** (5.4 s to 10.0 s) —
the driving scan is the edge table, not the node lookup. Computing it once as a
grouped join instead of 5,270 times drops it to 0.106 s, same rows, same order.

Both together: **about an hour to 0.11 s.**

The second half of this is worse than the slowness. The request is long gone —
the log has `walk the graph: context canceled` at 01:22:52 — and an hour and a
half later the process was still burning **three full cores**, because each
click started another walk that nothing stops. So the screen that does not load
also takes the rest of the interface down with it, which is why "everything is
slow" and "Dependencies is broken" are the same bug. Cancelling the HTTP request
has to cancel the query.

**Fixed — the dependency screen was a flat list, and is now the tree.** It now draws
the indented lazy tree `UIX-04` asks for, rooted at `sonic-broadcom`, with the
count on every node at every level so descending follows the findings rather
than guessing (`UIX-02`). Wide nodes show five and offer the rest, a component
already drawn higher up is marked *shown above* rather than expanded again, and
the pane beside it says what pulls the selection in and what it pulls in.

Two things the mockup has on that screen are **not** built, because no endpoint
answers them, and both are load-bearing rather than decoration:

- **The search box.** The mockup's own note says it plainly — *"nobody finds
  anything in a tree this size by opening nodes, so searching is the way in"*.
  There is no component search endpoint. The input is left out rather than
  drawn dead.
- **The header totals** ("8,374 components, 27,366 edges"). Nothing reports
  either number.

A third is possible but not done: the mockup opens straight to a component with
the path above it expanded when you arrive from a finding. That needs walking
`above` upward one request at a time, so it is a small design decision rather
than a line of code.

Two CSS rules the screen needs — `.detail` and `.upward` — were **missing from
`web/src/index.css` entirely**, having been skipped when the styles were lifted
from the mockup. That is the extraction trap recorded below, found a second
time, and the lesson holds: check the containers, not just the leaves.

**Fixed — the findings filters only narrowed the page you were looking at.** The endpoint
takes `limit` and `offset` and *nothing else* — no severity, no exploited, no
component. So the chips and the "at least" control in `Findings.tsx:49` filter
`all`, which is the fifty rows already fetched. Asking for "exploited" on page
one shows the exploited rows **among those fifty**, not in the build, and paging
with one active walks through a different arbitrary subset each time. The screen
does announce how many it hid, which is the honest half of a thing that should
not be client-side at all.

**Built — the toggles asked for on the findings view.** Two, and the second is the one the
data argues hardest for.

*Group by component and version, ignoring the path.* The list is already one row
per issue and component rather than per place, so the path is collapsed — what
this asks for is the level above: one row per component at a version, carrying
how many issues it has, so you can see that `openssl 3.5.6` has twelve rather
than reading twelve rows.

*Hide a component that drowns the list.* Measured on the demo build: the list is
**6,822 rows, and 4,943 of them — 72% — are the kernel**, one package. It also
holds 425,098 of the 441,108 places, 96%. The next largest contributor is
`binutils` with 58 rows. Hiding one component takes the list from 6,822 rows to
1,879, which is the difference between a list somebody reads and one they scroll
past.

Both are the server's, for the reason directly above. `REJ-10` is held to: the
total is counted through the same filter, and what is hidden is named on screen
rather than quietly subtracted.

Worth saying once: none of these is a case of the design being unclear. The
panel was specified, drawn, and cited, and the implementation went its own way —
the same failure the note at the top of this document records about the palette,
caught then by comparing against the mockup rather than the decision text.

## The split, and the reload it needs

**Resolved upstream, not here.** The 156 deb packages that appeared twice —
`pkg:deb/sonic/openssl` beside `pkg:deb/debian/openssl`, `pkg:deb/sonic/bash`
beside `pkg:deb/bash` — were one defect in SONiC's own generator, not a
disagreement worth modelling. `merge_components` already keyed on
`(name, version)` so its three producers collapse into one record, but the key
also carried architecture, read from a `sonic:arch` property that only the
recipe-emit and observation fragments set. All 5,826 syft components therefore
compared as architecture `""` and never met the recipe fragment describing the
same `.deb`.

Fixed in `sonic-net/sonic-buildimage` PR #29237, commit `ad3211ee`: architecture
leaves the dedupe key (no producer there knows it — the recipe reads a filename,
the observation stamps `CONFIGURED_ARCH`, only syft reads dpkg, and the three
disagree), syft's `distro=` qualifier moves to the merge winner so 65 packages
keep the advisory-feed context the loser held, and a new `scripts/sbom_purl.py`
escapes package identifiers for both producers so `+fips` stops arriving as
`+fips` from one and `%2Bfips` from the other. Replaying the merge over the
current fixture: 156 split packages to 0, 8,374 components to 7,680.

**Nothing in `ING-36` changes.** Dropping qualifiers and decoding escapes was
right for its own reasons and stays. The namespace is meaningful — it is the
scope in npm and the groupId in Maven — and the tempting option, ignoring it for
OS-package ecosystems, would have hidden a producer bug behind a rule.

**Pending: the fixture still holds the bad data.** Everything measured against
`internal/sbom/testdata/switch-image.cdx.json.xz` — and everything in the demo
database seeded from it — was produced by the generator before the fix, so the
duplicate packages and their doubled findings are still in every number the
interface shows. A fresh SBOM is being built. When it lands:

1. `xz -9` it over `internal/sbom/testdata/switch-image.cdx.json.xz`, which is
   the path `make demo-seed` decompresses and uploads and the path
   `internal/sbom/fullsize_test.go` reads.
2. `make demo-reset` to rebuild the demo database from it, since `urgency` is
   computed at ingest and nothing already stored moves.
3. Re-check the counts quoted in `DECISIONS.md` `ING-36` and `ING-37` — 8,374
   described components naming 7,858 packages, 516 collisions, 30 pedigree
   against 535 qualifier — because they were measured on the old file and the
   first two of them will have moved.

**Measured and left alone: `/v1/running-out` takes about eight seconds.** It is
the query behind "due soon, still undecided", and it is slow for a reason no
index fixes: it groups **every** open finding — 441,108 of them — once per
urgency band, because a deadline window differs per band and the list spans
products so nothing narrows it first. The `NOT EXISTS` against decisions that
looks expensive is not: 0.1 s, well indexed. This is the "one package drowns
everything" problem again, since 96% of those rows are the kernel. Worth
reshaping before anybody relies on the screen; not reshaped here, because it
wants a decision about whether the bands can share one pass.

## Audit: the mockup's screens against what is built

Done at the end of a stretch of interface work, screen by screen, against the
published mockup rather than against the decision text — which is the rule this
document already records, and the reason the first attempt came out wrong.

Every one of the mockup's fifteen screens now exists. Three did not at the
start of this stretch: **people and access**, **release comparison**, and
**who is working on what**. A fourth, the mockup's "adding a release", is now
the declare forms on the catalogue screens rather than a screen of its own.

| Mockup screen | Built as | Standing |
|---|---|---|
| home | `Home.tsx` | All eight panels, in the mockup's order. "Assigned to you" reads as "Being worked on". An earlier version of this audit said the charts came first: that was **wrong**, and wrong in a way worth remembering — it read the order the panels are *defined* in, which for a page assembled from components is not the order they are drawn in |
| findings | `Findings.tsx` | Built, and now goes further than the mockup: filters are the server's, and a by-component view the mockup does not have |
| finding detail | `Finding.tsx` | Deciding and assessing are both on it now. What remains is smaller — see below |
| review queue | `Queue.tsx` | Built |
| release comparison | `Compare.tsx` | Built this stretch. The mockup's release-over-release chart is not: it needs a per-release open count that nothing reports |
| people and access | `People.tsx` | Built this stretch |
| scans | `Scans.tsx` | Built. The mockup's "what the last run was measured against" — scanner and database version — is not shown, though the scan carries it |
| settings | `Settings.tsx` | Every setting the server exposes renders. The mockup's four named groups are one list plus deadlines, so the grouping is missing rather than the function |
| who is working on what | `Work.tsx` | Built this stretch, two of the three tabs. The third, "nobody assigned", is the `Unassigned` screen that already had its own rail entry, linked across rather than duplicated |
| decide several together | `Together.tsx` | Built |
| products / branches / variants | `Products` `Streams` `Variants` | Built, and each can now declare as well as list |
| adding a release | folded into the above | The mockup gave it a screen; it is a form on the screen it belongs to |
| dependencies | `Tree.tsx` | Rebuilt this stretch to the mockup, with search |

**The finding detail is an orientation screen, and the mockup's is a working
screen.** It has six of the mockup's twelve sections: what it is, how bad,
upstream, also known as, where it sits, and the evidence — plus the bumped-and-
came-with-it banner, which the mockup also has. What is missing is everything
about *deciding*:

- ~~The Decide card is not on it~~ — **built.** `Assess` and `Decide` are both
  on the finding now, reusing `ui/Outcome`, `ui/Editor` and `ui/Reach` rather
  than a second form that drifts from the first.
- ~~A decision already made is invisible here~~ — **built.** It reports how
  many of the places have been answered, and links to a claim where one stands.
- **"How the reasoning changed" and "what has been said"** live on
  `Decision.tsx` and are not reachable from the finding.
- **"What this decision covers"** (`REL-06`, `REL-07`, `TRI-29`) is a hint
  sentence rather than the panel the mockup draws.
- **"What we think of the issue itself"** is absent, and correctly so: the
  mockup marks it unbuilt and `DECISIONS.md` §4 still has it open.

That is one structural divergence rather than five omissions: the built
interface splits a finding across three screens where the mockup has one. It is
worth deciding on purpose rather than inheriting.

**Two smaller things the mockup has and nothing reports:** a per-release open
count, for the release-over-release chart; and what a scan run was measured
against, which the scan document carries and the API does not surface.

**Nothing was found that is built and unreachable.** Every screen has a rail
entry or is reached from one, which was not true at the start of this stretch —
people and comparison existed as endpoints with nothing pointing at them.

## The dependency tree, and why its counts are worked out live

Reported three times and fixed three times, each a different defect. Kept
together because the next change to that screen will meet all three.

1. **It never loaded.** `finding` carried no index containing `component_id`,
   so counting what was open against each of the root's 5,270 children scanned
   all 441,108 open findings each time: 637 ms a row, about 56 minutes.
   Migration 00012 and a grouped join fixed it — 1.35 s.
2. **It was a list, not a tree.** Ordering children by findings buries the
   structure, because a container holds none of its own: of 5,270 children only
   96 open at all, and the first sat at position 546. What opens now comes
   first, and branches and leaves truncate separately.
3. **Every container read zero.** The count was what is filed against the
   component itself. It is now what is open in everything under it.

**That last one is worked out per request, not stored, and that was a
correction.** Storing it after a scan was the first attempt and it is the wrong
shape: the number comes from findings, and findings move between scans —
dismissed, assigned, reconsidered — so a stored total is right the moment a
scan ends and drifts afterwards. Live costs two queries and a walk over 19,192
edges, and took the request from 0.16 s to 0.57 s.

**Branches are ordered by name, not by the rollup.** Ordering by it was tried
and made the screen worse: an edge means "contains or depends on", the document
does not distinguish them, and forty kernel-module packages each depending on
the one kernel all reported its 425,098 and filled the first screen — putting
the containers back out of sight, which is defect 2 arriving from the other
side. **If the producer ever distinguishes containment from dependency, this is
the decision to revisit.**

**Still open on that screen:** whether a dismissal should subtract from the
count. Today it does not — the count is every open finding, answered or not —
and that is the behaviour that predates this work rather than a choice made
here.

## The rebuilt SBOM: what it settled, and the one class left

Built 2026-09-01 carrying all four generator fixes, and **in as the fixture**.
Every prediction made from replaying the merge held against the real build:

| | before | after | predicted |
|---|---:|---:|---:|
| duplicate package URLs | 156 | **0** | 0 |
| merged components | 8,374 | **7,693** | 7,693 |
| `upstream=` qualifiers | 535 | **537** | ~535 |
| `publisher` | 587 | **592** | ~587 |
| lockfile deps on the image root | 16 | **0** | 0 |
| lockfile deps attributed to a program | 0 | **85** | ~60 |
| dangling references | — | **0** | — |

`openssh-server` states `1:10.0p1-7+fips` again, and the eight epoch-losing
packages have their epochs back.

**What moved in openpsirt.** The fixture reads as 7,693 components and the
reader now collapses nothing, because there is nothing left to collapse — which
is the stronger statement than the one the test used to make. The findings list
went 7,906 rows to **7,546**, and total open findings to 241,161, both because
516 duplicate components merged upstream and took their split findings with
them. `ING-36` and `ING-37` keep their rules and carry updated evidence: the
duplication they were measured on is fixed at the source, and `ING-37`'s "no
overlap at all" is now 16 overlapping, because the halves carrying each
mechanism became one component carrying both.

### The class that was reported, and what it turned out to be

`golang.org/x/net` sitting under the product with nothing above it is **not**
a Debian package's contents, which is what these notes said first and what I
told Brad. It is the **build container's own Go compiler**: the path is
`usr/share/go-1.19/src/go.sum`, harvested from
`versions/build/log-*/lockfiles.tar.gz`, and **the image ships no golang
package at all**. No shipped scope's harvest contains a single `usr/` path —
checked across all twenty docker and host tarballs.

The two situations were never conflated in the data. The x/net actually linked
into what we ship is tracked separately and correctly: v0.55.0 under
sonic-gnmi, v0.53.0 under sonic-mgmt-framework, v0.49.0 under
docker-sonic-otel, v0.34.0 under sysmgr. Fifteen versions, fifteen origins,
kept apart. What was wrong is that build tooling sat in the same list as image
contents, and having no parent is what made it conspicuous.

Split by harvest path: 294 lockfile components come from `sonic/` source trees
(legitimate — statically linked into shipped Go binaries) and **658 from
`usr/` toolchain trees** (not shipped), carrying **216 findings, 2.9% of the
7,546 rows** on the working list.

**Measured, not assumed** — and this is the part worth keeping:

| where the component sits | valid CycloneDX 1.6 | grype matches |
|---|---|---|
| `components`, no scope | yes | 20 |
| `components`, `scope: excluded` | yes | **20** |
| `formulation` | yes | **0** |

**grype ignores `scope` entirely**, on 0.112.0 (the version the build uses) and
0.118.0. Marking a component excluded is correct for a human and inert for the
scanner, so "mark it" would have changed nothing. `formulation` — the section
CycloneDX 1.5 added for how a thing was built — is what actually works, and it
keeps the data, so a build-chain compromise question stays answerable.

**A correction worth remembering:** the first figure recorded here was 489
findings, from matching component *names*. Names like `golang.org/x/net` are
shared between the toolchain copy and legitimately shipped copies, so that
over-counted by more than double. Matching on name *and version* gives 216,
which is exactly what grype's own count dropped by. **Counting by name where
the same name means two things is a trap this project has now hit twice.**

### The build never validated its own output

Found while checking that the formulation change validated:
`cyclonedx validate --input-version v1_6` **rejects every SBOM the build has
ever produced**. `pedigree.patches[].diff` carries `url` and `text` and nothing
else, and the generator emitted a `hashes` array beside them; 530 components
carry one, so a single unschema'd field invalidated the whole file.

Nothing in the build runs the validator, which is why it was silent — the tools
we happened to use are lenient. Fixed upstream by moving the digest to a
`sonic:patch_sha256` property, with the change-detection signature reading it
from there and still reading the old spelling, so comparing against an older
document does not read as every patch having changed.

**The real fix is that the build now checks**, at both write sites, using the
cyclonedx-cli it already provisions for the SPDX export. A warning by default,
fatal under `SBOM_STRICT=1`.

**And the check needed `--fail-on-errors` to be a check at all.** Without it
cyclonedx-cli prints "BOM is not valid." and **exits 0** — so the obvious
spelling passed on a document the same command had just rejected, and I only
noticed because I ran it against the known-bad file expecting a failure.
Written into the code rather than left to be rediscovered. It is the same shape
as `check-engines` here: a green result that means *nothing failed* rather than
*it was checked*, which is now the third time this has come up.

## Traps found the hard way

**A multi-edit script that stops halfway leaves the rest unapplied, silently.**
Twice today a Python batch failed an assertion partway and never wrote, and the
edits after the failure looked done because the ones before them were. Both
were caught by checking the *data* rather than the diff — one had the triage
line hiding 91,040 findings it should not have, the other had a deadline band
matching a literal word instead of the folded one. Check the result, not the
script's exit.

**Two servers, one port.** `kill $(cat api.pid)` leaves an older process
holding 8080, the new one fails to bind, and every check then reads the *old*
binary while looking like it worked. It cost two wrong conclusions today.
`pkill -f "bin/openpsirt"` before restarting, and confirm the port is free.


Worth keeping until they are covered by a test or a rule.

**The frame is drawn outside the routes it wraps**, so `useParams` in it is
always empty. Nothing failed — the rail and the scope picker simply never
learned what was selected, and every screen below them worked. `web/src/app/
scope.ts` reads the path instead.

**Two test runs against the same three databases corrupt each other** into
failures that look like real defects — duplicate keys, foreign keys, fixtures
half-deleted. The runner at
`scratchpad/fourengine.sh` refuses to start beside another for that reason. A
`pkill -f "go test"` also kills the shell that is running it, which reads as
the command having failed.

**A rule lifted from the mockup by matching selector prefixes silently skipped
`.panels`** — the grid container — while matching every `.panel*` rule inside
it. Everything was styled correctly and stacked in a column. Extraction by
brace-matching, and check the container is there.

**`urgency` is computed at ingest and stored**, so changing the ranking needs a
re-scan before anything moves.

**Every count on screen today came from an SBOM with known duplicates.** Until
the fixture is replaced, 156 packages are counted twice and so are their
findings. A total that looks slightly high is not necessarily a defect in the
query.

**The dev machine has an HTTP proxy configured** (`HTTP_PROXY` to a Squid
cache) which intercepts `curl` to the local hostname and answers 403. Every
local request needs `--noproxy '*'`.

### Running the wrong gate, and calling it green

CI failed on work reported as passing, twice over, because the gate being run
was `make lint` plus a bare `go test` rather than `make check`. Those are not
the same command and the difference is not cosmetic:

- `check` also runs `unreachable`, the OpenAPI drift check and the frontend.
  `Scope.Everything` sat exported with nothing calling it, and only
  `unreachable` says so.
- A bare `go test` drops `-race` and `-p 1`, and it runs SQLite alone.

The SQLite part was the expensive one, and migration 00016 was wrong twice over
in ways only a real server states.

Its table has a foreign key to `person` and was never added to `tables` in
`internal/dbtest`, so the cleanup deleted people out from under it. SQLite let
that pass. PostgreSQL, MySQL and MariaDB all refused, and **40 tests failed**
the first time they were pointed at a real server. The comment directly beneath
that list already warns that a new table with a foreign key silently breaks the
cleanup of every package predating it; it was right, and it was not read.

Its rollback then dropped the table's indexes before the table. Every other
migration here just drops the table and lets the indexes go with it; this one
did not, and MySQL and MariaDB refuse to drop an index a foreign key needs to
enforce itself — `assessment_issue_idx` leads with `vulnerability_id`, which is
constrained. PostgreSQL and SQLite allowed it, so the rollback test was green
on half the engines and had never run on the other half.

Fixing that then exposed the same rule in 00013, which the failure at 16 had
been hiding: the rollback walks down from the top and stops at the first
migration that refuses, so one broken "down" masks every one beneath it.
`finding_due_idx` leads with `closed_run_id` and is the only index on `finding`
that does, so InnoDB adopted it to enforce `finding_closed_run_id_fk` and then
would not let it go. InnoDB *picks* an index for a constraint rather than
owning one, so an index can acquire that duty simply by being created. That
rollback now drops the constraint, drops the index, and puts the constraint
back.

All three are the same shape: **SQLite is permissive about foreign keys, so it
cannot be the engine a schema change is judged on.**

Now mechanical rather than remembered: `make check-engines` runs the same greps
CI does, `make check` says out loud when it has tested one engine of four, and
the URLs live in a git-ignored `local.mk` so the machine is set up once. The
below-floor server CI uses is a local container too (`g-pg13`), so the refusal
is exercised rather than skipped. AGENTS.md carries all of this.

**Why greps rather than trusting the suite:** a skipped engine passes. Green
means nothing failed, which is also what running almost nothing looks like.

## Not yet true

`DESIGN-interface.md` is now behind the code rather than ahead of it. It still
describes release comparison and people as unbuilt, and knows nothing about the
tree, the server-side filters, the by-component view, "who is working on what",
or declaring from the catalogue screens. Bring it up to date from the audit
above before trusting anything it says about what exists.
