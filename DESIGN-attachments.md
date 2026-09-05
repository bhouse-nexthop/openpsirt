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

## What is built now, and it is only the seam

Markdown refers to an attachment by an **opaque identifier, never by a URL**
(ATT-05). That is the whole of the early commitment, and it is the one that
cannot be deferred: a URL in stored text pins the storage arrangement into
every justification and comment ever written, and moving buckets then means
rewriting the record a decision rests on. An identifier resolves through the
application, which is also the only way the authorization below can exist.

Nothing else is built. There is no upload path, no storage interface with an
implementation behind it, and no fetch.

## Why it waits for private findings

Every fetch is authorized against **the visibility of the finding the
attachment hangs off**, and only then redirected to a short-lived signed URL
(ATT-06). That is the same rule private findings need, and building it twice is
how the second one ends up weaker.

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
