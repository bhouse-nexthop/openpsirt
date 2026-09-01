# Working notes

Where the interface work is, what is decided, and what is still open. **This
document is temporary**, like `IMPLEMENTATION.md`: it holds the state of one
stretch of work so it survives a break. Anything durable belongs in
`DECISIONS.md` or a `DESIGN-*.md` and should be moved there rather than left
here.

## Where to pick up

The interface is built out and running. `make demo` brings it up; the details
and the two settings people get wrong are in `DESIGN-interface.md` under
"Running it locally".

**The screens the mockup has that are still not built**: people and access,
release comparison. Both have endpoints. `DESIGN-interface.md` lists the
smaller gaps under "Not built yet".

**The mockup is the approved design** and is the reference for anything
visual. It is published at
`https://claude.ai/code/artifact/fe3e1df3-fa9d-4d12-9545-b31d9366d078`. The
palette, type scale and shell in `web/src/index.css` were taken from it
verbatim rather than approximated — that was the correction after a first
attempt built from the decision text alone and came out looking nothing like
it.

## Decided during this stretch

Each of these is recorded in `DECISIONS.md`; they are listed here only so the
reasoning is findable while the work is fresh.

| | |
|---|---|
| **RNK-06 amended twice** | Likelihood used to outrank severity. Measured wrong: a 2004 negligible with no score outranked all 379 criticals on a likelihood of 0.80. Multiplying the two was tried next and reversed on the same evidence — 95% of 5,661 open issues sit between 0.001 and 0.01 likelihood, so multiplying amplifies noise inside a spike. **Severity now leads; likelihood orders what is equally severe** |
| **The trend counts issues, not places** | It counted finding rows, which are per place, and reported 441,108 open where 5,661 issues were open — a 78× inflation measuring how much the graph shares rather than how much there is to answer |
| **A component is addressed by name and version** | A name is not unique in a build. Resolving by name alone answered about whichever was interned first, so two of three rows for one library said "no such finding". An ambiguous name with no version is now a 409 that says to give one |
| **ACC-57 to ACC-59** | Capabilities per product in one answer; the sign-in providers are public; mention candidates are only people who can already read the finding |
| **UIX-37** | The interface is embedded in the binary; a path the router has no route for belongs to the page |

## Open, and needing a decision

**One package arrives as two components.** 158 of 7,859 components split
because the purl namespace differs — `pkg:deb/sonic/openssl`,
`pkg:deb/debian/openssl` and `pkg:deb/openssl` with none. Each split
double-counts its findings. The options and their costs were laid out in full:
ignore the namespace for OS-package ecosystems only, ignore it everywhere
(wrong — it is the scope in npm and the groupId in Maven), group in the list
only (cosmetic, leaves every total wrong), normalise at ingest rather than in
identity, or leave it and wait for a second real producer. **Not decided.**

**Should home be scoped to the selected product?** Asked for, and it reverses
UIX-08 ("the home page panels still summarize *across* products"). Three of the
four home endpoints — `running-out`, `trend`, `unassigned` — have no product
filter, so it needs server work as well as the decision. **Not decided, not
built.**

**A vulnerability has no published date.** A findings list ordered by when
something was disclosed was asked about; nothing stores it. That is ingest work
rather than a screen.

## Traps found the hard way

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

**The dev machine has an HTTP proxy configured** (`HTTP_PROXY` to a Squid
cache) which intercepts `curl` to the local hostname and answers 403. Every
local request needs `--noproxy '*'`.

## Not yet true

`DESIGN-interface.md` claims the screens it describes. Two things it names as
built deserve a second look before they are trusted: the release comparison and
people screens are **not** built, and the design document says so — but the
rail does not link to what does not exist, so nothing points at the gap from
inside the running application.
