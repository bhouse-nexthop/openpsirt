# Triage

What people decide about findings, and when a decision stops applying.

Satisfies TRI-01 to TRI-03, TRI-05 to TRI-08, TRI-10 to TRI-18, TRI-20 to
TRI-36, REM-25, UIX-35, REL-05, REL-06, REL-09, MDL-08, MDL-19, ACC-09. The
text rules themselves are in `DESIGN-text.md`, and the reports these numbers
feed are in `DESIGN-reporting.md`.

## A decision is a claim about code, not about a release

Nothing in what identifies a decision names the release it was made in. It
records which product, which issue, which place, and the upstream versions of
the component and of the thing that pulls it in.

That is what makes a later release inherit a decision by **looking it up**
rather than by copying it. There is nothing to synchronize, so there is nothing
to drift, and a release nobody has thought about since is not carrying a stale
copy of anything.

It is also why a decision carries across variants exactly when the code is the
same. Nothing extra tests for that: a variant whose chain differs computes a
different key and simply fails to match.

## Expiry is not a mechanism

A decision is stored under the versions it was a claim about. A place asks
under the versions it has now. When the code moves the two stop matching and
the decision does not come back.

Nothing sweeps, nothing runs on a timer, and there is no second rule that could
disagree with the first. **Identity is structural and expiry is version-based,
and neither reaches into the other** — overlapping them is how a bump at the
top of a build invalidates a judgment made about a leaf.

### Only the upstream version counts

A shipped package is rebuilt constantly and carries a version of its own that
moves each time. A rebuild is not somebody's reasoning becoming wrong, and a
tool that asked the same question every night would be unusable by the second
week.

What changes the reasoning is the code changing, which is what an upstream
version moving says. So `10.5.4` becoming `10.6` lapses a decision and a
packaging revision does not.

A version changing in the middle of a chain needs no rule of its own. It either
changes the direct consumer of something, which lapses that decision, or it
does not.

### What this deliberately does not catch

A producer that changes code by patching rather than by bumping versions will
not lapse decisions this way at all. That is accepted rather than worked
around: a patch is our own change to our own build, and re-asking every
question every night to catch the few a patch made stale is a poor trade.

What compensates is that **a decision's age is shown wherever it appears**. An
eight-year-old judgment should look like one rather than reading the same as
yesterday's.

### A deferral runs out on a date

A different claim, so a different mechanism. A version bump does not change a
judgment about priority, and a calendar does not change one about
applicability. When the date passes the deferral stops standing and the finding
returns to the queue — marked as something that was deferred rather than as
something new.

## Four outcomes

| | |
|---|---|
| **Affected** | It applies, and goes to remediation |
| **Not applicable** | It does not affect this product here |
| **Deferred** | It affects us, and is not being worked on until a date |
| **Won't fix** | It affects us, and will not be addressed |

Two would not be enough. A vocabulary with only "affects us" and "does not" has
nowhere to put the most common real answer — *yes, but not now* — and the
absence shows up as people recording it as one of the other two, after which no
report can tell the difference.

**A deferral publishes as affected, never as not-affected.** Deferring is an
internal scheduling judgment; publishing it as not-affected would tell the
outside world we assessed something as harmless when we had only put it off.

Only "not applicable" carries a reason, and it is required there: the claim
that something does not affect us *is* which of the recognized reasons applies.
The vocabulary is the one the exchange format already defines rather than one
of ours — it encodes exactly this reasoning, and using it makes publishing
close to free, where a private vocabulary would need a mapping nobody maintains
and that loses meaning at every step.

## Two people, and the words they agreed to

The proposer and the approver are always different people, with no override. A
deployment with one person therefore cannot approve anything. That is the
control working rather than a gap in it, and it is better said plainly than
quietly relaxed.

**An approval names one revision of the reasoning, not the decision.** The
whole value of a second pair of eyes is that they read particular words. An
approval that floated free of the words would still be standing after somebody
rewrote them, and nothing would report it.

So the reasoning is revised and never overwritten, every revision is kept and
readable, and **editing it takes back the approval** and returns the item to
the queue. It is marked there as previously approved rather than reading as a
fresh proposal: somebody meeting it again should know they are re-reading
something.

That needs no approval of its own. Hiding risk needs a second person; putting
it back on the table does not — which is also why withdrawing needs none.

### Undoing at the size it was done

