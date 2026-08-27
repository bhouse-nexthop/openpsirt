package migrate

import (
	"context"
	"fmt"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// lockName identifies the migration lock. The numeric form is for engines that
// take an integer; the text form is for those that take a name.
const (
	lockName = "openpsirt_migrate"
	lockID   = 8_147_263_001
)

// unlock releases a migration lock.
type unlock func(context.Context) error

// acquire takes the migration lock, so that several instances starting at once
// do not migrate at the same time.
//
// This is one of the few places the portable-SQL rule does not hold: every
// engine spells advisory locking differently, and SQLite has no equivalent
// because it does not need one.
func acquire(ctx context.Context, db *database.DB) (unlock, error) {
	switch db.Server.Engine {
	case database.Postgres:
		if _, err := db.ExecContext(ctx, "SELECT pg_advisory_lock(?)", lockID); err != nil {
			return nil, fmt.Errorf("take migration lock: %w", err)
		}
		return func(ctx context.Context) error {
			_, err := db.ExecContext(ctx, "SELECT pg_advisory_unlock(?)", lockID)
			return err
		}, nil

	case database.MySQL, database.MariaDB:
		// GET_LOCK returns 1 when granted, 0 on timeout, NULL on error. A
		// timeout means another instance is migrating, which is not a failure
		// of ours but does mean we must not proceed.
		var granted *int
		row := db.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, lockTimeoutSeconds)
		if err := row.Scan(&granted); err != nil {
			return nil, fmt.Errorf("take migration lock: %w", err)
		}
		if granted == nil || *granted != 1 {
			return nil, fmt.Errorf("another instance holds the migration lock")
		}
		return func(ctx context.Context) error {
			_, err := db.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", lockName)
			return err
		}, nil

	case database.SQLite:
		// SQLite has no advisory lock, and needs none: it is only ever used by
		// a single process here, so there is no other process to exclude.
		// Concurrency within this process is handled by the migration mutex,
		// and the single connection and busy timeout set at open time do the
		// rest.
		return func(context.Context) error { return nil }, nil
	}
	return nil, fmt.Errorf("no migration lock for %s", db.Server.Engine)
}

// lockTimeoutSeconds bounds how long we wait for another instance to finish.
// Long enough for a real migration, short enough that a stuck one is noticed.
const lockTimeoutSeconds = 300
