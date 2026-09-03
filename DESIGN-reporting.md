# Reporting

The numbers this produces, and what each of them is honestly a number of.

Satisfies RPT-05 to RPT-07, RPT-10, RPT-11, RPT-13, RPT-15 to RPT-17, REM-25,
REL-08, ING-13. RPT-09 is only half done and says where.

Everything here reads what the findings and decisions already hold. Nothing in
this document has a table of its own.

## Nothing is made faster until it is measured slow

Every count here is worked out when it is asked for. Nothing is precomputed,
cached, or refreshed on a schedule.

This was nearly decided the other way, on somebody else's evidence. The
best-known tool in this space stores metric snapshots and runs a refresh job,
and copying that looked like the obvious thing — but its reasons are its own: a
hosted portfolio with far more traffic than a self-hosted deployment sees. The
honest number here is dozens of people in a month, so throughput is not the
constraint, and a stale answer buys nothing anybody asked for.

There is a second cost that is easy to miss. Visibility is per subject, so a
precomputed total is a total *for somebody* — and either it is computed per
person, which is not a saving, or it is computed once and then filtered, which
is a second path through the visibility rules. Two paths is how the one nobody
looks at gets it wrong.

**What would change this**: somebody reporting a dashboard that is slow to
open, measured on a real deployment. Then the shape is known and should not be
reinvented under pressure — precompute at the grain access is granted at, one
row per product per day, so a portfolio number stays the sum of what the reader
may see and no second path is needed.

## Trends

Three series over time — new, resolved and open — with open split by severity.

Three rather than one, because separately they are three numbers and together
they say whether the team is keeping pace. New consistently outrunning resolved
is a growing backlog, and it should be visible without anybody doing the
arithmetic. The severity split is there for the same reason: an open count that
barely moves while its critical share rises is getting worse, and one line
hides that.

### What counts as resolved

A finding closing is not the same as an issue going away, and two of the
closure reasons are not resolutions at all:

| | |
|---|---|
| `superseded` | The component's version moved and the issue came with it. Counting this as resolved draws a line saying work was completed while the same chart's new line rises by exactly as much |
| `unexplained` | The scanner stopped reporting it with the component present and unchanged. That is a fault to investigate, not a fix |

The other three — removed, upgraded, revised — are resolutions and are counted.

### One axis of the two

Trends plot on calendar time. That is right for a branch, which is scanned
nightly and has continuous data.

**Release over release is not built.** A tagged release is one frozen point,
and releases months apart make a calendar count read as slow drift rather than
the step change it was — so the axis should follow what is being viewed
(RPT-09). Recorded here because a calendar chart of tags looks like it works.

### Reading only what can be in range

The window is turned into predicates before the statement runs: a finding
opened after the last point contributes to nothing, and one closed before the
first contributes to nothing either. Without that the query reads every finding
ever recorded and discards most of them, so the cost of drawing a chart grows
with the age of the deployment rather than with the range being asked about.

The step and the number of steps are both bounded. They arrive as query
parameters and the loop walks whatever they say, so a step of zero would draw
one instant twelve times and a step of a century would draw a range nothing
falls in.

## Comparing two builds

What was fixed, what is newly present, and what is still there, between **any**
two builds of one product — not only adjacent ones. What a release note has to
answer is usually about the last release a customer actually has, which is
rarely the previous one.

Both builds are authorized, not one. The first version of this authorized the
later target and applied that answer to the earlier one, so somebody who could
reach one product could read findings out of another through the comparison.

**Public findings only unless asked otherwise.** The destination is usually a
public document, so including something undisclosed is a deliberate act rather
than something pasted in without noticing. Where the two builds differ in what
the reader may see, the narrower answer governs.

What comes back is JSON. **The other half of RPT-06 — a form that goes straight
into release notes — is not built**, and turning three lists of issues into
prose somebody would paste is a formatting decision better made against a real
release note than in the abstract.

### Each fixed entry says why

"Fixed by upgrading to 2.4" and "fixed by a carried patch" are different
sentences to a reader, and the closure reason already distinguishes them.

`superseded` is the one to read carefully: the version moved and the issue came
with it, so nothing was fixed. Before that reason existed, such a bump was
recorded as an upgrade — which put one issue in both the fixed and the
newly-present column of the same comparison, in a document written to go
straight into release notes.

**The explanations are read once for the whole list**, not once per entry. A
comparison against the release a customer has been on for a year has as many
fixed entries as the note is long, and asking about each separately made the
screen's cost a count of round trips rather than a count of rows. The statement
narrows by the issues and the components separately rather than by the pairs —
no engine here spells a comparison against a pair of columns the same way, and
building one out of concatenated strings is a portability trap of its own — so
what comes back is a superset and the pairing is done on the way out.

**What a comparison reads is bounded by the size of a build, not by the
calendar.** Every open entry of both builds, which is what diffing them means:
there is no page of a diff, because a release note is not paginated. A
deployment that has run for years does not make this read more; a larger
product does.

### And each still-present entry says whether somebody tried

A still-present entry carries the version its place arrived from, where the
version moved since the earlier build. That is the same failure seen from the
other side: somebody did the bump, and it did not reach the fix
(`DESIGN-findings.md`). It is on the still-present column only — on a fixed
entry the closure reason already says what happened, and on a new one there was
nothing to bump.

## Deadlines

A finding carries a deadline from how urgent it is: a configured window,
counted from when it was first seen. Being overdue is reported and never acted
on automatically.

