# Ingest

What happens to a scan when it arrives.

Satisfies ING-01, ING-02, ING-04 to ING-08, ING-11, ING-12, ING-14 to ING-35,
MDL-19, ACC-12, ACC-50, ACC-53, DAT-39, SCP-01, SCP-11, SCP-15, SEC-05, SEC-06.
What arrives from a static analyzer is intended scope and is not built; the
section at the foot says what has been decided about it.

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
identity in its pedigree — what it was forked from, and at what version.

**A scanner does not read that.** Measured rather than assumed: given a forked
package carrying its ancestor in `pedigree.ancestors`, the reference scanner
matched nothing; given the same package with no pedigree at all but with its
distribution named in the package identifier, it matched forty-three advisories.
What it matches on is the identifier and the distribution context, and it
compares the version the package actually carries.

Pedigree is kept for two other reasons, both of which stand. It explains a
finding to whoever reads it — why a package at a version nobody recognizes is
being reported against advisories for another. And expiry is keyed on the
upstream version, because a fork's own revision moves for packaging reasons
that have nothing to do with whether a vulnerability is still there.

What this does mean is that a component arriving with no distribution context
in its identifier is one nothing will match, and that is invisible rather than
an error.

### Suppressions are applied here, not upstream of us

The build's judgment about its own carried patches is never refuted. What
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
is nothing in the report to normalize.


## What happens at the door

A build sends everything in one request: the inventory, and however many
suppression documents it has, as named parts. One request means one
authorization and one transaction — a build whose inventory landed and whose
suppressions did not would have every carried patch reported as an outstanding
vulnerability, which is worse than the upload having failed outright.

The order of work is chosen so that nothing expensive happens before anything
that might refuse it.

| Order | Step | Why here |
|---|---|---|
| 1 | Is the backlog already too deep? | The cheapest refusal there is. Deciding afterwards means storing tens of megabytes and discarding them, on a deployment that is by definition already behind |
| 2 | Is the target declared? | One query, and the answer names which of product, stream or variant is missing so whoever sees the failed upload knows what to declare |
| 3 | What does the inventory say about itself, and what is its hash? | One pass over the file answers both. The hash is over the bytes that arrived, not a value the sender supplied |
| 4 | The arrival decision | Future, already held, not newer — in that order, for the reasons above |
| 5 | Store the documents and leave the work behind | One transaction. A scan row without documents is unreadable, documents without a job are work nobody picks up, and a job without either is a worker failing on something that was never there |

The reply says what happened in a producer's terms: taken and queued, or
matched against something already held. **Already held is a success**, because
the ordinary case is a retry after a timeout that had in fact succeeded.

| Refused with | When |
|---|---|
| Not found | The product, stream or variant has not been declared. The message names which |
| Bad request | The build time is ahead of our clock — the producer's own clock is wrong, which is a fault in what was sent |
| Conflict | Something newer, or something with the same build time, is already held |
| Unprocessable | The inventory could not be read at all |
| Service unavailable | Too much is already waiting to be read. The caller is told to come back |

### The label on a part is not trusted

What a part *is* gets decided by reading it. A build pushing a file with an
ordinary command-line client labels it as opaque bytes, which is not wrong and
is not worth refusing an otherwise good scan over.

### A large part is not held in memory

Parts above a few kilobytes are spilled to a temporary file on the receiving
node for the length of the request, then read into the database from there. The
deployment therefore needs a writable temporary path even though its root
filesystem is read-only, which the packaging provides.


## What happens after the response

A worker claims the scan, reads its documents, and applies what they describe.
Every replica does both: a separate worker deployment would be a second thing
to run and a second thing to get wrong for an installation this size, and the
queue already stops two of them taking the same scan.

The suppression documents are read here even though applying them waits on the
scan itself. A document that cannot be read is a fault in what the build sent,
and finding that out while the producer still has the build in front of them is
worth more than finding out later.

