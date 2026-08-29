# Database

How OpenPSIRT talks to a database, and why the awkward parts are awkward.

Satisfies SCP-15, DAT-01 to DAT-17, DAT-30 to DAT-32, and the portability
constraints in Section 6 of `DECISIONS.md`.

## Four engines, one set of queries

| Engine | Role | Floor |
|---|---|---|
| PostgreSQL | Production | 14 |
| MySQL | Production | 8.0 |
| MariaDB | Production | 10.6 |
| SQLite | Development and testing only | 3.35 |

MySQL and MariaDB are **separate targets**, not one. They share a wire protocol
and a driver, and have diverged in JSON handling, sequences and partitioning —
the last of which is what data purging is built on, so an untested difference
there deletes the wrong rows rather than raising an error.

Queries above this package are written once and run unchanged everywhere.
Engine-specific code is confined to three places, and nowhere else:

| Where | Why |
|---|---|
| Schema migrations | Data-definition language genuinely differs |
| The migration lock | Every engine spells advisory locking differently |
| Connection setup | Driver-specific settings |

## Which engine is answering

The URL says which engine to expect. **The server is asked anyway**, because a
MySQL-protocol connection could be either MySQL or MariaDB and the URL cannot
settle it. Believing the URL would apply the wrong version floor and let an
unsupported server through unnoticed.

Detection is the version string: MariaDB says so in its own.

## Refusing an unsupported server

Too old, and the process will not start. The alternative is a confusing failure
much later, in whichever query first needs something the server cannot do.

The message names the version found and the version required, because an
operator seeing an unexplained startup failure has nothing to act on.

## Migrations

Embedded in the binary. Applied at startup by default, so a deployment is one
artifact and an upgrade is deploying it. Automatic application can be turned
off, and `openpsirt migrate up|down|status` runs them separately — for an
operator who would rather use different credentials, at a time they choose,
having first seen what will change.

Migrations are written in Go rather than SQL files, because they branch on the
engine. The first migration is the clearest example: a timestamp column has no
portable spelling. PostgreSQL has no `DATETIME`; MySQL's `TIMESTAMP` is a 32-bit
value that can acquire an implicit default and an on-update clause depending on
server configuration. Each engine gets the column it should have.

### Two locks, for two different problems

| Lock | Excludes | How |
|---|---|---|
| Migration mutex | Other goroutines in this process | An ordinary mutex |
| Advisory lock | Other instances | `pg_advisory_lock`, `GET_LOCK`, or nothing on SQLite |

**The advisory lock is taken on a pinned connection**, not on the pool. These
are session locks: released from the pool, the release can land on a different
connection, and neither engine reports that as an error — it simply fails to
release, and the lock is then held for the life of the process. The release
result is read rather than assumed, because both engines report "you did not
hold this" as a value rather than an error.

The wait is bounded on both engines. An unbounded wait means an instance wedged
mid-migration blocks every replacement silently, and the startup probe kills
each one in turn.

Both are needed and neither substitutes for the other. The in-process mutex
exists because the migration library keeps its dialect in package-level state,
so two goroutines migrating at once race on it regardless of any database lock.
The advisory lock exists because a rolling deployment starts several instances
at once and they would otherwise migrate simultaneously.

SQLite needs no advisory lock — it is only ever used by one process — but it did
need the other two settings below.

## Any number of replicas, coordinated only through the database

Replicas are identical and there is no leader. Every one serves requests, reads
scans and runs vulnerability scans, and adding one is adding one. Nothing is
held in a process that decides anything: sessions are rows, settings and roles
are read per request, and the only in-memory lock stops a single process
migrating twice — the database lock is what stops two processes doing it.

Four things need coordinating, and each is coordinated where every replica can
see it:

| | How |
|---|---|
| Two workers taking one job | The claim selects a row and locks it, skipping locked rows, inside a transaction |
| Two replicas migrating at startup | A database-level lock, with a bounded wait, so one migrates and the rest wait and then serve |
| Two scans of one build | The apply takes the build's own row first, so the second waits rather than interleaving |
| An administrator changing a setting | Read per request, so a change takes effect on every replica at once rather than after a restart |

SQLite cannot take part — it is a single file — so a scaled deployment runs on
one of the three servers.

## A cluster refuses a write when it commits, not when it runs

This is the difference between a design that works on one server and one that
works on several, and it is easy to miss because nothing about the failing code
looks wrong.

A clustered deployment certifies a write across nodes **at `COMMIT`**. Two
nodes that touched the same rows both run every statement successfully, and one
of them is told at the end that the whole transaction was rolled back. Code
written for a single server checks each statement, sees them all succeed, and
never learns that none of them happened.

So **every transaction is retryable as a whole**, through one helper, on the
failures that mean a race was lost — deadlock, lock-wait timeout, serialization
failure. Anything else is reported rather than retried: an unrecognized failure
treated as retryable turns a constraint violation into a deployment that
hammers its database and hangs, where reporting it turns a lost race into an
error somebody can read.

