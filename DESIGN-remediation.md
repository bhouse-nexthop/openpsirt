# Remediation

What somebody intends to fix, where, and how the tool finds out whether it
happened.

Satisfies REM-01 to REM-03, REM-06 to REM-10, REM-13, REM-15 to REM-24.
REM-04's escalation view and REM-05's work distribution read what is here and
are described in `DESIGN-reporting.md` and `DESIGN-findings.md`. REM-11 and
REM-12 are the external hand-off, which is not built. Of "Publication" below,
the CSAF document is built and every adapter that would send it somewhere is
not.

## Nothing here is declared done, with one exception that proves the rule

A fix is **declared** and never **completed**. Somebody says which releases
they intend to fix an issue in; whether it arrived is answered by the next scan
of each of those releases (REM-09).

This is the whole shape of the feature, and it is worth being explicit about
what is being refused. The obvious design has a state on each target — planned,
in progress, done — that somebody moves along. It is obvious because every
tracker works that way, and it is wrong here for one reason: we already have
independent evidence. A scan of the release either finds the issue or does not.
A second record of the same fact is one somebody has to keep true, and the way
that fails is the tool reporting a fix that shipped in nobody's release.

So the stored row is the declaration and nothing else: which build, who said
so, and when. There is no state column, no completed-at, and nothing to move.

**The exception is a flaw somebody recorded by hand** (REM-28), and it exists
because the argument above turns on having independent evidence. No scan
reports such a flaw — a run is the authority on what it found, and it found
none of this — so there is no second opinion to check a declaration against,
and refusing the declaration does not leave the tool cautious. It leaves the
finding open forever. That one class is closed by a person, with who, when and
why on the record, and nothing else is: a scanner's finding declared fixed by
hand is exactly the failure this section refuses. `DESIGN-findings.md` carries
the shape.

## The unit is the work, not the finding

An issue in a component is one thing to fix, however many places it sits at and
however many variants ship it. That is the unit assignment already uses
(REL-01, REM-10) and the plan uses the same one: one plan, one owner, covering
every build it names.

The build is what the plan is *about*, so the key is that trio plus the build.
A release is named by its stream and its variant together, never by one of
them: the same branch built two ways is two builds, and a plan that named only
the branch would claim a fix for hardware nobody built for.

**Acting on the plan acts on the product, not on the build in the path.** A
route names a finding so a screen has somewhere to link to; what is planned
belongs to the work that finding is part of. This is the same rule assignment
follows, for the same reason: planning one build would leave the identical work
unplanned beside it.

## What a request may say

The plan is a **set, written whole** (REM-08). Intent spans several releases
and is decided in one sitting, so a request says what the answer now is rather
than adding and removing one release at a time. An empty set withdraws it.

Withdrawing is the same operation as declaring, not a second one. Two paths
that do opposite halves of one job drift, and the half that gets tested is the
one somebody uses more.

**A build already in the plan keeps the date it was first chosen on.** When
somebody committed to a release is a fact about a moment. Rewriting the set to
add a second release must not move the first one's date to today, or an edit
that said nothing about the first release silently rewrites the record of what
was promised when.

**A build of another product is refused, and the whole request with it.** Not
narrowed to the ones that belong here: a partly-applied plan leaves somebody
believing a release is covered. What a request can name is a release and a
variant, resolved against the product in the path, so crossing a product
boundary is not something the API can express at all — the refusal in the store
is for callers that name a build by identifier.

**Declaring is triage work.** Being able to see a finding is not being able to
plan the work on it, and the plan is a write.

## The six things a build can be

Every build of the product that has ever held the issue is listed, chosen or
not, and so is every build that was chosen.

| | What it means |
|---|---|
| **missed** | Chosen, a scan has finished since, and the issue is still there |
| **fixing** | Chosen, and no scan has looked since |
| **undecided** | Holds the issue, and nobody has said whether it will be fixed here |
| **clear** | Chosen, and the issue is gone |
| **gone** | Nobody chose it, and the issue has left anyway |
| **retired** | Out of support, so it carries no target at all |

**A build the issue has left is listed even though nobody planned it.** This is
the other half of the question — "gone from main, still present in 2.4 and 2.3"
(REM-06) — and it is derived only from scans, because that is the only evidence
there is. Leaving it out would drop a fixed build from the list entirely, which
reads identically to a build that never shipped the component; those are
opposite answers, and the first is the one somebody came to find out.

It is not tickable. There is nothing left to plan for a release the issue has
already left, and offering the box would invite a claim on work that is done.

**Undecided is not a kind of outstanding.** Nobody is made to answer the same
question for six releases, so silence is allowed — but "open because we chose
not to fix it here" and "open because nobody thought about it" are different
answers, and a list holding both loses the second (REM-13).

**A missed target needs a scan that ran after the claim.** Without the "since",
every declaration made between two nights would be flagged the moment it was
written, which is most of them, and the flag would be worthless within a week.
Only a *finished* run counts: one still going may be about to report the issue.

**A clear build is still listed.** It is the one that worked, and dropping it
would leave finished work looking like work nobody ever planned. What separates
it from **gone** is only whether anybody said in advance that it would happen —
the evidence is the same scan either way.

**A retired release is listed and not counted.** Nothing on it will be fixed
(REM-16), so counting it as outstanding fills the figure permanently and
counting it as delivered claims a fix nobody shipped. It is neither. Choosing
one that is already out of support is refused rather than accepted and ignored,
because silently dropping it leaves somebody believing a release is covered;
a release that goes out of support *after* it was chosen is the case that
actually happens, and it drops out of the counts and stays on the list.

