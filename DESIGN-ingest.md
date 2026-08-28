# Ingest

What happens to a scan when it arrives.

Satisfies ING-01, ING-02, ING-04, ING-05, ING-07, ING-11, ING-14 to ING-23,
ING-28 to ING-35, MDL-19, ACC-12, SEC-05, SEC-06.

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


## Reading a document

### The header is read separately from the contents

Everything the arrival decision turns on — when the build says it was made,
what the document is about, and the identity the document carries for itself —
is answered by a pass that skips the contents entirely. Parsing a file we are
about to refuse is work nobody asked for, and on the largest producer it is
most of the cost of taking the file at all.

Skipping is not free: key order is the producer's business, and the reference
producer sorts its keys, which puts tens of thousands of components ahead of
the metadata. Walking past a value is still far cheaper than building something
out of it.

### It is read as a stream, and it is bounded

A scan file is somebody else's output arriving over a link we do not control.
Three bounds apply, all settable per read: how large a document may be, how
many components it may describe, and how deeply it may nest. A fourth bounds
how many dependency edges it may declare — component count does not imply it,
since a thousand components can declare a million edges between them.

The depth bound is the reason the document is walked rather than decoded into a
structure. Decoding is simpler and has no depth limit anyone can set, so a file
nested far enough to exhaust the process is something we would discover by
running out of memory rather than by refusing the file. Nesting is bounded
everywhere, including inside the parts of a document nothing reads.

An oversized document is refused **as** oversized. Truncating it and letting
the reader fail reports a malformed file, which sends whoever sees the message
looking at their build instead of at the limit that stopped it.

### The file's own identifiers are used to join it to itself, and nowhere else

A document names its components so its edges can refer to them. Those names are
the producer's, and nothing guarantees they are stable between builds or
consistent between producers — so they resolve edges while the file is being
read and are then discarded. What is stored is derived from the component
itself.

This is what makes a producer renumbering its identifiers a change to nothing.
The test that pins it replaces every identifier in a real document and asserts
that the components, their identities and the graph between them are
unchanged.

Two components sharing one identifier are refused: every edge naming it would
otherwise be a coin toss between them.

### Nesting is structure the producer declared

A component may contain components. That is the producer stating what is
assembled from what, so it is kept as an edge — for some producers it is the
only structure stated, and dropping it would leave everything under nothing.

The edge is derived from the containment the file declares, never from an
assumption about where something must belong. That distinction is the whole of
`ING-31`: a component nothing leads to is ordinary and is left where it is.

### Two names for one component

Content-derived identity can discover that two of a document's own identifiers
describe the same component. The producer could not have known — its
identifiers differ — so an edge between them is not a producer error. It says
nothing, so it is dropped and counted rather than stored as a component
depending on itself.

### Refusals

Reading is all or nothing. A partial inventory is indistinguishable from a
product that shrank, and acting on one closes findings that are still
somebody's problem.

| Refused | Because |
|---|---|
| Not the format we read, or a major version we have not been written against | A reader that guesses eventually guesses wrong on a file that looks close enough |
| No component of its own | There is nothing for the document to be about. Build fragments — one artifact on its way into an inventory — look like documents and are refused here |
| A component with no name or no version | It cannot be identified, so it cannot be tracked |
| An edge naming something the document never describes | Inventing the missing component would report a dependency nobody declared |
| Two components sharing one identifier | Every edge naming it is ambiguous |
| A build time nothing can read | The build time is what orders scans against each other |
| Past any of the four bounds | A broken or hostile file has to fail rather than exhaust the process |

Everything a refusal quotes back came from the file, so what it quotes is
bounded in length. An error is one of the few places a scan file's contents
reach a person.

### What a producer emits that we do not read

A field nobody reads because it was considered and a field nobody reads
because nobody noticed it look identical in the code, and only one of them is a
decision.

So every key path our recorded documents contain is written down, with what is
done with it — acted on, or seen and deliberately left alone. A document
containing a path that list does not have fails the check, which makes a
producer's new field a thing somebody answers rather than a thing that quietly
goes nowhere. The reverse is checked too: a path the reader acts on that no
recorded document contains is a branch nothing exercises.