A reviewer may agree to a long selection in one action, so undoing has to be
available at the same size. What a bulk approval covered is recorded with it,
and undoing takes the whole batch back. Hunting for what it touched, one row at
a time, is not an undo anybody will use.

Undoing an approval returns the claims to proposed rather than withdrawing
them. The claims still stand; it is the agreement to them that was taken back.

## An approval that was later withdrawn is still part of the record

Approvals are kept rather than reduced to a flag. A withdrawn one says a second
person did once agree, and to which words — which is what somebody
reconstructing how a decision came to stand needs to read.

## Deciding is its own right, held per product and per visibility

Reading a finding is not judging it. An approver reaches plenty they may not
propose about, and a reporter reaches more still — so the question asked here
is whether somebody holds triage on this product, not whether they can see it.

The two triage rights are not one. Somebody trusted with what has been
disclosed is not thereby trusted with what has not, so arguing about an
undisclosed finding needs that right specifically.

**Every check reads the product and the visibility from the row**, never from
what a caller stated about it. A caller that could name the product would be
deciding what it may reach. And a place that states no visibility is treated as
undisclosed: something that forgot to state it would otherwise make an
undisclosed finding argueable by anybody who can triage the disclosed ones.

That normalization happens before the check and not only before the write. The
first version did it only on the way to storage, which let a place stating
nothing pass the check for disclosed findings and then be recorded as
undisclosed — authorized as one thing and kept as another.

A decision somebody may not reach and one that does not exist give the same
answer, so guessing identifiers says nothing.

### Three rights, not one

Arguing, agreeing and reading are separate questions, and asking one of them
for all three was wrong in both directions.

| Act | Needs |
|---|---|
| Propose, revise, withdraw | Triage on the product, at the finding's visibility |
| Approve, undo an approval | The approver capability **or** triage, plus being able to read the finding |
| Read what was decided | Either of the above |

Approving asked for the triage role, which made the approver capability
decorative: somebody granted exactly the right to approve could not approve
anything, and the only people who could were the ones who could also have
proposed it. A capability grants no visibility, so it is asked alongside
whether they may read the finding — otherwise handing somebody the ability to
approve hands them everything there is to approve.

A triager may still approve, on somebody else's claim. Two triagers agreeing to
each other's work is the ordinary shape of a small team, and the control that
matters is that the two are different people, which is checked separately and
has no override.

Reading follows whichever act produced the thing being read. An approver has to
read a claim to judge it, and somebody who took part in a discussion has to be
able to read it back — but a reader who holds neither right sees a decision as
one that is not there, the same answer everything else here gives.

## Everything written down can be read back

Each way of adding to a decision has a matching way of reading the result: the
decision itself with the reasoning it currently rests on, the earlier
justifications, who agreed to which of them, and the discussion. Decisions can
also be listed across products and filtered — by outcome, to answer what has
been dismissed; by state, to separate what is agreed from what is waiting; and
by whether a deferral's date has passed.

Without these the only list of decisions is the review queue, which by
definition holds the ones nobody has agreed to yet. Somebody auditing what was
dismissed, or re-reading what an approver actually saw, had nowhere to look —
and a control nobody can inspect after the fact is not much of a control.

Each list carries the reasoning with the row, for the same reason the queue
does: a list where seeing why means opening every entry is a list nobody reads
before acting on. Names come with it too — the product, the issue, the person —
because a row saying product 4, issue 91 is a row somebody has to make two more
requests to understand, fifty times a page.

Undoing a bulk approval narrows to what the person undoing it may reach. A
batch is one reviewer's afternoon and may span products, so taking it back
wholesale would let somebody act on products they hold nothing on.

## The queue carries what an approver needs

A reviewer works down a list. A list where judging each row means opening it is
a list that gets approved without being read — which is the failure the queue
exists to prevent, arriving by a different route. So each row carries the
reasoning as it currently stands, whether this was agreed to before, and how
long the finding has already been put off.

It shows only work the reader can actually do, narrowed in the query. A work
list containing work somebody cannot do teaches them to skip rows.

### Three things are waiting, not one

A claim awaiting agreement is the obvious one. The other two are what happens
when a judgment stops covering anything, and leaving them out is how reasoning
gets stranded:

- **A deferral that has run out.** It has said what it was going to say and the
  finding is back. If it does not appear here the finding simply resurfaces as
  new, with what somebody wrote about it last time attached to nothing anyone
  is looking at.
