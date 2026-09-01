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

## Open, and needing a decision

**Should home be scoped to the selected product?** Asked for, and it reverses
UIX-08 ("the home page panels still summarize *across* products"). Three of the
four home endpoints — `running-out`, `trend`, `unassigned` — have no product
filter, so it needs server work as well as the decision. **Not decided, not
built.**

**A vulnerability has no published date.** A findings list ordered by when
something was disclosed was asked about; nothing stores it. That is ingest work
rather than a screen.

## Issues seen in the running interface

Reported from clicking around the demo. Each one is checked against the mockup
and against `DECISIONS.md` before it is written down, because the interface was
built once already from decision text alone and came out wrong.

**"Where it sits" shows one flat row, not the chain.** `UIX-14` says it plainly:
*opening a finding shows the complete chain, root to component, with the version
at each step*. The mockup draws exactly that — `sonic-broadcom 202411.0`, then
`docker-sonic-mgmt-framework 1.0.0` indented under it, then `openssh` indented
under that, and the whole chain repeated for the second route through
`docker-platform-monitor`. What `web/src/screens/Finding.tsx:150` builds is one
row per place holding the direct consumer's **name only**: no root, no
intermediate steps, no versions. `UIX-12` is unimplemented the same way — the
findings row is meant to show both ends of the chain and shows only the
immediate parent (`web/src/screens/Findings.tsx:324`).

**"The product itself" is most of what you see, and it is not a fallback.** It
renders whenever `finding.consumer_id` is null. In the demo database that is
**284 of the 450 components** that carry findings — so on a component picked at
random it is the likely answer, which is what makes it read as the screen giving
up. It is not wrong: the SBOM's edges are mostly *containment* (image → container
→ package), so a host package genuinely has the image as its only parent, and
4,818 of 8,374 components in the fixture have exactly that and nothing else.
Naming something more useful there needs richer edges from the producer, not a
change here. The phrase is also borrowed from the wrong screen — in the mockup
it belongs to the tree's "What pulls this in" panel, for a node with no parents.

**Two places can render as the same row.** 207,606 findings have two places
whose consumer name is identical. It is not version drift — no consumer name in
the database exists at two versions — it is the duplicate-component bug below:
`opennsl-modules` interned twice from one artifact, so one place is listed twice
with nothing to tell the two apart. Most of this should dissolve when the
rebuilt SBOM lands. What will not dissolve is that the row has no ancestry to
distinguish it by, which is the first item.

**"What was decided →" is the wrong affordance on that panel.** The mockup ends
"Where it sits" with a single **"Show where this sits in the build →"** that
opens the tree. The built panel instead hangs a per-place "What was decided →"
link off every row, which is a different question from the one the panel asks
and puts a decision route where the design put an orientation route.

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

**Every count on screen today came from an SBOM with known duplicates.** Until
the fixture is replaced, 156 packages are counted twice and so are their
findings. A total that looks slightly high is not necessarily a defect in the
query.

**The dev machine has an HTTP proxy configured** (`HTTP_PROXY` to a Squid
cache) which intercepts `curl` to the local hostname and answers 403. Every
local request needs `--noproxy '*'`.

## Not yet true

`DESIGN-interface.md` claims the screens it describes. Two things it names as
built deserve a second look before they are trusted: the release comparison and
people screens are **not** built, and the design document says so — but the
rail does not link to what does not exist, so nothing points at the gap from
inside the running application.