What the build argued is stored as data at this point, against the target
rather than against the scan, because it is what the next vulnerability scan
has to apply — and by then the documents may be gone.

**A failure is recorded against the scan, not only against the job.** A
producer sending files nothing can read has to be visible as exactly that. A
job that keeps retrying is visible only to whoever operates this deployment,
which is the wrong person to be the only one who knows.

Once a scan has been read, what it sent is either discarded or kept, according
to whether the line it was filed against moves.

### Which is why a sender can ask what became of it

Recording a failure against the scan is only half of making a bad producer
visible: somebody has to be able to read it. The acceptance a build received
was issued before any of this ran, so it cannot carry the answer, and a build
that never checks again is a build that goes green on a file nothing could
read.

So what was filed against a build can be asked for, newest first, and each
upload reports how far it has got: taken and not yet read, read and awaiting a
vulnerability scan, done, or refused with the reason. A key sees the uploads it
sent itself, which is what makes this reachable from the pipeline that has the
build in front of it rather than only by whoever operates the deployment.

The four states are ours, not the queue's. Reading and scanning are two jobs
with different rhythms and a producer has no business knowing which of our
queues its work is sitting in — that would tie a build script to an internal
arrangement it cannot see and we would like to keep changing.

### Two scans of one target are kept apart

Uploads are accepted in the order they arrive and read in whatever order
workers pick them up — the queue hands different jobs to different workers by
design. Two consequences follow, and both are guarded.

A scan can reach the reader after a newer one has already been applied.
Applying it then would replace today's picture with yesterday's and reopen
everything the newer one closed, which is the harm the arrival check prevents
at the door, arriving from behind. So a scan that has been overtaken is not
applied, and that is recorded rather than treated as a failure.

An overtaken scan is set aside on the strength of the newer one's row alone;
the newer one may not have been read yet. If it then cannot be read, the build
would be left showing whatever came before both — the older scan's work is
done and nothing reads it again, and its receipt waits for a scan run that
never comes. So once a scan has failed, the newest that still stands is read
again, where the build does not already show it and nothing is on its way to
reading it. A scan can therefore be read more than once, and the latest
reading is the one its receipt reports.

A read cut short by a shutdown is not recorded against the scan. Nothing is
wrong with the document, and marking it failed would leave the receipt saying
so after a retry had stored it — the job is handed back and read again once
the process is running.

And two applies for one target must not interleave. Both would read the same
open rows, both compute the same difference, and both write it, leaving two
open rows where everything downstream assumes one. Applying takes the target
row first — an ordinary update, so every engine takes the lock and the second
worker waits.

### Waiting rather than being told

The queue is polled. A notification mechanism exists on one of the four
supported engines and nothing portable replaces it, so an idle reader asks
again after a few seconds. That interval bounds how long a producer waits to
see its scan reflected, which nobody is watching a clock for — and a queue that
is not empty drains at the speed of the work rather than the speed of the poll.


## Where a document lives between arriving and being read

Reading is asynchronous, so a document has to be somewhere from the moment it
is accepted until a worker picks it up. It lives in the database.

The alternatives each fail a constraint already settled. More than one replica
runs, so a file on the receiving node is not there for whoever takes the work.
An object store is optional by decision — everything except attachments works
without one — so ingest cannot be the thing that makes it mandatory. The
database is the one place every deployment already has.

### Writes are split the same way reads are

The ceiling that splits a document across rows applies to every statement, not
just to the one carrying a document. One real image opens tens of thousands of
graph rows in a run and produces over three hundred thousand findings; sent as
one statement each, those are tens of megabytes of SQL, which two of the four
engines refuse outright and all four hold in memory.

Closing is the same shape: naming what a scan no longer contains lists as many
identifiers as opening wrote rows. Both directions are bounded.

**Content is split across rows.** A single value of tens of megabytes runs into
a maximum packet size on two of the four engines, and that limit is server
configuration rather than anything a client can discover. Bounded rows stay
inside every default in circulation, and let a document be read as a stream
rather than held whole — which is the same reason the parser streams.

