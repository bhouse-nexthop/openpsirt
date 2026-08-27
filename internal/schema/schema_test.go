package schema_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMigrationsApplyOnEveryEngine(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		if err := schema.Up(ctx, db, quiet()); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		version, err := schema.Version(ctx, db)
		if err != nil {
			t.Fatalf("read version: %v", err)
		}
		if version == 0 {
			t.Fatal("nothing was applied; the migration set is empty")
		}
		// The table the migration creates must actually be usable, not merely
		// recorded as created.
		if _, err := db.ExecContext(ctx,
			"INSERT INTO application_setting (name, value, updated_at) VALUES (?, ?, ?)",
			"probe", "value", "2026-01-01 00:00:00"); err != nil {
			t.Fatalf("insert into the migrated table: %v", err)
		}
		var value string
		if err := db.QueryRowContext(ctx,
			"SELECT value FROM application_setting WHERE name = ?", "probe").Scan(&value); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if value != "value" {
			t.Errorf("read back %q", value)
		}
		t.Logf("%s: schema version %d", db.Server.Engine, version)
	})
}

func TestMigrationsAreIdempotent(t *testing.T) {
	// Every instance runs migrations at startup. A second run must be a no-op
	// rather than an error, or a rolling restart fails on the second pod.
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		if err := schema.Up(ctx, db, quiet()); err != nil {
			t.Fatalf("first run: %v", err)
		}
		first, _ := schema.Version(ctx, db)
		if err := schema.Up(ctx, db, quiet()); err != nil {
			t.Fatalf("second run: %v", err)
		}
		second, _ := schema.Version(ctx, db)
		if first != second {
			t.Errorf("version moved on a second run: %d then %d", first, second)
		}
	})
}

func TestMigrationsRollBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		if err := schema.Up(ctx, db, quiet()); err != nil {
			t.Fatalf("up: %v", err)
		}
		if err := schema.Down(ctx, db, quiet()); err != nil {
			t.Fatalf("down: %v", err)
		}
		// The table must be gone, not merely unrecorded.
		if _, err := db.ExecContext(ctx, "SELECT 1 FROM application_setting"); err == nil {
			t.Error("the table survived a rollback")
		}
	})
}
