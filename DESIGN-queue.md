# Work queue

Durable background work, held in the application's own database.

Satisfies DAT-17, ING-03, ING-19, SEC-06.

## Why not a library

The mature Go queues are tied to one database engine or need a separate
service. Neither suits software an operator installs against whatever database
they already run — one would cut our engine support to a quarter, the other
would add a component to every deployment for the sake of one internal
mechanism.

## Why durable

A job is a row. Work survives a restart, and a worker that dies mid-job leaves
the job claimed until the claim goes stale, after which another worker takes
it.

Losing work to a rescheduled pod would mean a scan silently never arriving —
the producer reported success, and nothing else ever says otherwise.

## Handing work out exactly once

Two workers must never get the same job. On an ingest, the same scan processed
twice does not look like an error; it looks like real change.

This is guaranteed by a **conditional update**: the statement that claims a job
repeats the conditions that made it claimable, so it only succeeds if the job
is still claimable. A second worker's update matches nothing and it moves on.
That works identically on every engine.

**Row locking is separate, and is about throughput.** `FOR UPDATE SKIP LOCKED`
stops workers queueing behind one another on the same row: without it they all
select the oldest job, one wins, and the rest did a round trip for nothing.

The split matters. If locking were the guarantee, then getting it wrong for
some future engine would mean handing the same work out twice. As it stands the
worst case is slow. This is verified: with the locking removed entirely, the
exclusivity test still passes.

SQLite needs no locking at all — one process, one connection, so the
surrounding transaction already excludes every other claim.

## Failure

| | |
|---|---|
| **Retry with a growing delay** | A dependency that is briefly unavailable should not be hammered while it recovers |
| **A limit on attempts** | Without one, a job that can never succeed retries for ever and crowds out work that could |
| **Set aside, not deleted** | A job that exhausted its attempts is kept with its last error. Deleting it would remove the evidence of why it failed |

## A claim goes stale

A worker that dies holds its claim. After a timeout another worker may take the
job, so work is never stranded by a lost process.

The timeout must exceed the longest a job legitimately takes. Set too short,
two workers run the same job — which is the one thing the conditional update
cannot prevent, because from the database's point of view the second claim is
legitimate.

## Backlog

New work is refused once too much is waiting, rather than accepted and queued
behind a growing pile.

This is what stops one runaway producer pushing everyone else's scans behind
its own. The producer gets a clear refusal and retries; the alternative is
every other product's scans arriving late for reasons nobody can see.

## What a job holds

A kind, a reference, and its state. **Never a payload.** The reference points at
the thing being worked on, so the queue stays small however large that thing
is — which matters when the thing is a scan file measured in tens of megabytes.
