# Remediation

What somebody intends to fix, where, and how the tool finds out whether it
happened.

Satisfies REM-02, REM-03, REM-07 to REM-10, REM-13, REM-16. REM-04's escalation
view, REM-05's work distribution and REM-06's cross-branch status read what is
here and are described in `DESIGN-reporting.md` and `DESIGN-findings.md`.
REM-11 and REM-12 are the external hand-off, which is not built.

## Nothing here is declared done

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

## The five things a build can be

Every build of the product that holds the issue is listed, chosen or not, and
so is every build that was chosen and no longer holds it.

| | What it means |
|---|---|
| **missed** | Chosen, a scan has finished since, and the issue is still there |
| **fixing** | Chosen, and no scan has looked since |
| **undecided** | Holds the issue, and nobody has said whether it will be fixed here |
| **clear** | Chosen, and the issue is gone |
| **retired** | Out of support, so it carries no target at all |

**Undecided is not a kind of outstanding.** Nobody is made to answer the same
question for six releases, so silence is allowed — but "open because we chose
not to fix it here" and "open because nobody thought about it" are different
answers, and a list holding both loses the second (REM-13).

**A missed target needs a scan that ran after the claim.** Without the "since",
every declaration made between two nights would be flagged the moment it was
written, which is most of them, and the flag would be worthless within a week.
Only a *finished* run counts: one still going may be about to report the issue.

**A clear build is still listed.** It is the one that worked, and dropping it
would leave finished work looking like work nobody ever planned.

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
approval threshold. A plan says where, not when.

**No hand-off to an external tracker.** REM-11 and REM-12 record that the seams
should exist; the integration does not, and private work defaults to no
hand-off whether it is built or not.
