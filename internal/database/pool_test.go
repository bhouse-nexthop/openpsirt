package database_test

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
)

func TestIdleConnectionsAreReaped(t *testing.T) {
	// The whole defense rests on this: a connection must be closed by us
	// before anything in the path kills it behind our back. Asserting the
	// setting was applied would prove nothing, so this checks the pool's own
	// count of connections it closed for being idle too long.
	for name, env := range map[string]string{
		"postgres": dbtest.PostgresURLEnv,
		"mysql":    dbtest.MySQLURLEnv,
		"mariadb":  dbtest.MariaDBURLEnv,
	} {
		t.Run(name, func(t *testing.T) {
			url := os.Getenv(env)
			if url == "" {
				t.Skipf("%s is not set", env)
			}
			target, err := database.ParseURL(url)
			if err != nil {
				t.Fatal(err)
			}
			db, err := database.OpenWithPool(t.Context(), target, database.Pool{
				MaxOpen: 4, MaxIdle: 4,
				IdleTimeout: 200 * time.Millisecond,
				Lifetime:    time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()

			// Open several at once so more than one lands in the pool.
			var wg sync.WaitGroup
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					var n int
					_ = db.QueryRowContext(t.Context(), "SELECT 1").Scan(&n)
					time.Sleep(50 * time.Millisecond)
				}()
			}
			wg.Wait()

			// Wait for the reaper. Go runs its cleaner on a timer with a
			// one-second floor, however short the idle timeout is — so a
			// shorter wait than that proves nothing either way.
			deadline := time.Now().Add(10 * time.Second)
			var stats sql.DBStats
			for time.Now().Before(deadline) {
				time.Sleep(250 * time.Millisecond)
				stats = db.Stats()
				if stats.MaxIdleTimeClosed > 0 {
					break
				}
			}
			if stats.MaxIdleTimeClosed == 0 {
				t.Errorf("no connection was closed for being idle: %+v", stats)
			}
			t.Logf("%s: closed %d idle connections, %d still open",
				name, stats.MaxIdleTimeClosed, stats.OpenConnections)
		})
	}
}

func TestPoolIsBounded(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		got := db.Stats().MaxOpenConnections
		want := database.DefaultPool().MaxOpen
		if db.Server.Engine == database.SQLite {
			// One writer, so more connections add contention rather than
			// concurrency.
			want = 1
		}
		if got != want {
			t.Errorf("%s: max open connections is %d, want %d", db.Server.Engine, got, want)
		}
	})
}

func TestValidateAnswersQuickly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		start := time.Now()
		if err := db.Validate(t.Context()); err != nil {
			t.Fatalf("validate: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Errorf("validate took %v", elapsed)
		}
	})
}

func TestValidateFailsQuicklyAgainstSomethingUnreachable(t *testing.T) {
	// A dead address, so the check has to give up on its own deadline rather
	// than waiting for the operating system to.
	target, err := database.ParseURL("postgres://u:p@127.0.0.1:1/db?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	start := time.Now()
	db, err := database.Open(ctx, target)
	if err == nil {
		_ = db.Close()
		t.Fatal("connected to an address with nothing on it")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("gave up after %v, which is too slow to be useful", elapsed)
	}
}