**Known-exploited has its own window and it is the shortest**, whatever the
severity says. Severity is how bad a flaw is; being exploited is a fact about
the world. Without a separate window the deadline contradicts the ranking — the
list says look at this first while the clock says ninety days.

**Anything the reports did not rate takes the medium window.** Nobody having
scored it is not a claim that it is mild, and giving silence the longest window
puts the findings least is known about at the back of the queue.

The clock runs on what nobody has answered. A dismissal takes a finding off it
entirely, because the claim is that it will not be fixed; a deferral replaces
the deadline with its own date. What is left is time passing with nothing said,
which is the only part worth interrupting somebody about.

The clock stops only for a decision that applies: approved, or proposed where
no second person is required, at the versions this build ships, and a deferral
only until its date. A proposal still waiting for an approver stops nothing.
"Who is holding what" counts overdue by the same condition, spelled once in
`finding.OffTheClock`, so the two screens cannot disagree.

### Asked band by band

The list of what is running out is asked once per window, each with its own
first-sighting cutoff, and the answers are merged by deadline.

One query cannot do it. The deadline is the first sighting plus a window that
differs per band, so ordering by it means date arithmetic — which has no
portable spelling across the four engines (DAT-02). Within a band, though, the
oldest finding *is* the most overdue, so a band answers in the right order by
ordering on the first sighting alone.

It was written the other way first, and that list was not the list it claimed
to be: the statement took the oldest findings and a loop then discarded
whatever was not due, so an exploited finding first seen yesterday and due
tomorrow lost its place to a low from two years ago that filled the buffer.

**One row per issue at a component**, not per place. A kernel flaw sitting at
sixty places is one thing somebody has to answer, and sixty rows of it is a
list with one entry in it. A person is named on the row only where every place
has the same one — reporting one of several would say a finding is being dealt
with when most of it is not.

Days remaining are rounded down rather than truncated toward zero. Truncation
reports something twelve hours overdue as having zero days left, which reads as
due today.

## Who is holding what

How much work each person has, and how much of it is past its deadline.

The number that matters is not how many findings exist but how many are stuck
behind somebody: an idle account holding nothing is harmless, and work waiting
on a person who is not here is the thing worth surfacing. The overdue count is
what separates somebody keeping up with a large list from somebody sitting on
one.

It carries the same narrowing every other query does. A count is as much a
disclosure as a row — "somebody holds six" tells a reader there are six.

## What a new line would inherit

Asked before a line is created, because the answer is what somebody is agreeing
to, and a carry that happens silently is one nobody reviews. Declaring a
release is the natural moment to choose it.

Four groups, because they need four different things:

| | |
|---|---|
| **Reach it by matching** | Counted, never offered. A decision is a claim about a combination of code, so these have already happened |
| **Held a claim at a version this line does not have** | Each comes across as a *proposal carrying the old reasoning* — never as a decision, because the version moved and the old conclusion is not a conclusion about the new code |
| **Were deferrals** | Offered separately and never carried by default. "Not this sprint" was about that sprint, and carrying it silently gives a new line expiry dates nobody chose |
| **Cover nothing here** | Counted and left behind |

Which product this is about is read from the build rather than taken from the
caller. The first version selected by live key and a matching place alone — and
a place is a hash of component names carrying no product, so a shared
distribution package matched across products and the reasoning of undisclosed
claims came back to anybody who could read one product.

Both upstream versions are compared, not just the component's. Comparing only
one reported a build whose *consumer* had moved as already covered, when the
claim does not reach it and the finding surfaces unanswered.

### A deferral says how long it has already run

Each postponement carries the total time it has been put off, across every line
it has been carried through — not the length of the one being offered.

That total is what carrying it again agrees to. Four consecutive carries of
"not this release" are a year nobody decided on, and each of them looks
reasonable on its own.

Withdrawn deferrals are not counted. What was taken back was not time the
finding spent put off.

## Settings

What an administrator changes from inside the application, as opposed to what
an operator sets when deploying it. A configuration file is edited by whoever
can reach the filesystem and restart the process, and an administrator is
generally not that person.

| | |
|---|---|
| The five deadline windows | Exploited, critical, high, medium, low |
| The deferral threshold | How long something may be put off before a second person has to agree |
| The session lifetime | Also the window in which somebody who moved out of a team still holds what the team gave them |
| The token ceiling | The longest a personal token may be set to last |
| The limit on one action | How many findings a single judgment may cover |

**The shipped numbers are a starting point rather than a recommendation.** What
a deployment can hold to is a question about that deployment, and a deadline
nobody agreed to produces an estate that is permanently late and a signal
everybody ignores.

Only the names in that list may be set. A name the deployment does not know is
refused, because storing it would create a setting nothing ever reads.

### A value nothing can read is refused, not stored

Every reader falls back to the shipped default where a setting is unset or
unparseable, which means a stored value nobody can read is a policy that
quietly stopped applying. So the value is checked before it is written — as a
duration for the durations, and as a whole number for the one that is a count.

**Zero and negative are refused too.** Every reader treats them as unset, so
storing one produces a setting that looks set on the administration screen and
does nothing at all.

A failure to *read* a setting is not "unset" either. Every caller has a
default, so a database that could not answer would silently swap the
deployment's configuration for the shipped one — including the threshold
deciding which deferrals need a second person. That is reported rather than
answered with the default.

### Everything offered is read

A setting listed on the administration screen and read by nothing is worse than
not offering it: somebody sets it, sees it recorded, and gets the old behavior.
The session lifetime was exactly that for a while — offered here, while sign-in
took its value from the deployment's environment and never looked.

The order is the administrator's setting first, then whatever the process was
started with, then the built-in default.
