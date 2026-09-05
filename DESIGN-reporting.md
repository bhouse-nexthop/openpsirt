# Reporting

The numbers this produces, and what each of them is honestly a number of.

Satisfies RPT-01 to RPT-02, RPT-04 to RPT-08, RPT-10 to RPT-13, RPT-15 to
RPT-17, REM-25, REM-26, REL-08, ING-13. RPT-09 is only half done and says
where; RPT-03 is not built and says so at the foot.

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

The other four — removed, upgraded, revised, and a flaw somebody recorded being
declared fixed — are resolutions and are counted.

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

What comes back is JSON, and **the same comparison is offered as prose**
(RPT-06): markdown, as `text/markdown` rather than as a string in a JSON field,
because the point of it is that it goes straight in.

**Rendered on the server, not in the browser.** What an API caller gets and
what the screen shows have to be the same words; two implementations of how a
release note reads is one that drifts, and the half that drifts is the one
nobody is looking at.

**A bump that carried the issue with it is listed apart from the fixes.** It is
the opposite answer to whether something was fixed, and this document goes to
customers who keep it — so putting churn under "Fixed" is telling somebody
something untrue in writing. An empty section is not written at all, because a
heading with nothing under it is a question about whether something is missing.

**Ordered worst first and stably**, so two runs over the same pair of builds
produce the same document. A release note that reorders between reads is one
nobody can diff.

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

**The deadline is worked out when a scan is ingested and stored on the
finding** (REM-26), and every open finding's is recomputed when the policy that
sets it changes. Computing it per request costs a pass over every open finding
*per urgency band*, because each band allows a different number of days —
measured at about eight seconds over 441,108 findings, on the screen whose
whole purpose is noticing something before it runs out. Stored, it is an
indexed range scan.

Urgency is stored at ingest for the same reason and carries the same staleness,
but nobody edits the ranking and people do edit deadlines, so the stale window
is not acceptable here: changing "a high may stay open sixty days" has to move
the list rather than wait for the next nightly scan.

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

### One query, on the stored deadline

The list of what is running out is one range scan ordered on `due_at`, which is
what storing the deadline bought.

It was asked band by band before that — once per window, each with its own
first-sighting cutoff, merged afterwards — because the deadline was the first
sighting plus a number of days that differs per band, and ordering by it meant
date arithmetic, which has no portable spelling across the four engines
(DAT-02). Storing the deadline removes the arithmetic from the read, so the
bands collapse into one statement.

Before *that* it was written the way it should not be, and the lesson is worth
keeping: the statement took the oldest findings and a loop then discarded
whatever was not due, so an exploited finding first seen yesterday and due
tomorrow lost its place to a low from two years ago that filled the buffer. A
list ordered on a proxy for the answer is not the list it claims to be.

**One row per issue at a component**, not per place. A kernel flaw sitting at
sixty places is one thing somebody has to answer, and sixty rows of it is a
list with one entry in it. A person is named on the row only where every place
has the same one — reporting one of several would say a finding is being dealt
with when most of it is not.

Days remaining are rounded down rather than truncated toward zero. Truncation
reports something twelve hours overdue as having zero days left, which reads as
due today.

## Release readiness

A branch beside the last release cut from it: *8 criticals now, v2.4.1 shipped
with 4*. The question asked before shipping, and the reason a branch trend is
worth having at all (RPT-12).

Both halves come from scans already collected — the branch is scanned nightly
and the release was scanned when it was cut — so this asks nothing new of a
build pipeline.

**The same variant on both sides.** A branch built for one chip beside a
release built for another compares two different pieces of software, and the
difference reads as a regression somebody then goes looking for.

**The release is the newest one cut from this branch that has been scanned
here.** A tag is cut at a moment and never moves again, so the newest
declaration is the last thing shipped. One declared and never built has no
counts, and answering with zeroes would report a clean release that does not
exist — so where there is nothing to compare against, the comparison is absent
and what is missing is said instead. *We shipped with none* and *we do not know
what we shipped with* are answers a person acts on differently.

**A tag is not compared against itself.** It is one frozen point and was not
cut into anything, so there is no "since we shipped" for it. Said rather than
answered with an empty comparison, which reads as a branch that has released
nothing.

Counted as issues at components, at or above the deployment's line, and the
line is named beside the number (RPT-14).

