// Package dbtest runs a test against every database available to it.
//
// SQLite always runs, so the suite is useful with nothing installed. The
// production engines run when the environment points at them, and are skipped
// loudly otherwise — a skipped engine is reported rather than silently absent,
// because a portability suite that quietly tests one engine is worse than none.
//
// The schema is built once per test binary, not once per test. A test binary
// is one package, so on SQLite one file is migrated on first use and copied
// for each test — a copy is milliseconds where eighteen migrations were about
// a second — and on the three servers each binary gets a database of its own,
// named for the package, dropped and created on first use and migrated once.
// Packages therefore share nothing and can run in parallel; tests within a
// package share the database and empty it between them with Reset, as before.
package dbtest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/uptrace/bun"
)

// URL environment variables for the production engines. Absent means skip.
const (
	PostgresURLEnv = "OPENPSIRT_TEST_POSTGRES_URL"
	MySQLURLEnv    = "OPENPSIRT_TEST_MYSQL_URL"
	MariaDBURLEnv  = "OPENPSIRT_TEST_MARIADB_URL"
)

// EnginesEnv narrows which engines run, as a comma-separated list of names.
// Unset means every engine that is configured. The quick loop sets it to
// "sqlite" so that iterating does not wait on three servers; the gate leaves
// it unset. An engine excluded this way is skipped with a message that says
// so, which is a different message from one that is not configured at all.
const EnginesEnv = "OPENPSIRT_TEST_ENGINES"

type candidate struct {
	name database.Engine
	env  string
}

func candidates() []candidate {
	return []candidate{
		{name: database.SQLite},
		{name: database.Postgres, env: PostgresURLEnv},
		{name: database.MySQL, env: MySQLURLEnv},
		{name: database.MariaDB, env: MariaDBURLEnv},
	}
}

// Each runs fn once against every database available, as a subtest.
//
// This is for a test that pins what a query does: every portability defect
// found so far was a query behaving differently on one engine, so a store
// test earns all four. Each subtest gets its own connection, and it is
// closed afterwards.
func Each(t *testing.T, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	run(t, fn, nil)
}

// Two runs fn against SQLite and PostgreSQL only.
//
// This is for a test that pins something above the store — routing, which
// role reaches which endpoint, the shape of a response — where the queries
// underneath already have their own tests on all four engines. Running such
// a test four times proves nothing the store tests did not, and the API
// package alone was a quarter of the whole suite's time. The rule for
// choosing (DAT-37): if the test would fail on one engine and pass on
// another only because of SQL, it belongs in Each — and that includes a
// handler test that pins what a query returns, hides, conflicts on or spells.
func Two(t *testing.T, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	run(t, fn, map[database.Engine]bool{database.SQLite: true, database.Postgres: true})
}

// Only runs fn against one engine.
//
// The narrowest of the three, and it needs the narrowest reason: not "the
// other engines are slow" but "the other engines cannot disagree". What
// qualifies is a test that asks what *we* wrote rather than what an engine did
// with it — the shape of the schema we declared, say, where the statements are
// one list and only the column types differ. A test that would fail on one
// engine and pass on another belongs in Each, whatever it costs (DAT-37).
//
// It also earns its keep where reading the answer is spelled four different
// ways, since the alternative is engine-specific code in a test to check
// something no engine varies.
func Only(t *testing.T, engine database.Engine, fn func(t *testing.T, db *database.DB)) {
	t.Helper()
	run(t, fn, map[database.Engine]bool{engine: true})
}

func run(t *testing.T, fn func(t *testing.T, db *database.DB), only map[database.Engine]bool) {
	t.Helper()
	wanted := enginesWanted()
	ran := 0
	for _, c := range candidates() {
		t.Run(string(c.name), func(t *testing.T) {
			if only != nil && !only[c.name] {
				t.Skipf("%s is not asked for by this kind of test", c.name)
			}
			if wanted != nil && !wanted[c.name] {
				t.Skipf("%s is excluded by %s", c.name, EnginesEnv)
			}
			base := ""
			if c.env != "" {
				base = os.Getenv(c.env)
				if base == "" {
					t.Skipf("%s is not set, so %s is untested here", c.env, c.name)
				}
			}
			db := Open(t, prepared(t, c.name, base))
			ran++
			fn(t, db)
		})
	}
	if ran == 0 {
		t.Fatal("no database was available, so nothing was tested")
	}
}

