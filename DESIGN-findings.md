# Findings

What a scan run found, and where.

Satisfies MDL-05, MDL-06, MDL-14, MDL-19, MDL-20, ING-02, ING-21, ING-29,
ING-30, STA-03, STA-04, STA-05, STA-08, STA-17, RNK-04.

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

A rating, not a score, and often with the method given as unspecified. Numeric
scores come from the ranking feeds instead — there is nothing in a report to
normalize.

## Fix state is kept

No fix available, upstream declined to fix, and a fixed version exists are
three different situations to whoever is triaging. "Upstream will not fix this"
is a permanent condition that changes the outcome somebody should reach, and it
is invisible if the only thing recorded is that a fix is absent.

## Findings are held over intervals, like the graph

Open until closed, never deleted. Re-scanning happens nightly against a
vulnerability database that has barely moved, so a run that found the same
things writes nothing at all — the same property the graph has, for the same
reason.

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
scanners without saying so would be a rumour rather than a report.

A run that failed is recorded as a run that failed, with the reason. A scanner
that stopped working is otherwise indistinguishable from a product that stopped
having problems.

## Choices the decisions did not cover

| Choice | Why this way |
|---|---|
| A component nothing leads to still has a place — itself | It ships. An incomplete graph is normal, and a component the producer could not place is not a component nobody has to think about |
| Two rows that turn out to be one issue are refused rather than merged | Merging changes findings and decisions already made. Reading a scan is the wrong moment to do that silently |
| Severity is stored on the issue, fix state on the finding | Severity is a property of the vulnerability; whether a fix exists is a property of the version in front of you |
| A place under the product records no consumer at all | Rather than recording the root and excluding it later. The root's name differs per variant, and a key that has to be remembered to ignore is one somebody will forget to |
