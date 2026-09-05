# Findings

What a scan run found, and where.

Satisfies MDL-05, MDL-06, MDL-14, MDL-15, MDL-19, MDL-20, MDL-22 to MDL-30,
ING-02, ING-21,
ING-29, ING-30, ING-39, ING-40, STA-03, STA-04, STA-05, STA-08, STA-17,
RNK-01 to RNK-07, MDL-09, STA-01, STA-02, STA-06, STA-07, STA-09, STA-10,
STA-13 to STA-16, STA-18, REL-01.

## A scanner reports a package; we work out the places

A scanner says a package at a version is affected and stops there. It has no
idea the package sits at twelve places, because it never saw the dependency
graph — it was given a list.

Fanning one reported issue out across the places it occupies is ours. It is the
step where one line in a report becomes the number of decisions somebody
actually has to make, and it is derived from the edges the inventory described
rather than from anything the scanner said.

A place is the component and what directly pulled it in. One issue in a shared
library used by two things is two findings, because different consumers use
different parts of what they depend on and one may be affected where the other
is not.

### Reported against something we do not hold

Counted and reported rather than dropped. A vulnerability report that does not
match the inventory it was produced from is worth seeing — it means the two
documents describe different builds, and silently discarding the mismatch is
how that goes unnoticed.

## One issue, however many names

The same vulnerability arrives as a national identifier from one database and
an advisory identifier from another. Which one a report calls primary is a
preference of whichever source the scanner consulted, not a property of the
issue, so identity spans the names: every name an issue is known by resolves to
one row, and a decision made about it holds across all of them.

**The lookup is over every name a report supplies, not the one it led with.**
One scanner reports the national identifier alone; another reports its own and
knows the first as an alias. Matching only the leading name makes those two
issues, splitting the findings and every decision between them. That ordering
is what the test pins, because the other ordering passes either way.

An issue is filed under the most widely recognized of its names, so what a
person sees is the name they will find in an advisory. The rest are kept, and
any of them finds the row.

Identifiers are compared in one case. Every scheme that issues them treats them
as case-insensitive and reports disagree about which case to write, so two rows
for one issue would otherwise be a matter of which scanner ran.

### When two held issues turn out to be one

A report can supply the alias that joins two rows recorded separately. That is
a merge of findings and decisions already made against both, so it is refused
rather than done quietly as a side effect of reading a scan.

## Severity is a word

What is filed against a finding is the **word**, and often with the method
given as unspecified. That is what ranks and what sets a deadline.

A number is taken as well where the report carries one: the first rating
stating both a score and the vector it assumed, kept beside the word with the
worst anybody claimed winning (RNK-05, which reversed the premise that a
scanner supplies no scores). The vector travels with the number so that what
the number assumed is readable rather than lost.

## Fix state is kept

No fix available, upstream declined to fix, and a fixed version exists are
three different situations to whoever is triaging. "Upstream will not fix this"
is a permanent condition that changes the outcome somebody should reach, and it
is invisible if the only thing recorded is that a fix is absent.

## A flaw in our own product, recorded by hand

Not everything is something a scanner found. A vulnerability in what this
deployment ships is usually known here before it is known anywhere, and it has
to be triaged, assigned, decided, clocked and reported like everything else —
so it is a finding, of a different kind (MDL-22).

**It is filed under an identifier this deployment mints** (MDL-24): the
product's name, the year, and a number — `SONIC-2026-0001` — which is the shape
a vendor advisory already takes. There is nothing else to file it under. A flaw
nobody has published has no CVE, and waiting for one would mean the record of
what we knew starts after the work does.

Nothing is configured for that prefix. The product has a name people type, and
a setting for it would be a second thing to keep in step. When a CVE is
assigned later it is recorded as another name for the same issue and the issue
is then filed under the CVE — and because identity is the issue rather than
what it is called, nothing moves: not the finding, not the decisions, not the
approvals.

The number is read and used inside one transaction, so two people recording at
the same moment cannot be handed the same one; the second waits, reads the
first's row, and takes the number after it.

**It starts undisclosed, and recording one asks for the private triage right**
(MDL-25). That is the case this exists for, and defaulting the other way makes
the dangerous mistake the quiet one. The two triage rights are separate
precisely here: somebody who may argue about known issues in shipped components
has not been handed the ones nobody has announced. A finding that is already
public is a flag on the request and asks for the ordinary right.

**A severity is checked against the words rather than folded.** A report's is
folded — anything unrecognized becomes medium, because a scanner that rated
nothing is silent and silence is not a claim that something is mild. A person
typing "urgent" is not silent. They are wrong, and folding it would replace
their judgment with one nobody made.

**What carries it is a component of the build, or the build itself.** Naming
nothing puts it on the root, which is the honest answer where the flaw is in
how the pieces fit together rather than in one of them. Naming something the
build does not hold is refused: that is a claim about somebody else's software.

**A name the build holds more than once is refused with the choices**, and the
version — with the ecosystem where two components share one — settles it. A
name is not unique within a build and not rarely: a real switch image ships
three vendored copies of one library, and thirteen names in it are held at one
version by two components, a source repository and the package built from it.

This resolution goes through the same lookup every other component reference
takes, rather than a second one beside it. It did not, and the one here took
the first row a name matched — the guess that was already measured wrong
elsewhere, where two of three findings answered about a version nobody asked
about. Recording is where that guess costs most: a lookup that picks wrongly
answers the wrong question and can be asked again, and a flaw filed against the
wrong version of three is a record that reads as deliberate.

**A person closes it, because nothing else can** (REM-28). Resolution is
computed from scans everywhere else, and that rule is right: it removes the gap
between somebody marking work done and the work being done. It also needs
evidence, and here there is none — the one path that closes a finding is a scan
applying what it found, and it passes over anything a person recorded on
purpose, because a run is the authority on what it found and it found none of
this.

