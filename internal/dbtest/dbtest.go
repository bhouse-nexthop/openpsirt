// Package dbtest runs a test against every database available to it.
//
// SQLite always runs, so the suite is useful with nothing installed. The
// production engines run when the environment points at them, and are skipped
// loudly otherwise — a skipped engine is reported rather than silently absent,
// because a portability suite that quietly tests one engine is worse than none.
package dbtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// URL environment variables for the production engines. Absent means skip.
const (
	PostgresURLEnv = "OPENPSIRT_TEST_POSTGRES_URL"
	MySQLURLEnv    = "OPENPSIRT_TEST_MYSQL_URL"
	MariaDBURLEnv  = "OPENPSIRT_TEST_MARIADB_URL"
)

type candidate struct {
	name   string
	env    string
	urlFor func(t *testing.T) string
}

func candidates() []candidate {
	return []candidate{
		{name: "sqlite", urlFor: func(t *testing.T) string {
			// A file rather than :memory: — every pooled connection to an
			// in-memory database gets its own empty database, which makes
			// migrations appear to vanish between statements.
			return "sqlite://" + filepath.Join(t.TempDir(), "test.db")
		}},
		{name: "postgres", env: PostgresURLEnv},
		{name: "mysql", env: MySQLURLEnv},
		{name: "mariadb", env: MariaDBURLEnv},
	}
}

// Each runs fn once against every database available, as a subtest.
//
// Each subtest gets its own connection, and it is closed afterwards.
func Each(t *testing.T, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	ran := 0
	for _, c := range candidates() {
		t.Run(c.name, func(t *testing.T) {
			url := ""
			switch {
			case c.urlFor != nil:
				url = c.urlFor(t)
			default:
				url = os.Getenv(c.env)
				if url == "" {
					t.Skipf("%s is not set, so %s is untested here", c.env, c.name)
				}
			}
			db := Open(t, url)
			ran++
			fn(t, db)
		})
	}
	if ran == 0 {
		t.Fatal("no database was available, so nothing was tested")
	}
}

// Open connects to url and closes the connection when the test ends.
func Open(t *testing.T, url string) *database.DB {
	t.Helper()
	target, err := database.ParseURL(url)
	if err != nil {
		t.Fatalf("parse %q: %v", url, err)
	}
	db, err := database.Open(context.Background(), target)
	if err != nil {
		t.Fatalf("open %s: %v", target.Redacted, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close %s: %v", target.Engine, err)
		}
	})
	return db
}

// tables lists every table, in an order safe to delete from: children before
// the rows they reference.
//
// **Add new tables at the top.** A table missing from this list leaves rows
// behind between tests, and one in the wrong position fails on the engines
// that enforce foreign keys during a bulk delete — which is not all of them,
// so it will look engine-specific rather than like the ordering mistake it is.
var tables = []string{
	"decision_approval",
	"decision_revision",
	"decision",
	"personal_token",
	"person_identity",
	"group_admin",
	"group_role",
	"session",
	"api_key",
	"role_grant",
	"person",
	"finding",
	"suppression",
	"scan_run",
	"vulnerability_alias",
	"vulnerability",
	"scan_document_chunk",
	"scan_document",
	"graph_edge",
	"graph_node",
	"component",
	"job",
	"scan",
	"target",
	"variant",
	"stream",
	"product",
	"application_setting",
}

// Reset empties every table, leaving the schema in place.
//
// It lives here rather than in each test package so that adding a table is one
// change instead of one per package. Before this, a new table with a foreign
// key silently broke the cleanup of every package that predated it.
func Reset(t *testing.T, db *database.DB) {
	t.Helper()
	ctx := context.Background()

	// A tag points at the branch it was cut from. MySQL and MariaDB enforce
	// that self-reference during a bulk delete even though every row is going;
	// PostgreSQL and SQLite happen not to.
	if _, err := db.ExecContext(ctx, "UPDATE stream SET parent_id = NULL"); err != nil {
		t.Fatalf("detach stream parents: %v", err)
	}
	for _, table := range tables {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}
}
