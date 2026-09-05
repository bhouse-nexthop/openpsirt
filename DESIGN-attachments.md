# Attachments

Files hanging off a finding, and why none of this is built yet.

Satisfies ATT-01 to ATT-12, and carries the phase split (SCP-02) that decides
when they arrive. **Nothing here runs.** They were to arrive with the private
findings whose access rule they need; those have shipped and these have not, so
what remains is the work rather than a dependency. This document exists because
the seam is what has to be right early: an attachment is referred to from text
that people are writing today, and a reference format chosen later means
rewriting what they wrote.

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
