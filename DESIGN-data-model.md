# Data model

What a scan is filed against, and why the shape is what it is.

Satisfies MDL-01, MDL-02, MDL-11, MDL-17, ING-09, ING-11.

## The tracked unit

A scan is filed against **(product, stream, variant)**. All three are declared
before anything may target them.

| | |
|---|---|
| **Product** | A thing that ships |
| **Stream** | A branch or a tag of that product |
| **Variant** | One of the ways that stream is built |

### Branches and tags share a table

They differ in exactly one respect — a branch moves and a tag never does — and
share everything else: a product, variants, findings, an end-of-life date, and
declaration before use. Two tables would duplicate all of that to express one
difference.

A tag records the branch it was cut from, where that is known. That parent is
what lets a branch be compared against its last release, and what a new line
seeds its decisions from.

### A variant belongs to a stream, not to a product

A variant introduced in a later release must not appear to have existed in
earlier ones. So the same variant name in two streams is two variants, and a
scan naming a variant that stream does not have is rejected rather than
matched against a similarly-named one elsewhere.

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

Bounded and exact. No leading or trailing spaces, nothing empty, and a length
that keeps a unique index inside every engine's key-length limit.

Uniqueness is within the parent: a product name is unique globally, a stream
name within its product, a variant name within its stream.

## Engine differences

Only the column declarations differ; everything queried is portable.

| | Reason |
|---|---|
| Generated keys | Three different spellings for the same idea |
| Timestamps | No portable spelling exists — one engine has no `DATETIME`, another's `TIMESTAMP` can acquire an implicit default and an on-update clause |
| Booleans | One engine has no boolean type |
| Text lengths | A unique index needs a bounded width on some engines |

One behavioural difference is worth knowing beyond the declarations: a stream
points at its parent branch, and **MySQL and MariaDB enforce that
self-reference during a bulk delete** where PostgreSQL and SQLite do not. Any
code clearing streams must detach the parent first.

## Not yet decided

Whether dependency paths are materialised. A path is the unit of triage and the
graph is stored as nodes and edges, but enumerating every path through a
dependency graph can produce far more rows than there are components, depending
on how much sharing the graph has. That is a question a real SBOM answers, not
one to guess at.