The hash is computed from the bytes as they are stored. A hash the sender
supplied describes the file they meant to send, not the one that arrived.

### Retention is not symmetric

A nightly scan's *contents* are deleted once it has been read: the next night
supersedes it, and keeping them grows storage with the calendar rather than
with what is being tracked. A tagged release keeps both its inventory and its
suppressions, because re-scanning it years later needs what it contained *and*
what the build had already argued about its own carried patches. Keeping only
the first would quietly undo every one of those arguments on the next re-scan.

**The record of what arrived outlives the contents.** What kind of document it
was, how large it was, and the hash of the bytes as they were received are kept
either way, and an upload reports them (ING-07). Two things need that. A
re-parse means asking the build to send the file again — that is the whole
recovery path for a document that has been let go — and without the hash the
second copy is taken on trust. And an upload whose contents are gone otherwise
reads back as an upload that arrived with nothing, which is indistinguishable
from one that failed to store anything.

It costs a few hundred bytes against tens of megabytes, so it is kept for every
upload rather than for the ones somebody might ask about later.


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

### What is tolerated rather than refused

The specification requires very little — a format and a version on the
document, a type and a name on a component — and producers following it differ
enormously in what they fill in. **A document that is valid and sparse is not a
broken one**, and refusing it rejects a legitimate scan while telling a build
engineer to fix something that is not wrong.

| Tolerated | What happens |
|---|---|
| The document names no component of its own | What the scan was filed against stands in. Nothing is lost: the root is excluded from identity and from expiry anyway, precisely because what it says about itself changes every build |
| A component states no version | Kept and counted. What it costs is matching — nothing can say whether a vulnerability applies to a version nobody stated — and it ships either way, so it is better visible than discarded along with the rest of the document |
| An edge names something the document never describes | Dropped and counted. The missing component is still not invented; the edge simply goes nowhere. One unresolvable edge is not a reason to reject tens of thousands of good components |
| Fields we do not read | Ignored. A producer carrying more than we read is the ordinary case |

The counts matter as much as the tolerance. Each is a number that should be
stable build to build, so a change in one says the producer changed — which is
the thing that would otherwise be silent.

All or nothing still holds, and it is about a *failed parse* rather than about
producer variation: a document either lands whole or does not land.

### Refusals

Reading is all or nothing. A partial inventory is indistinguishable from a
product that shrank, and acting on one closes findings that are still
somebody's problem.

| Refused | Because |
|---|---|
| Not the format we read, or a major version we have not been written against | A reader that guesses eventually guesses wrong on a file that looks close enough |
| A component with no name | It cannot be identified, so it cannot be tracked |
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
itself ignores anything it does not recognize**, because producers following
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
| A tolerated case is counted rather than merely allowed | A number that changes says the producer changed. Allowing something silently is how that goes unnoticed |
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
it lets a build's judgment go missing silently, which is the failure applying
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

**A claim that matched nothing is reported, not dropped.** A build's judgment
that went nowhere means a finding it already answered comes back as noise, and
nothing distinguishes that from a finding nobody has looked at yet. The
producer's own automatically-extracted claims name source trees rather than
packages, so this is the ordinary case rather than the exceptional one.

### Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| A claim naming an unreadable status refuses the document | A claim we cannot act on is one the build believes it made. Failing loudly is better than a build's judgment silently going missing |
| A claim with no justification is kept rather than refused | The format only requires one for a single status, and what to do about an unjustified claim is a triage question, not a reading one |
| Both shapes read into one thing, with the origin recorded | They differ in precision rather than in meaning, and the difference is worth keeping without needing two of everything downstream |
| Only a security claim is read from a patch | A patch resolves defects and improvements as readily as vulnerabilities |


## Scanning again, on a schedule