So the computation has no input, and what came of that was not "not yet
resolved" but a finding that stayed open forever: a fix that shipped three
releases ago still reading as present, invisibly, because every screen looked
right and the wrongness was only that it never left.

The exception is exactly that wide. **A scanner's finding is refused**, because
for that one the evidence exists and overruling it is what computing resolution
was chosen to prevent. It is closed **per build**, across every place the issue
sits at there, because a fix ships in a release. It carries **who, when and
why**, and a closure with no reason is refused for the same reason moving a
disclosure date without one is. It asks the **same right recording it asked**,
checked against each row rather than the issue: somebody who may argue about
disclosed findings has not been handed the undisclosed ones here either.

**Nothing reopens one.** Undoing a closure needs somewhere to keep the closure
that was undone, and that is a table rather than a column — a closure whose
history nobody can see is the state the rest of this design keeps refusing to
ship. Closing is a considered act.

**Each refusal is the caller's to fix, and says so.** A name that reaches
nothing, a name that reaches several, a summary of nothing but whitespace, and
a build with no contents to record against are four different answers. The
third is the one worth naming: a minimum length passes whitespace, so it
arrives from a request rather than only from inside this process, and it used
to be answered as something having gone wrong at our end.

## One recorded flaw, in every build that ships it

Satisfies MDL-27 to MDL-30.

The same code goes out on several lines and as several variants at once, so a
flaw in it is not a fact about one build. It is filed under **one identifier
this deployment mints** — the product's name, the year and a number, which is
the shape a vendor advisory already takes — and it opens **one finding per
build**.

That is the shape a scanner's findings already have. So a flaw somebody
recorded lists, ranks, comes due, carries decisions, groups across variants and
appears in a comparison exactly as a reported one does, rather than in a scheme
of its own that everything downstream would have to know about.

**Every build is resolved before anything is written.** A component name one
build holds and another does not is a question about which builds are affected,
so it is refused and names the build — rather than recorded against some of
them and silently not the rest, which is the version of this that looks like it
worked.

**One product.** The identifier is minted per product, so a flaw in two
products is two records. Builds of different products are refused rather than
resolved by taking the first.

**Every row gets the same embargo, rank and deadline.** They are the same flaw;
a deadline that differed per build would be the tool deciding that one release
matters more than another.

**A severity may be left unstated** (MDL-28). Somebody recording what they have
just found has not decided it is mild, and making them choose a word to get the
record written is how a guess ends up stored as a judgment. It is not given
*no* deadline: the windows already answer for a severity they do not recognize,
which is what every unrated finding from a scanner gets, and a finding that is
never late is one that is forgotten.

### The score is a vector, and the number comes from it

What is stored is the CVSS base vector and a score **derived from it** (MDL-29).
A score is never taken alongside a vector: two values a caller states separately
can disagree, and afterwards nothing says which was meant — the number is what
sorts and the vector is what somebody can argue with.

**Base metrics only.** Temporal and environmental scores describe a moment and a
deployment, and the deployment reading a finding is not the one it is about.

**Version 3.0 and 3.1, and anything else is refused by name.** They share a base
formula; version 4 does not and version 2 is a different scheme. A vector scored
with the wrong formula produces a number nothing downstream can tell apart from
a real one, which is worse than a refusal.

**Rounding is the scheme's own, in integer arithmetic.** Doing it in floating
point gets a different answer for some inputs, because a value that should be
exactly 8.6 is not representable and a naive ceiling returns 8.7.

**An unstated vector is not a score of zero.** Zero says "harmless", which is a
judgment nobody made during early triage.

**Weaknesses are recorded as given, against no catalogue** (MDL-30). Trimmed,
upper-cased and de-duplicated, and otherwise whatever somebody meant. What they
are for is making a set of findings comparable to things outside this
deployment, and a list that refused an identifier it had not heard of would
refuse next year's — arriving as a failure to record a flaw.

## How a finding was reached, and why it decides what to do about it

A scanner reaches a finding one of two ways, and on a distribution's package
they mean very different things (MDL-26).

**By advisory**: the people who package it published one for this package in
this ecosystem. It counts the packaging — "fixed in 1.37.0-r15" — so what it
says about a fix is about the version actually installed.

**By identifier**: a published identifier compared against an upstream version
range. It knows nothing about packaging. A distribution backports fixes without
moving the upstream version, so `busybox 1.37.0-r14` and `1.37.0-r15` are the
same release to an upstream range, and the match fires whether or not the patch
is already in. Neither confirmed nor refuted — somebody has to look.

**The range and the source are kept with it**, because "somebody has to look"
is easier to act on with the thing to look at in hand. The range the match
fired on, read beside the version that ships, is the argument in a line: a
range naming no packaging revision against a version that has one. The source
is which body of data answered, as the scanner names it, which is finer than
these two words and says which of the two answers above it is. Neither is
compared against anything — that would need an ordering per ecosystem, which is
a different project.

That distinction is the first question anybody asks about a distribution's
packages, and the scanner answers it in every result. It was being discarded,
which left a finding nobody had confirmed looking exactly like one the people
who package it had.

**Unrecognized reads as the weaker of the two.** A word this does not know is a
word whose strength nobody has checked, and reading it as authoritative is the
direction that hides something.

**Unknown is not unconfirmed.** Where a scanner said nothing, nothing is
claimed — and the working list of "somebody has to look at this" excludes it. A
list of that kind that quietly holds everything nobody classified is a list
nobody can work down.

**Where the match came from is kept on the finding, not on the issue.** One
issue reached through two ecosystems has two answers and the issue can hold
only one, so an issue first seen in a Debian image and later matched in an
Alpine one showed Debian's tracker against the Alpine package. The issue still
carries where it is written up; the finding carries where *its* match came
from.

