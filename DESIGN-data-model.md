# Data model

What a scan is filed against, and why the shape is what it is.

Satisfies MDL-01 to MDL-08, MDL-11, MDL-17, MDL-18, MDL-21, ING-09, ING-11,
ING-36 to ING-38, STA-08.

## The tracked unit

A scan is filed against **(product, stream, variant)**. All three are declared
before anything may target them; the pair of the last two is recorded on first
use rather than declared, since it names nothing new.

| | |
|---|---|
| **Product** | A thing that ships |
| **Stream** | A branch or a tag of that product |
| **Variant** | One of the ways the product is built |
| **Target** | One release built as one variant — the pair a scan is filed against |

### Branches and tags share a table

They differ in exactly one respect — a branch moves and a tag never does — and
share everything else: a product, variants, findings, an end-of-life date, and
declaration before use. Two tables would duplicate all of that to express one
difference.

A tag records the branch it was cut from, where that is known. That parent is
what lets a branch be compared against its last release, and what a new line
seeds its decisions from.

### A variant is the product's; a target is a release built as one

What a product is built as — a chip, an architecture, an operating system — is
a property of the product. It is declared once, named once, and it does not
change because a new release came out.

**A target is one release built as one variant.** That pair is what a scan is
filed against and what everything downstream points at, so a single identifier
runs from a scan through its graph to a finding.

| | |
|---|---|
| **Variant** | Declared, once per product. Its name is unique within the product |
| **Target** | Recorded on first use. Nothing new is named — the product, the release and the variant were all declared, so a scan saying this release was built as that variant reports a fact |

Nobody restates the list per release. Somebody made to retype it will
eventually retype it differently, and `win`, `windows` and `win32` across three
releases are three variants as far as everything downstream is concerned —
three sets of findings, three sets of decisions, three columns in every report
— with nothing in the data saying they were meant to be one. Declaring the
variant once makes that impossible rather than merely discouraged.

A variant introduced later still does not appear in earlier releases. Those
releases have no target for it, because nothing was ever filed against them
with that variant. The property survives; it is a consequence of what was
scanned rather than of where the row lives.

Each variant records whether it is customer-facing. It feeds ranking — a
critical in a test-only artifact matters less than a medium in something a
customer runs — and defaults to customer-facing, because an unclassified
artifact should rank as though it ships.

## Declared before use

A scan naming something undeclared is refused.

The alternative was to create whatever a scan named. That reads as convenient
until a pipeline has a typo in a stream name: the misspelling becomes a stream
that looks entirely genuine, with its own findings, counts and place in every
report, while the real stream appears to have stopped being scanned. Two wrong
answers from one keystroke, neither visible.

The rejection names **which part** is missing — product, stream or variant.
Whoever sees the failed upload needs to know what to declare, not that
something somewhere was wrong.

## Names

Bounded. No leading or trailing spaces, nothing empty, and a length that keeps
a unique index inside every engine's key-length limit.

Uniqueness is within the parent: a product name is unique globally, and a
stream name and a variant name are each unique within their product.

**Capitals do not distinguish two names.** These get typed by hand into build
scripts, so a pipeline saying `sonic` against a product declared as `SONiC` is
the same typo problem that declaring before use exists to catch — and refusing
it teaches somebody the product is not declared when it plainly is.

What is stored for matching is the normalized form, and the spelling somebody
wrote is kept beside it and is what gets shown back. Reading a product back as
`sonic` when it was declared `SONiC` looks like the tool got the name wrong.

Normalizing the value is what makes this behave the same everywhere, rather
than asking each engine to compare loosely: a lower-case value compares the
same under any collation, and the unique constraint means one thing on all
four. It is worth saying which way round that goes, because the first attempt
went the other way — treating it as an engine difference to be configured
around, when the schema had already settled that and what was actually
undecided was what a name means.

An identity a sign-in provider hands over is a different thing and is compared
exactly. It is not typed by us and not ours to reinterpret; deciding on our own
that two accounts are really one person merges access nobody granted.

## Engine differences