The vulnerability data is produced here rather than by the build (ING-20). That
is what makes counts comparable between products, and it has a consequence the
ingest path alone does not cover: **a release built a year ago has the same
components it always had and a different answer every month.** An inventory
arriving puts one scan on the queue. Nothing else asked for the next one, so a
build that is never rebuilt was measured once — and it is exactly the build an
advisory published this morning is most likely to be about.

So a pass asks. Every build holding an inventory that has not been scanned
within the interval gets a scan put on the queue.

**Four things it does not do, each for its own reason.**

*It does not scan a build with no inventory.* A build is declared before
anything is filed against it, and scanning one with no components records a run
that found nothing against a build that has nothing — an empty answer that
reads exactly like a clean one.

*It does not queue a second scan for a build already waiting on one.* The pass
looks far more often than anything is due, because the interval is a setting an
administrator may shorten and a pass that woke once a day would take a day to
notice. Looking often means a build the queue has not reached yet would collect
one job per cycle, each of which does the same work when it finally runs. What
is already queued is read out and compared as identifiers rather than joined in
the statement: a job's reference is text and a build's identifier is a number,
and converting one inside a query has no spelling the four engines agree on.

*It does not ask for more than a slice at a time.* The bound is the reason every
bulk write here has one: a deployment tracking a great many builds would fill
the queue in one pass and push a producer's arriving inventories behind work
that is not urgent. What is left over is still due next cycle.

*It stops rather than failing when the queue is full.* What is due stays due,
and a full queue is a fact about how much is in flight rather than an error
anybody can act on.

**One replica asks.** Two reading the same list at the same moment would both
see the same build as due and both queue a scan for it — the check that nothing
is queued already is made when the list is read, which is before either has
written anything. Which replica asks is settled by a lease, the same mechanism
the upstream pass uses (`DESIGN-queue.md`).

**How often is a setting.** The databases the scanner reads are published
daily, so scanning more often measures the same data twice and scanning much
less often means an advisory waits to be noticed. What a scanner costs to run
over a whole estate is a question about that estate, so it is tuned rather than
compiled in. It ships at a day.

**What this does not do is discover.** The component list still comes from the
build (ING-23). What changes between one scan and the next is what is known
about those components, never what the build is thought to contain.

## A build that stops being scanned

Every other failure here is loud: a document that will not parse produces a
receipt saying so, and a scan filed against a name nobody declared is refused
rather than quietly creating one. **Silence is the failure that is not.** A
build nothing files against reports no new findings, closes nothing, fails
nothing, and every number about it holds still — which is indistinguishable
from a build with nothing wrong.

So it is asked rather than waited for. Every declared build is reported with
when a scan last arrived for it, longest-silent first, and whether that is
longer ago than the deployment allows. Ordering by silence rather than by name
is the point: the answer somebody needs is which build stopped, and an
alphabetical list buries it among the ones that are fine.

**A build nothing has ever been filed against is the same failure caught
earlier**, so it is reported too, measured from when it was declared rather
than from a scan that never came. A pipeline pointed at a name nobody declared
is refused loudly; a build declared and never pointed at anything fails
silently, and this is what says so.

**How long counts as quiet is a setting**, not a constant. How often a
deployment expects to be scanned is a fact about that deployment — nightly for
some, on a release cadence for others — and a threshold nobody agreed to
produces either an alert that is always on or one that never fires. It ships at
a week: long enough that a nightly build missing one night is not an alert,
short enough that a pipeline switched off is noticed in the week it happened.

**A release out of support is never reported as quiet** (RPT-04). It stopped
being scanned because it stopped being supported, which is expected rather than
a fault, and coverage that filled with those would stop catching the product
that dropped out by accident — which is the whole point of it.

It is still listed, and still says how long it has been: "not scanned, and that
is fine" and "not listed" are different answers, and only one of them is true.
The screen says so quietly rather than raising it.

**It is a person's question.** A pipeline key sees the receipts for what it
sent and nothing more, and when a build was last scanned by anybody is a fact
about the deployment rather than about that key's uploads.