- **A decision the code moved out from under.** Somebody made a judgment, a
  version bump means it no longer applies, and they are the person who should
  be told — which is the entire reason a lapse is marked rather than the
  decision being deleted.

A claim that needed nobody — a deferral under the threshold — is not here at
all, by the same rule that keeps unreachable work out.

## Hiding risk needs a second person; a quick postponement does not

A deferral shorter than a configured threshold stands on its own. A quick "not
this sprint" is ordinary triage, and putting every routine act through a queue
is how a queue stops being read.

**Short is measured against everything the finding has already been put off
for**, not against the deferral being asked for. Otherwise the exception
swallows the rule one twenty-nine-day deferral at a time: four of them in a row
are a year nobody approved.

The time counted is what each deferral *asked for*, not what it has spent. The
question is how long something has been put off, not how far into that it
currently is.

Two things are excluded from that total. A deferral that was **taken back** was
not time the finding spent postponed, and counting it would make the number an
approver is shown describe time that did not happen. And a deferral asking for
a date **already past** asks for nothing — allowing it to count as a negative
would let a back-dated request subtract from what a finding has already been
put off for and slip the total back under the threshold.

## One live claim per combination of code

A decision is a claim about a combination of code, so there is at most one of
them: propose where one already stands and the answer is to revise that claim
rather than to record a second one beside it.

**Live, not ever.** A withdrawn or lapsed decision covers nothing and is
history, so it stops holding the place the moment it stops applying. Otherwise
one lapse would wall a place off permanently.

**Per combination, not per place.** The versions are part of what a decision is
about, so a claim about one version and a claim about the next are different
claims and both stand. That is not an exception to the rule — it is the rule,
since carrying a judgment forward is exactly the case where the code differs.

**Why revising is the right answer to a disagreement.** It keeps the old words
readable, takes back the approval given for them, returns the claim to the
queue and records who wrote the new version. Two people disagreeing then
produces one legible argument rather than two rows neither of them can see at
once. Approving your own rewrite is already refused, because an approval is
compared against whoever wrote the current revision.

**What it replaced.** Nothing prevented two contradictory claims about one
finding, and nothing marked them as contradictory: both sat in the review queue
looking ordinary, and since what applies is chosen by agreed-beats-waiting and
then newest-wins, approving both left one silently governing while the other
stayed on the record as agreed. Neither approver had any sign the other
existed.

**Enforced by the database, not by a check.** A key is held while a decision is
live and set to null once it is not, under a unique index — null values do not
collide in a unique index on any of the four engines, which is what makes a
rule that applies to only some rows portable. A read-then-write check is
exactly the shape two proposals arriving together both walk through, and the
test drives that case directly: two people proposing at once produce one claim,
and removing the index lets both through.

## A judgment says how much it covers

A decision is about a component at a place, and one place can hold a great deal
— a kernel issue reaches dozens of modules, and the answer is almost always the
same for all of them. So the answer to making a decision says how many findings
it covers, rather than letting somebody discover afterwards that they answered
for sixty-two things.

It also says how many distinct versions sit there. One is the ordinary answer.
More than one means the build ships the same package twice at different
versions under the same consumer, and a single decision cannot honestly cover
both — so whoever is deciding is told, instead of being given a judgment about
one version silently applied to another.

Choosing a narrower set than "all of them" is an interface question and is not
built. What exists is the count, which is what makes the choice an informed one
when it arrives.

## A carried patch does not lapse a decision

Expiry compares upstream versions. A producer that fixes things by carrying a
patch rather than by taking a new upstream release therefore moves no version
this can see, and the decision stands until somebody revisits it or the finding
closes on its own.

This is accepted deliberately rather than worked around. A patch is our own
change to our own build, and a reachability claim usually rests on structure a
patch is unlikely to alter. But the consequence is worth stating plainly: for a
producer that patches heavily, a decision made once may never be automatically
re-examined however much the code moves.

The compensating control is that a decision's age travels with it everywhere it
appears. An eight-year-old judgment should look like one rather than reading
the same as yesterday's.

## Where a judgment lands, in three parts

A decision is a claim about a code combination rather than about a release, so
it reaches further than the finding it was made on. How much further is not one
number, and presenting it as one is what turns a considered judgment into a
reflex.