## Resolved is a count, not a flag

An issue is resolved when every build somebody chose is clear, and progress is
readable at any point: two of three clear, one outstanding (REM-09).

It is worked out from the same list every time it is asked for, so there is no
second place for it to be wrong. Nothing stores it, nothing refreshes it, and
nothing has to be invalidated when a scan closes a finding.

## What this deliberately does not do

**No commit or pull-request linkage.** Nothing here watches a repository, so
what can be recorded is which releases a fix is meant to reach — declared
intent (REM-07). Branch-by-branch backport tracking derived from commits is a
different feature needing a different integration.

**No per-item due dates.** A deadline comes from how urgent the finding is
(REM-14, REM-25), and moving one is a deferral, which carries a reason and an
approval threshold — and **the deferral date then becomes the effective
target**, with deferred items reported apart from plainly overdue ones
(REM-15). A conscious, approved postponement and an item nobody looked at are
different states, and a figure holding both says nothing about either. A plan
says where, not when.

**Assigning notifies; planning does not** (REM-01). Somebody is told when work
is put on them, because that is a thing done *to* a person. Saying which
releases the work is meant to reach is a thing done to the work, and it changes
nothing about who owns it.

**No hand-off to an external tracker.** REM-11 and REM-12 record that the seams
should exist; the integration does not, and private work defaults to no
hand-off whether it is built or not.

## Publication: the document is generated, and nothing is published

Phase 1 publishes nothing (REM-24). Publication arrives with private findings,
because of what it is for: an advisory is about a vulnerability in our own
product, and a known CVE in a shipped third-party component is dependency
hygiene that a consumer can already see from the inventory (REM-23). Issuing a
vendor advisory for every upstream CVE in a dependency is not what an advisory
is. That is a scope choice for now rather than a permanent boundary — some
vendors do publish on third-party components.

**It is a pluggable output with several adapters, not one integration**
(REM-17). Publishing routes differ completely by product: an open-source
project uses its forge's advisory system, an appliance vendor publishes on its
own site under its own process. One integration would be one of those wearing
the name of the general case.

**We own the triage record; the platform owns the published advisory** (REM-18).
That question decides whether an integration works or rots. Ours is the
decision history — who judged what, who agreed, when — and theirs is the public
artifact. Neither is a copy of the other and neither syncs.

**CSAF is the adapter that matters for commercial products** (REM-21): a
machine-readable advisory document the vendor publishes wherever they like. It
is nearly free from what is already held, because VEX is a CSAF profile and the
dismissal vocabulary was aligned to it from the start.

**A forge's advisory system is one adapter, for products hosted there**
(REM-22) — draft privately, request an identifier, publish, and use its
temporary private fork for fix work under embargo. That covers advisory
publication, coordinated disclosure and the private fix space together, and it
applies to nothing hosted elsewhere.

**Publishing aggregates** (REM-19). One advisory covers a product and a version
range, not a path — which is the one place this design collapses rather than
separates, and it is stated because everything else here is deliberately
granular. A reader of an advisory is asking "am I affected", and the answer is
a version, not a dependency chain.

**Optional and configured per deployment** (REM-20), like any other hand-off.
Nobody should need an account anywhere to use this.

### What is built: the CSAF document

A CSAF 2.0 document is generated for one issue in one product, from what is
already held (REM-21). **Nothing is sent anywhere**, and nothing records that
an advisory was issued: the triage record is ours and the published artifact
belongs to whoever publishes it (REM-18), and a second copy of the same fact is
one somebody has to keep true.

**Only for a flaw in what we ship.** An issue a scanner reported against a
third-party component is refused by name rather than answered with a document
that looks the same and means something else (REM-23).

**The publisher is deployment configuration**, not an administrator's setting:
it is the identity of the organization running this, in the way the address
people arrive on is. Both a name and a namespace are required, because a
document naming no publisher is not a CSAF document — so an unconfigured
deployment is refused with the reason rather than handed something that fails
validation after it has been sent.

**A document about an undisclosed flaw is a draft**, and says so in the field a
reader checks before acting on one. Reaching a disclosure date discloses
nothing (ACC-47), so generating a document does not either.

**The releases are named by stream and variant together**, never by one of
them: the same branch built two ways is two builds, and naming only the branch
would claim something about hardware nobody built for. Every release a status
refers to is named in the product tree, and the list is ordered here rather
than by the engine, so two documents generated from the same facts are the same
bytes and a diff between them is a real change.

**A release that fixed the flaw is named as fixed**, rather than left out —
leaving it out reads identically to a release that never shipped the thing at
all, and those are opposite answers with one of them the answer a reader is
hoping for. What fills that list is somebody saying so (REM-28), because for a
flaw recorded by hand no scan ever will; `DESIGN-findings.md` says why the
exception is exactly that wide.

### What is not built

**The VEX profile.** The document is categorized as a security advisory rather
than as VEX, because the VEX profile's point is "not affected, and here is
why", and those justifications are not assembled into it. The vocabulary is
already the right one — the dismissal words were aligned to VEX from the start
(TRI-06) — so what is missing is not the words but the mapping from a decision
to the releases it covers.

**Every adapter.** REM-17's several outputs, and REM-22's forge advisory
system, are not built. What exists is the document and a way to fetch it, which
is the part every route needs and the part that is ours; where it goes next is
the part that differs completely by product.
