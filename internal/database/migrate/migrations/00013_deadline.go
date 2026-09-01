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
	goose.AddMigrationContext(upDeadline, downDeadline)
}

// When a finding is due, worked out once rather than on every request.
//
// A deadline is when the finding was first seen plus how long something of
// that urgency may stay open (REM-25). Derived on demand it costs a pass over
// every open finding **per urgency band**, because each band allows a
// different number of days and the window has to be applied before the rows
// are read rather than after — measured at about eight seconds over 441,108
// findings, on the screen whose entire purpose is noticing something before it
// runs out.
//
// Stored, the same question is an indexed range scan. The column is nullable
// because a finding recorded before this existed has no answer until the next
// scan reopens it, and a null deadline is honestly "not known" rather than
// "not due".
//
// The index leads with the two columns that are always compared — a finding
// that is closed or already answered is not running out of anything — so the
// deadline itself is the range at the end of a narrow prefix.
func upDeadline(ctx context.Context, tx *sql.Tx) error {
	e := migrate.EngineFrom(ctx)
	t := typesFor(e)
	if t == nil {
		return fmt.Errorf("no schema for %s", e)
	}

	statements := []string{
		`ALTER TABLE "finding" ADD COLUMN "due_at" ` + t.timestamp + ` NULL`,
		`CREATE INDEX "finding_due_idx" ON "finding" ("closed_run_id", "suppressed_by", "due_at")`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}

func downDeadline(ctx context.Context, tx *sql.Tx) error {
	drop := `DROP INDEX "finding_due_idx"`
	switch migrate.EngineFrom(ctx) {
	case database.MySQL, database.MariaDB:
		drop += ` ON "finding"`
	}
	for _, stmt := range []string{drop, `ALTER TABLE "finding" DROP COLUMN "due_at"`} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", firstLine(stmt), err)
		}
	}
	return nil
}