**One answer per group.** Every place of an issue at a component comes from the
same line of a scanner's report, and the applier writes that line to all of
them, so there is nothing to reconcile.

## A scan governs what a scan found, and nothing else

A run is the authority on what it reported. It opens what it found and
**closes everything open that it no longer reports** — that is how a component
leaving a build stops being a finding without anybody saying so, and it is the
whole reason the record can be trusted to describe the build as it is now.

So the sweep has to be bounded by what a scan can have an opinion about. A
finding carries a kind saying what produced it, and the sweep covers only the
kind a scan produces.

Without that bound, a finding somebody recorded by hand — a flaw in what this
deployment ships, which no scanner knows about — is closed by the first run
after it is written. Silently, and with a closure reason that reads like the
issue went away: the row is indistinguishable from a component that stopped
shipping. Nothing reports it, because from the sweep's point of view nothing
went wrong.

The kind exists ahead of the second thing to put in it for exactly this reason.
A model that assumed every finding came from a scan could not take one that did
not without changing how closure works, and closure is the part that is easy to
get subtly wrong and hard to notice.

## A finding carries when it opened

The row holds the moment it became true, rather than reaching the run that
opened it for a timestamp.

Three passes did the reaching, and all three did it as an inner join: the
trend, the deadline rewrite when the policy changes, and the urgency recount
when an issue's rating moves. A finding with no run therefore did not appear in
any of them — not wrongly, but **absent**. That is the worse failure of the
two. A number that is wrong invites somebody to check it; a row that is missing
looks like there was nothing to say.

It is also less work. Each of those joins existed to read one column, and two
of them grouped on the run only so they could reach it — so the run is one
table fewer in each, at the same cardinality, because every finding a run
opened carries that run's start.

**The run is still recorded where there is one**, and it is what "what did this
run change" is counted by: that question is about a run, and a finding no run
opened is correctly absent from the answer.

**And when it closed, for the same reasons and one more.** Closure was readable
only as "a run closed this", so the trend reached the run for the moment with
the same join the opening used to — and a finding closed by a person would have
been absent from the chart exactly as one opened by a person was. The stronger
reason is that with closure spelled that way, a finding a run will never close
could not be closed at all: the column that says a finding is over could only
be filled in by something that never looks at it.

So the row carries the moment, and what closed it sits beside as provenance —
the run, or the person and their reason. Every index that carried the run in
order to answer "is this open" carries the moment instead, because an index on
the provenance answers a question nobody asks. A closure a scan wrote is dated
by the run's own start, which is what the opening side does: a finding's life
is measured against the runs that observed it rather than against when a write
happened to land.

## Findings are held over intervals, like the graph

Open until closed, never deleted. Re-scanning happens nightly against a
vulnerability database that has barely moved, so a run that found the same
things writes nothing at all — the same property the graph has, for the same
reason.

### One writer at a time, per target

Recording what a run found begins by taking the target row, before anything is
read. Two runs against one target can be in flight at once — the queue hands
different jobs to different workers by design — and without a hold, both read
the same open findings, both compute the same difference, and both write it.
That leaves two open rows for one finding, which everything downstream reads as
two problems, and which two separate triage decisions can be made about.

An ordinary update of that row is a lock every supported engine honors, so the
second worker waits rather than racing. Applying a graph takes the same row for
the same reason, in its own column: they are two passes over one target and
each needs its own hold.

The test runs two overlapping applications and counts what ends up open. It was
checked by removing the hold, which reproduces the double-open on all three
server engines.

### A finding that is already open still moves

A fix appears. Upstream declines to fix it. The build answers it. None of those
open or close anything, and somebody waiting on a fix is waiting for exactly
the first — so a run compares what it found against what is recorded and
updates the parts that can move, stamping when they moved.

Only those parts are compared. Everything else is what makes it that finding
rather than a different one, and a change there would be a different finding.

A finding carries when it last moved rather than relying on a record kept
elsewhere. One open for years outlives any log that gets purged, which is the
whole reason current state holds its own summary.

### A closure records why

| Reason | What it means |
|---|---|
| Removed | The component is not in the build any more |
| Upgraded | Its upstream version moved — the version a vulnerability is matched against |
| Revised | The shipped version changed while the upstream version did not, which is what a carried patch looks like from outside |
| Superseded | The upstream version moved and the issue came with it: this row closed and the same issue is open against the new version. Told apart from Upgraded because they are opposite answers to "was this fixed", and conflating them put one issue in a release comparison as both fixed and newly present |
| Fixed | Somebody said a flaw they recorded by hand is fixed here. The only closure a person writes, because a run is the authority on what it found and it never found this (REM-28) |
| Unexplained | The component is present and unchanged, and the scanner stopped reporting it |

**The last one is always reported and never suppressed.** There is no volume at
which "we cannot account for this" stops mattering, and folding it into the
others is how a scanner fault or a silently changed database becomes invisible.

The reason is worked out from what the build now contains, compared against
what the finding was about. That comparison reads the departed component from
the component catalog rather than from the current build — the reason it is
closing is usually that it is no longer there to read.

## What the build already argued

A build sends what it has already decided does not apply to it — usually
because it carries a patch. Those claims are **kept as data when the scan is
read**, not left in the document they arrived in.

They have to be. A nightly scan's documents are discarded once read, the
vulnerability scan runs after that, and it runs again on a schedule against
data that keeps moving. A claim that lived only in the file would be gone by
the time anything needed it, and every carried patch would come back as an
outstanding vulnerability on the first re-scan — which is exactly what applying
the claims here was meant to prevent.

A statement naming several packages becomes several claims, one per subject.
Each is about one thing, because that is what gets matched against a component,
and because a claim that reached one package and not another is worth being
able to see.

