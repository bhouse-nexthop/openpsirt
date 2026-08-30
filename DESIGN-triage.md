# Triage

What people decide about findings, and when a decision stops applying.

Satisfies TRI-01 to TRI-03, TRI-06 to TRI-08, TRI-10 to TRI-16, TRI-24,
TRI-25, REL-05, REL-06, REL-09, MDL-08, MDL-19.

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

Undoing a bulk approval narrows to what the person undoing it may reach. A
batch is one reviewer's afternoon and may span products, so taking it back
wholesale would let somebody act on products they hold nothing on.

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