| | What it is | What the person does |
|---|---|---|
| **Places in this build** | The same component under several consumers — the same code, the same versions | Nothing. One judgment covers all of them |
| **Other builds already matching** | Variants and branches whose upstream versions and chains are identical | Nothing. The decision reaches them by matching, not by copying |
| **Builds where the code differs** | The same issue at the same place, at another version | Chooses, one row at a time, each unchecked |

The first two follow from the matching rules and are not offered as choices —
there is nothing to agree to, only something to be told. The third is the only
choice, and it is the one worth slowing down.

**Two rules that read as contradictory are not.** All-places-by-default and
one-unchecked-box-per-match cover different axes: the first is places running
identical code, where making somebody answer sixty-two times guarantees they
stop reading; the second is builds running *different* code, where a tick is a
claim about a version nobody has looked at. Built from both without
distinguishing them, an interface collapses into a single apply-everywhere
control.

**What makes carrying acceptable at all** is that one action writes a separate
record per target, each keyed to its own versions. They are not one blanket
claim: a dismissal carried onto an older branch lapses by itself the moment
that branch moves, without anybody remembering it exists.

**Nothing is withheld for sitting past the named fix.** It would be the right
rule — ticking a build whose version already carries the fix while the scanner
still reports the issue asserts something visibly wrong — and it is not
enforced, because deciding whether one version sits past another needs
per-ecosystem ordering that does not exist here (`DESIGN-findings.md`). What is
recorded instead is the case that needs no ordering: a version moved and the
issue came with it, which travels with the finding and marks the still-present
column of a release comparison.

### An approval names the reach, not only the words

An approval points at one revision of a justification, which binds it to
particular words and says nothing about how far those words travel. The same
sentence covers one finding or four hundred.

So the three parts are shown to the approver before they act, and **how much
the claim covered is counted at that moment and kept**.

Kept rather than worked out later, because the number moves on its own. A
decision reaches by matching, so a branch cut next month that still ships the
same versions is covered too — with nobody having acted, and nobody having
agreed to the larger number. Asking afterwards what a claim covers is a useful
question and a *different* one from what somebody consented to, and only one of
the two survives if it is not written down when it happens.

One number rather than three. The split between this build, the builds already
matching and the ones ticked deliberately is how it is presented; what the
record needs is how much was agreed to.

## Many issues at one component

Grouping runs one way: one issue across many places. The transpose has no
answer at all, and it is the case that breaks a queue.

A kernel carries thousands of issues, most of them in drivers a given image
never builds. One action can therefore record the same judgment against a set
of issues at one component — one outcome, one justification, one reasoning, one
approval — writing a separate decision per issue, each keyed and expiring
independently like any other.

**A separate decision per issue and per place.** A decision is keyed on a
place, so a claim built from one place of an issue would silence one consumer
and leave the rest open while reporting that it had covered them. The places
are resolved from the findings inside the transaction that writes them — a
caller free to name a place would be choosing which decisions apply where, and
a place read before the write is a fact about a database that has since moved
(DAT-31).

Whatever is offered to narrow the set — a weakness class, a subsystem named in
the advisory text — is a starting point for a person, never a selection the
tool asserts is right. **No SBOM says whether a driver was compiled in**, so
this is a claim somebody makes and signs for, with the kernel config or
whatever else supports it written into the reasoning.

**How the set was narrowed is recorded with every claim in it**, separately
from the reasoning. The two are not the same thing: narrowing is how a
candidate was found, and the reasoning is why the claim is true — "these
matched a word" is not a defence anybody would accept. Keeping it is what gives
"how were these chosen" an answer months later.

**Bounded, and the bound is a setting.** One action writing an unbounded number
of rows is a denial of service somebody triggers by accident. Two limits, not
one: how many issues a request may name, and how many findings the action may
write. They differ because each name may sit at many places, and the second is
checked against what the names actually resolve to — a limit checked against
the names would let a request naming two thousand issues write sixty thousand
rows.

**It always needs a second person**, whatever the outcome. The exception that
lets a short deferral stand on its own is about one finding somebody is putting
off for a fortnight; one person answering hundreds in a single action is the
case a second pair of eyes exists for.

The alternative, letting people hide these from the counts instead, is refused
(REJ-10): a total that depends on who is looking is not a total. A filter
narrows what somebody is looking at, is carried in the URL, and states what it
excluded — it never subtracts from a number anybody else is reported.