Claims are held over intervals, like everything else a build describes. A build
argues the same things night after night, so re-sending them writes nothing.
Withdrawing one closes it rather than deleting it: what a release argued is a
question asked years later.

### A covered finding is marked, not dropped

**The finding is created and stays visible**, carrying the claim that answers
it. This is the whole reason the claims are applied here rather than upstream:
a finding that simply never arrived is indistinguishable from a scanner fault,
and that is the bucket nothing is allowed to explain away.

| The build says | What happens |
|---|---|
| It carries a fix, or the vulnerability does not apply | The finding is marked with the claim and is not work anybody has to do |
| It is affected, or it has not decided | Nothing is marked. The build is telling us it looked, which is information rather than an answer |

Where two claims cover one finding, the one that arrived **attached to the
component** wins over one that named something we had to match — the first
knows exactly what it is about, while the second may name a whole source tree.
Where neither is attached, the claims are read in a stable order so that which
one is recorded does not change between runs.

## The intervals are the change record

Change over time is a primary query rather than an audit trail, and the graph
and the findings answer it directly: every node, edge, finding and claim
records the run that opened it and the run that closed it. Asking what changed
between two points is a query over those, not a walk through a separate log.

So there is no second table duplicating them. What a purgeable event log is
for is the high-volume record that has no interval of its own — what people
did, rather than what a scan found — and that belongs with triage rather than
here. The rule it exists to protect still holds either way: current state is
never purged, and it carries its own summary fields so that dropping anything
older cannot silently break a finding that has been open for years.

## What the fan-out actually costs

Measured on one real switch image, scanned live:

| | |
|---|---:|
| Components scanned | 8,523 |
| Distinct vulnerabilities | 5,652 |
| Components carrying at least one | 437 |
| **Findings** | **335,021** |
| …of which one kernel | **305,487** |

The kernel is 91% of it: 4,849 issues in it, and 62 modules built against it.
Those edges are real and the producer emits them deliberately, so that anyone
can reason about kernel-ABI risk — but which module is loaded does not change
whether the kernel has a bug, and nobody triaging wants the same judgment
sixty-two times.

The model is not wrong: a finding is a component at a place, and those are the
places. What the number settles is that **grouping cannot be an afterthought in
how this is presented**. A person decides about an issue in a component; the
decision is recorded against every place it covers, and which places those are
is something they can see and narrow.

So what is read back is one row per issue in a component, carrying how many
places it occupies and how many of those the build has already argued about.
The same image reads as 7,906 rows rather than 335,021 — a screen a person can
work through, over a record that kept every place.

The grouping is done by the database rather than after the fact. A page of
fifty grouped rows read out of a third of a million findings is not a page of
fifty findings, and counting in the application would mean reading all of them
to show any of them. Only the names are fetched afterwards, in a second pass:
aggregating text is spelled differently on every engine, and none of the
spellings is worth an engine-specific path in the core.

## A finding carries its own evidence

There may be thousands of findings and very few people to work them. What
decides whether that queue gets worked is whether somebody opening one can act
on it, or has to go and find out about it first — so everything a report says
about an issue is kept.

Measured on a public switch operating-system image, per scan of 7,917 findings:

| | |
|---|---:|
| Known to be exploited | **5** |
| Published likelihood of exploitation | 7,835 |
| Where the issue is written up | 7,917 |
| Description | 7,879 |
| Severity as a number, with the vector it assumed | 6,136 |
| When a fixing version became available | 5,390 |
| References | 11,593, of which **420 are patches** |

The first row is the point. Five of nearly eight thousand are being exploited,
and that number is what turns an impossible queue into an afternoon — it was
being parsed and thrown away.

None of it is recoverable later. A report is not kept once it has been read,
so anything dropped at ingest is dropped for good, and the cost of keeping it
is one pass over output already being parsed.

### What the scanner pointed at is not all there is

Reading a finding also offers addresses nobody supplied (UIX-52), worked out
from the two identifiers already held: the issue's name and the package's. The
issue's own record and the record under each other name it goes by; where a
distribution packages the component, that distribution's answer about the
issue; and the package's page in its own ecosystem.

They are derived at read time and stored nowhere. An address worked out from
two names cannot go stale while the names are right, and storing it would put a
second copy of the templates somewhere to fall behind the first.

Kept apart from the references a report carried, because the provenance is a
different claim and the difference is the reason this exists at all: what a
scanner points at is whatever its data happened to carry. On our own image the
references for a package matched by identifier were a vendor bulletin, two
gists and two mailing-list attachments, the issue's write-up was another
distribution's tracker — the artifact MDL-26 anticipates — and the record for
the identifier on the screen appeared nowhere.

Nothing here fetches any of them. They are addresses handed to a person, so
the restriction on what this reaches over the network is untouched (SEC-07).

### Telling a patch from a write-up

A report gives a flat list of references and does not say what any of them is.
They are classified by the shape of the address — a commit, a pull request, a
diff — because somebody deciding whether to backport rather than upgrade needs
the change itself, and hunting for it by hand is the step that does not happen
when a thousand findings are waiting.

The guess errs toward saying less. An address that is not recognized is
reported as discussion rather than asserted to be a patch, because a wrong
label costs more than a missing one: nobody is misled by an unlabelled link.

### What a later report adds, and what it may not take away

Reports disagree and arrive in an order nobody controls. So a later one fills
in what an earlier one did not know and overwrites nothing — otherwise what is
stored would depend on which scan happened to run last.

Two things are not descriptions and do not work that way.

**Known-exploited moves forward only.** It is a claim about the world rather
than a description of an issue: a later report not mentioning it is a gap in
that report, not the exploitation having stopped.