Only the column declarations differ; everything queried is portable.

| | Reason |
|---|---|
| Generated keys | Three different spellings for the same idea |
| Timestamps | No portable spelling exists — one engine has no `DATETIME`, another's `TIMESTAMP` can acquire an implicit default and an on-update clause |
| Booleans | One engine has no boolean type |
| Text lengths | A unique index needs a bounded width on some engines |

One behavioral difference is worth knowing beyond the declarations: a stream
points at its parent branch, and **MySQL and MariaDB enforce that
self-reference during a bulk delete** where PostgreSQL and SQLite do not. Any
code clearing streams must detach the parent first.

## The dependency graph

A scan describes what a build contained: a set of components, and which of them
depends on which. That is stored as **nodes and edges**, never flattened into a
list — a flat list cannot answer "why is this here", which is the first
question anyone asks about a finding.

### A component is shared; a node is not

| | |
|---|---|
| **Component** | A package at a version. One row, however many products ship it |
| **Node** | That component's presence in one variant |
| **Edge** | One node depending on another |

A component reached by several parents is **one node with several edges**. The
graph is a graph, not a tree. Storing a node per route would multiply the graph
by its own sharing, and the routes are derivable from the edges anyway.

### Identity comes from content, not from the file

A component's identity is derived from what it is — the package identifier
where the producer emits one, name and version where it does not — and hashed
to a fixed width.

Identifiers the file supplies are not used. Nothing guarantees they are stable
between builds of the same product or consistent between producers, and an
identity that moves takes every triage decision attached to it along.

### One package, however a producer spells it

A package identifier is read for what it says rather than byte for byte:
escapes decoded, the ecosystem lowercased, and the qualifying parts — an
architecture, a distribution, the source package a binary came from — left out.

A real inventory needs this. A build that merges two sources emits the same
package twice, once with those qualifiers and once without, sometimes escaping
the version differently and sometimes disagreeing with itself about the
architecture. Measured on a public switch operating-system image: 8,374
described components name 7,858 packages, and every one of the 516 collisions
was the same name at the same version. Taking the identifier verbatim would
count those packages twice, split their findings across both halves, and leave
only one half carrying the identifier a scanner matches on.

Architecture is the one that looks like it belongs. What a product is built as
is already a dimension of this model — it is the variant — so putting it in a
component's identity states it twice, and the same package then reads as two in
a report that has already separated them by variant.

What this costs: two artifacts distinguished only by a qualifier read as one
component. A component is tracked for which vulnerabilities apply to it, and a
qualifier does not change that, so it is a conflation worth accepting.

The reduction is the one the identifier specification describes, applied the
same way to every ecosystem. Nothing here knows which producer wrote a
document, and it must not — the inventories this will be given come from build
systems nobody has seen yet.

Upstream name and version are carried alongside (MDL-04). A shipped fork often
has a version string of its own while the vulnerability lives on the upstream
one.

Producers state that two ways and mean the same thing: the format has a place
for it, and several hang it off the package identifier instead. Both are read.
In a measured image the two did not overlap at all — 30 components stated it
one way and 535 the other — so reading a single place captured a twentieth of
what was there. It is also the name a build's own suppressions use, because a
patch is written against a source tree rather than against the binaries cut
from it.

A bare upstream name with no version is not a lesser answer. For a binary cut
from a differently named source package it is the whole of what is knowable,
and it is the half that matching a claim needs.

### Two descriptions of one component are combined

A document that describes the same package twice is describing one component,
and the two descriptions are not always the same description. Keeping whichever
arrived first discards whatever only the other one knew — which, in the
measured image, was the vulnerability-database identifier for 204 of them,
depending only on which half the producer happened to emit first.

So they are combined. The first statement of anything stands, and anything it
did not state is taken from the next description that does. Nothing is
overwritten: two producers disagreeing is not something a reader can settle,
and the first answer is the one everything downstream has already been given.