This is a check on the recorded documents, not on what we accept. **The reader
itself ignores anything it does not recognise**, because producers following
the same specification differ in what they choose to fill in, and refusing a
document for carrying more than we read would reject perfectly good scans.

The same walk answers the question a large document otherwise hides: how many
*distinct structures* it contains, as opposed to how many components. The
structures carried by a single component are the ones a small document does
not have and a hand-written one does not think of.

### Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| Containment from nesting becomes an edge | Nesting is declared structure, not an inference. Some producers state nothing else |
| An edge whose ends turn out to be one component is dropped, not refused | It is a consequence of our own identity rule, not a fault in the file |
| A fourth bound, on declared edges | The three that were decided do not bound it, and it is not bounded by the others |
| Only the first ancestor supplies upstream identity | It is what the component was forked from. Anything further back is history, and a scanner matches against the fork point |
| Default bounds sit several times above the largest producer | Refusing a legitimate scan is its own failure. The ceiling only has to be low enough to protect the process |


## Reading what the build already answered

A build's claims about vulnerabilities in what it ships arrive two ways, and
they are not equally precise.

| | |
|---|---|
| **On the component** | A patch in a component's pedigree recording which vulnerability it fixes. It arrives attached to the thing it is about, so nothing has to work out what it applies to |
| **In a document of their own** | Statements naming what they apply to by package identifier — which may be one version, every version of a package, or a whole source tree |

Both are read into one shape, with where each came from recorded, because the
second can point at something we cannot resolve and the first cannot.

### The vocabulary is kept rather than translated

A build saying "we carry the fix" and one saying "the vulnerable code is never
reached" are making different claims. Collapsing them into "suppressed" at the
door would lose the distinction before anyone triaging saw it.

Two of the four remove a finding from what somebody has to look at; the other
two say the build looked, which is information rather than an answer. A claim
whose status is not one we can read is refused rather than ignored — ignoring
it lets a build's judgement go missing silently, which is the failure applying
the claims here exists to remove.

### A carried patch reports as fixed

The vulnerable code was there and a patch resolved it. That is not the same
statement as the vulnerability never having applied, and the difference matters
to whoever reads it later.

Only what a patch *claims* is read: a patch names a vulnerability in its own
name or in a header saying what it fixes. A vulnerability mentioned in passing
in a patch is not that patch claiming to fix it. The producer draws the same
line, which is why the claim can be taken as read.

### Matching a claim to a component

| Rule | Why |
|---|---|
| Qualifiers and subpaths are discarded before comparing | A claim is written as the package and the version. The same package in an inventory carries the architecture it was built for, so comparing the two as written matches nothing |
| A claim naming no version covers every version | The format says so, and it is how a build states something about whatever it happens to ship |
| A claim against a source tree matches a component of that name, or a fork of one | The build knows which packages came out of a tree and we do not. Name equality is the most that can be said, and it is what the producer intends by writing one |
| A claim is matched at every place its component sits | One claim, however many places — the fan-out is ours either way |

**A claim that matched nothing is reported, not dropped.** A build's judgement
that went nowhere means a finding it already answered comes back as noise, and
nothing distinguishes that from a finding nobody has looked at yet. The
producer's own automatically-extracted claims name source trees rather than
packages, so this is the ordinary case rather than the exceptional one.

### Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| A claim naming an unreadable status refuses the document | A claim we cannot act on is one the build believes it made. Failing loudly is better than a build's judgement silently going missing |
| A claim with no justification is kept rather than refused | The format only requires one for a single status, and what to do about an unjustified claim is a triage question, not a reading one |
| Both shapes read into one thing, with the origin recorded | They differ in precision rather than in meaning, and the difference is worth keeping without needing two of everything downstream |
| Only a security claim is read from a patch | A patch resolves defects and improvements as readily as vulnerabilities |


## A note on column types

Fixed-width character columns blank-pad on some engines, so a hash read back
carries trailing spaces that make an exact-match lookup fail. Variable-width
columns do not. Both cost the same for a value that is always the same length.