**A score or a likelihood keeps the worst anybody claimed.** These were
overwritten outright, which is exactly the dependence on scan order the rest of
this exists to avoid. Filling only the gap would have been no better — just
dependent on which report arrived first instead of last. A maximum is the only
answer that comes out the same whatever the order, and it errs in the safe
direction: a report saying something is worse is news, and one saying it is
milder is a gap in that report. There is a test that puts the same two reports
through in both orders and asserts they agree, and it fails on the overwriting
it replaced.

It is written out rather than using the engines' greatest-of function, which is
not agreed on when one side is absent: two of the four answer "unknown" where
the useful answer is the number that is known.

### A place names the claim standing on it

Each place a finding sits at carries the live claim covering it, not only the
decision. At most one live decision stands per combination of code, so the two
reach the same row — but the claim is what a person acts on and what the
finding shows, and without it a claim shown there can count its places and
cannot name them.

## A row says how far it has been decided

Each row of the findings list carries how far we have decided it, in the same
four words the state filter takes and by the same definition: undecided when
no place has a decision of any kind, waiting when a claim stands proposed and
nobody has agreed, agreed when every place is answered by a standing decision,
lapsed when a decision here stopped applying and nothing replaced it. Some
places approved and the rest never decided, with nothing waiting or lapsed, is
none of the four: the row carries no state, and the interface labels it partly
decided, since some of its places are agreed and the rest have never been
decided.

A live decision covers a place at the versions it was keyed on and no other,
so these counts match a live decision by product, issue, place and both
versions, and match a lapsed or withdrawn one — which holds no key, its
versions by definition no longer matching — by place alone. Matched by place
alone, a decision approved against one build's version read as agreed over the
next build shipping another at the same place, while everything that asks
whether a decision actually applies said it covered nothing there.

It is counted by the same definition the filter uses, so the row and the
filter that found it cannot disagree, and a page of fifty costs no query per
row. Before this the row carried only what the build had argued away, and an
interface reading that as the decision state showed "undecided" over
forty-four proposed records.

The page itself is read in two statements. The first groups every open
finding in the build by issue and component and keeps the fifty most urgent,
reading only the columns a covering index on `finding` holds, so it is one
walk of that index however large the build is; the total rides on the same
statement as a window count over the groups, so it is counted through
exactly the page's narrowing without a second walk. The second reads what
the page shows about those fifty groups and no others: likelihood and
score, the four decision counts above, how many ways down there are, the
fix. Every filter narrows both statements
through the same clauses, and none of them needs the issue or the component
joined under the grouping — a rating or a name is asked as a membership test
against the table that holds it. The decision-state filter is built from the
decisions outward: the open rows that have a decision of ours in this
product, with what kind, joined to the grouping by the finding's identifier,
rather than a lookup per open row. Measured on the full-size build, 241,479
open rows in 7,329 groups: the page went from 2.0 s to 0.12 s, and asking
for what is undecided from 2.3 s to 0.18 s.

A row also says when a live claim at one of its places is with its author,
sent back for more. That is the row a proposer is looking for in the list, and
it is the one the queue no longer shows.

## What somebody with an hour sees first

A findings list has to open on what matters. Ordering by how many places
something occupies puts whatever is most widespread at the top, which on a real
image is the kernel — everywhere, and not therefore the thing to look at first.

So each finding carries an urgency, worked out when a scan is applied and read
back as written. Computing it while reading would mean joining every signal it
is made of, for every row, on every page of every list.

**It is worked out from what is on record about the issue, not from the report
being applied.** A report is one source's account of one moment: it may omit
that something is being exploited, or carry a score lower than last week's
report gave. What the issue holds is the worst anybody has claimed, moving only
toward worse, plus a rating of ours where somebody has made one. Ranking from
the report in hand would make the order depend on which scan ran last, and
would move a finding up and down as sources disagreed.

**What storing it buys, stated exactly.** The list groups a target's open
findings and orders by the worst urgency in each group, so the sort is over
groups and no index can serve it however it is built. What an index does remove
is the row lookup behind the aggregate, which is the part that scales with the
size of the image — so there is one covering the target, whether the finding is
open, and the number. An earlier version of this claimed the index served the
sort, and no such index existed.

Four signals, in this order:

| | |
|---|---|
| **Known to be exploited** | The difference between a risk and an incident |
| **Reaches customers** | A critical in something only the build system runs matters less than a medium in what people install |
| **Severity** | How bad it would be if it happened |
| **Likelihood of exploitation** | Which of two equally severe things to look at first |

**Severity above likelihood, measured rather than assumed.** The original order
had them the other way, and on a real image that put a 2004 negligible with no
score at all above every one of 379 criticals: its likelihood was 0.80 where
theirs topped out at 0.073, and any difference in a higher signal wins
outright. Multiplying the two — the published practice where these scores are
well spread — was tried next and reversed on the same image: 95% of its open
issues sit between 0.001 and 0.01 likelihood, so multiplying mostly amplifies
what is inside that spike and mediums jump criticals on noise. Severity leads
and likelihood orders what is equally severe. What that gives up is letting a
very likely medium jump a high; the case that actually matters, something known
to be used, is a fact rather than a forecast and ranks above both.

The number is **packed rather than weighted**: each signal owns a range of
digits, so a signal never trades against a lower one. That is deliberate, and
the reason is explainability — somebody has to read a position and see why, and
"it scored 0.4 higher on a weighted sum of four things" is not something anyone
trusts or argues with. Packing gives a rule statable in a sentence.

The trade is that a small difference in a higher signal beats any difference in
a lower one. Clearly right for exploitation, arguably strong for where
something ships, and exactly the judgment to make against a real queue rather
than in the abstract — which is why it is one function.

A signal reported out of range is clamped. A source sending something
impossible would otherwise carry into the band above and rank as though it were
being exploited, which is the one thing this must never invent.

**Why something ranks where it does travels with it.** A ranking nobody can
explain is one people stop trusting, and then they sort by something else and
lose the point of the order entirely.

