# Findings

What a scan run found, and where.

Satisfies MDL-05, MDL-06, MDL-14, MDL-15, MDL-19, MDL-20, ING-02, ING-21,
ING-29, ING-30, ING-39, ING-40, STA-03, STA-04, STA-05, STA-08, STA-17,
RNK-01 to RNK-06, MDL-09, STA-01, STA-02, STA-06, STA-07, STA-09, STA-10,
STA-13 to STA-16, STA-18.

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

## What somebody with an hour sees first

A findings list has to open on what matters. Ordering by how many places
something occupies puts whatever is most widespread at the top, which on a real
image is the kernel — everywhere, and not therefore the thing to look at first.

So each finding carries an urgency, worked out when a scan is applied and read
back as it was. Computing it while reading would mean joining every signal it
is made of, for every row, on every page of every list.

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
| **Likelihood of exploitation** | Whether it is going to happen |
| **Severity** | How bad it would be if it did |

The number is **packed rather than weighted**: each signal owns a range of
digits, so a signal never trades against a lower one. That is a first cut and
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