## An approver has a third option

Approve and withdraw were the only two, and withdrawing throws away somebody's
work over a missing sentence. So what actually happened was a comment, and the
claim sat in the review queue looking untouched — which is worse than either,
because the queue then contains rows nobody is going to act on.

Sending a claim back takes it out of the approval queue and returns it when the
author revises. **A reason is required**, and travels as a comment: the words
are what the author needs, and a reason recorded anywhere else is one nobody
reads.

**Not a state of its own.** The claim is still proposed and still suppresses
nothing; what changed is whose turn it is. A fifth state would have to be
reasoned about everywhere the other four are, for a distinction that is about
attention rather than about standing.

It needs no approval, for the same reason revising and withdrawing do not — it
puts risk back on the table rather than taking it off. And nobody sends back a
claim whose current words are their own: that is theirs to revise, and doing it
would put their own work out of everybody's sight.

## How long something may stay open

A finding gets a deadline from how urgent it is, counted from when it was first
seen. **Being known-exploited has its own, and it is the shortest**, whatever
the severity says — severity is how bad the flaw is, being exploited is a fact
about the world, and without separating them the deadline contradicts the order
the findings list is in.

**The clock runs on what nobody has answered.** A dismissal takes a finding off
it, because the claim is that it will not be fixed. A deferral replaces the
deadline with its own date, because that is what a deferral is. What is left is
time passing with nothing said, and that is the only part worth interrupting
anybody about.

Being late is reported and never acted on. The failure to design against is a
deadline nobody agreed to, applied to everything, so that the whole estate is
permanently overdue and the signal is ignored inside a month.

The windows are settings, and the shipped numbers are a starting point rather
than a recommendation: what a deployment can hold to is a question about that
deployment. How the list of what is running out is built, and what an unrated
finding gets, are in `DESIGN-reporting.md`.

## A lapse is marked when the scan that caused it runs

A decision stops applying the moment its versions move, because what applies is
matched on them. That much needs no mechanism. What needs one is somebody
finding out: without it the finding reappears as though nobody had ever looked
at it, and the reasoning sits on a row nothing points at — which is precisely
the outcome that keeping the old decision exists to prevent.

So a scan, having just recorded the versions that moved, marks the judgments
they moved out from under. Those land in the review queue as work.

It is **one statement, not one per place**. A real image holds tens of
thousands of places, and a sweep costing a write per place is a sweep somebody
turns off — after which nothing lapses and the whole mechanism is decorative in
a way nobody notices.

Two boundaries matter, and both are tested by breaking them:

- **A rebuild that moved nothing marks nothing.** Rebuilds are nightly, so a
  sweep that marked too much would unpick judgments nobody had revisited, every
  night.
- **A decision covering nothing here is not marked.** A component that is gone
  altogether closed its findings and there is nothing to ask anybody about. A
  component still present at a different version is exactly the question
  somebody has to answer again.

The version a decision is compared against is written by the same expression
that wrote it in the first place, shared rather than spelled twice. Two
spellings is how a decision starts lapsing on one path and standing on the
other.

Failing to mark is reported and not fatal. What the scan found is recorded and
correct; the marking is a prompt, and losing a scan over a prompt is the wrong
trade.

## Re-affirming after the code moves

Two people already agreed to the claim. A version bump is a prompt to re-check
rather than a new claim, so the person who made it may re-make it with a fresh
reason and no second approver. Requiring full approval on every bump produces
rubber-stamping, which costs the control its meaning everywhere rather than
only here.

Two things send it back for full approval, and both fire on something having
actually changed:

- **A different justification** is a different claim, which nobody has
  reviewed. Letting it inherit an approval granted for other reasons is the
  same failure as an approval surviving a rewrite.
- **A severity that has risen since** means the original judgment was made
  about a smaller thing. What was agreed to was that this did not matter much;
  that is not an agreement about what it has become.

How bad it was judged to be is kept with the decision rather than read from the
issue later. An issue's severity is rewritten in place as reports revise it, so
reading it now would compare a number against itself.

**A count of re-affirmations deliberately does not trigger it.** That would
fire on nothing having changed, which every other rule here refuses to do.

What may be carried is read from the row rather than from what a caller
supplied. A caller holding a stale copy would carry an agreement since
withdrawn; a caller inventing one would carry an agreement that never existed.
A withdrawn decision keeps its approval rows — who agreed, and to what, is part
of the record — so without that check a version bump would undo a withdrawal.