Where a report rates an issue only in words, the word stands in for a number,
so a finding rated in words does not sort below everything rated at all.

A group is one decision about one issue in one component, so it takes the
urgency of the worst place it covers.

### What is known changes under a finding that has not

An issue gets worse after the findings against it opened. A score is revised, a
likelihood is published, a name lands on an exploited catalog — none of which
is a change to the software, and all of which change what to look at first.

**The rank follows the issue.** A scan that finds the record has moved rewrites
the urgency of every open finding of that issue. Held as it was at opening, the
list would order by a number nobody could reconcile with the row drawn beside
it.

This does not make the rank flap nightly, and the reason is the paragraph
above: the stored signals only move toward worse, so a rewrite is a real
event rather than two sources disagreeing in turn.

**The deadline follows only exploitation**, because that is the only signal in
it — severity is the flaw, exploitation is a fact about the world, and neither
score nor likelihood sets a clock. A clock reset whenever a number was revised
would never arrive, which is the same failure as resetting it nightly.

**A recount runs from the scan that learned the fact, never from when the
finding opened.** An issue that becomes exploited after six months, clocked
from the opening, would be given three days that ran out five months ago — a
deadline nobody could have met, applied across the estate, which is exactly how
an overdue figure stops being read. Counted from the scan that learned it, the
deadline says what it should: three days from now.

This is also how the published exploited catalogs are used in practice. Their
due dates run from the date an entry was added, not from when anybody first
shipped the affected package.

## The same code built two ways is one piece of work

A product is often built several ways — a chip variant, an architecture — and
the builds are mostly the same software. A judgment carries no variant: it is
keyed on the product, the place and the upstream versions, so answering it on
one build answers it on every build of that product holding the same code.

So the screens that ask "what is there to do" show **one item per issue in a
component in a product**, not one per build (REL-01). Listed per build,
importing a second variant doubles the list while doubling none of the work —
and a queue twice as long as the work is one people stop reading. That is not
hypothetical: it is what happened the day a second variant was seeded, and the
list went from 7,354 items to 14,681 against the same estate.

**Genuine differences break out by themselves.** A component row is one name at
one version, shared by every build that ships it, so two variants at the same
version group together and two at different versions do not. Nothing has to
decide which case it is looking at.

**The product stays in the key.** A decision is a claim about one product's
code, so two products shipping the identical component at the identical version
are two judgments — answered separately, and usually by different people.

**An item still names a build**, because a screen has to link somewhere and an
action has to name a finding. It is one of the builds rather than the only one,
chosen stably so a row does not move between reads, and the count of builds is
carried beside it so a screen can say "2 builds" rather than presenting one of
them as the answer.

**The findings list follows this too, and did not used to** (UIX-53). It was
the one list confined to a single build, so the property this section
describes could be read everywhere except on the screen where the work is
actually done: the same component at the same version in two variants was two
rows, in two visits, and nothing said they were one thing. It now answers for
whatever is selected — one build when the branch and the variant are named,
every build under the product when either is not — counting places across all
of them and naming one build with the count beside it, exactly as the lists
above it do.

**What that is worth, measured on two variants of one switch image**: 7,587
rows on one and 7,610 on the other, which is 15,197 rows read one build at a
time and no way to tell how much of it was the same work twice. Across the
product it is 7,612 — so 7,585 of those rows were one piece of work seen
twice, and 27 were the genuine differences between the two builds. Finding
those 27 was not previously possible from this screen at all.

**What it gives up across builds is the way down.** A dependency chain belongs
to one build's graph, and a row covering three builds is reached three ways, so
the column that names the two ends of the chain is empty there rather than
filled from whichever build the row named. That is the one thing in this
section that genuinely cannot be answered for a product, and it is why the
screens that are *about* a chain — a finding, the dependency tree, deciding a
place, deciding several at a component, and the list of inventories — still
require a whole build. Narrowing the list to a subtree is refused for the same
reason, rather than answered with the empty list that walking no build would
produce.

**And acting on it acts on all of it.** Assigning covers every build of the
product holding the component, not the one named in the path — the path says
which finding is being looked at, and what is assigned is the work it belongs
to. Assigning one build would leave the identical work unassigned beside it,
and the person would hold half of what they think they hold.

**And counting it counts the same way.** How much each person is holding is
counted in pieces of work, like every list it links to. Counted in findings it
was a different measurement wearing the same word: one kernel flaw assigned to
one person read as forty-eight held against her on the summary and as the
single item it is in her own list. The larger number is not a stricter version
of the smaller one — it moves with how far a component fans out through an
image rather than with how much anybody has to do, so it says nothing about
whether somebody is keeping up.

The fan-out is still reported, beside the work rather than instead of it: one
thing to answer, forty-eight rows to write, and both are true. **Late is
counted the same way** — a piece of work is late when any of its places is,
because a group with one late place among forty is late, and calling it a
fortieth of one is a number nobody acts on.

## A finding carries what it takes to act on it

There may be thousands of findings and very few people, so the difference
between a finding that carries its own evidence and one that sends somebody to
a search engine is the difference between a queue that gets worked and one that
does not.

One request returns everything held about an issue in a component: the
write-up, the advisory, every reference the data carries, the score and the
statement of what that score assumes, whether it is known to be exploited and
how likely exploitation is, what kind of flaw it is, what upstream has done
about it, and every place the component sits at here.

**References list patches first.** Somebody deciding whether to backport rather
than upgrade needs the change itself, and hunting for it among the write-ups is
the step that does not happen with a thousand findings waiting. What is a patch
is guessed from the shape of the address, erring toward saying less.

