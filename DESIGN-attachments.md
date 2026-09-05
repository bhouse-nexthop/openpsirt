# Attachments

Files hanging off a finding, and why none of this is built yet.

Satisfies ATT-01 to ATT-14, and carries the phase split (SCP-02) that decides
when they arrive. This document exists because the seam is what has to be right
early: an attachment is referred to from text that people are writing today,
and a reference format chosen later means rewriting what they wrote.

## What each phase is

**Phase 1 is public findings from scans; Phase 2 is private findings, entry by
hand, and publication** (SCP-02). The split is not about which is more
important — it is that private work can already be done in a forge's advisory
system today, and scanning a shipped image cannot be done anywhere.

Two things ship in Phase 1 anyway, because retrofitting them is the expensive
version: **the visibility field on every finding, and the role split that reads
it**. Every query is already narrowed by what the asker may see, so the day a
finding is entered by hand and marked undisclosed, nothing has to be revisited
to keep it that way. Attachments are the third: the reference format is settled
now so that text written before Phase 2 does not have to be rewritten.

## How the text refers to one

Markdown refers to an attachment by an **opaque identifier, never by a URL**
(ATT-05), written as `attachment:` followed by it. A URL in stored text pins
the storage arrangement into every justification and comment ever written, and
moving buckets then means rewriting the record a decision rests on. An
identifier resolves through the application, which is also the only way the
authorization below can exist.

The identifier is unguessable rather than sequential. Authorization is what
protects a file; this is what stops the existence of one being discoverable by
counting.

**A reference is recognized from the parsed document, not by searching the
text.** One inside a fenced block or an inline code span is being shown rather
than made, and somebody explaining how to write one has not attached
anything.

## The rule it waited for private findings to settle

Every fetch is authorized against **the visibility of the issue the attachment
hangs off**, and only then served (ATT-06). That is the same rule private
findings need, and building it twice is how the second one ends up weaker.

**A file somebody may not see and a file that is not there answer
identically**, in the same words. Told apart, a reference somebody guessed
becomes a way to ask which products exist and which of them hold undisclosed
work (ACC-08).

A private finding's attachment sitting in a readable bucket is exactly the
disclosure the public and private split exists to prevent, and it would be
invisible from inside the application — every screen would look correct while
the bytes were served by something else entirely. **No bucket is ever public.**

## Where the bytes live

An object store, reached through the S3-compatible API, and never the database
(ATT-02). Blobs in the database inflate every backup and every replica, and
they sit across the partition and purge rules that were written for rows.
The S3 API is what MinIO, Ceph and every cloud provider speak, so one
implementation covers self-hosted and managed alike.

**A local filesystem backend exists for development** (ATT-03), mirroring what
SQLite does for the database: the tool has to be runnable without standing
anything up first.

**The object store is optional** (ATT-04). With none configured, attachments
are off and everything else works. An operator who wants none should not have
to run one, and a feature that makes a dependency mandatory for everybody is a
feature that decides the deployment.

## What is served, and how

**A content type we chose, never the one that was uploaded**, with
`Content-Disposition: attachment` (ATT-07). What a browser does with a file it
was told is HTML is not something to leave to whoever uploaded it.

**Inline display only for a small allowlist of raster image types** (ATT-08).
Everything else downloads. The allowlist is raster deliberately: a vector image
is a document with a scripting engine.

## Limits, removal and what is not solved

**A maximum file size and a per-deployment quota, both configurable** (ATT-09).

**An attachment is never deleted while anything references it** (ATT-10).
Removal is an explicit administrative redaction, recorded, leaving the
reference and a tombstone where the file was — so text that pointed at
something says what happened rather than pointing at nothing.

**Uploads that were never attached to anything saved are reaped** (ATT-11).
Somebody who attaches a file and then abandons the form has left bytes nothing
will ever reach.

**Uploads are not scanned for malware** (ATT-12). Stated rather than solved: an
operator handling files from outside their organization should know that this
does nothing about it, and a claim to scan that rested on one engine's
signatures would be worse than the honest sentence.


## What an attachment hangs off

**The issue in the product**, which is the unit a decision, an embargo and a
comment already use. Not the finding row: text is written against a decision,
a decision covers every place an issue sits at, and binding a file to whichever
of forty-eight rows somebody happened to be looking at would make it
unreachable the day that row closed while its siblings stayed open.

**Its visibility is the issue's, and is never stored on it.** A comment carries
no visibility of its own either — it is authorized through the decision it
hangs off, and inherits what that covers. An attachment is the same kind of
thing and is read the same way, so anybody who may read the text may read what
the text refers to.

