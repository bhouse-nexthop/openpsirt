# Notifications

What somebody is told, and through what.

Satisfies NTF-01 to NTF-19. Mail is built, behind the one interface a chat
adapter will use; what is missing is named at the foot rather than left to be
discovered.

## The area that works with nothing set up

A self-hosted operator who never configured mail would otherwise have every
operational alert sent into a void (NTF-08). So the notification area is the
channel that always exists, and it is in front of whoever is actually using the
tool rather than relying on somebody reading mail.

**Everyone has one** (NTF-10). A triager sees work arriving, a proposer sees a
claim sent back, an approver sees what waits on them, an administrator sees
that the tool itself is unwell. What differs by role is the content; the
mechanism does not.

## Two lifetimes, and the difference is the design

**An event happened once and is acknowledged.** You were assigned this, your
claim was sent back, somebody named you. It goes away when the person says they
have seen it, because that is the only thing that can end it.

**A condition is true while something is true, and clears itself** (NTF-09). A
build that stopped being scanned should leave the list when it is scanned
again, without anybody dismissing it. Otherwise the count fills with problems
that already went away, and then nobody reads the count — which is the same
failure the digest rules were written to avoid.

So a condition names what it is about, and the pass that derives conditions
reconciles rather than appends: what is true is opened, what has stopped being
true is cleared, and running the same pass twice changes nothing. A pass never
has to remember what it said last time.

**A condition that returns is a new row rather than an edit of an old one.**
Keeping the cleared row is what makes "this cleared, and then came back" a
thing somebody can see, instead of a single row whose history has been
overwritten.

**Acknowledging a condition hides it rather than resolving it.** The thing it
is about is still true. It is worth offering because somebody may have decided
to live with it, and worth distinguishing on screen because the two acts are
not the same.

## What is told, and when

| | Kind | |
|---|---|---|
| Work arriving | event | The thing a triager most wants to notice, and the category that deserves interrupting somebody for (NTF-02) |
| A claim sent back | event | It goes straight back into its author's queue, so silence leaves it sitting while they wait to hear (NTF-05) |
| Somebody named you | event | Not built: the editor writes `@name` and nothing resolves it to a person yet (UIX-24). There is no word for it either — a name nothing can produce, advertised in an API description and drawn by a screen, is a promise the tool does not keep |
| An approval an edit withdrew | event | Not built. NTF-13 wants the people who granted it told, so it does not quietly stop counting |
| A build that stopped being scanned | condition | Built. A sweep derives every declared build with when it was last scanned, compares it against how long this deployment allows, and reconciles — so a build that resumes leaves the list on its own |
| An embargo whose date arrived | condition | Built. Reaching the date discloses nothing (ACC-47), so this is a question still waiting for an answer rather than a thing that happened — and it clears when the date is moved or the finding is disclosed, because both of those are somebody answering it |

**A mention notifies the person mentioned, immediately** (NTF-12), and is the
row above is built: a name written after an `@` is resolved when the text is
saved, and whoever it names is told at once (NTF-12).

**Only somebody who could already read it**, from the same query the editor's
autocomplete uses — so a mention cannot tell somebody that a finding exists
when they may not see it. On an undisclosed finding the notification itself
would be the disclosure.

**A name nobody holds and a name held by somebody who may not read this are
not told apart.** Both simply reach nobody. A refusal naming which it was would
answer, one comment at a time, whether a given person can see undisclosed work.

**Never the author**, who is not asking themselves a question, and never a
failure of the write: the words are on record by the time anybody is told, and
losing a comment because a notification could not be stored would be
sacrificing the wrong half.

**A new build notifies nobody** (NTF-04). A build arriving is the ordinary
state of a tool that is scanned nightly, and a message per night per build is
how somebody learns to ignore the channel. What a build *changed* is on the
receipts and in the trend; that a build happened is not news.

**A newly-critical vulnerability in a shipped release is an operational alert**
rather than a line on a dashboard (NTF-11), and operational alerts are their
own category, outside the opt-in digest (NTF-07) — because a category somebody
has to opt in to is one that is silent on the deployment that most needs it.