The rule that makes retrying safe is the one worth checking in review:
**nothing a transaction depends on may be read outside it.** A retry re-runs
the closure against a database that has moved, so a value read before the
transaction began — or carried over from the attempt that just failed —
describes a world that no longer exists. A write decided from it is worse than
the conflict that forced the retry, because the conflict was reported and this
is not. Anything a closure uses but does not fetch is a defect, however
reasonable it looks.

Recognizing which failures are worth retrying is engine-specific, and is the
third place allowed to be, beside the migration data-definition and the queue's
locking. What each engine calls "you lost a race" is a different code in a
different error type, and there is no portable spelling of the question.

## Connections

The failure worth designing against is not a connection that closes — it is one
where **the far end goes without a FIN or an RST**. A firewall drops an idle
flow, a load balancer times out, a database fails over. Our side still believes
the socket is fine, so the pool hands it out, the query writes into nothing, and
the read blocks until TCP retransmission gives up. That is roughly fifteen
minutes on common defaults, with nothing logged and a goroutine simply stuck.

Go's pool cannot detect this. It never validates a connection before handing it
out, and the two driver hooks it does call before reuse only inspect local state
in our drivers — neither performs a round trip, so neither notices a half-open
socket.

So the defense is to ensure a connection is never idle long enough to be killed
behind our back.

| Setting | Default | Why |
|---|---|---|
| Idle timeout | 1 minute | **The one that matters.** Must be shorter than the shortest idle timeout in the path — firewall, load balancer, or the server's own |
| Lifetime | 30 minutes | Recycles regardless of use, so connections redistribute after a failover instead of holding on to a machine that is no longer primary |
| Max open | 25 | Unbounded, a burst opens more than the server permits, and PostgreSQL is expensive per connection |
| Max idle | 25 | Go's own default is 2, low enough that moderate load churns connections open and closed continuously |

**Go's cleaner never runs more than once a second**, however short the idle
timeout is set. A value below a second reaps no faster — a setting that looks
aggressive and does nothing.

### What is deliberately not done

**Queries are not bounded.** No statement timeout, no blanket driver read
timeout. Any such bound eventually kills legitimate slow work — a large report,
an ingest transaction over tens of thousands of components — and the usual
result is per-query exceptions until the bound means nothing. It would also cut
off a migration part way through.

**Connections are not validated on checkout**, because there is no hook for it.
A validation helper exists with a short deadline, for callers that would
otherwise block a long time on a dead connection. Readiness uses it. It is not
a default: it costs a round trip per use, and it can only say a connection was
alive a moment ago.

## SQLite is not simply "the easy one"

Two things had to be set explicitly, both found by testing rather than assumed:

- **One connection.** SQLite has a single writer. Letting the pool open more
  adds contention rather than concurrency: transactions on different
  connections collide instead of queueing.
- **A busy timeout.** Without one, a concurrent access fails immediately with
  "database is locked" rather than waiting its turn. That reads as a bug and is
  really impatience.

## Testing

The portability harness runs a test against every database available to it. SQLite
always runs, so the suite is useful with nothing installed; the production
engines run when the environment points at them and are **skipped loudly**
otherwise.

Test packages run **one at a time**. They share the database servers, and the
rollback test drops every table — in parallel, that makes unrelated packages
fail depending on timing. Giving each package its own database would also work;
serializing is one flag and the suite runs in seconds.

CI provides all four, and then checks that all four actually ran. A skipped
engine passes silently, which would make the matrix decorative.

What the suite pins:

- Every engine is identified, with its version parsed
- MariaDB is distinguished from MySQL by asking the server
- A server below the floor is refused, proved against a real old server rather
  than against arithmetic
- Migrations apply, are idempotent, and roll back on every engine
- The advisory lock excludes a second connection while held, and admits it once
  released — driven directly, from two pools

Note what the concurrency test in the schema package does **not** cover. Every
caller serializes on the in-process mutex before reaching the advisory lock, so
a test driving goroutines through the normal path passes with the entire
advisory lock deleted. It pins the mutex and nothing else, and says so.

Reading the schema version performs no schema changes: the bookkeeping table is
checked for before it is read, so the inspection command does not create it.

## Collation is pinned, not inherited

Two of the four engines compare text case-insensitively by default; the other
two do not. Left alone, declaring `Widget` and then filing a scan against
`widget` resolves on one pair and creates a second product on the other — so
the declaration rule that exists to catch a typo would itself behave
differently depending on which database an operator runs.

The tables that need it are created with binary comparison, which is what the
other two already do.

## Text a producer supplies is not given a width

A component's name, its version, what it was forked from, an issue's
identifier, a fix's version list: all of it arrives in somebody else's file,
and nothing in any format bounds it. A column with a width turns a merely
unusual value into a failure of the whole scan that carried it — and a scan
that failed is indistinguishable from a product that stopped having problems.

The exception is text carrying a unique index, which needs a width. There the
value is truncated on the way in rather than refused, because two identifiers
agreeing for a hundred and ninety-one characters are the same identifier by any
reading.