Storing a visibility at upload would freeze it. The embargo that made an issue
private ends, and the file documenting it has to become readable along with the
words describing it — a copy taken at upload would leave it private forever,
describing a moment that has passed rather than a fact that still holds.

**Before the text exists there is no decision yet.** A claim and its first
reasoning are written in one transaction, so a file attached while composing
one is bound to the issue alone until the text referencing it is saved. That is
the same unattached state the reaper already deals with (ATT-11).

**A file may hang off the issue itself rather than off text** (ATT-15).
Evidence for a
flaw somebody recorded — a test case that proves it — is pointed at by the
issue the moment it arrives, so it counts as attached at once. Waiting for text
that will never be written would mean the sweep took it a day later, and a
screen listing what is attached would never show it. The caller says which it
is; the default is the other one, because a file uploaded while somebody is
part way through writing is the case that can be abandoned.

## How the bytes are reached

**An image displayed inline is served by the application; everything else is
redirected** (ATT-13). Both are authorized identically and first; what differs
is only who sends the bytes.

The reason is the content security policy. A page served here may load images
from this origin and nothing else (SEC-20), so an `img` whose source redirects
into an operator's bucket is refused by the browser with nothing on the page or
in a log to say why. Carrying those bytes ourselves keeps the policy a constant
— and the constant is most of its value, because a policy assembled from
configuration fails open when the configuration is wrong.

What we carry is bounded twice over: by the raster allowlist (ATT-08), and by
the size limit every upload passes (ATT-09). A log, an archive, anything large
is not an inline image and is still redirected, so the case that would cost
bandwidth is the case that never touches us.

**The signed URL carries the headers we chose**, as response overrides rather
than as whatever was stored, so `Content-Disposition` and the content type
survive the redirect (ATT-07).

## How it is referred to

`attachment:` followed by the opaque identifier, as the target of an ordinary
markdown link or image. Nothing else in the text changes, and no address of the
store ever appears in it (ATT-05).

This is the one scheme added to what a link may use, beside `http`, `https` and
`mailto`. **An image is permitted only with this scheme** — the rule that
nothing is fetched from anywhere else is unchanged, and is what
`DESIGN-text.md` describes: an image from a third party reports who read a
finding and when, which on an undisclosed one is a disclosure channel.

## The client

The official AWS SDK for Go, v2 (ATT-14). Chosen by measuring it against a
signer written here rather than by preference: the credential chain is what a
deployment on a cloud provider actually needs, and it is the part that cannot
be tested anywhere else.


## What is built

All of it. The table, the two stores, the upload, the authorized fetch, the
limits, the redaction and the sweep.

**One object per attachment, and never content-addressed.** Two identical files
uploaded twice are two attachments with two keys. Deduplicating by digest would
mean a redaction blanking a file somebody else was relying on, and ATT-10 makes
that a real case rather than a hypothetical one — the whole point of a
redaction is that it removes bytes somebody else can see. The digest is kept
beside the row anyway, so that a redaction can say what it removed once the
bytes are gone.

**The row is written after the bytes and removed if it cannot be.** The other
order leaves a reference to something that never arrived. What this order can
leave is bytes with no row, for exactly as long as the failing write takes, and
those are removed on the way out rather than left for a sweep that has no
record to work from.

**The sweep and a redaction remove things in opposite orders, on purpose.** A
redaction marks the row and then removes the file, because a file removed with
nothing saying so reads as a store that lost it. The sweep removes the file and
then the row, because nothing refers to these — a file gone with its row still
there is collected again next time, where a row gone first would leave bytes
nothing can ever find.

**The quota is asked twice**: once before anything is carried, so an upload
that cannot be kept is refused rather than transferred and thrown away, and
once inside the transaction that writes, because the first answer was read
before the bytes were and the deployment may have filled up while they were.

**The local store is confined structurally rather than by checking.** Names
go through a directory handle that cannot be escaped, instead of a path
comparison this package performs. The keys are ours and contain nothing
anybody typed, so nothing can reach outside today; confinement that depends on
every future caller staying careful is the kind that stops holding.

## What is deliberately not solved

**Uploads are not scanned for malware** (ATT-12). Stated rather than implied by
silence: an operator handling files from outside their organization should know
that this does nothing about it, and a claim to scan resting on one engine's
signatures would be worse than the honest sentence. Scanning belongs in front
of the bucket, where an operator can choose it.
