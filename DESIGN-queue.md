# Work queue

Durable background work, held in the application's own database, and which
replica does the work that should happen once.

Satisfies DAT-17, DAT-25 to DAT-28, DAT-38, DAT-39, SCP-15, ING-03, ING-19,
SEC-06.

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

Two workers running the same job is the one thing the conditional update cannot
prevent, because from the database's point of view the second claim is
legitimate: the row says the worker holding it has not been heard from.

## A claim is renewed while its work runs

**The timeout bounds how long a worker may go silent, not how long a job may
take.** A running worker renews its claim on an interval well inside the
timeout, so a renewal can fail several times over before the claim is at risk.

Without renewal the two questions collapse into one, and the timeout becomes a
ceiling on job duration: a scan that legitimately runs longer is handed to a
second worker while the first is still doing it.

**Losing the claim ends the work.** A renewal refused because another worker
now holds the job cancels what the first worker was doing, and it is told the
claim was lost rather than that the work failed. From that moment the work is
being done twice, and this is the copy whose ending nobody will record —
carrying on spends a scanner run and every write it makes to lose a race that
is already lost.

A renewal that fails for any other reason is reported and retried on the next
interval. The claim is not lost until the timeout passes with nothing landing,
which is several intervals away, so abandoning the work over one failed write
would throw away a job for a database hiccup.

**The renewing stops before the ending is recorded**, and the worker waits for
it to stop. Otherwise something is still writing to the job while the worker
writes how it ended.

### Why the timeout is not shortened to match

Renewal would allow a much shorter timeout — a dead worker's job recovered in
minutes rather than half an hour. It stays generous anyway, because on SQLite
the pool is one connection, so a renewal waits behind the job's own statement
and a long transaction can hold it for minutes. A timeout that assumed
renewals were prompt would take the job away from exactly the work it exists to
protect.

## Only the holder finishes a job

A job is marked done, or handed back for retry, by the worker that holds its
claim — the finishing statement carries the same condition the claim did. A
worker that ran long enough for its claim to go stale is still running when
another takes the job over, and without the condition whichever finishes first
records the ending of what the other is in the middle of: a job marked done
while the second worker is still working it, or handed back for retry while
it is being finished.

The refused finish is reported to the worker as "no longer held" rather than as
a failure. The work it did stands; the job's ending is the other worker's to
write, and there is nothing to retry. It is logged, because a job that outran
its claim while renewing it means the renewals were not landing.

## How a job ended is recorded after a shutdown

A shutdown cancels the work, and the record of how the job ended is written
after the work — with the same cancelled context, if nothing intervenes. Both
writes then fail, and the job stays claimed by a process that has gone until
the claim goes stale, half an hour later, while whatever the job had begun
stays open for ever.

So the writes that record an ending run under a context of their own: detached
from the cancellation, and bounded by a few seconds so a database that is not
answering cannot hold a shutdown either. A shutdown mid-job hands the job back
as a failed attempt, which is honest — the attempt did not complete — and a
failure to hand it back is logged rather than silently absorbed.

## Which replica does work that has no work item

Every replica runs the same binary with no leader and no process-local state
that decides anything, so a pass that runs on a timer runs on all of them.

For discrete work that is already answered: there is a row, and claiming it
settles who does it. **A recurring pass has no work item**, so what it takes is
the name of the work — a lease, held by the same conditional update a claim
uses and portable for the same reason.

A lease lapses rather than only being handed back. A replica that died holding
one would otherwise stop the work happening at all, which is the failure the
whole arrangement exists to avoid. The holder keeps it by asking for it again
each cycle, so there is one thing to get right rather than two, and it is handed
back on a clean shutdown so a handover does not wait out a lease.

**A lease has to outlast a cycle of the work, not an instant of it.** It is not
renewed while the work runs, unlike a job's claim, because the work here is a
pass with no natural point to renew from.

### Two shapes of work, two answers to losing the race

**Work that may be skipped** does nothing this cycle and asks again next time.
Two passes are this: asking the public indexes what upstream has released, and
deciding which builds are due a scan. Whoever holds the lease is doing it, and
neither is urgent to the minute.

**Work that has to happen** waits for its turn. Rewriting deadlines after
somebody changed the policy is this: the change has to be applied, so a replica
that loses the race applies it afterwards.

**What the waiting one reads, it reads after its turn comes.** Reading the
policy first and waiting second would leave two replicas rewriting the same
rows from whatever each read when it started, with the later finisher winning —
so what is stored could describe a policy that had already been superseded, and
nothing would say so. It is the same rule a retryable transaction holds to:
anything the work decides from is fetched inside the thing that serializes it.

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

## A worker takes only its own kind of work

Reading an inventory and scanning one are different jobs sharing one queue. A
worker claiming whatever was oldest would take work meant for another, and that
does not fail: a job's reference means something different to each of them, so
the wrong worker acts on it, gets an answer, and marks it done. The work it was
left for never happens, and nothing says so.

So the kind is part of claiming rather than something a worker checks
afterwards. It was found by running the whole thing end to end — every test
until then had one worker.