**The weakness classification is kept** where the data carries it. It groups
findings by the shape of the mistake rather than by the package it landed in —
eleven use-after-frees in eleven libraries are one conversation, and eleven
unrelated issues in one library are not. A report usually names the same
weakness from several sources, so what is stored is deduplicated and ordered;
otherwise a re-scan would rewrite the row having learned nothing.

## A bump that did not reach the fix

Two things are already held and never put together: that a component's upstream
version moved since the previous run, and what version the data says fixes the
issue. When a finding stays open across a version change and the shipped
version is not the one named as fixing it, that is somebody's remediation
failing to land.

Today it surfaces as a lapsed decision and a finding to judge again, which is
aimed at a triager. The useful reading is aimed at whoever did the bump, and it
says the bump fell short. For a producer that patches and bumps rather than
upgrading wholesale, this is the failure that otherwise goes unnoticed for a
whole release cycle.

**A version change is not an update in place**, which is what makes this cheap.
Component identity carries the version, so a bump closes one finding and opens
another — both versions are in hand at the moment of the change. The old row
records that it was *superseded* rather than upgraded, and the new row records
what it arrived from, so saying "3.7.0 → 3.9.0" costs no second query.

Telling those two closures apart is not a refinement. Recording a bump that
fixed nothing as "fixed by upgrading" put one issue in both the fixed and the
newly-present column of the same release comparison — and that comparison is
written to go straight into release notes.

**It is stated as inequality, not ordering**, and that limit is real rather
than a simplification worth removing later without noticing. Nothing here can
compare 3.0.12 against 3.0.14: there is no version comparison anywhere in this
codebase, the fixed-in field is free text and is sometimes a list of versions,
and doing it properly means per-ecosystem ordering — Debian epochs, RPM
release segments, semantic versions, and the ecosystems that follow none of
them. The cheap form still says the useful thing: this moved, and it is still
not the version that fixes it. Ordering would sharpen the wording and change
nothing about when the flag is raised.

**Where it shows up.** On the finding, as the version it arrived from. And in
a release comparison's still-present column, which is the reading aimed at
whoever did the bump rather than at a triager — the same failure seen from the
other side of the release. It does not yet appear in the review queue, which
lists decisions rather than findings; surfacing it there means carrying the
finding's own history onto a queue row, and that is not built.

The inverse — a component sitting at or past the named fix while the scanner
still reports the issue — would mean the scanner and the fix data disagree,
which is not a triage question at all. **It is not detected**, and cannot be
under the limit above: deciding whether a version sits past another is exactly
the ordering this refuses to do. Written down because the rule is easy to state
and reads as though something enforces it.

## Several disappearances at once are one fault

A finding that closes with its component still present and unchanged is flagged
on its own. Several of them in one scan additionally raise a scan-level
warning, which says nothing the individual flags do not — only that the likely
fault is one broken scan rather than a dozen independent oddities, so nobody
spends the morning chasing them separately.

A count rather than a proportion. On a large image a handful of genuine
disappearances is ordinary and a handful of unexplained ones is not, and
dividing by the size of the image would hide exactly that.

## Scanning is separate work from reading

An inventory is read once, when it arrives. It is scanned again and again, as
the vulnerability data moves underneath it — a release built a year ago has the
same components it always had and a different answer every month. So reading an
inventory leaves a scan to be done rather than doing it, and the two are
separate jobs with different rhythms.

The scanner is given what we stored, not the file the build sent. That file is
not kept for a moving line, so anything not stored can never be scanned — which
is the argument for capturing a component's second identifier scheme at ingest
rather than when a scanner first asks for it.

The product itself is left out of what the scanner sees. It is not a package
any vulnerability database has heard of, and including it invites a match on a
name that happens to collide.

## A run is recorded whether or not it found anything

Which scanner, which version of it, which vulnerability database, and whether
it ran here or arrived from a producer. Counts are only comparable between
products measured the same way, so a portfolio report that averaged two
scanners without saying so would be a rumor rather than a report.

A run that failed is recorded as a run that failed, with the reason. A scanner
that stopped working is otherwise indistinguishable from a product that stopped
having problems.

## What a year of nightly scans does to it

The interval storage was shaped so that a rebuild changing nothing writes
nothing, and a test asserts that. What nobody had checked was the shape after a
year of them — `DECISIONS.md` §4 recorded it as unmeasured, with the design
"should be fine" and *should* doing the work. It has been measured now.

**The model, stated because every number below depends on it.** A build of 700
components, each sitting in 34 containers, so 23,800 places; 260 issues open at
the start; 365 nightly rebuilds; 1% of components changing version each night;
three new issues a night arriving from the vulnerability database and matching
something already shipped. That is a real image at about a tenth of its size:
6,845 components become 700, 241,021 places become 23,800, and roughly 2,600
issues become 260. The shape is kept and the constant shrunk, so the *growth*
is measurable in an hour.

The earlier text here said a twenty-seventh, which came from dividing a real
image's places by this model's first-night finding rows — two different
quantities. Every ratio the model actually states is ten.

**The table grew 16.8 times over the year**, from 8,840 rows to 148,614. The
graph grew alongside it: 23,834 edges to 110,466, and 736 nodes to 3,284,
because a component whose version moves opens a new node and 34 new edges while
the old ones stay as closed intervals. Neither is a leak — every row is an
interval somebody can ask a question about — but a deployment sizing a disk
should know the shape is multiplicative in consumers, not additive in
components. Over the year 2,548 component versions moved, which is the whole of
what the nights cost.

Where it ends up, after 365 nights, on each engine:

| | findings list | running out | trend | a night, average | a night, worst |
|---|---:|---:|---:|---:|---:|
| SQLite | 27 ms | 72 ms | 419 ms | 0.31 s | 0.51 s |
| PostgreSQL | 24 ms | 75 ms | 136 ms | 0.67 s | 1.11 s |
| MySQL | 65 ms | 100 ms | 259 ms | 4.82 s | 12.56 s |
| MariaDB | 24 ms | 115 ms | 215 ms | 0.32 s | 0.83 s |