**All of it is one transaction.** Written as three steps — read the old claim,
write the new one, carry the agreement — a process that stopped in the middle
left a claim standing that nobody had agreed to and that no review queue would
ever show, because it was recorded as needing nobody. Everything the act turns
on is read inside that transaction for the same reason every other write here
is (DAT-30, DAT-31).

**The carried agreement is guarded on the revision**, the way every other
approval is. An agreement is an agreement to particular words, so it takes
effect only while those are still the words the claim rests on.

## What the code moved out from under is marked

Reading finds a lapsed decision by its key not matching, which is enough to
stop it applying. Marking it is for the queue: somebody has to be shown that a
judgment they made no longer covers anything, or it simply disappears and the
finding returns looking new with the reasoning stranded behind it.

A decision lapses when **either** version moves, not when both do. A component
bumped under an unchanged consumer is the ordinary case.

## A comment is not the reasoning

Two different things, and the obvious mistake is treating all text on a finding
as one. They behave differently on purpose:

| | Revising the reasoning | Adding a comment |
|---|---|---|
| The approval standing on it | Withdrawn | Untouched |
| What it said before | Kept and readable | Overwritten |
| Who may change it | Anybody who may triage | Its author, and nobody else |

Annotating an approved decision months later — *re-checked, still true* — is
ordinary, and an approval that fell over each time somebody added a note would
teach people not to add notes.

A comment is overwritten when its author edits it, and nothing is kept but a
mark that it happened. That is a deliberate exception to keeping everything:
discussion is not the record a decision rests on. That record is the revisions
and the approvals, and those are kept in full.

Nobody else may edit somebody's words. An edit anybody could make is not a
correction; it is a forgery with a timestamp.

## Every field somebody types into runs through the same policy

The reasoning, a revision of it, and a comment all go through the check before
they are stored — so stored text is known to have passed what was in force when
it arrived. What that policy is, and why it runs twice, is in
`DESIGN-text.md`.

## The same issue in other builds

A decision is a claim about a combination of code rather than about a release,
so a build running the same versions picks it up by looking it up. Nothing is
copied, nothing synchronizes, and nobody is asked — that is inheritance done as
a lookup, and it is why a release nobody has thought about since is not
carrying a stale copy of anything.

What remains is the builds where the versions differ. The decision does not
reach them, because it was a claim about code they are not running, so somebody
has to say whether the same reasoning holds there.

Those are offered **one at a time rather than as one answer**. A component may
be used in a later release and not an earlier one, and the reasoning that made
something harmless in one build is not automatically true in another. All-or-
nothing would be a single click that made a claim about builds nobody looked
at.

Builds already covered are counted and never offered. The count is part of what
somebody is shown before they decide, because a judgment reaching eleven other
builds is worth knowing; what would be wrong is putting a tick box beside them,
which asks somebody to agree to something that has already happened and teaches
people that these lists are noise.

## What a person actually does

The names in a request are resolved against what was scanned, never taken as
given. A caller who could name a place freely would be choosing which decisions
apply where — and the versions a decision is keyed on come from the rows for
the same reason, since they are what expiry compares.

An issue is named by any identifier it is known under. Somebody who read a
national identifier in an advisory and somebody looking at a report that used a
database's own identifier are asking about the same thing.

A decision that does not exist and one somebody may not reach give the same
answer, so guessing identifiers says nothing.

Whether a claim is waiting for a second person is answered when it is made, not
discovered later. A short deferral takes effect at once, and somebody who has
just written one should be told that rather than left watching a queue.

## Choices the decisions did not cover

- **A version nobody stated and a version that is empty are different.** Both
  occur, and comparing them as equal would let a decision made about a
  component with no known version stand over one whose version is merely blank.
  Two absences match each other and nothing else.
- **The claim and its first reasoning are written together**, in one
  transaction. A claim with no reasoning is not something a second person can
  agree to, and leaving the reasoning to a later write is how an item reaches
  the queue with nothing in it to review.
- **A decision that has lapsed is found by the structural half of its key
  alone.** That is what lets the reasoning behind it be offered back to
  whoever has to make the judgment again. Making them start from a blank page,
  having thrown away what was written last time, is how a tool teaches people
  to stop writing reasoning at all.
