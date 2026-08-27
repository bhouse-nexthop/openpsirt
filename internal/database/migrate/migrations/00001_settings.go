package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/database/migrate"
)

func init() {
	goose.AddMigrationContext(upSettings, downSettings)
}

// Settings an administrator can change at runtime — thresholds, precedence
// orders, modes. Configuration files hold what an operator sets at deploy time;
// this holds what an administrator sets from inside the application.
//
// The data-definition language is written per engine because it genuinely
// differs. Timestamps are the clearest case: PostgreSQL has no DATETIME, and
// MySQL's TIMESTAMP is a 32-bit value that can acquire an implicit default and
// an on-update clause depending on how the server is configured. Neither is a
// portable spelling of "a moment in time we set ourselves".
func upSettings(ctx context.Context, tx *sql.Tx) error {
	var stmt string
	switch e := migrate.EngineFrom(ctx); e {
	case database.Postgres:
		stmt = `CREATE TABLE application_setting (
			name       VARCHAR(191) NOT NULL PRIMARY KEY,
			value      TEXT         NOT NULL,
			updated_at TIMESTAMPTZ  NOT NULL
		)`
	case database.MySQL, database.MariaDB:
		// 191 keeps the key inside the index limit on older servers using a
		// four-byte character set.
		stmt = `CREATE TABLE application_setting (
			name       VARCHAR(191) NOT NULL PRIMARY KEY,
			value      TEXT         NOT NULL,
			updated_at DATETIME(6)  NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`
	case database.SQLite:
		stmt = `CREATE TABLE application_setting (
			name       TEXT NOT NULL PRIMARY KEY,
			value      TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`
	default:
		return fmt.Errorf("no schema for %s", e)
	}
	_, err := tx.ExecContext(ctx, stmt)
	return err
}

func downSettings(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE application_setting`)
	return err
}