**Read the read columns as an order of magnitude, not as a benchmark.** Each is
one sample, and the harness happens to take two of them seconds apart on
identical data: MySQL's trend was 1.19 s and then 259 ms, MariaDB's findings
list 212 ms and then 24 ms. A ninefold spread between two readings of the same
query on the same rows is what a single figure here is worth. What the run is
for is the *growth*, and that is stable across both samples.

**The reads hold up, and that is the answer §4 wanted.** The findings list grew
between 1.5 and 3.5 times while the table grew 16.8, because it is indexed on
the target and whether a finding is closed — the shape the interval storage was
chosen for. Running out behaves the same way.

**Trend is the one that grows**, and it grows about linearly: 18 ms to 394 ms on
SQLite, 7 ms to 114 ms on PostgreSQL, 15 ms to 1.19 s on MySQL. It reads every
interval overlapping the window rather than a page of them, and the open set
itself grows as issues accumulate. It is the query to watch, and the first one
to reshape if a deployment reports a slow front page.

**MySQL writes seven times slower than PostgreSQL and fifteen times slower than
MariaDB** — 4.82 s a night against 0.67 s and 0.32 s, and 12.56 s at its worst
against 1.11 s. A nightly scan taking thirteen seconds is not an operational
problem; the same code being fifteen times more expensive on one supported
engine than on its own sibling is a fact to have before somebody chooses one.

**And the cost is per statement, not per row.** The harness counts statements
as well as seconds, and a night issues **1,699 of them on every engine** — the
same code, the same work, the same number of round trips. What differs is what
one costs: 203 µs on MariaDB, 404 µs on PostgreSQL, 2,835 µs on MySQL, which is
exactly the ratio the whole night shows. There is nothing to find in what the
apply does; the lever, for anybody who wants MySQL faster, is issuing fewer
statements rather than doing less work.

A quiet night issues **more** statements than the first one — 1,699 against
1,077 — because the first night is bulk inserts five hundred at a time and a
quiet night is an update per finding that moved. Recorded rather than chased.

**And the correction to the model says what that cost is made of.** An earlier
run applied twice the churn it documented, and its figures were withdrawn
rather than halved. Halving the churn halved MariaDB (0.64 s to 0.32 s) and cut
PostgreSQL by a third (1.04 s to 0.67 s) — and moved MySQL by four percent,
from 5.01 s to 4.82 s. A cost that barely responds to how many rows changed is
a cost paid per statement rather than per row. That is the thing to measure
next on that engine, and it is also why the withdrawal was right: the error did
not scale the four engines alike, so no arithmetic on the old numbers would
have recovered these.

**Two reads that grow with the calendar rather than with a build**, measured in
the same run because a year of nights is exactly what they need. The receipts
page reads every finished run of a target and every scan filed against it,
whatever page is asked for, and pairs them; the per-build open counts a release
chart is drawn from read every open finding of a product.

| after | receipts, first page | receipts, last page | release counts |
|---|---:|---:|---:|
| night 1 | 0–3 ms | 0–1 ms | 11–17 ms |
| night 365 | 3–4 ms | 2–3 ms | 71–88 ms |

Receipts grew from about a millisecond to about four over 365 uploads and 365
runs, and **the last page costs what the first does** — the pairing is done over
all of history precisely so that it does not depend on which page is being
read, and the measurement says that costs nothing worth acting on. The pairing
is quadratic in the number of runs, so a decade would be a hundred times this
work; four milliseconds times a hundred is still not a screen anybody notices.
It is written down rather than changed.

The release counts grew about sixfold against a table that grew 16.8 times.
That is one build; a product with thirty of them counts over thirty builds'
findings, and the query already groups in the database rather than in the
application.

**What this does not measure.** It was read as an administrator, who sees every
product — so the queries ran with no narrowing by product at all, which is the
cheapest plan available rather than the one an ordinary reader gets. One build,
not the several a deployment tracks — the table grows with builds, so a
deployment following ten of them multiplies these rows by ten and the queries
narrow by target before they read. It also assumes a churn rate rather than
observing one: at 1% a night the year moves 2,548 component versions, and a
deployment whose builds move more will grow faster in proportion. `make
measure` re-runs it, and the constants at the top of the harness are the model.

## Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| A component nothing leads to still has a place — itself | It ships. An incomplete graph is normal, and a component the producer could not place is not a component nobody has to think about |
| Two rows that turn out to be one issue are refused rather than merged | Merging changes findings and decisions already made. Reading a scan is the wrong moment to do that silently |
| Severity is stored on the issue, fix state on the finding | Severity is a property of the vulnerability; whether a fix exists is a property of the version in front of you |
| A place under the product records no consumer at all | Rather than recording the root and excluding it later. The root's name differs per variant, and a key that has to be remembered to ignore is one somebody will forget to |
| A derived address refuses a name that is nothing but dots, rather than escaping it | A name and a version become path segments, and "." and ".." are resolved by the browser before the request leaves it — so escaping is not enough on its own and a component named that would quietly reach a different page on the same site. Nothing that is really a package is called "..", so the whole link is dropped. Everything else, separators included, is escaped into the segment it belongs to (SEC-04) |
| An identifier is matched against an anchored scheme before it resolves to anything | A link is offered on the strength of the name looking like a CVE or a GitHub advisory. A flaw this deployment recorded is filed under a name it minted (MDL-24), and a loose match would send somebody to a public page about something else — or to a page about nothing, which reads as the record being missing rather than the flaw being ours |
| A package kind nothing here knows produces no link | There is no general answer, only a place per ecosystem. A link that lands on the wrong thing costs more than no link, because it is followed before it is disbelieved |