## What was dismissed, and what is being scanned

**Dismissals are a reportable dataset in their own right** (RPT-01) —
everything decided not to be fixed, and why. That is the core of the reporting
role, and it is the question an auditor asks first. It is answered two ways:
the findings list filters on outcome and on whether a rating was changed, and
the record screen lists every judgment with its reasoning, its approvals and
the dates they happened on — where the justification is shown. It is not a
filter: the outcome is the question an auditor asks the list, and which of the
five reasons applied is what they read on the row they stopped at.

**What is being scanned, and when each build was last seen, is its own view**
(RPT-02) — the shape of it is in `DESIGN-ingest.md`, and the reason it matters
belongs here. A product silently dropping out of scanning is the failure that
quietly makes everything else wrong: every number on every screen goes on
looking reasonable, and each one is describing a build nobody has looked at
since. A broken scan reports itself; one that stopped arriving reports nothing,
which is why absence is derived rather than waited for.

**Releases past end-of-life are reported apart from live ones and raise no
coverage alert** (RPT-04). A dead release not being scanned is expected. Left
in, the view that exists to catch the product that dropped out silently fills
with releases that stopped on purpose, and stops being read within a month.

**This is a report an operator uses, not the tool publishing** (RPT-08). We
produce the numbers and a person decides what to do with them; nothing here
emits anything outward, which is why publication being entirely Phase 2 does
not contradict a comparison that reads like release notes.

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
| The triage floor | The severity below which findings are still recorded and counted but kept off the working list. A product may state its own instead |
| Quiet after | How long a build may go without a scan arriving before it is reported as having gone quiet |
| Scan every | How often everything tracked is scanned again against the day's vulnerability data — what finds an advisory published after a release shipped |
| Upstream currency | Whether to ask public package indexes what the newest version of a component is. Off unless turned on: the only thing here that reaches the network |

**The shipped numbers are a starting point rather than a recommendation.** What
a deployment can hold to is a question about that deployment, and a deadline
nobody agreed to produces an estate that is permanently late and a signal
everybody ignores.

Only the names in that list may be set. A name the deployment does not know is
refused, because storing it would create a setting nothing ever reads.

### A value nothing can read is refused, not stored

Every reader falls back to the shipped default where a setting is unset or
unparseable, which means a stored value nobody can read is a policy that
quietly stopped applying. So the value is checked before it is written, against
the kind the name is: a **duration** for the windows, the threshold and the two
lifetimes; a **whole number above zero** for the limit on one action; a
**severity word** for the triage floor; and **on or off** for upstream
currency.

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

## Remediation metrics

Fix velocity, average time to remediate by severity, and aging buckets over a
window (RPT-03). Built, and narrowed by the scope picker like every other
figure.

It is worked out from what was already here — a finding records when it opened
and when it closed, so how long something took is a subtraction rather than a
second record somebody maintains. Two rules make such a figure honest or
useless, and both are enforced rather than described:

**A closure only counts as resolved if the issue actually went away** (RPT-15).
A bump that carried the issue with it, and a finding the scanner silently
stopped reporting, are not fixes, and a velocity figure counting them measures
churn.

**Counted as issues, not places.** One kernel flaw across sixty modules is one
thing that was fixed, and a mean time to remediate weighted by how far a
component fans out is a measurement of the dependency graph.

**Subtracting two moments has no portable spelling**, so this is one of the few
places an engine is asked directly. It is confined to a single expression
rather than spread through the query, and the answer comes back as a fraction
of a day: declaring it a whole number scanned on none of the four — one refused
a float outright and three handed back a decimal string.

## What keeps being put off

Places deferred more than once, with how often and for how long in total
(TRI-19). The cumulative threshold already refuses a *further* deferral past a
point, one item at a time; what it cannot show is the shape across everything,
and one item deferred three times is a judgment where forty of them is a policy
nobody wrote down.

**Counted over the judgments rather than the findings they cover**, for the
same reason the metrics above are: counting findings would order the list by
how far a component spreads through an image.

**A withdrawn deferral is not time anything spent put off.** Somebody taking a
decision back would otherwise look like somebody avoiding the work.

## Where these are read

A **Reports** screen, beside the record rather than inside it: the record is
what was judged, and these are how the judging is going. Both follow the scope
picker.