The other identifier scheme in circulation — the platform enumeration the
national vulnerability database keys on — is kept beside the package
identifier and **excluded from identity**. A scanner given one matches things a
package identifier alone misses: vendor firmware, operating systems, appliances,
anything never published to a package ecosystem. Deriving identity from both
would move the identity of every component carrying the second, which takes
every decision attached to it along.

It is captured at ingest rather than later because a scan file is not kept once
it has been read. What is discarded there is not recoverable by re-reading, only
by asking the producer to build again.

### The top level is marked

The root — the product itself — is stored as a node like any other, flagged.
Its version changes on every build and its name differs per variant, so it is
excluded from identity and from expiry (MDL-07). Marking it is what lets
everything walking upwards stop there.

A scan that names no root of its own is filed against the unit it was sent for
— the product, stream and variant it arrived against. Standing in costs
nothing, for the same reason the root is excluded from identity in the first
place: what it says about itself was never load-bearing.

## History is intervals, not snapshots

Every node and edge records the scan that opened it and, once gone, the scan
that closed it. An open row is what is present now; a closed row is what a past
release contained.

**Rows are closed, never deleted.** What a release shipped is a question asked
years later, and a deleted row cannot answer it.

### An unchanged build writes nothing

Applying a scan compares it against what is currently open and writes only the
difference. A rebuild that changed nothing writes no rows at all — not an
insert, not a re-stamped timestamp.

This is the point of the whole shape (STA-08). Scans arrive nightly and change
very little; storage that grows per scan grows with the calendar, and a product
tracked for a year would cost the same whether or not anything happened to it.
There is a test that asserts it, and it has been checked by breaking the
comparison and watching that test fail — an assertion nobody has seen fail is
not evidence.

### One transaction

A scan's graph is applied whole. A half-applied graph is indistinguishable from
components having been removed, which would close findings that are still
present and are still someone's problem.

### Refusals

| | |
|---|---|
| A component with no name | Cannot be identified, so cannot be tracked. A component with no *version* is kept: the format does not require one, nothing can match a vulnerability against a version nobody stated, and it ships regardless |
| An edge naming a component the snapshot does not list | Inventing the missing component would report a dependency nobody declared. This is the store's own guard: an edge that named something a *document* never described was already dropped and counted when the document was read, so one reaching here means the snapshot was built wrong rather than that a producer emitted something odd |


## A place is a component and what pulled it in

The unit of triage is a component **at a place**, and a place is the pair: this
component, under the thing that directly depends on it. Names only, no
versions, hashed. Where the thing above is the root, the component's name
stands alone — the root's name differs per variant, so including it would break
cross-variant grouping.

### Why the pair, and not the route

The first form of this was the whole chain of names from the top down. Measured
against a real switch image, that does not survive its own tail.

| | Chain of names | Component and its consumer |
|---|---:|---:|
| Places in one image | 134,509 | 27,366 — which is the edge count |
| Worst single component | 49,170 | 63 |

The worst case is one shared library. Ten sub-packages are built from its
source, each depending on the others, and one package depends on all ten — so
every route arriving at that family multiplies through it, and the same fact is
restated tens of thousands of times. A vulnerability there would have produced
49,170 findings for one issue, none of which a person could act on.

The same component has 48 direct consumers: the containers that ship it, its
own siblings, and the six packages that actually call it. That list is what
somebody triaging wants, and it is what the pair records.

Nothing is lost that the graph cannot answer. Which container something runs in
is one step further up, and the whole route is there to be walked.

## Paths are walked, not stored

Storing a row per route was left open until a real SBOM could be measured. It
was, and there is nothing worth storing: under the definition above a place
*is* an edge, and the questions asked of the graph are answered in milliseconds
by walking it.

| Question, asked of the worst component in a real image | PostgreSQL | MySQL |
|---|---:|---:|
| What directly pulled this in? (48 answers) | 3 ms | 11 ms |
| Everything above it, up to the image (78 answers) | 8 ms | 18 ms |

Precomputing to save that would cost writes on every scan and bound nothing:
how many routes exist is a property of the producer's graph, and nothing stops
the next one being worse than this.

Recursive traversal is portable across all four engines, so this needs no
engine-specific path.
