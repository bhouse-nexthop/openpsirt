# Database

How OpenPSIRT talks to a database, and why the awkward parts are awkward.

Satisfies DAT-01 to DAT-17, and the portability constraints in Section 6 of
`DECISIONS.md`.

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