func enginesWanted() map[database.Engine]bool {
	raw := strings.TrimSpace(os.Getenv(EnginesEnv))
	if raw == "" {
		return nil
	}
	wanted := map[database.Engine]bool{}
	for _, name := range strings.Split(raw, ",") {
		wanted[database.Engine(strings.TrimSpace(strings.ToLower(name)))] = true
	}
	return wanted
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

// prepared returns the URL of a migrated database for this test.
//
// SQLite: a copy of a template migrated once per binary, in a directory of
// the test's own. A file rather than :memory:, because every pooled
// connection to an in-memory database gets its own empty database, which
// makes migrations appear to vanish between statements.
//
// Servers: the binary's own database, created and migrated on first use.
func prepared(t *testing.T, engine database.Engine, base string) string {
	t.Helper()
	if engine == database.SQLite {
		template, err := sqliteTemplate()
		if err != nil {
			t.Fatalf("build the SQLite template: %v", err)
		}
		path := filepath.Join(sqliteDir(t), "test.db")
		if err := os.WriteFile(path, template, 0o600); err != nil {
			t.Fatalf("copy the SQLite template: %v", err)
		}
		return "sqlite://" + path + sqliteTestPragmas
	}
	own, err := serverDatabase(engine, base)
	if err != nil {
		t.Fatalf("prepare a %s database for this package: %v", engine, err)
	}
	return own
}

var (
	sqliteOnce  sync.Once
	sqliteBytes []byte
	sqliteErr   error

	serverMu   sync.Mutex
	serverURLs = map[database.Engine]string{}
)

// sqliteTestPragmas is what a test database adds to the pragmas every SQLite
// connection gets: no syncing at all. A test database is thrown away at the
// end of the test, so durability across a crash buys nothing, and SQLite's
// syncing was most of what a test cost — the API package took 37 s on
// SQLite with it and takes about one without. The journal mode is the one
// the application uses, so a test sees the same locking it does.
const sqliteTestPragmas = "?_pragma=synchronous(OFF)&_pragma=journal_mode(WAL)"

// sqliteDir returns a directory of the test's own for its SQLite file, removed
// when the test ends. The ordinary temporary directory: with syncing off the
// disk is not what a test waits on, and a memory-backed directory in a
// container is often 64 MB.
func sqliteDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// sqliteTemplate migrates one file per binary, the first time it is asked,
// and keeps its bytes rather than the file: a test binary has no hook after
// its last test, so a file would outlive it, and a migrated empty database
// is a few hundred kilobytes.
func sqliteTemplate() ([]byte, error) {
	sqliteOnce.Do(func() {
		dir, err := os.MkdirTemp("", "openpsirt-dbtest-")
		if err != nil {
			sqliteErr = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		path := filepath.Join(dir, "template.db")
		if sqliteErr = migrateFresh("sqlite://" + path + sqliteTestPragmas); sqliteErr != nil {
			return
		}
		sqliteBytes, sqliteErr = os.ReadFile(path) //nolint:gosec // G304: the path is one this function just chose inside its own temporary directory
	})
	return sqliteBytes, sqliteErr
}

// serverDatabase gives this binary its own database on the server the
// configured URL names: dropped if left over from an earlier run of the same
// package, created, and migrated. Named for the package so that two packages
// never share tables and a run leaves at most one database per package
// behind — and dropped on the next run rather than at exit, because a test
// binary has no hook that runs after its last test. Two runs of the same
// package from the same checkout against the same server at once would
// collide; nothing here prevents that, and it is stated so it is not
// discovered.
func serverDatabase(engine database.Engine, base string) (string, error) {
	serverMu.Lock()
	defer serverMu.Unlock()
	if own, ok := serverURLs[engine]; ok {
		return own, nil
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", engine, err)
	}
	name := packageDatabaseName()

	target, err := database.ParseURL(base)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, target)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", target.Redacted, err)
	}
	// Quoted, as every identifier is (DAT-33); the MySQL connections accept
	// the standard quote (DAT-34).
	for _, statement := range []string{
		`DROP DATABASE IF EXISTS "` + name + `"`,
		`CREATE DATABASE "` + name + `"`,
	} {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			_ = admin.Close()
			return "", fmt.Errorf("%s: %w", statement, err)
		}
	}
	if err := admin.Close(); err != nil {
		return "", err
	}

	parsed.Path = "/" + name
	own := parsed.String()
	if err := migrateFresh(own); err != nil {
		return "", err
	}
	serverURLs[engine] = own
	return own, nil
}