It is a **condition**, so it clears when the finding closes or when somebody
answers it, and nobody dismisses it. **Tags only**: a critical on a branch is
ordinary work in progress, and sending both would make the alert as frequent as
the findings list and therefore ignorable. What separates them is the whole
signal.

**It goes to whoever may read it and may triage it** rather than to
administrators (NTF-19). The other operational alerts are about the tool — a
build nothing has been filed against names no finding — so administrators are
the right audience for those. This one names an issue, a component and a build.
Since an administrator does not read a product by administering it (ACC-64),
sending it to administrators alone would disclose to people who may not read it
and stay silent for the people who can act. Reading is not enough on its own
either: an alert interrupts, and interrupting somebody who cannot act is noise
they will learn to ignore — it is on their findings list in any case.

**The proposer's own view lists their dismissals still awaiting review**
(NTF-06). Silence covers both "approved" and "nobody has looked", and those are
not the same thing to the person waiting — which is what the queue's own
"mine, pending" tab answers.

## Somebody away, still holding work

An administrator is told when somebody has not signed in for a configured
period and is still holding assigned work (ACC-45). A condition like the
others: it becomes true on its own and clears on its own when they sign in,
without anybody dismissing it.

**Both halves, and the second is what makes it worth saying.** An account
nobody has used in a month is harmless if it holds nothing. Work stuck behind
somebody who is not here is the problem, and this is the prompt that makes an
administrator realize they have gone.

**It asks rather than acts.** Long leave and having left look identical from
here, and nothing detects somebody leaving (ACC-44) — so it opens something an
administrator reads, and handing the work back stays a deliberate act by a
person. Withdrawing somebody's last role on a product does hand back what they
held there (ACC-43), and that is somebody deciding.

**Never having signed in counts as absent, measured from when they were
added.** Somebody granted a role and given work who has not arrived is exactly
the case worth raising — but compared against the moment alone, an
administrator adding a colleague and assigning them something raises an alert
about them in the same breath. It is the same shape as a build declared and
never scanned, which is measured from its declaration for the same reason.

**Counted in pieces of work rather than in findings**, like every other figure
about what somebody holds: "6 items" meaning 288 findings would make an
ordinary absence look like a catastrophe.

## What is not built, and what it will be

**A chat adapter**, behind the same interface mail uses (NTF-01). Not built.

**Mail carries the markdown as its text part** (NTF-14), which it does. A chat
adapter translates rather than forwarding it, and mostly sends a summary and a
link rather than the whole thing — a channel people read on a phone is not a
channel to paste a justification into. That adapter is not built, and neither
is the HTML part of a mail, which is the only remaining reader for the
server-side renderer and why it is kept rather than deleted.

**The digest is off by default** (NTF-03), and so is the part of it that lists
what nobody owns (NTF-17). Both are built and both are described above; they
are named here because "opt-in" is the half people look for in this section. A
digest nobody asked for is mail somebody filters, and a filtered channel is
worse than none: it looks like it is working.

## Carrying something out of the application

**Mail is a channel behind one interface** (NTF-01), and a chat adapter is a
second implementation of how to carry rather than of what to say. What may be
said is decided once, in one place, for every channel there will be — a second
place for that is the one that gets it wrong.

**What is unsent is the work list.** A sweep reads notifications nobody has
carried out yet, sends them, and marks them. A message that failed needs no
state of its own to say it should be tried again: it is simply still unsent.
That also means a deployment that configures mail after running for a week
finds the week waiting rather than lost.

**A message is tried five times and then left alone.** A mailbox that refuses
every time has gone, and a sweep that keeps trying it is one that eventually
does nothing else. The row stays unsent and stays readable: the area inside the
application still has it, and nobody is told a message arrived that did not.

**Somebody with no address is sent nothing**, and the query says so rather than
the loop, so a deployment where nobody has one does no work at all.

**The password is not protected from being logged, because nothing logs it.**
A formatter that redacts the configuration was written and then removed: the
gate that refuses exported code nothing reaches was right to refuse it, and
the reasoning it rejected — "nothing logs it today, the point is that it stays
safe when something does" — is the reasoning that gate exists to catch. If
anything ever formats the configuration, the redaction goes in with it, where
it can be seen to work.

