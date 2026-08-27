# Database

How openpsirt talks to a database, and why the awkward parts are awkward.

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

Both are needed and neither substitutes for the other. The in-process mutex
exists because the migration library keeps its dialect in package-level state,
so two goroutines migrating at once race on it regardless of any database lock.
The advisory lock exists because a rolling deployment starts several instances
at once and they would otherwise migrate simultaneously.

SQLite needs no advisory lock — it is only ever used by one process — but it did
need the other two settings below.

## SQLite is not simply "the easy one"

Two things had to be set explicitly, both found by testing rather than assumed:

- **One connection.** SQLite has a single writer. Letting the pool open more
  adds contention rather than concurrency: transactions on different
  connections collide instead of queueing.
- **A busy timeout.** Without one, a concurrent access fails immediately with
  "database is locked" rather than waiting its turn. That reads as a bug and is
  really impatience.

## Testing

`internal/dbtest` runs a test against every database available to it. SQLite
always runs, so the suite is useful with nothing installed; the production
engines run when the environment points at them and are **skipped loudly**
otherwise.

CI provides all four, and then checks that all four actually ran. A skipped
engine passes silently, which would make the matrix decorative.

What the suite pins:

- Every engine is identified, with its version parsed
- MariaDB is distinguished from MySQL by asking the server
- A server below the floor is refused, proved against a real old server rather
  than against arithmetic
- Migrations apply, are idempotent, and roll back on every engine
- Several instances migrating at once all succeed