// packageDatabaseName names a database for the package this binary tests:
// the package's own name for a person reading the server's list, and a hash
// of its import path and of the directory it is tested from.
//
// The directory is in the hash because the import path is not enough. Two
// checkouts of this repository — a second worktree, say — hold the same
// package at the same import path, and pointed at the same servers they got
// the same name, so one dropped the other's database while it was in use.
// A test binary runs in the directory of the package it tests, and that
// directory includes the checkout's path, which tells the two apart.
func packageDatabaseName() string {
	path := os.Args[0]
	if info, ok := debug.ReadBuildInfo(); ok && info.Path != "" {
		path = info.Path
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = ""
	}
	return databaseName(path, dir)
}

// databaseName is the name for the package at path when tested from dir.
// Short enough for every engine's limit on identifier length.
func databaseName(path, dir string) string {
	base := strings.TrimSuffix(filepath.Base(path), ".test")
	base = notIdentifier.ReplaceAllString(strings.ToLower(base), "_")
	if len(base) > 24 {
		base = base[:24]
	}
	sum := sha256.Sum256([]byte(path + "\x00" + dir))
	return "openpsirt_t_" + base + "_" + hex.EncodeToString(sum[:3])
}

var notIdentifier = regexp.MustCompile(`[^a-z0-9_]+`)

// migrateFresh applies every migration to the database at url and closes it.
func migrateFresh(url string) error {
	target, err := database.ParseURL(url)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := database.Open(ctx, target)
	if err != nil {
		return fmt.Errorf("open %s: %w", target.Redacted, err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := schema.Up(ctx, db, quiet); err != nil {
		_ = db.Close()
		return fmt.Errorf("migrate %s: %w", target.Redacted, err)
	}
	return db.Close()
}

// tables lists every table, in an order safe to delete from: children before
// the rows they reference.
//
// **Add new tables at the top.** A table missing from this list leaves rows
// behind between tests, and one in the wrong position fails on the engines
// that enforce foreign keys during a bulk delete — which is not all of them,
// so it will look engine-specific rather than like the ordering mistake it is.
var tables = []string{
	// Points at nothing, so its position says nothing. First because that is
	// where new tables go.
	"lease",
	// Before person, which it points at.
	"notification",
	"decision_comment",
	"vulnerability_reference",
	"decision_approval",
	"decision_revision",
	"decision",
	// Before person, which it points at, and after decision, which points
	// at it.
	"claim",
	// Before person and vulnerability, which it points at.
	"assessment",
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

	// One transaction, not thirty statements. SQLite in its default mode
	// syncs the file at every commit, and thirty commits of that between
	// every pair of tests was a large part of what a test on SQLite cost.
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// A tag points at the branch it was cut from. MySQL and MariaDB
		// enforce that self-reference during a bulk delete even though every
		// row is going; PostgreSQL and SQLite happen not to.
		if _, err := tx.ExecContext(ctx, "UPDATE stream SET parent_id = NULL"); err != nil {
			return fmt.Errorf("detach stream parents: %w", err)
		}
		// A claim points at the claim it was derived from, which is the same
		// shape and the same two engines.
		if _, err := tx.ExecContext(ctx, "UPDATE claim SET derived_from = NULL"); err != nil {
			return fmt.Errorf("detach derived claims: %w", err)
		}
		for _, table := range tables {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
}