**Credentials are refused over a connection the server would not secure.** The
sweep offers STARTTLS and will not send a password without it: a password sent
in the clear to whatever answered on that port is a password given away, and an
operator who configured one is entitled to assume it is not.

## The digest, and why it was nearly empty

Everything built so far leaves immediately. Work arriving and a claim sent back
are explicit human actions (NTF-02, NTF-05); a build that stopped being scanned
and an embargo that reached its date are operational alerts, which go to
administrators and sit outside the opt-in (NTF-07). A daily message carrying
what was left would carry nothing, and a channel that arrives empty is the one
people stop opening — which takes the rest of the channel with it.

**So the digest carries what nothing else told you** (NTF-16), and there are
exactly two of those. It is built, and it is assembled as its reader: every
query narrows by the subject it is handed, so a digest cannot contain something
the person it is for could not open. The sweep holds no rights of its own.

**Work that became yours without a message.** The immediate one is not sent
when somebody assigns something to themselves, when nothing could send at that
moment, or when the person had no address yet. Those are silent today, and
silence about work that is now yours is the failure the channel exists to
prevent.

**Findings nobody owns** (NTF-17), for whoever asks for them. Somebody
triaging a product wants to know what arrived; somebody who only holds work
assigned to them does not want a daily list of everything found anywhere. That
is why it is a second switch rather than part of the first, and why neither is
derived from a role: a role says what somebody may do, not what they want to
read, and a channel nobody can turn off without giving up an ability is one
people route to a folder.

**Nothing is repeated.** A thing already sent immediately is not in the digest,
which is what makes "carries what nothing else told you" a rule rather than a
description. An event records what it was about for exactly this — kept apart
from the name a condition clears against, which is a uniqueness key and would
deduplicate two unrelated events into one.

**A digest with nothing in it is not sent**, and the clock still moves. A daily
message that says "nothing" is how somebody learns to stop opening the daily
message, and the ones that say something go unread with it; but leaving the
mark unmoved would make a quiet week report itself as new the following
Monday.

**A first digest reports nothing under "nobody owns".** There is no "since" to
measure against, and arriving to a list of everything ever opened is the same
as arriving to no channel at all.

**One message is bounded.** What is over the bound stays in the application,
which is where somebody works through it rather than in a mail client.

**It names what has been disclosed and gives numbers for what has not**
(NTF-18). A public finding is listed with its issue, component and build. The
undisclosed ones become a count and the figures that say how urgent they are —
how many at each severity, how many known to be exploited, how many nobody owns
— with the way in NTF-15 gives.

Lateness is deliberately not among them. An embargo whose date has passed
already raises an alert of its own (ACC-47), and repeating it here would say
one thing twice while leaving out the figure that actually decides whether this
is tonight or tomorrow.

A bare count would not have been enough. "Three undisclosed" does not tell
somebody whether to open the tool now or after coffee, and a channel that
cannot answer that is one people stop reading. Severity and lateness answer it
and identify nothing.

**The figures name nothing, including the product.** "Two critical embargoed
items in this product" is a statement about that product, and a mail server has
no business holding it. The recipients already hold the right to read these
(ACC-47); what the aggregate protects is not them but the path the message
takes to reach them — an inbox, a preview, a forward, a server this deployment
does not run. So what is urgent is a number, and what it is about is behind the
link.

## What leaves the building says less than what stays in it

**A message about an undisclosed finding carries no detail — that there is
something, and a link** (NTF-15). Not the identifier, not the component, not
the summary, and not in the subject line, which a preview shows without
anybody opening anything.

The in-app area is not held to this, and the difference is the whole reason for
the rule: reaching a notification there means holding a credential and passing
the visibility check, so the area can say what the thing is. A message cannot
re-check its reader. It sits on a mail server this deployment does not run, in
an inbox, in a phone's lock screen, and in whatever it is forwarded to — so an
alert about a flaw nobody has announced is itself the announcement unless it
says nothing.

**A disclosed finding is not held to this**, and saying so matters as much as
the rule. A public vulnerability in a shipped component is public: the
identifier, the component, the version and the build are in the message,
because that is what makes it worth opening, and none of it is anything a mail
server could leak that a vulnerability database does not already publish. A
channel that says nothing about anything is one people stop reading, and then
the embargo alerts go unread with the rest.

