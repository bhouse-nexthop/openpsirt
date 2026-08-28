# Ingest

What happens to a scan when it arrives.

Satisfies ING-07, ING-11, ING-14 to ING-23, ING-28 to ING-35, ACC-12.

## Deciding before parsing

Three questions get answered from a scan's metadata, before anything reads its
contents. Parsing is expensive; refusing is not.

The order matters, and each ordering is deliberate.

| Order | Question | Refused because |
|---|---|---|
| 1 | Is the build time in the future? | Once the current scan is dated ahead, **nothing legitimate is ever newer** and that variant takes no further scans at all. One bad clock would wedge it permanently |
| 2 | Have we already taken this exact file? | Answered with success, not an error. The ordinary case is a retry after a timeout that had actually succeeded — failing it turns a landed scan into a red build, and the usual response is retry logic that swallows errors, which then hides real ones |
| 3 | Is it newer than what we hold? | Uploads do not arrive in the order they were made — retries, slow transfers, queued jobs. Taking an older one replaces today's picture with yesterday's, reopening closed findings with no symptom anyone would notice |

Equal build times are refused too. Neither is newer, so choosing between them
would be a coin toss over which picture is current.

## Ordering is by build time, not arrival

The producer's timestamp orders scans. The time we received one says nothing
about which is newer.

A few minutes of clock skew is tolerated, because build machines are seconds
out rather than hours, and refusing those would fail legitimate scans for no
benefit.

## Timestamps are rounded to what the database keeps

Go carries nanoseconds; no supported engine stores them.

Without rounding, a value written and read back is fractionally *older* than
the one still in memory — so a scan compares as newer than itself, and a second
file claiming the same build time is accepted when it should be refused. The
comparison and the stored value are both rounded to the finest resolution every
engine keeps, so the two agree.

This was a latent fault on every engine. Only one exposed it; the others passed
on timing luck. It is the sort of thing a single-engine suite never finds.

## What a scan record holds

Which variant, the content hash, when it was built, when it arrived, the parser
version, and the credential that sent it.

The file itself is not kept for a branch — the next night supersedes it — so
this row and the extracted data are the whole record. The hash makes a
re-upload idempotent. The parser version bounds the damage if a parser fault is
found later: it says exactly which scans were read by the faulty code.

## What actually arrives

Measured against a real producer's output rather than assumed from the
specification. A build sends two things:

| | |
|---|---|
| **Inventory** | Every component that ships, with its dependency edges |
| **Suppressions** | Findings the build has already argued are not applicable, usually because it carries a patch |

**The vulnerability data is produced here, not sent to us.** The inventory is
reproducible and a vulnerability report is not — new issues are disclosed daily
— so a producer that emitted both would have to give up one of the two
properties. Keeping the inventory standalone is also what lets a year-old
release be re-scanned against today's data without rebuilding anything.

Running the scan ourselves is what makes counts comparable between products. A
producer running its own scanner measures each product with whatever version its
pipeline installed, so a difference between two products may only be a
difference in their build images.

A producer-supplied vulnerability report is still accepted. Findings from one
carry their scan provenance, so a portfolio report never silently averages two
scanners.

### The join is on package identifier, and it carries no path

The vulnerability report says "this package at this version" and stops. It has
no idea a component sits at twelve places, because it never saw the graph.

**Fanning one reported issue out across the places it occupies is ours.** It is
the step where one line in a report becomes the number of decisions a person
actually has to make, and it is derived from the inventory's edges rather than
from anything the scanner said.

### The graph is incomplete on purpose

Edges are emitted where the producer can derive them. What it cannot resolve it
records as a note rather than inventing an edge — so a component with no parent,
which is not the root, is an ordinary thing to receive.

Neither alternative is acceptable: refusing the file rejects a legitimate scan,
and synthesising a parent reports a dependency nobody declared.

### The shipped version is often meaningless

A locally-built package commonly carries a placeholder version, with the real
identity in its pedigree — what it was forked from, and at what version. That
is what a scanner matches issues against, and it is why pedigree is kept rather
than flattened away.

### Suppressions are applied here, not upstream of us

The build's judgement about its own carried patches is never refuted. What
changed is where it is applied: rather than receiving results a producer already
filtered, we receive the suppressions and apply them to our own scan.

Same outcome, and it removes a failure that the other arrangement made
invisible. A finding suppressed upstream simply stopped appearing — component
present, version unchanged, pedigree unchanged — which is indistinguishable from
a scanner fault, and lands in a bucket STA-05 makes unsuppressable by design.
Applying them here means a suppressed finding is a thing we can see and account
for.

### Identifiers for the same issue vary

The same vulnerability arrives under different identifier schemes depending on
which database matched it, and a report carries the aliases alongside whichever
one it chose as primary. That choice is a scanner's preference, not a property
of the issue, so identity spans the aliases (MDL-19). Keying a decision on the
primary identifier would lapse every decision the day a scanner changed its
mind.

### Severity arrives as a word

A rating, not a vector, and often with the method given explicitly as
unspecified. Numeric scores come from the feeds instead (ING-10, RNK-04) — there
is nothing in the report to normalise.


## A note on column types

Fixed-width character columns blank-pad on some engines, so a hash read back
carries trailing spaces that make an exact-match lookup fail. Variable-width
columns do not. Both cost the same for a value that is always the same length.
