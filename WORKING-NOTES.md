# Working notes

Where the interface work is, what is decided, and what is still open. **This
document is temporary**, like `IMPLEMENTATION.md`: it holds the state of one
stretch of work so it survives a break. Anything durable belongs in
`DECISIONS.md` or a `DESIGN-*.md` and should be moved there rather than left
here.

## Where to pick up

**Everything decided so far is built.** The state of it, in the order it would
be picked up:

| | |
|---|---|
| **Done** | **Performance and the development loop** — measured, done and re-measured, in the section of that name below. The two figures to beat were the findings list at 2 s a page and the test suite at over half an hour: the quick loop went from 110 s to 4.4 s, the four-engine gate from 551 s to 174 s, the findings page to two statements with the groups read off an index, and the graph stays in the database as three recursive statements. What is still open there is the `make measure` re-run, further down this table |
| **Done** | The mellanox build of the switch image is kept as a fixture and seeded as a second variant of the same product. It is the first real cross-variant data: a decision made on broadcom reaches it by lookup where chains match, and the review walks it where versions differ, judged by the version the decision is keyed on |
| **Done** | The demo and the dev loop seed a second product: OpenPSIRT itself, from the inventory the image carries at `/usr/share/openpsirt/openpsirt.cdx.json`. Two products is what makes the cross-product screens and the scope picker's "all" mean anything |
| **Done** | The first day of use against the demo, 2026-09-03: the scope picker stays open until the variant is picked; the queue card and the decision screen carry the finding's context (`TRI-09`) and link to the finding; findings rows carry a server-side decision state; the tree's counts are distinct issues per path and its node pane links to the findings at, and beneath, a node (`beneath=` filter); arriving from a finding opens the chain; a menu opens the rail on narrow screens; the trend is the mockup's two-band chart; three class names that collided with Tailwind utilities were renamed; "issues" and "findings" are the two words for the two units, and home counts issues |
| **Done** | What the 2026-09-02 workflow review decided — see the section below. `TRI-45` to `TRI-47` in the backend (a claim table, the queue at claim grain, approval and rejection by claim with set-aside, extension), and `UIX-43` to `UIX-51` in the interface, rebuilt from the restyled mockup and audited against it screen by screen with a headless browser, as two identities. Approving a 2,000-row claim is 11 statements and 55 ms; it was one statement per row and 15.6 s for 1,760 on the demo before that was measured. `make check` and `make check-engines` were green at the commit, generated files included. **A development database against the four servers has to be recreated** (`make engines-down && make engines-up`), and so does the demo's (`make demo-reset`): the triage migration was edited in place |
| **Done** | The rebuilt SBOM is in as the fixture, the demo is reseeded from it, and `ING-36`/`ING-37` carry their new numbers |
| **Built** | `ING-41`, end to end: the `upstream.currency` setting (off by default), the pass that fills the columns, a third worker beside the reader and the runner, and the finding screen showing both what upstream has released and why there is no fix |
| **Done** | `DESIGN-interface.md` is level with the code again: the picker and what a partial scope refuses, home's order, the upstream and age columns, the decided count, the currency panel, the ambiguous-name chooser, and the finding-split divergence recorded as a divergence |
| **Done** | `make engines-up` stands the four servers up and writes `local.mk`, so a fresh machine is one command rather than a document followed by hand |
| **Done** | `make demo` is the image behind a proxy and needs only docker; `make dev` is the old native loop. The image builds the interface itself, which it was not doing at all |
| **Done** | The fixture before that one was the 2026-09-02 16:13 build: valid CycloneDX 1.6, toolchain in `formulation`, 219 fewer findings rows measured against the same scanner and database |
| **Done** | The in-app notification area, both lifetimes, two event producers and the sweep that derives conditions. Mail and chat are not built; see `DESIGN-notifications.md` for what is told and what is not |
| **Done** | An adversarial review over everything since the last one, and every finding worked through — see below. One was refused, and the gate then caught a regression in one of the fixes |
| **Done** | **`make measure` re-run**, and `DESIGN-findings.md` carries figures again. The withdrawal was right rather than merely cautious: halving the churn halved MariaDB and cut PostgreSQL by a third, and moved MySQL by four percent — the error did not scale the four engines alike, so no arithmetic on the old numbers would have recovered these. The reads hold up and trend is the one that grows, both confirmed rather than discovered. One new thing to chase, recorded in `DECISIONS.md` §4: **MySQL's write cost barely responds to how much changed**, which is a cost paid per statement rather than per row |
| **Done** | The fixture is the 2026-09-02 21:21 build, which is the first one to have *run* the producer fixes rather than have them replayed over an old document. The image root's direct children went 5,198 → 30, unreached components 237 → 39, and 190 lockfile entries under unbuilt source trees moved to `formulation`. `packages` was the only asserted constant that moved; the upstream and CPE counts held because none of the 190 stated either, which was checked against the old fixture rather than assumed |

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


The interface is built out and running. `make demo` brings it up — the image,
behind a proxy that signs you in, needing nothing installed but docker — and
`DESIGN-interface.md` under "Running it locally" says what it settles and how
`make dev` differs.

**Every screen the mockup has now exists.** People and access, release
comparison and "who is working on what" were built in this stretch, and the
catalog screens can declare as well as list. What is left is one structural
divergence and two missing numbers — see "Audit: the mockup's screens against
what is built" below, which is the place to start rather than
`DESIGN-interface.md`.

**Researched: what a slow query does to the rest of the process.** Measured
against the real database with the driver this build uses, so two of the three
questions are answered and the third is narrowed.

*Cancellation works.* `modernc.org/sqlite` honors the context: a query
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

## Performance, measured, and the plan to fix it

Two problems that look like one: the development loop is slow, and some of
the application's own work is slow. Everything here was measured on
2026-09-03; the numbers are what to check against afterwards. The bet is that
fixing the second helps production as much as it helps the loop.

### The development loop

| Measured | What it says |
|---|---|
| The full gate ran six times in one day, where the rule is once before a push | Cadence, not code. The gate belongs before a push; while iterating, the package that changed, on SQLite, is the loop. Now written into `AGENTS.md` |
| SQLite only, no race detector, packages in parallel: the API package alone takes **120 s** for 98 tests, findings 78 s, triage 76 s, access 65 s | About a second per test of pure setup: each SQLite test creates a fresh database file and runs all eighteen migrations before it does anything |
| The full four-engine run, `-race -count=1 -p 1`: **551 s wall** (9.2 minutes) on this machine. Per package: API 152 s, findings 84 s, triage 69 s, access 57 s, currency 40 s, ingest 34 s, schema 15 s, catalog 14 s, graph 14 s, scanner 11 s. Longer runs seen the same day overlapped a docker build or another suite against the same servers | Beside the SQLite-only numbers, the three server engines add little per package — they share one migrated database and only empty its tables between tests — so the time is the per-test SQLite migration and `-p 1` running the 23 packages one at a time. The race detector and `-count=1` add on top |
| About 200 of 480 tests run as four subtests, one per engine | The store-level ones earn it — every portability bug so far was a query behaving differently on one engine (`DAT-33` to `DAT-36`), one of them reached only through a handler. Of the 98 tests in the API package, over half prove routing, authorization mapping and JSON shapes that do not vary by engine, over store queries the store tests already run on all four |

**Plan, in order — done 2026-09-03, numbers below:**

1. **`make test` becomes the quick loop**: SQLite only, packages in parallel,
   the cache on. The four-engine run is `make test-all`, which `make check`
   runs once before a push, and CI keeps all four, so `DAT-12` holds.
   Nothing proved changes; when it is proved does.
2. **Build the schema once per package.** One SQLite file per test binary,
   migrated on first use and written out per test; on the three servers a
   database per binary, named for the package, dropped and recreated on
   first use. `-p 1` is gone.
3. **The API package's handler tests run on SQLite and PostgreSQL only**
   through `dbtest.Two`; the rule is written at it. Decided by reading the
   tests. Of the 99 test functions in the package, 45 pin routing,
   authorization or a response's shape and run on two; 42 pin what a query
   returns, hides, conflicts on or spells through the handler and run on
   four; 12 open no database at all. Nine had been moved to two and were
   moved back when a review read them as pinning a query — the rule is
   `DAT-37` now, not a comment.
4. Re-measured.

**What it was, and what it is:**

| | Before | After |
|---|---|---|
| The quick loop, SQLite only, packages in parallel, cache off | 110 s | **4.4 s** (`make test`, cached, is under 2 s when nothing changed) |
| The API package on SQLite alone | 77 s | 1.3 s |
| Four engines, race detector, cache off | 551 s (`-p 1`) | **174 s** (packages in parallel) |
| `make check-engines` | 27 s | 23 s |