*No decision covered this.* It was found by auditing the interface against the
approved mockup, which leads its inventories screen with a product that has not been
scanned for eleven days while nothing has failed.

## Which run answers an upload

A receipt says how far an upload got, and a scan run covers a *build* rather
than an upload, so deciding which run speaks for a given upload is a rule
rather than a lookup.

**The run that answers an upload is the earliest successful one to finish
after that upload was parsed.** A run that failed answers it only while nothing
has succeeded since.

The second half of that is not a detail. The first version took the earliest
run to finish after parsing whatever became of it, so a scanner that fell over
once poisoned every receipt already waiting on it — permanently, however many
clean runs came afterwards — and the inventories screen got steadily more wrong the
longer a deployment ran. A later clean run did read this document, because the
upload is the newest one its build holds, so reporting the old failure is
simply untrue. While every run since has failed the receipt still reports that
failure, which is the honest answer to "did anything come of my upload".

## What each run changed

A receipt says how far an upload got. What it did — how many issues opened and
how many closed — is counted when it is asked for rather than stored, from the
runs the findings already point at.

Counted as issues at components rather than as places, like every other number
here. **A run covers a build rather than an upload**, so several uploads can be
answered by one run; the count is attributed to the newest upload that run
covered and left off the rest, because one fact repeated down three rows reads
as three separate changes.

## A note on column types

Fixed-width character columns blank-pad on some engines, so a hash read back
carries trailing spaces that make an exact-match lookup fail. Variable-width
columns do not. Both cost the same for a value that is always the same length.

## What an inventory turned out to be made of

Two numbers are kept on a scan once it has been read: how many components the
inventory described, and how many of them anything placed in the graph. They
appear on the receipt.

**The pair, not either one alone.** A component nothing places is ordinary — a
producer emits the edges it can derive and records what it could not, and
inventing the rest would report dependencies nobody declared. A document that
places *none* of them is a different thing: it is a list rather than a graph,
and every finding it produces will be individually correct and unable to answer
"why is this here" about any of them. Only the ratio tells those apart.

This was already counted and went only to a log line. The design said the count
was worth keeping "because a sudden change in it says the producer's derivation
changed, which is worth seeing" — and nobody could see it. It is on the screen
that exists to answer what became of an upload, marked when nothing at all was
placed, because that is the case somebody has to notice rather than discover by
wondering why a dependency tree is empty.

## What a second kind of finding would arrive as

The primary input is an inventory a build produced, and the vulnerability data
is produced here rather than sent to us (SCP-01). Findings from a static
analyzer or a fuzzer are intended scope and **are not built** (SCP-11). What
has been decided is recorded here, because the parts that are expensive to
retrofit are the parts that were settled early.

**They are scoped the same way everything else is**: to a product, a release
and a variant. A finding that cannot say which build it is about cannot be
compared release to release, and comparison is most of what this tool is for.

**SARIF is the default interchange**, the way CycloneDX is for inventories, and
not the only path (ING-24). It is what the analyzers already emit, and taking
their native output instead would mean one reader per tool with nothing in
common between them.

**One adapter per source, normalizing into the internal model** (ING-25) — the
same seam the inventory producers arrive through. What that seam is for is that
the rest of the system never learns which tool produced anything.

**Some sources are pulled rather than pushed** (ING-26). A hosted analyzer has
its own store and no reason to call us, so a deployment holds credentials to it
and polls. That is a different shape from the upload path: there is no request
to refuse, so a source that starts returning nothing has to be noticed the way
a build that stopped being scanned is noticed, rather than by an error.

**Identity is computed here, never taken from the producer** (ING-27). The same
rule the inventory reader already follows for components, for the same reason:
an analyzer's identifier for a result is stable until the tool is upgraded or
the file is reformatted, and a finding whose identity moves is a finding that
reopens as new and loses every judgment made about it.
