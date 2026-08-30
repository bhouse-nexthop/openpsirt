package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

// Attempts is how many times a transaction is tried before its failure is
// reported.
//
// Contention that survives this many tries is not contention: it is a design
// that has two writers permanently fighting over the same rows, and retrying
// forever would turn that into a hang rather than a report.
const Attempts = 5

// InTransaction runs fn inside a transaction and retries the whole of it if
// the database refuses the write for a reason that going again can fix.
//
// **Everything fn depends on must be read inside fn.** A retry re-runs the
// closure from the beginning against a database that has moved, so a value
// read before the transaction started, or carried over from a previous
// attempt, describes a world that no longer exists — and writing a decision
// made from it is worse than the conflict that forced the retry, because
// nothing reports it. This is the rule a reviewer should check first.
//
// Retrying matters more than it looks. A clustered deployment certifies a
// write when it commits, not when the statement runs, so two nodes that
// touched the same rows find out at COMMIT and the loser is told its whole
// transaction was rolled back. Code that guards each statement and trusts the
// commit sees none of this: the statements all succeeded.
func InTransaction(ctx context.Context, db *bun.DB, fn func(context.Context, bun.Tx) error) error {
	var err error
	for attempt := 1; attempt <= Attempts; attempt++ {
		err = db.RunInTx(ctx, nil, fn)
		if err == nil {
			return nil
		}
		if !WorthRetrying(err) {
			return err
		}
		if ctx.Err() != nil {
			return err
		}
		// Backing off with a little randomness, so two writers that collided
		// do not line up and collide again on the same schedule.
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff(attempt)):
		}
	}
	return fmt.Errorf("gave up after %d attempts: %w", Attempts, err)
}

// WorthRetrying reports whether a failure is one that going again can fix.
//
// This is the one place besides the migrations and the queue's locking that
// knows which engine it is talking to, and it has to be: what each of them
// calls "you lost a race, try again" is a different code, and treating an
// unrecognized failure as retryable would hammer a database over a constraint
// violation that will never come out differently.
//
// So the list is of what is known to be worth retrying, and everything else is
// reported. A conflict wrongly treated as permanent surfaces as an error
// somebody sees; a permanent error wrongly retried surfaces as a deployment
// that hangs under load.
func WorthRetrying(err error) bool {
	if err == nil {
		return false
	}

	// MySQL and MariaDB. A cluster reports a certification failure at commit
	// as a deadlock, which is why this matters on a write that appeared to
	// have succeeded statement by statement.
	var my *mysql.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1213, // deadlock found, and what a cluster says when it could not certify
			1205, // lock wait timeout
			1180: // an error during commit, which is how some cluster failures arrive
			return true
		}
		return false
	}

	// PostgreSQL.
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "40001", // serialization failure
			"40P01": // deadlock detected
			return true
		}
		return false
	}

	// SQLite has no error type worth matching on through this driver, and a
	// single-file database contends differently anyway: a writer that arrives
	// while another holds the file is told the database is busy or locked.
	text := strings.ToLower(err.Error())
	for _, known := range []string{"database is locked", "database table is locked", "sqlite_busy"} {
		if strings.Contains(text, known) {
			return true
		}
	}
	return false
}

// backoff is how long to wait before trying again.
//
// Growing with each attempt, and jittered, so that two writers which collided
// do not line up and collide again on the same schedule. The randomness is for
// spreading load and decides nothing, which is why an ordinary generator is
// right here — a cryptographic one would be slower and no better at it.
func backoff(attempt int) time.Duration {
	step := time.Duration(attempt) * 10 * time.Millisecond
	//nolint:gosec // G404: this spreads retries apart; nothing is kept secret by it.
	return step + time.Duration(rand.N(int64(step)+1))
}

// IsNoRows reports whether a query found nothing.
//
// One place, because there were three, and all three matched on the words "no
// rows" appearing somewhere in the message. That is wrong in both directions:
// an unrelated failure whose message happens to contain the phrase reads as an
// empty result, and a driver wording it differently reads as a failure. Both
// mistakes are silent, and "not found" is a control-flow answer in enough
// places here that getting it wrong changes behavior rather than logging.
//
// The comparison is against the standard sentinel, through the wrapping, which
// is what every driver here actually returns.
func IsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