The surprise was where the SQLite time actually went. Migrating per test was
about a second, as measured — but with that gone the API package still took
37 s on SQLite, and its whole run was I/O wait: SQLite as we opened it synced
the file twice at every commit, a test is nothing but small commits, and
`Reset` alone was thirty of them. On a memory-backed directory the same
package took about one second, which named the cost. The fix is not the
directory — a container's `/dev/shm` is often 64 MB — but the syncing: a test
database is thrown away at the end of the test, so `dbtest` opens it with
`synchronous(OFF)` on the ordinary temporary directory, which measures the
same as shared memory did (the SQLite-only suite, cache off, 2.8 to 4.4 s
across runs; the machine was scanning at the time). `Reset` empties the
tables in one transaction, and the migrated template is kept as bytes in the
process rather than as a file, because a test binary has no hook after its
last test and a file would outlive it.

The same finding reached the application: it opened SQLite with only
`busy_timeout` and `foreign_keys`, so the demo ran in the default
rollback-journal mode with `synchronous=FULL`, the slowest write path SQLite
has, and a scan applies 240,000 rows through it. It now opens with
`journal_mode=WAL` and `synchronous=NORMAL` (DESIGN-database.md says why),
and a URL may add `_pragma` entries of its own after the defaults.

Measured on the demo, `make demo-reset && make demo`, from the upload's
timestamp in the proxy log to the scanner's "scanned a target" line, which
includes the scanner's own run over the cached vulnerability database:

| | Rollback journal, FULL | WAL, NORMAL |
|---|---|---|
| The switch image read in (6,845 components, 18,561 edges) | 4.6 s | 4.3 s |
| The switch image scanned and 241,479 findings applied | 41.5 s | 39.5 s |
| The second image scanned and 31,060 findings applied | 17.6 s | 17.7 s |

No change worth the name, and the reason is instructive: an apply is one
transaction of 500-row batches, so it commits a handful of times however many
rows it carries, and the syncing that dominated the tests — thousands of tiny
commits — is not what a scan does. The setting stays because it is the right
one: a reader no longer waits on a writer, and a commit appends rather than
rewriting pages. What the scan's time actually is — the scanner's run, then
the Go work of matching and the inserts — is the application plan's business.

The four-engine run is now bounded by the servers: twenty binaries hit three
servers at once, and MySQL takes about seven seconds to drop, create and
migrate a database where PostgreSQL takes one. What remains per package is
mostly waiting on them. `make check` end to end is dominated by the web build
and the tool checks rather than the tests.

One thing to know: two runs of the *same* package against the same server at
once share a database and will collide, because the name is stable on purpose
— a per-run name would leave a database behind every run, since nothing runs
after a binary's last test to drop it. The next run drops and recreates it.

### The application

| Measured on the demo's SQLite, 7,292-row build, 240,945 open finding rows | |
|---|---|
| Findings list, one page: **2.0 s warm, 5.6 s cold**; `limit=1` also 2.0 s | The cost is the grouping, not the page: every page groups all 240,945 rows by issue and component to find the fifty most urgent, then groups them all again to count. Both walk `finding_urgency_idx`, which stops at urgency, and then build temporary B-trees for the group and the order: 0.33 s and 0.32 s in raw SQL. The joins for likelihood and score, and the five correlated decision lookups per row for the row's state, add about 0.1 s; the rest is Go — the chain walk and the names pass |
| The same grouping with a covering index over `(target_id, closed_run_id, visibility, vulnerability_id, component_id, urgency)`: **0.02 s**; the total **0.01 s** | An index-only scan on every engine. The composite is the portable shape: PostgreSQL could combine single-column indexes with a bitmap scan, MySQL and MariaDB mostly will not |
| By-component view 0.94 s, unassigned 0.93 s, the kernel's issue list 2.0 s, trend 0.8 s | Same family — grouping over the whole build per request. Tree root 0.54 s, a finding 0.9 s, the queue 0.24 s, running-out 2 ms |
| Approving a claim: **15.6 s for 1,760 rows** as a loop with a count per row; **55 ms for 2,000 rows** as 11 statements, since fixed | The shape to look for elsewhere: one statement per row where a set would do |
| The chain walk reads **every edge of the build, ~18,500 rows**, into memory and walks them in Go — once per findings page for the two ends of each row's path, once per finding for its chains, and for the tree's cumulative counts. The subtree filter computes descendants in Go and binds them back as up to 6,845 ids | Work that should stay in the database. The walk itself was measured at 3 to 18 ms, which is true, and it is a couple of megabytes allocated per request on the screen opened most, and it scales with the graph rather than with the page |
| The review queue reads the deferred-so-far total with one query per entry | Small today; the same one-per-row shape |
| Bulk inserts already go in as multi-row `INSERT ... VALUES` of 500 rows, about 5,000 placeholders a statement, and identifier lists on deletes are bounded the same way | Nothing to do here. The batch size was chosen for the statement-size cap on MySQL and MariaDB, well inside every engine's placeholder cap |

**Plan, in order, each step measured before and after:**

1. **The covering index** on `finding`, in the finding migration (DAT-29:
   edit the migration; recreate development and demo databases).
2. **Split the findings page in two.** First the grouping over `finding`
   alone — issue, component, count, max urgency — ordered and limited, off
   the covering index. Then decorate only the fifty groups on the page:
   the joins for likelihood, score and names, the decision-state counts,
   the consumer count and the fix data. The correlated lookups run fifty
   times instead of 240,945. Count the total with the same index-only
   grouping. Same shape for the by-component view, the unassigned list and
   the issue list at a component.
3. **Keep the graph in the database.** `WITH RECURSIVE` works on all four
   engines (Section 6 of the decisions): the two ends of a chain for a page
   of rows, a finding's chains, the members of a subtree for `beneath`, and
   the count beneath a node each become one statement, and the in-memory
   edge pass goes.
4. **One query per page, not per row**, wherever the queue and the
   finding's `standing` still do the latter.
5. **The same numbers on PostgreSQL**, before and after, since the demo is
   SQLite and production is not. `make measure` is where a year of nightly
   scans is modeled; the per-screen timings here are the other half and
   want a target of their own so they are re-run rather than remembered.

**What "fast enough" means, so this is done at some point:** a findings page
under 100 ms on SQLite with the full-size build, the full four-engine suite
under ten minutes, and the quick loop under two.

**Measured as each step lands.** End to end against the demo through its
proxy, best of three warm requests, taken while another test suite had the
machine (load 2 to 3, so the absolute numbers are pessimistic and the
comparison is what to read). The baseline column is the demo before its
database was recreated for the index: the broadcom build held 240,945 open
rows in 7,292 groups and a few claims had been made. Every later column is the
recreated demo — 241,479 open rows in 7,329 groups for the same build, the
mellanox build beside it, and no claims until step 3's measurement of the
queue, which made forty deferral claims (32 landed; the rest hit a place
already claimed) and kept them from there on — which is why "one finding"
and the queue read slower in that column than in the one before it: they
were reading claims for the first time.

| Request | Before | 1. Index | 2. Split | 3. Graph | 4. Per page |
|---|---|---|---|---|---|
| Findings page, warm | 2.02 s | 1.06 s | 0.34 s | 0.32 s | **0.13 s** |
| Findings page, first request after start | 2.04 s | 1.08 s | 0.33 s | 0.48 s | 0.14 s |
| Page two | 2.05 s | 1.09 s | 0.34 s | 0.32 s | 0.12 s |
| Findings, `exploited` | 2.01 s | 1.51 s | 0.24 s | 0.22 s | 0.10 s |
| Findings, `search=ssl` | 2.01 s | 1.08 s | 0.34 s | 0.32 s | 0.13 s |
| Findings, `state=undecided` | 2.33 s | 1.60 s | 0.38 s | 0.36 s | 0.20 s |
| Findings, `beneath` the root | | | | 0.58 s | 0.23 s |
| By component | 0.81 s | 0.79 s | 0.26 s | 0.26 s | 0.17 s |
| The kernel's issue list | 1.08 s | 0.68 s | 0.15 s | 0.15 s | 0.10 s |
| One finding, with claims standing | 0.48 s | 0.02 s | 0.02 s | 0.21 s | 0.01 s |
| Unassigned | 0.18 s | 1.07 s | 0.40 s | 0.39 s | 0.24 s |
| Tree root | 0.53 s | 0.81 s | 0.82 s | 0.41 s | 0.42 s |
| Around the kernel | 0.28 s | 0.94 s | 0.95 s | 0.43 s | 0.43 s |
| Review queue, 32 claims | 0.28 s | 0.00 s | 0.00 s | 1.27 s | **0.06 s** |

