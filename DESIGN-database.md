# Database

How OpenPSIRT talks to a database, and why the awkward parts are awkward.

Satisfies SCP-15, DAT-01 to DAT-24, DAT-30 to DAT-36, and the portability
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
| Advisory lock | Other instances on the same database | `pg_advisory_lock`, which belongs to the database; `GET_LOCK` on a name that carries the database, since a MySQL named lock belongs to the server; nothing on SQLite |

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

## Identifiers are quoted, everywhere

A reserved word is only reserved when it is bare. Leaving identifiers unquoted
makes the set of usable column names the intersection of four engines' keyword
lists — a set nobody knows, that grows with every release, and whose violations
show up only on whichever engine somebody is least likely to be developing
against.

That is not hypothetical. A column named `rank` was accepted by three engines
and refused outright by the fourth, where it had become a window function.

So every identifier in the data definition is quoted. Two of the engines quote
with backticks by default, so their connections are asked for standard quoting.
Backticks keep working, so anything generating them is unaffected, and string
literals are untouched — this changes what a double quote means, not what a
quote means.

**It is appended to the mode already in force, never assigned**, and that
distinction is the whole of it. Setting the mode replaces it, and what it
replaces includes the strictness that makes an oversized value an error rather
than a quiet truncation. The first version assigned, and a nine-character
string stored in a four-character column came back four characters long, with
no error, on these two engines and on neither of the others.

**A name a query invents needs the same care.** The rule was written about the
data definition and quietly read as being only about it, so a grouped count
wrapped its subquery in `AS groups` — and `GROUPS` is a reserved word in MySQL
8, where it names a window frame type. Three engines parsed it and one returned
a syntax error, which the handler above it turned into a 500 with the driver's
message discarded. The alias is now quoted and named around the keyword, and a
test reaches both of the counts that use this shape on all four engines.

Silent truncation is the worst shape a portability difference can take:
nothing fails and the data is wrong. Two tests hold it — one that a value is
never accepted and silently changed, one that reserved words still work as
identifiers — and both were checked by reverting the fix and watching them
fail. One engine does not constrain a text column's width at all, which is
documented behavior and loses nothing, so what is asserted is that the data
never changes rather than that the write is refused.

### An affected-row count means rows matched

The same connection asks for one more thing, for the same reason: so a count
means the same thing everywhere.

A conditional write — take this decision to approved, but only if the reasoning
is still the revision that was read — has no way to report a lost race except
the number of rows the update touched. Zero means somebody got there first.

By default two of the engines report how many rows the update *changed*, where
the other two report how many it *matched*. Under that reading, a write whose
condition held but whose values already happened to be correct reports zero,
and the caller announces a conflict that never happened. It surfaced exactly
that way: an approval refused with "the reasoning changed while this was being
agreed to", for a decision nobody had touched, on those two engines and neither
of the others.

The alternative was to write every conditional update so its values are
guaranteed to differ from what is already there. That leaves a portability
quirk for every future caller to remember, and the one who forgets is handed a
false conflict rather than an error. Asking the connection for matched rows
makes the count answer the question being asked, identically everywhere.

A test asserts the count on all four engines, and it was checked by removing
the setting and watching exactly the two fail.

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

And one thing set for speed, found by measuring:

- **Write-ahead log, synchronous NORMAL.** SQLite's default is a rollback
  journal synced to disk twice per commit — its slowest write path — and a
  scan applies hundreds of thousands of rows through it. In WAL mode a commit
  appends to the log, readers do not block the writer, and NORMAL syncs the
  log at a checkpoint rather than at every commit. A crash of the process
  loses nothing; a power loss can lose the last commits, never the database's
  consistency. Right for the only place SQLite runs. The pragmas are set on
  the connection string, and a URL may add its own `_pragma` entries after
  them — the test harness adds `synchronous(OFF)`, since a test database is
  thrown away.

## Testing

The portability harness runs a test against every database available to it. SQLite
always runs, so the suite is useful with nothing installed; the production
engines run when the environment points at them and are **skipped loudly**
otherwise.

The schema is built **once per test binary**, not once per test. On SQLite a
file is migrated on first use and copied for each test; on each server the
binary gets a database of its own, named for the package and the checkout it
is tested from, dropped and created on first use. Packages therefore share
nothing and run in parallel; tests within a package empty the tables between
them. Two checkouts against the same servers get different databases — the
name hashes the directory as well as the import path, because the import path
alone was the same in both, and one dropped the other's database mid-run.

Which tests run on which engines is a rule, not a per-file choice (DAT-37): a
test that pins what a query returns, hides, conflicts on or spells runs on
every engine, and a test that pins routing, authorization mapping or a
response's shape runs on SQLite and PostgreSQL, because nothing it pins varies
by engine. The line is drawn at SQL rather than at the package — a handler test
that pins a query keeps all four.

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

## No index repeats the front of another

A B-tree on `(a, b)` answers a lookup on `a` exactly as well as one on `(a)`:
the seek is the same, and the only difference is a slightly wider entry. So an
index whose columns lead another index on the same table is machinery nobody
chose — maintained on every insert and every update to those columns, earning
nothing.

Eight of them existed. Seven repeated the front of a unique constraint, which
is where they come from: a constraint declares an index without ever saying the
word, so the obvious index on a foreign key gets written beside one that
already covers it. The eighth repeated the front of a wider index on the
decision table.

**A test asks the schema rather than the source**, because what matters is what
an operator ends up with, and constraints declare indexes silently. It runs on
SQLite alone, which is the one place in this suite that is deliberate rather
than a compromise: the engines disagree about reserved words, types and
affected-row counts, and they do not disagree about which columns an index is
on. The statements are one list. Asking three more engines would cost three
more schema builds to learn the same answer, and reading index metadata is
spelled four different ways — so checking on four would mean engine-specific
code in a test, to check something no engine varies.

**One exception, and it is measured rather than argued.** The narrow index on
what is open in a build leads the wider covering one, and scanning the narrow
one reads fewer pages — over hundreds of thousands of findings that is a trade
rather than a tidy-up. The same argument does not carry to the decision table,
where there are thousands of rows rather than a finding per place.

**Dropping one never costs a foreign key its index**, on the two engines that
require one: in every case the constraint that made the wider index leads with
the same column, which is what those engines ask for.

## What is stored that no query reads

Some columns are written and never selected. That is worth stating rather than
leaving to be found, because the rule here is that code doing something no
design document describes gets re-examined — and a column is the same kind of
claim.

| | |
|---|---|
| **Which parser read a scan** | The document is deleted for a branch, so after a parser changes there is otherwise no way to tell which stored records were read by the old one and need re-uploading. Its reader is a person doing that, not a query |
| **Whether we ran the scan** | Always true today, because the deployment runs every scan. A producer sending its own findings is intended and not built, and a schema that assumed we ran it could not take that later — the same argument the finding's kind carries, and it is recorded here for the same reason |
| **How large a stored document is** | Retained documents are the storage question, and "storage at tag volume is trivial" is a claim nobody can check without this. Derivable from the chunks, and kept because deriving it means reading the blobs to answer a question about their size |
| **When a finding last moved** | A finding open for years outlives whatever record of the change was kept elsewhere. It carries its own so that "what changed about this, and when" has an answer after the events have been purged |
| **When an identity was bound to a person** | Which identifier a provider handed over, and when it was pinned to whom, is the audit trail for the one decision that cannot be undone by editing a role |

A column that fails this — written, never read, and with no answer to "who
would want it" — is a defect and goes. That is how the redundant indexes above
were found: by asking the same question of the schema and not liking the
answer.

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
