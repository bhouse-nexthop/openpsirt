package schema_test

import (
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// probeCounter keeps rows unique between runs.
var probeCounter atomic.Int64

// uniqueName returns a row name no other run will collide with.
//
// Tests are run repeatedly against the same server — "go test -count=2", or a
// developer with a local database — and a fixed name made the second run fail
// on a duplicate key, with three engine failures unrelated to any change. CI
// only survived it because its containers are fresh.
func uniqueName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("probe-%s-%d-%d", t.Name(), time.Now().UnixNano(), probeCounter.Add(1))
}

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
		// The table must be usable with the same portable Go on every engine,
		// which means writing and reading a real time.Time.
		//
		// An earlier version of this test inserted the timestamp as a string
		// literal and read back only the value column — so the one column the
		// migration branches per engine for was never read, and SQLite's
		// column type was wrong for months of nobody noticing.
		name := uniqueName(t)
		want := time.Now().UTC().Truncate(time.Second)
		if _, err := db.ExecContext(ctx,
			"INSERT INTO application_setting (name, value, updated_at) VALUES (?, ?, ?)",
			name, "value", want); err != nil {
			t.Fatalf("insert into the migrated table: %v", err)
		}
		var value string
		var updated time.Time
		if err := db.QueryRowContext(ctx,
			"SELECT value, updated_at FROM application_setting WHERE name = ?", name).
			Scan(&value, &updated); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if value != "value" {
			t.Errorf("value read back as %q", value)
		}
		if !updated.UTC().Equal(want) {
			t.Errorf("timestamp read back as %v, wrote %v", updated.UTC(), want)
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