*Step 1, the index.* `finding_group_idx (target_id, closed_run_id,
visibility, vulnerability_id, component_id, urgency)` replaces
`finding_urgency_idx`, which had the same prefix and covered nothing the
grouping read. On a copy of the demo's database the page's grouping went
from 0.33 s to 0.048 s and the count from 0.32 s to 0.038 s; `EXPLAIN QUERY
PLAN` reads `SEARCH f USING COVERING INDEX finding_group_idx (target_id=? AND
closed_run_id=? AND visibility=?)`, so the table is not touched. A temporary
B-tree for the GROUP BY remains, because `visibility IN (two values)` is two
ranges of the index and the groups arrive out of order across them; it is
over the 7,329 groups rather than the rows and costs nothing worth removing.
End to end the page halved rather than vanished, because the joins, the five
correlated decision lookups per row and the exploited flag were still read
per row — that is step 2. The `exploited` and `state` filters halved less
for the same reason: both read a column the index does not hold.

The three requests that read *slower* in the index column are not the index.
The same statements on the old and the recreated database copies take the
same plan and the same time (the unassigned grouping 0.11 s against 0.13 s;
the tree's edge and pairs reads 6 ms and 52 ms on both), and the rest of the
difference is the other suite running: those three are the ones that walk
the graph in Go, which is what a loaded machine slows most. Step 3 takes
them off the machine's memory and onto the database anyway.

*Step 2, the split.* The findings page is two statements: the groups, from
the covering index alone — issue, component, places, worst urgency, filtered,
ordered and limited — and then what the page shows about those fifty and no
others. The joins that were under the grouping are gone from every
narrowing: a severity floor, a component name, a search, an ecosystem are
each a membership test against the table that holds the answer, and whether
anything is exploited is read off the urgency, whose top band is exactly
that. The decision-state filter was the largest remaining cost — a
correlated lookup per open row, 241,479 probes to say which groups had no
decision on a build with none — and is now built from the decision table
outward, one row per decided finding, joined to the grouping by identifier:
1.4 s to 0.06 s in raw SQL. The same shape for the by-component view (its
joins are gone), the issue list at a component (the kernel holds 222,435 of
the build's open rows, and grouping them with three joins cost 0.35 s where
the index alone answers in 0.04 s) and the unassigned list (four joins and
five `MIN(name)` reductions under the grouping, now a names pass over the
page). Statements per findings page, counted by the test's hook: 8 before
(which product, page, count, two names passes, and three for the chain
ends), 9 after — the page is two statements. What is
left of the 0.34 s is mostly step 3's: the chain ends still read every edge
of the build into memory, and the tree and the unassigned list pay the
loaded machine as before.

*Step 3, the graph.* The three walks are `WITH RECURSIVE` statements, bound
at sixty-four steps: upward from the components on a page for the way down
to each (2 ms for a page, fifteen rows back, where the edge read was 18,561
rows into Go); downward from a component for the set beneath it, which the
findings list now narrows by as a subquery rather than a bound list of up to
6,845 identifiers; and downward from a row of the tree's children for the
distinct issues beneath each, one statement for the row (0.08 s for the
root's thirty children in raw SQL, against two statements and a walk).
`Subtree`, `edges` and the in-memory walkers are gone. Two things found on
the way, both in the tree:

- **SQLite's planner put the recursion's queue on the inside of the
  downward join** and scanned every edge in the build once per queued row:
  6.6 s to list what sits under the root, 9 s for the tree's first screen.
  `CROSS JOIN ... WHERE` in place of `JOIN ... ON` is an inner join on every
  engine and, on SQLite, the instruction to keep the queue outside; the same
  two statements take 0.018 s and 0.08 s. Written down in the code and in
  the data-model design, because the spelling looks like a whim.
- **The covering index from step 1 created a trap in the tree's per-child
  count.** The correlated `COUNT(DISTINCT vulnerability_id)` per child could
  take the index on (target, component, open) or the new one on (target,
  open, visibility), and SQLite without statistics took the second, which
  matches every open row in the build, once per child: 0.30 s for the
  root's thirty. It is now one grouped pass over the build joined in, like
  the child count beside it, 0.09 s. And the count beneath is asked once
  per request rather than once for the children and again for the root.

The findings page did not move — the edge walk was never its cost; the
per-statement profile puts the page at 100 ms for the groups, 76 ms for the
count and 150 ms for the decoration, of which the last two are step 4's.
`beneath` under the root costs 0.58 s because each of the page's statements
tests 241,479 rows against the subtree; the shape that would fix it is an
index led by the component, which is not in this plan. The tree's first
screen went from 0.82 s to 0.41 s and the neighbours of the kernel from
0.95 s to 0.43 s; what is left of each is one 150 ms statement for the
children and one 200 ms recursion for what is beneath them, on the loaded
machine.

*Step 4, one statement per page.* Four things, found with a query hook
that times every statement a store call makes (the profile is not
committed; it is a test that skips without a `PROFILE_DB`):

- **The total rides on the page.** Every list counted its total as a
  second statement making the same grouping over the same rows. It is now
  `COUNT(*) OVER ()` on the grouping statement — the groups the filter
  admits, counted after HAVING and before the limit, on all four engines —
  and the count statement runs only when a page comes back empty. The
  findings list, the by-component view, the issue list at a component and
  the unassigned list.
- **The decoration by two lists, not fifty pairs.** Reading the page's
  fifty groups as `(issue = ? AND component = ?) OR ...` made SQLite walk
  every open row in the build against the fifty: 150 ms, half the page.
  `issue IN (...) AND component IN (...)` is fifty index seeks, 2 ms, and
  the groups it admits beyond the page's are read and dropped.
- **The queue's per-entry statements.** `DeferredSoFar` per entry and the
  three statements per bulk claim's outliers are one statement for the
  page and three for all its bulk claims. Statements per queue page of 32:
  5 + 32 before, 8 after. That was not the queue's cost, though.
- **The queue's cost was a join SQLite planned from the wrong end.** Every
  statement that reads the findings a decision is about joins `decision`
  to `finding` on the issue and the place. Planning without statistics,
  SQLite takes "open" — `closed_run_id IS NULL` on an index — as an
  equality matching ten rows, so it started from every open finding in the
  deployment and probed the decisions once per row: 0.96 s to describe a
  page of thirty-two decisions, 0.2 s to name the builds they cover, the
  same 0.2 s on a finding with a claim standing. Two changes: the decision
  is on the outside of those joins by construction (`CROSS JOIN ... WHERE`,
  the same standard-SQL spelling the graph walk uses, an inner join on
  every engine), and `finding_place_idx` carries the issue beside the
  place, because one place in this image carries every one of the kernel's
  4,900 issues and a lookup by place alone read all of them to find the one
  the decision meant. Describe: 0.96 s to 35 ms; the queue's store call
  216 ms to 22 ms; a finding's standing claims 195 ms to 1.6 ms. The
  migration is edited in place again (DAT-29), so the demo was recreated
  a second time.

Statements per findings page, by the test's hook: 9 after step 2, 7 now
(which product, groups with their total, what is shown about them, two
names passes, the build's product and one climb). The findings page is
under the 100 ms the plan set as done in raw statements — 102 ms for the
grouping in-process on this machine, of which the SQLite driver is a
known half: the same statement is 48 ms in C — and 0.13 s end to end
through the proxy with another suite loading the machine.

**What the profile says is left, and the shape of the fix**, none of it
in this plan:

- The tree's first screen is 0.42 s: one 150 ms statement for the
  children (a grouped pass over 18,561 edges for the child count, and one
  over the build's open findings for the per-child count) and one 200 ms
  recursion for what is beneath the root's thirty children, whose
  subtrees are together most of the build. A stored per-node subtree
  size is not the answer (the count is over findings, which move); a
  narrower one is to ask for what is beneath a child only when the row is
  opened.
- `beneath` under the root is 0.23 s because each of the page's statements
  tests every open row against the subtree; an index led by the component
  would let the subtree drive the scan.
- **SQLite plans without statistics unless somebody runs `ANALYZE`**, and
  three of the four traps found in this work were that: an equality on a
  wide index taken for ten rows. `ANALYZE` after an ingest, on SQLite
  only, is one statement and engine-conditional (the migrations already
  branch on the engine for `DROP INDEX`); it was not added here because
  DAT-02 says core carries no engine-specific SQL and that is a decision to
  take rather than to slip in. Without it, every new join against
  `finding` on SQLite needs to be checked with `EXPLAIN QUERY PLAN` on a
  full-size database, which is what this work did.

*Step 5, the same numbers on PostgreSQL.* The image from before this work
(3689ed2) and the image after step 4, each stood up against its own
database on the PostgreSQL 16 server `make engines-up` runs, the same two
switch builds sent to each and scanned there, the same forty deferral
claims made, the same requests timed the same way. Both under the same
load (4 to 7), so read the comparison. The SQLite columns beside them are
the demo's before and after from the table above.

| Request | PostgreSQL before | PostgreSQL after | SQLite before | SQLite after |
|---|---|---|---|---|
| Findings page, warm | 1.07 s | **0.18 s** | 2.02 s | **0.13 s** |
| Page two | 1.08 s | 0.19 s | 2.05 s | 0.12 s |
| Findings, `exploited` | 1.08 s | 0.17 s | 2.01 s | 0.10 s |
| Findings, `search=ssl` | 1.12 s | 0.18 s | 2.01 s | 0.13 s |
| Findings, `state=undecided` | 1.36 s | 0.14 s | 2.33 s | 0.20 s |
| Findings, `beneath` the root | 1.14 s | 0.12 s | | 0.23 s |
| By component | 0.21 s | 0.07 s | 0.81 s | 0.17 s |
| The kernel's issue list | 0.22 s | 0.03 s | 1.08 s | 0.10 s |
| One finding, with claims standing | 0.04 s | 0.02 s | 0.48 s | 0.01 s |
| Unassigned | 0.25 s | 0.09 s | 0.18 s | 0.24 s |
| Tree root | 0.14 s | 0.15 s | 0.53 s | 0.42 s |
| Around the kernel | 0.09 s | 0.19 s | 0.28 s | 0.43 s |
| Review queue, 32 claims | 0.83 s | **0.03 s** | 0.28 s | **0.06 s** |

PostgreSQL was never as far behind as SQLite, because it plans with
statistics and its old plans were merely doing too much work rather than
the wrong work; the findings page is six times faster there and the queue
thirty times, for the same reasons as on SQLite. Two rows read the other
way and are worth saying plainly. **The neighbours of the kernel are
slower on PostgreSQL than before** (0.09 s to 0.19 s): the kernel has
forty-odd parents, each row carries what is beneath it, and the recursive
statement walks forty subtrees that mostly contain the kernel's own,
where the old code walked an in-memory map it had already paid to read.
`EXPLAIN ANALYZE` puts the recursion at 86 ms for the root's thirty
children; on SQLite the same screen halved because reading 18,561 edges
through its driver was the larger cost there. The tree's first screen is
a wash on PostgreSQL for the same reason. Judged within margin and left
alone (2026-09-03); the fix, should it ever matter, is the one named
above: ask what is beneath a row when it is opened. And the
unassigned list is the one screen where SQLite reads slower after than
before; the before number was on the earlier database with a different
set of open rows and no claims, so it is the after column that is the
measurement, and its statement is a grouping over every open row in the
deployment with two joins under it, which is the shape step 1 removed
from the findings list and could remove here with the same index led by
`assigned_to`.

## Decided on 2026-09-02, from the workflow review

The mockup was restyled and then reviewed as a workflow against thousands of
findings and few people. Each of these is recorded in `DECISIONS.md`; they are
listed here so the build order is visible while the work is fresh. The
restyled mockup is `https://claude.ai/code/artifact/c4a16431-f5ba-4148-bba6-7d4592777b64`
and is the reference for everything visual; the original mockup is kept as the
record of the approved workflow and is not edited.

| | |
|---|---|
| **`TRI-45`** | A claim is one proposer's action and the unit the approver works at. Confirmed in code before deciding: `internal/triage/queue.go` is one row per decision with no grouping, and `approve` takes one decision id |
| **`TRI-46`** | A bulk claim's card shows its outliers, and an approver may set them aside — approve the rest, return those as their own claim with the table as the reason |
| **`TRI-47`** | An approved claim may be extended to a new issue at the same component, consumer and justification; recorded as derived from it, queued as an extension |
| **`UIX-43`** | Triage mode on the findings list: decide inline, keys move and submit-and-advance. Amends the reading of `UIX-36` — width was the cause, not page-ness |
| **`UIX-44`** | Location scope: all by default, exclusions grouped by consumer |
| **`UIX-45`** | Where a decision applies beyond this build is a guided review on submit — matching builds named, differing versions walked one at a time, past-fix builds shown and not offered, a confirmation the approver also sees. The old checkbox list sat after the comments |
| **`UIX-46`** | The finding is the working screen after a decision: standing decision, activity, revisions, comments, previous decisions at this location. Settles the one-screen-versus-three question |
| **`UIX-47`** | Conventional labels everywhere. Reject, Trend, Assignments, Unassigned, Justification, Path, EPSS, Locations, Users and roles, Lapsed decisions, Submit |
| **`UIX-48`** | Adding is a header action and a floating action opening a drawer, not a form above the table |
| **`UIX-49`** | Upload an inventory from the interface — top bar everywhere, and the Inventories screen, which is what Scans is now called. Exactly the endpoint's two parts: one CycloneDX 1.x inventory, any number of OpenVEX suppressions. No producer report (ING-28's secondary path is not built), no SPDX (intended, not built) |
| **`UIX-50`** | The restyled mockup is the visual reference; its three looks are token sets, Dojo default, chosen per person in the browser; fonts bundled |
| **`UIX-51`** | Home leads with four figures that follow the scope |

**Proposed in the same review and left open**, recorded in `DECISIONS.md`
Section 4: a claim scoped to a consumer subtree (the third bulk axis), and
ownership by subtree rather than assignment per finding.

**A test gap noted and not closed:** the CycloneDX reader accepts any 1.x and
refuses 2.x by name, and every fixture is 1.6 — a 1.7 fixture through the
existing reader tests would turn "accepted by construction" into something
checked.

**Build order for the above.** The backend half of `TRI-45` to `TRI-47` first,
because it is a schema change and free before release (`DAT-29`); the
interface can be rebuilt against the current API for everything else and take
the queue last.

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

## The rebuilt SBOM with the toolchain split out, and what it measured

Built 2026-09-02 and **in as the fixture**. It carries the two fixes made after
the last one: build tooling moved to `formulation`, and the patch digest moved
off `pedigree.patches[].diff.hashes` to a property.

| | previous fixture | this one |
|---|---:|---:|
| components | 7,693 | **7,035** |
| duplicate package URLs | 0 | **0** |
| patch diffs carrying `hashes` | 530 | **0** |
| `sonic:patch_sha256` properties | 0 | **530** |
| formulation components | 0 | **667** |
| dangling references | 0 | **0** |
| validates against CycloneDX 1.6 | **no** | **yes** |

The validator is the one the build itself now runs, cached under
`sonic-buildimage/target/sbom-tools/`. It was pointed at both documents: the
new one passes and **the previous fixture fails**, on exactly the field the fix
moved. A validator that had passed both would have proved nothing.

**Nothing shipped went with the toolchain.** Checked set-wise rather than by
count: 658 moved to `formulation`, and the 34 that appear to have vanished are
the same OCI metadata components with a new build hash in the version. The
sharpest case is `golang.org/x/net`, which was misdiagnosed once before — 12
shipped copies stayed in `components`, 3 toolchain copies moved, and the
fifteen origins are still fifteen.

**What it changed here, isolated.** The obvious comparison is against the
numbers recorded for the previous fixture, and it is wrong: the vulnerability
database has moved since, so the delta would mix two causes. Scanning the
previous fixture again today, with the same scanner and the same database, is
what separates them — which the demo now makes cheap enough to bother with:

| | previous fixture | this one |
|---|---:|---:|
| findings list rows | 7,593 | **7,374** |
| finding places | 241,240 | **241,021** |

**219 rows and 219 places**, against the 216 predicted from matching name and
version. Each toolchain finding sat at exactly one place, which is what a
lockfile component with a single consumer looks like. Of the apparent
difference against the recorded 7,546, **47 rows were the vulnerability
database moving** rather than anything about the document.

## What the demo turned out to be

`make demo` ran the binary on this machine plus the interface's dev server, and
needed Go, node and grype installed here. Two things were wrong with that, and
neither was a trade-off:

**Its documented failure mode was the thing it exists to show.** Without grype
on the path the triage screens stay empty — and the design document called that
"the honest outcome", which is a limitation written up as a virtue.

**It never ran the artifact that ships.** Serving the interface from the dev
server means it did not exercise the embed (UIX-37) at all.

It is now the image, behind a small proxy that adds the sign-in header — the
shape a deployment actually has — and the only thing it needs is docker. The
old loop is `make dev`, which is what it always was.

**The image did not contain the interface, and CI was testing that image.** The
Go build embeds a git-ignored directory and CI builds from a clean checkout
with no `make web` first, so the image it built and asserted things about had
no interface in it. Nothing said so, because an API-only binary is a supported
build. The image builds the interface itself now, and both `check-packaging`
and CI check that it serves one — verified by building an image without it and
watching the check refuse: the page's own path answers 401 rather than a page.

**State lives in a git-ignored `.demo/` in the tree**, not under `$HOME`. The
scanner's vulnerability database is 2.0 GB, is fetched once, and `demo-reset`
keeps it deliberately: it is the slow part and not what anybody is resetting.
The host's proxy variables are passed into the container, without which grype
cannot fetch that database on a network like this one.

**Done, twice over.** The fixture was replaced when the merge fix landed and
again when the toolchain moved to `formulation`; both are recorded above. The
steps that replacement needs, kept because the next one will need them too:
`xz -9` over `internal/sbom/testdata/switch-image.cdx.json.xz`, `make
demo-reset` and re-seed, re-check the counts quoted in `DECISIONS.md` `ING-36`
and `ING-37` and in `internal/sbom/testdata/README.md`, and update the measured
constants in `internal/sbom/fullsize_test.go` — checking *why* each one moved or
did not, rather than pasting whatever the test reports.

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
the declare forms on the catalog screens rather than a screen of its own.

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

**Two smaller things the mockup had and nothing reported** — a per-release open
count for the release-over-release chart, and what a scan run was measured
against — are both built now, and both are on their screens.

**No screen is built and unreachable.** Every one has a rail entry or is
reached from one, which was not true at the start of this stretch — people and
comparison existed as endpoints with nothing pointing at them. The same
question asked of the *API* rather than the screens turns up four endpoints no
screen calls — creating a pipeline key, and the three for a person's own
tokens. The mockup draws neither, so that is an API surface without an
interface rather than a screen missing a panel.

## The adversarial review, and what it found

Three reviewers over everything since the previous one, split by dimension:
authorization and disclosure, correctness and portability, and tests and
claims. Everything below is fixed unless it says otherwise.

**The worst of it was a filter reading the wrong table.** "How far decided" was
computed from `suppressed_by` — which is not a decision of ours at all. It
points at a *suppression*, a claim the **build** made in its own scan file,
written only by the SBOM reader and never by triage. So "agreed" meant "the
vendor's own document argued this away", by a different author, reviewed by
nobody; and a claim actually approved by a second person matched none of the
four states.

**Worth remembering how it survived.** The name reads exactly like what it is
not, and it had been mutation-tested: breaking the clause made the test fail,
which proved the clause ran and nothing about it reading the right table. A
control can be tested and still be wrong about what it controls.

**A claim in one product decided findings in another.** The correlation was
issue plus place identity, and neither carries a product — a place identity is
`sha256(consumer\0component)`, deliberately, so a place is recognized across
variants. A proposal in a product a reader cannot see made their rows read
"waiting" and hid rows genuinely undecided. `due.go` had the correct form ten
lines away, with the product *and* the live key.

**And that correlation was a join, so it multiplied**: one place with three
historical decisions reported as three places. The same shape as the 78×
inflation recorded twice already. It is a correlated subquery now.

The rest, each fixed: a failed run poisoning every older receipt for ever; two
replicas aborting each other's sweep because a duplicate key is not retryable;
a notification handing somebody the name of a product they cannot see; a
condition key of three 191-character names in a 191-character column; `Shapes`
counting without a subject; a search box that was accidentally a pattern
language; run numbers rendering on two pages; a missing index behind a query
four callers make; an unbounded endpoint; two notification kinds nothing could
produce; two documented ecosystem values that match nothing.

**Four tests proved nothing**, which is the half worth reading twice:

- One filtered on `Under: "swss"`. Nothing is called that — `swss` is the Go
  variable, the component is `libswsscommon`. It kept nothing, and "narrowed to
  a subset" passed because nothing is fewer than everything.
- An authorization test asked what a reader may see, from a catalog whose
  fixture built no builds at all. The endpoint answered an empty list to
  everybody, which is also what a missing visibility filter looks like.
- Three of four decision states were pinned by a fixture where nothing is ever
  decided, so "correct" and "always false" are the same result.
- The notification uniqueness index was satisfied entirely by an in-memory
  check inside a single sweep. It could have been absent.

**One finding was refused.** `make engines-up` was reported as passing when a
server never answers, reasoned from a shell without `set -e`. The makefile sets
`-eo pipefail` for every recipe, and the control was watched failing earlier by
giving the wait no time to succeed.

**And the gate then caught a regression in one of the fixes.** Escaping LIKE
wildcards used `ESCAPE '\'`, which MySQL and MariaDB read as an unterminated
string literal because a backslash escapes inside literals there — a syntax
error on two engines, parsed happily by the other two. The escape character is
`#` now. This is precisely what the four-engine run is for, and it is the
second time in this stretch that a change looked right on SQLite and Postgres
and was wrong on the MySQL pair.

## Audit: inside the screens

The audit above asked whether each screen exists, and they all do. This one
asks what is *on* them, panel by panel and column by column, against the
published mockup. Different question, different answers: everything below sits
inside a screen that the earlier audit passed.

**Decided divergences — these are right, and the decision says so.** Worth
listing so nobody re-reports them: the no-fix outcome set the mockup draws
(*contained another way*, *replace the component*, *accept it*) is `REJ-12`,
sayable already as not-applicable with `inline_mitigations_already_exist`;
home summarizing across every product is superseded by `UIX-38`; and the
server-side filters and the by-component view go beyond the mockup rather than
behind it.

**Two defects, not divergences.**

*~~`web/src/screens/Home.tsx:297` hardcodes `stream: "master", variant:
"broadcom"`.~~* **Fixed.** The Operational panel asks for the first product the person can
reach and then names a branch and a variant that only exist in the SONiC
fixture. Any other deployment gets a 404 the panel renders as "never" and "—",
so it degrades into a panel that is silently always empty. It is the only
hardcoded scope in the interface.

*Home's two charts were scoped and said "all products".* Found and fixed while
bringing `DESIGN-interface.md` up to date; recorded above under "Not yet true".

**Gaps with no decision behind them**, largest first:

| | |
|---|---|
| ~~**The credentials half of people and access**~~ | **Withdrawn — it is built.** The people screen lists pipeline keys and everybody's personal tokens and revokes either. This was reported from a search whose output was cut off above the lines that call them, and the conclusion was drawn from the absence rather than checked against the screen. What is genuinely uncalled is narrower and is not a divergence from the mockup, which draws neither: creating a key, and the three endpoints for a person's own tokens |
| ~~**The review queue has no lapsed card, and home links to it anyway**~~ **Fixed.** | The mockup's third card kind is a decision the code moved out from under, offering "say it still applies" and "write a new decision". `/v1/review-queue` returns decisions *proposed and not yet approved*, so home's "decisions that stopped applying → review them" opens a list that cannot contain them. The data is reachable — decisions carry a `lapsed` state and the list endpoint filters on it — but no screen asks for it |
| ~~**Nothing reports a product that has gone quiet**~~ **Fixed.** | The mockup leads the scans screen with "edge-router has not been scanned for 11 days" — nothing failed, nothing arrived. The built screen's own comment states that rationale and the alert is not there, and home's Operational panel reports the last scan of one product rather than noticing a silent one. This is the failure that makes every other number meaningless, and it is the one nothing watches |
| ~~**The findings row is missing three columns**~~ **Two fixed, one declined.** | Where it sits (`UIX-12`), variants (`REL-01`), and state. The row preview shows the immediate consumer, not both ends of the chain, so `UIX-12` is unbuilt in the list in both places it was drawn. `REL-01` is scheduled rather than missed. The row gained three things the mockup lacks — the score beside the word, the likelihood, and the age |
| ~~**The scans table has no opened/closed counts**~~ **Fixed.** | The mockup's columns say what each run changed; the built table says received, built, state and serial. What a run *did* is the reason to read the row |
| **Release comparison cannot be copied out** | The mockup's "copy for release notes" is the point of the screen for the person about to publish. Nothing exports, on the screen or in the API |
| ~~**The catalog screens list names where the mockup lists numbers**~~ **Fixed.** | Products draws a card grid of name and display name; the mockup's table carries branches, tags, variants, open findings and last scan. Same for variants, which the mockup gives "ships to customers", releases built as it, and open findings |
| **Phones get a scrolling rail, not the tab bar** | The mockup puts a three-item bar at the bottom on a phone; the build turns the side rail into a horizontal scrolling strip below 780px. Both work; they are not the same design |

**What the fixes turned out to need**, since none of it was a line of code:

*A build that has gone quiet needed a question nobody was asking.* The home
panel that was supposed to report it named a branch and a variant in its source
and so could only ever have worked on the deployment it was written against.
Both it and the scans screen now read one answer — every declared build,
longest-silent first — which also gave the catalog its "last scan" column for
free, from the same answer rather than a second query that could disagree with
it.

*A build nothing has ever been filed against is reported too*, measured from
when it was declared. It is the same failure caught earlier, and an inner join
to the scan table is exactly the query that cannot see it — which is what the
test for it breaks to prove.

*The queue's own comment already said it should carry three kinds* and warned
that showing only the first lets the other two disappear. It showed one. The
other two link to the decision rather than offering the judgment inline,
because a decision is keyed structurally rather than to a build (MDL-08), so a
row cannot know which build somebody means by "say it still applies".

*What a run changed is counted, not stored*, from the runs the findings already
point at — and a run covers a build rather than an upload, so the count sits on
the newest upload that run covered rather than repeating down three rows.

*Both ends of the chain (UIX-12) needed the row to admit when it is one of
several.* A row covers every place its component sits at and those places can
come down different ways, so it says "one of 4" rather than presenting one
route as though it were the only one.

**One cost is unmeasured, deliberately said so.** Both ends of the chain need
one pass over the build's edges per page. It is flat in the page size — a test
now proves that by comparing two page sizes rather than by a statement count
somebody has to keep up to date, and it was checked by adding a per-row query
and watching it fail. What it costs on a full-size build is not known here:
there is no seeded database on this machine, and the comparable number is the
tree's walk over 19,192 edges taking a request from 0.16 s to 0.57 s. The
findings list is the screen opened first against the largest product, so that
number is worth having. The cheaper shape, if it matters, is climbing a level
at a time from the consumers on the page instead of reading every edge.

**The state column was declined rather than built.** The mockup's single state
per row assumes one place; a row here is a group, and its places can be in
different states. What the row already carries — how many of its places have
been answered — is the more honest answer to the same question, and inventing a
group-level state would mean deciding what "waiting" means for a group where
three places are agreed and nine are untouched. That is a decision, not a
column.

**One thing that is not a divergence but reads like one.** Twelve files style
themselves with utility classes rather than the vocabulary lifted from the
mockup into the stylesheet — heaviest in `PlaceDecision`, `Decision`,
`Together` and `Unassigned`. Nothing is broken, and the tokens are the same
underneath. But the reason the palette and type scale were taken verbatim was
so screens would come out looking like the mockup without anybody eyeballing
them, and a third of the screens opt out of that. Worth settling as a
convention rather than leaving as two habits.

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
and that is the behavior that predates this work rather than a choice made
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

**A name can read exactly like the thing it is not.** `finding.suppressed_by`
sounds like the decision suppressing a finding. It points at a *suppression* —
what the build claimed in its own scan file — and only `internal/sbom` ever
writes one. A filter built on it reported the vendor's claims as our own
triage. Before reading a column, check what writes it.

**A column and the expression the code keys on are not the same thing.**
The decision is keyed on `COALESCE(NULLIF(c.upstream_version, ''), c.version)`;
the reach that says which builds a decision would cover read `c.upstream_version`
raw. That column is empty for anything that is not a patched fork, so every
other build read as "differing" — including one at the same version, which the
lookup already covered — and the version it named for each was that empty
column, so the interface applied the decision there with no version and a build
shipping the name at four versions refused it. Found by the live audit's one
console error, on the first demo with two variants. Where an expression is
exported for one path to key on (`finding.ComponentUpstreamExpr`), every path
comparing against it uses the export.

**Two test runs against the same databases corrupt each other, and a single
`go test` counts as a run.** This happened twice in one session, both times by
forgetting a gate was still going in the background. `ps -eo cmd | grep '[m]ake
check'` before starting anything. A gate started from a session task also gets
killed when the session tidies up, so long runs want `setsid nohup`.

**Escaping in SQL is engine-specific in a way that looks portable.** `ESCAPE
'\'` parses on SQLite and PostgreSQL and is an unterminated string on MySQL
and MariaDB, where a backslash escapes inside string literals. Any literal
carrying a backslash is a four-engine question.

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
half-deleted. So do not start a second run beside a first; there is one set of
servers and `-p 1` serializes packages within a run, not runs against each
other. A `pkill -f "go test"` also kills the shell that is running it, which
reads as the command having failed.

The servers themselves are `make engines-up` now, which is in the repository
rather than on whichever machine happened to have the script — the previous
runner lived in a scratch directory and went with it.

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

Nothing known. `DESIGN-interface.md` was brought level with the code from the
audit above, which is what that audit was for.

Two things found while doing it, both fixed, and the shape matters more than
the detail:

**A design document can be true screen by screen and still be behind.** What
was missing was not the screens — those had been caught up already — but
everything the picker does, which is a behavior of the whole interface and
belongs to no single screen. A screen-by-screen audit cannot find that, because
it is not on a screen.

**Home's two charts were scoped and said they were not.** The query carries the
picker's selection; the panel label said "all products" regardless, so a
narrowed chart claimed to be the unnarrowed one — ten lines below a comment
saying that a narrowed page must announce itself. The rule was written, the
page obeyed it, and a panel inside the page did not.

## The four-engine gate earned its keep twice in one sitting (2026-09-02)

Two defects, both invisible on the engine a quick local run uses.

**A condition insert poisoned the sweep on PostgreSQL only.** `notify.Reconcile`
tolerated a duplicate — two replicas saying the same true thing at once is the
ordinary case — by carrying on past the failed insert. On PostgreSQL a refused
statement aborts the whole transaction, so everything after it failed and the
commit turned itself into a rollback: the clears made earlier in the sweep were
thrown away and the returned counts described work that had not happened.
SQLite, MySQL and MariaDB all allow carrying on, so three engines out of four
said it was fine. Each insert now stands on its own `SAVEPOINT` — plain SQL that
all four take, so no engine branch. The sibling site in `triage/decision.go`
was already correct and says why in its own comment.

**And I nearly signed off the mutation test that proves it.** Breaking the
control and re-running looked convincing — the test passed with the fix and I
had the failure to compare against — except the run was sqlite-only: the URLs
live in `local.mk`, which is a *make include*, so `. ./local.mk` sets nothing in
a shell and `go test` quietly ran one engine. A mutation test that cannot reach
the engine the bug is on proves nothing, and it reports success while doing it.
Exporting the three URLs first made the mutant fail as it should.

So: when running `go test` by hand rather than through `make check`, export the
engine URLs, and check the subtest names in the output actually list four
engines before believing a result.

## Drafts stopped surviving a sign-out (2026-09-03)

Seventh of the 2026-09-03 review's open items, and the one that was a security
control rather than a defect: `UIX-31` says a draft is "cleared on successful
submission **and on sign-out**", with the reason spelled out — drafts hold
triage text, private findings included, and browser storage is no more exposed
than the application already open in the same profile *only while the person at
the browser is the person who typed the text*. The clearing on submission was
built; the clearing on sign-out was not.

Two halves, and the second is the one worth having:

1. **Sign-out clears every draft the browser holds**, every writer's rather
   than only the one signing out — a draft left by an earlier session is
   exactly the one nobody would think to clear. Cleared *before* the request
   that ends the session and whatever it answers: a sign-out that could not
   reach the server is the case where clearing matters most.
2. **A draft is kept under the identity that wrote it.** That covers the
   sign-out that never happened — a session expiring quietly, somebody else
   signing in on that browser and opening the same finding. The decision does
   not ask for this; it is what the decision's *reason* asks for, and the
   review's phrasing ("drafts survive sign-out") named only the half that had
   a control attached.

Text typed before anybody is recognized is now kept nowhere rather than kept
under nobody's name, which is what a key with an empty identity would have
been.

The six call sites each built their own key and called `localStorage`
directly. That is now one module: where a draft lives and under whose name is
decided once, because a control spelled at six call sites is a control that is
missing at the seventh.

**What is still not covered**: the sign-out control itself. There is no
component test here to click it, so what connects the button to the clearing is
checked by reading. Written into `DESIGN-interface.md` under what the interface
does not have tests for, rather than left to be discovered.

`UIX-32`, re-authenticating in place, is still not built and is still recorded
as open.

## A product can now say what it triages (2026-09-03)

Sixth of the 2026-09-03 review's open items. `TRI-43` says a deployment states
a line and a product may state something different. The column holding a
product's own has been **read** by `FloorFor` since it existed and written by
nothing — and the settings screen said "A product may state its own instead",
which was a claim about software that could not do it.

Built: a store method, a route (`PUT /v1/products/{product}/triage-floor`), the
line on the product row in the catalog list, and a control there for
administrators. No new decision — `TRI-43` already says all of this; what was
missing was the code.

Three things worth not rediscovering:

- **Clearing is its own act, not "set it to what the deployment says".** A
  product that stated the deployment's current line would stop following it the
  next time the deployment changed its mind, and nobody would see that happen.
  The mutant that copies instead of clearing fails two tests, which is how it
  should be.
- **Moving a product's line rewrites deadlines**, for the same reason changing
  a window does and from the other direction: below the line nothing is on a
  clock (`REM-27`). It goes through the same leased, off-request path, so the
  three settings that invalidate stored deadlines now have one spelling of
  "and then rewrite them" rather than three.
- **The line was untested end to end.** The existing floor tests build a
  `Floor` value directly, so `FloorFor` reading the product column had no test
  at all; the authorization went into the existing role × endpoint matrix
  rather than into a test of its own, which is where it belongs.

## The two "unbounded" reads: one measured fine, one really was broken (2026-09-03)

Fifth of the 2026-09-03 review's open items, and the interesting part is that
it was half wrong — which is what measuring rather than fixing is for.

**Receipts.** The page reads every finished run of a target and every scan
filed against it, whatever page is asked for, then pairs them in a nested loop.
That is quadratic in the calendar and it looked alarming. Measured over a year
of nights, in the same harness that measures everything else: **about a
millisecond at night one and about four at night 365**, and the last page costs
what the first does. A decade is a hundred times that work and still under half
a second of arithmetic in the application. Recorded, not changed — "nothing is
made faster until it is measured slow" cuts this way too.

**The release comparison** is bounded by the size of a build rather than by the
calendar: it reads every open entry of both builds, which is what diffing them
means. There is no page of a diff. Saying "unbounded" flattened two different
statements into one.

**But the same line named a real defect**, and this one was worth fixing: *why*
each fixed entry went was a query of its own, so a comparison against a
year-old release cost a round trip per line of the release note. It is one read
per batch now. The statement narrows by the issues and the components
*separately* rather than by the pairs, because no engine here spells a
comparison against a pair of columns the same way and concatenating them into
one string is a portability trap — so it fetches a superset and pairs on the
way out.

**`Compare` had no test at all.** Not in the store, not through the handler —
a whole reporting feature, whose own comment records an authorization fix
("the first version authorized the later target and applied that answer to the
earlier one as well, so a caller who could reach one product could read
findings out of another"), with nothing guarding it. Four now, all on four
engines, each watched failing under a mutant: the three groups and the reason
each fixed entry carries; that the explanations cost one statement rather than
forty; **that both builds are authorized, not one**; and that an undisclosed
finding stays out unless somebody asked for it.

That is the thing to take from this one. The N+1 was found by reading; the
missing tests were found by trying to change the code and having nothing tell
me whether I had broken it.

## The year of nightly scans, measured properly (2026-09-03)

The run the previous note called "Next". Numbers are in `DESIGN-findings.md`;
what is worth carrying here is why the withdrawal was the right call rather
than an over-cautious one.

Halving the churn to what the model documents **halved MariaDB (0.64 s to
0.32 s a night), cut PostgreSQL by a third (1.04 s to 0.67 s), and moved MySQL
by four percent (5.01 s to 4.82 s)**. The error did not scale the four engines
alike, so dividing the old figures by two — which is what "corrected in place"
would have meant — would have produced three wrong numbers and one right one,
with nothing saying which.

It also turned the engine gap into a question rather than a curiosity. A cost
that barely responds to how many rows changed is paid **per statement**, not
per row, so the thing to look at on MySQL is how the apply is batched rather
than how much work it does. Recorded in `DECISIONS.md` §4; nothing changes
until somebody measures which statement.

Two figures to hold on to when reading the table: each read column is **one
sample**, and the harness happens to take two seconds apart on identical data —
MySQL's trend came back 1.19 s and then 259 ms, MariaDB's findings list 212 ms
and then 24 ms. The growth is stable across both; the absolute figures are
worth an order of magnitude and no more. The design document now says so, since
the previous version of that table presented single readings as if they were
precise.

## Two passes that ran on every replica now run on one (2026-09-03)

Fourth of the 2026-09-03 review's open items, recorded as `DAT-39`. `SCP-15`
says every replica runs everything and all coordination is in the database, and
these two had the first half without the second.

**Asking the public indexes ran on every replica.** The politeness the pass is
built around — 200 components at a time, a quarter-second apart — was therefore
a rate per replica, not per deployment. Measured by removing the coordination:
two replicas asked six times about three components, on all four engines, two
thirds of it re-asking what the other had already answered.

**The deadline rewrite was worse, because it was wrong rather than rude.** It
read the policy *before* starting the goroutine and ran with no serialization,
so two replicas each handling a setting change would rewrite the same rows from
whatever each had read when it started — and the one that finished last won,
which could be the one holding the older policy. Stored deadlines describing a
superseded policy, with nothing saying so.

The mechanism is a **lease**: a named row taken by the same conditional update
a job claim uses. A job answers "who does this" for discrete work because there
is a row to claim; a pass on a timer has no work item, so what it takes is the
name of the work. It lapses rather than only being handed back, and the holder
renews by asking again each cycle rather than through a second mechanism.

Three things that were choices:

- **Two shapes of losing the race.** The index pass skips the cycle and asks
  again — nothing about it is urgent, and whoever holds it is doing it. The
  deadline rewrite *waits*, because a policy somebody just typed has to be
  applied, and waiting is exactly what makes the last rewrite the one holding
  the newest policy.
- **What the waiting one reads, it reads after its turn comes.** This is the
  whole fix for the rewrite, and it is `DAT-31`'s rule in another setting:
  anything the work decides from is fetched inside the thing that serializes
  it.
- **The lease row is made on first use rather than seeded by the migration.**
  What passes exist is decided in code, and a migration listing their names
  would be a second place to edit whenever one is added or retired. A second
  replica's insert is refused by the primary key, which is the answer rather
  than an error — both then go on to the update, and that is what decides.

`00004_job.go` had two unquoted identifiers (`attempts`, `max_attempts`),
against `DAT-33`. Fixed while the migrations were being recreated anyway.

## One assessment stands per issue, and the database says so (2026-09-03)

Third of the 2026-09-03 review's open items, recorded as `TRI-49`. The rule was
written in three comments — including above an index that was not unique and
claimed to hold it — and enforced by a count-then-insert. That is the shape
`TRI-33` rejects for decisions, and the measurement is blunt: with the
constraint removed, **six proposals at once left six claims standing**.

Same mechanism as decisions, and deliberately: a key naming the issue, held
from proposal and released on withdrawal, under a unique constraint. Two
details that were choices rather than transcription:

- **Held from proposal, not from the moment a rating comes into force.** A
  milder rating waits for a second person (`TRI-41`), so there is a window in
  which a claim is recorded and not yet in force. A rival proposed in that
  window would be agreed to by somebody with no sign the first existed, which
  is the whole failure.
- **A plain nullable copy of the issue id rather than a hash.** A decision's
  live key hashes five fields because the combination is five fields wide; an
  assessment's is one column, and hashing it would be indirection for its own
  sake.

**Assessments had no design section at all** — `TRI-40` to `TRI-42` existed as
decisions, and nothing in `DESIGN-*.md` described what an assessment is, how it
comes into force, or what it reaches. By the repository's own rule that is a
remnant. `DESIGN-triage.md` now carries it, along with the new rule.

**The assessment migration was edited in place**, so a development database
against the four servers has to be recreated (`make engines-down && make
engines-up`), and so does the demo's (`make demo-reset`). The commit that
landed this called it the triage migration, which is the wrong file — it is the
one that created the `assessment` table.

## Urgency was three policies; it is one now (2026-09-03)

Second of the 2026-09-03 review's open items, recorded as `RNK-07`. The three
were: kept as of the moment a finding opened, rewritten in full when somebody
assessed the issue, and compared on screen against the current published word.
Nobody had chosen any of them, and the effect was that a finding opened before
its issue reached an exploited catalog never got the top band or the
three-day window until it happened to close and reopen — the finding `REM-25`
exists for, quietly not working for the ones already open, which is most of
them.

**They became one: the rank follows the issue.** Freezing was the other
candidate and it does not survive contact with the code — `rerank` already
rewrote the number when *our* judgment moved, so the value was never frozen,
only frozen against the world.

Three things fell out of doing it, none of them obvious from the decision:

1. **The rank is read from the issue, not from the report being applied.** This
   is what stops it flapping. The stored signals are the worst anybody has
   claimed and move only toward worse, so a rewrite is a real event; ranked
   from the report in hand, one source omitting a likelihood would demote a
   finding and the next night's report promote it again, for ever. It also
   removed the second spelling of "which of a published score, a published word
   and a rating of ours decides the number", which existed once at ingest and
   once in `rerank` and had already drifted once.

2. **The deadline is narrower than the rank.** Every signal moves the order;
   only exploitation moves the clock, because only exploitation is in it. A
   clock reset by a revised score would never arrive — the same failure as
   resetting it nightly, which is why the comparison was dropped in the first
   place.

3. **A recount runs from the scan that learned the fact.** This was the part
   worth getting right. Counted from the opening, an issue exploited after six
   months would be given three days that ran out five months ago: a deadline
   nobody could have met, across the estate, which is exactly the failure
   `REM-25` names as the way an overdue figure stops being read within a month.
   It is also how the published exploited catalogs are actually used — their
   due dates run from the date an entry was added, not from when anybody
   shipped the package.

The test that pinned the old behavior was deleted rather than adjusted: it
asserted the frozen policy by name. Three replace it, and four mutants were
watched failing — the rank not written, the clock not recounted, the clock
recounted from an earlier moment, and the rank taken from the report.

`AGENTS.md` was wrong about this too, and is fixed: it used urgency as its
example of a derived value stored *to be correct*, "a fact about a moment".
Urgency is not one. Reading "stored" as "frozen" is how it ended up as three
policies, so the paragraph now uses values that genuinely cannot be worked out
again — what a place held before a version moved, and how many builds an
approval reached as granted — and says that storing does not freeze.

## The scan claim now renews itself (2026-09-03)

First of the 2026-09-03 review's open items to be settled, recorded as
`DAT-38`. The claim was a single shot of thirty minutes — the same thirty
minutes the scanner itself is given — so a scan that used its whole allowance
was re-claimable at the moment it ended, and anything slower was taken over
while it ran.

What the fix separated is two questions the single shot had confused: **is the
worker alive**, and **how long is the job**. A running worker renews the claim
every five minutes, so the timeout now bounds silence rather than duration.

**Losing the claim ends the work.** That was the part worth thinking about: the
renewal is refused when another worker holds the job, and from that moment the
work is being done twice with only the other copy's ending recorded — so
carrying on spends a scanner run and every write it makes to lose a race that
is already lost. The work's context carries `ErrNoLongerHeld` as its
cancellation cause, which is how the reader and the runner tell that from a
shutdown. The reader needed it in a second place too: it marks a scan document
as unreadable when a read fails, and a read stopped because the job moved must
not — nothing is wrong with the document.

**The timeout stays at thirty minutes**, which renewal would otherwise let us
shorten to a few intervals. On SQLite the pool is one connection, so a renewal
waits behind the job's own statement; a bound that assumed renewals were prompt
would take the job away from exactly the long transaction it exists to protect.

Four tests, all four engines, each watched failing under a mutant: the renewal
made a no-op (the job is taken over 45 minutes in), the holder check dropped
from the renewal (a worker that lost its claim renews it anyway, and a finished
job can be renewed), and the cancellation removed (the work is never stopped).
One of them also lands on `DAT-35` — the holder renews at the moment it
claimed at, so the write changes no value and still has to report the row as
matched, which is where MySQL and MariaDB would otherwise tell a worker it no
longer holds its own job.

## The second adversarial review (2026-09-03), and what it found

Four reviewers this time: three over the twenty-one commits since the first
review, split as before — authorization and disclosure, correctness and
portability, tests and claims — and a fourth over the whole repository,
because a diff review cannot see what several rounds by different agents
have quietly spelled two ways. Everything below is fixed unless it says
otherwise.

**The worst of it was the same omission, three times: a decision is matched
to a finding without the whole key.** A place identity carries no product
and no version, on purpose, so that a place is recognized across variants.
Every correlation from `decision` to `finding` therefore has to add the
product and, for a live decision, the two version expressions — and three
paths did not. The finding screen's per-place "decision" column named
another product's decision as standing here. The list's state, the row's
state and the finding screen matched by place alone, so a build shipping
libnl 3.9.0 read as "agreed" from a decision written against 3.7.0 — while
the reach, ten lines away, correctly said the decision covered nothing there
and asked a separate question. And the lapse sweep marked a decision lapsed
whenever *this* target moved off the key, ending a product-wide claim for
every other build still on the old code, and doing so across products at a
shared root-level place. The first two are fixed by matching live rows on
the version expressions and keyless rows by place. The third is a rule as
much as a fix, recorded as `TRI-48`: a decision lapses when no open finding
in its product still matches its key, not when one build moves.

**The queue disclosed what it counted.** The builds a claim reaches were
named without narrowing finding visibility, so a public approver learned
which undisclosed build carried an issue; a bulk claim's "fix available"
outlier read `MIN(fixed_in)` across every product sharing the place; the
similar-claims count on a finding counted rows the reader could not read.
Each is narrowed now, and `covering()` — the correctly narrowed form of the
same join, in the same package — is what they should have copied.

**The proposer could park their own claim.** Setting every row aside through
`except` skipped the same-person check that approving and sending back both
apply. The claim left every approver's queue while the finding still said
"waiting". Set-aside rows now answer to the same rule.

**"Off the clock" was spelled five ways.** `Applying` is the one place that
knows a proposal needing approval is not an answer and a deferral ends on its
date; `RunningOut` took any live key as an answer (a proposal waiting a
quarter vanished from the overdue tile), `HeldBy` excluded nothing (the same
finding still counted as overdue against its holder), and the `lapsed`
filter admitted a row whose own state said "waiting". One exported predicate
now says what stands, and the filter says what the row says.

**The upload cap was a comment.** Huma's `MaxBodyBytes` applies to the JSON
body path only; multipart uploads went through `ParseMultipartForm` with no
ceiling, spilling to the temp directory before the parser's own limit was
consulted. `http.MaxBytesReader` now wraps the upload route.

**And the version fix uncovered the next gap.** Once the cross-build post
named the right version, a build partly reached by lookup refused it as a
second claim about the places at matching versions. The route now takes
`remaining` — decide only what nothing stands at — and the guided review
sends it on every build it applies to. Two live audits on the two-variant
demo, one browser error each time, found both; the first review's lesson
holds that the audit is the control the tests are not.

**The test rule that halved the gate was never written down**, and eight of
the fifty-four handler tests moved to two engines pinned what a query
returns — exactly the class the MySQL pair caught twice in one week. They
are back on four; the rule is `DAT-37`; CI now runs the same make targets
as `make check`, including the interface's checks it never ran.

The rest, each fixed: a per-checkout database name that hashed the module
path, so two worktrees against one server dropped each other's databases; a
free-text batch name stored in a 64-character hash column; a malformed
database URL printed with its password; no security headers at all; a
session purge nothing called; `''` where NULL was expected in the trend's
severity; SIGTERM mid-scan calling the bookkeeping with a cancelled context
and a late worker marking a re-claimed job done; nightly UPDATEs of findings
whose urgency moved but was never written; a new finding's deadline from the
published severity while everything else used the assessed one; triage mode
skipping a row under a state filter; per-build refusals from the guided
review discarded in triage mode; five lists that showed one page as the
whole; a sent-back notice linking to a queue that excludes sent-back claims;
a stale pick-up table; British spellings; a chart that accepted SQLite with
two replicas; an overtaken scan stranded when the newer one failed to parse;
an unreachable second decision form deleted; the upload's already-held answer
shown rather than navigated past.

**Open, needing a decision** (recorded in DECISIONS.md Section 4):

- *Urgency is three policies.* Kept as-of-open at apply, rewritten on
  assessment, and compared as if current. A finding opened before its issue
  entered the known-exploited list never gets the three-day window or the
  top band until it closes and reopens. Either rank and deadline follow the
  vulnerability (`rerank` already knows how) or they are frozen and the
  documents say so.
- *The scan queue's claim is a single shot of thirty minutes*, equal to the
  scanner's own timeout, with no renewal; a slow scan can be re-claimed
  while it runs. A heartbeat on `claimed_at` is the fix.
- *The currency refresher and `Recompute` run uncoordinated on every
  replica* (SCP-15); the queue and the watch coordinate, these two do not.
- *An assessment's "one live claim per issue" is read-then-write* with no
  unique index — the shape TRI-33 rejects for decisions.
- *Dead columns:* `product.eol_on`, `stream.eol_on` (MDL-11, REM-16 and
  RPT-04 have no code), `product.triage_floor` (TRI-43's per-product
  override has no route), `scan.parser_version`, `scan_run.ran_here`,
  `scan_document.size_bytes`, `finding.last_changed_at`,
  `person_identity.bound_at`, `job.claimed_by`; seven indexes that are
  strict prefixes of unique constraints, three on `finding`.
- *Release comparison is unbounded and one query per fixed entry*; receipts
  read every run of a target per page; both grow with the calendar.
- *Identity and version columns are 191 wide with unbounded producers.*
- *Drafts survive sign-out (UIX-31) and there is no in-place re-auth
  (UIX-32).*

**Two findings were refused.** `finding_open_idx` being a strict prefix of
the new covering index is true, but the narrower index reads fewer pages
for the "open" scan the planner takes; dropping it is a measured trade, not
a fix. And the interface's "Partly" pill is not a state the server refuses
to send — it is the row the design says is none of the four, labeled; the
design now says so.