The line is the finding's own visibility. It is not a second judgment made at
the point of sending — it is the same field every query already narrows by, so
a message cannot disagree with what the screen would show the same person.

That is a different question from who is told, which ACC-47 already answers.
Narrowing the recipients and emptying the body are both needed: the first stops
it reaching somebody who should not know, the second stops it being disclosed
by the delivery itself.

**The address is part of what must say nothing.** The decision says a message
carries a link, and writing one showed that a link to the finding defeats the
rule it belongs to: a path carrying the identifier and the component announces
both to every server the message crosses, however careful the body was. So what
a private message carries is the way in — the deployment's own front door — and
the notification area behind it, reached with a credential and a visibility
check, says which thing and where.

What it costs is a click, which is the click the message was asking for anyway.

**Who hears about an embargo is narrower than who hears about the tool's
health.** Administrators, and whoever holds it (ACC-47) — and the second only
where they may still read undisclosed work in that product. Every one of these
alerts says an undisclosed finding exists, so the alert is a disclosure in its
own right; an assignment can outlive the role that allowed it, and delivering
one then would hand over exactly what withdrawing the role was meant to stop.

**A sweep reconciles everybody it has told, not only everybody it should tell.**
Reconcile makes one person's open set exactly what it is handed, so somebody
who is never handed a list is never reconciled — and their alert stands after
the thing it was about has been answered. That does not arise for a condition
that only ever goes to administrators, because that set does not move. It
arises the moment who hears about something depends on who holds it, and work
is handed around: the person who held it yesterday would keep an alert about a
date that has since been moved, with nothing left to clear it.

**Nobody is told they were unassigned.** A name being removed is not an action
directed at the person who held it, and a queue that gets shorter says so
already. Telling somebody something was taken away invites them to go and look
at what is no longer theirs.

**A failure to tell somebody is logged, not returned.** The assignment
happened, the claim was sent back; answering the caller with an error invites a
retry that does the first thing twice. The notification is the lesser half of
the pair, and it says so by failing quietly and loudly in the log.

**More than one process sweeps.** The chart ships two replicas and each runs
its own watch, so two of them opening the same condition at the same moment is
ordinary rather than a fault. The unique index is what makes it one row, and
the pass treats a duplicate as the answer already being there — without that it
would abort, and every administrator after the one it failed on would be told
nothing that cycle.

## Whose is whose

A notification identifier is a number a caller supplies, so reading and
acknowledging both check whose it is rather than trusting it, and one belonging
to somebody else answers exactly as one that does not exist.

**A key is not a person.** Identifiers for keys and people come from different
tables and collide as a matter of course, so what kind of subject is asking is
checked rather than inferred from the number — without it, a key numbered three
reads and acknowledges the notifications of person three. A test pins that by
giving the key the person's own number.

**A notification outlives the role that justified it.** Every check is at the
moment of telling — the writer asks whether the reader may read what it is
about (ACC-63) — and nothing asks again afterwards. Reading is narrowed by
person and by nothing else, so somebody whose private-triage is withdrawn keeps
a list naming undisclosed findings, in a product that is meant to read to them
as not existing. ACC-43 already hands their assignments back on that trigger;
notifications are not part of it.

Written down rather than fixed, because the shape of the fix is a question and
not an oversight: whether revocation *clears* those rows or merely stops
serving them is a decision about a record somebody was told something, and
clearing what a person was told is not obviously right. What makes it tractable
either way is that an event carries what it is about — the product, the issue
and the component as identifiers — so there is something to key on.

## What the line says

The body is stored rather than derived when it is read. It describes a moment:
the finding it names may since have been decided, closed or reopened, and
re-deriving the sentence later would describe the world now rather than the
world somebody was told about.

## Choices the decisions did not cover

**Events are not collapsed.** Being assigned the same finding twice is two
things that happened, and the second is the one they have not seen.

**The count is what the badge draws, counted through the same conditions as the
list.** A badge that disagrees with the list under it is the same class of
mistake as a total that ignores a filter.
