package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// lockName identifies the migration lock. The numeric form is for engines that
// take an integer; the text form is for those that take a name.
const (
	lockName = "openpsirt_migrate"
	lockID   = 8_147_263_001
)

// lockWaitSeconds bounds how long we wait for another instance to finish. A
// variable rather than a constant only so tests can shorten it; nothing in the
// application changes it.
// Long enough for a real migration, short enough that a stuck one is noticed
// rather than blocking every replacement pod indefinitely.
var lockWaitSeconds = 300

// unlock releases a migration lock and returns the pinned connection.
type unlock func(context.Context) error

// acquire takes the migration lock, so that instances starting at once do not
// migrate at the same time.
//
// This is one of the few places the portable-SQL rule does not hold: every
// engine spells advisory locking differently, and placeholders here are the
// driver's native form rather than the query builder's, because the lock must
// be taken on a pinned connection rather than on the pool.
//
// **The connection is pinned deliberately.** These are session locks. Taking
// one on the pool and releasing it on the pool means the release can land on a
// different connection — and neither engine reports that as an error, it just
// silently fails to release. The lock would then be held for the life of the
// process and every other instance would block on it.
func acquire(ctx context.Context, db *database.DB) (unlock, error) {
	if db.Server.Engine == database.SQLite {
		// SQLite has no advisory lock and needs none: it is only ever used by
		// a single process, so there is no other process to exclude.
		// Concurrency within this process is handled by the migration mutex.
		return func(context.Context) error { return nil }, nil
	}

	conn, err := db.DB.DB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("pin a connection for the migration lock: %w", err)
	}
	closeConn := func() {
		if err := conn.Close(); err != nil {
			_ = err // returning the connection to the pool; nothing to do
		}
	}

	switch db.Server.Engine {
	case database.Postgres:
		// Bound the wait. Without this, pg_advisory_lock waits forever: an
		// instance wedged mid-migration blocks every replacement silently,
		// and the startup probe kills each one in turn.
		if _, err := conn.ExecContext(ctx,
			fmt.Sprintf("SET lock_timeout = '%ds'", lockWaitSeconds)); err != nil {
			closeConn()
			return nil, fmt.Errorf("bound the migration lock wait: %w", err)
		}
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
			closeConn()
			return nil, fmt.Errorf("take migration lock: %w", err)
		}
		return func(ctx context.Context) error {
			defer closeConn()
			// pg_advisory_unlock returns false when this session did not hold
			// the lock. Executing it without reading the result would report
			// success while leaking the lock.
			var released bool
			if err := conn.QueryRowContext(ctx,
				"SELECT pg_advisory_unlock($1)", lockID).Scan(&released); err != nil {
				return fmt.Errorf("release migration lock: %w", err)
			}
			if !released {
				return fmt.Errorf("migration lock was not held by this session when released")
			}
			return nil
		}, nil

	case database.MySQL, database.MariaDB:
		// GET_LOCK returns 1 when granted, 0 on timeout, NULL on error. A
		// timeout means another instance is migrating, which is not our
		// failure but does mean we must not proceed.
		var granted *int
		row := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, lockWaitSeconds)
		if err := row.Scan(&granted); err != nil {
			closeConn()
			return nil, fmt.Errorf("take migration lock: %w", err)
		}
		if granted == nil || *granted != 1 {
			closeConn()
			return nil, fmt.Errorf("another instance holds the migration lock")
		}
		return func(ctx context.Context) error {
			defer closeConn()
			// RELEASE_LOCK returns 0 when held by another session and NULL
			// when no such lock exists. Neither is an error to the driver.
			var released *int
			if err := conn.QueryRowContext(ctx,
				"SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
				return fmt.Errorf("release migration lock: %w", err)
			}
			if released == nil || *released != 1 {
				return fmt.Errorf("migration lock was not held by this session when released")
			}
			return nil
		}, nil
	}

	closeConn()
	return nil, fmt.Errorf("no migration lock for %s", db.Server.Engine)
}

// versionTableExists reports whether the migration bookkeeping table is there.
//
// Asking before reading the version keeps "migrate status" from creating it.
// The library's version query creates the table when it is missing, which
// makes a read-only inspection command perform schema changes — and DAT-11
// says the running application may hold read and write rights only.
func versionTableExists(ctx context.Context, db *database.DB) bool {
	var probe int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM goose_db_version LIMIT 1").Scan(&probe)
	return err == nil || err == sql.ErrNoRows
}
