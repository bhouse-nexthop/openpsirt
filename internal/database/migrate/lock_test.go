package migrate

import (
	"context"
	"os"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
)

// TestLockExcludesAnotherConnection is the only test that exercises the
// advisory lock.
//
// It cannot be written through schema.Up: every caller there serializes on the
// in-process mutex before reaching acquire, so a test driving goroutines
// through Up passes with the entire advisory lock deleted. It proves the mutex
// and nothing else. This drives acquire directly, from two separate pools.
func TestLockExcludesAnotherConnection(t *testing.T) {
	// Otherwise the second attempt waits five minutes.
	restore := lockWaitSeconds
	lockWaitSeconds = 2
	t.Cleanup(func() { lockWaitSeconds = restore })

	for name, env := range map[string]string{
		"postgres": dbtest.PostgresURLEnv,
		"mysql":    dbtest.MySQLURLEnv,
		"mariadb":  dbtest.MariaDBURLEnv,
	} {
		t.Run(name, func(t *testing.T) {
			url := os.Getenv(env)
			if url == "" {
				t.Skipf("%s is not set, so the migration lock is untested here", env)
			}
			ctx := t.Context()

			// Two pools, as two instances would be.
			first := dbtest.Open(t, url)
			second := dbtest.Open(t, url)

			release, err := acquire(ctx, first)
			if err != nil {
				t.Fatalf("first instance could not take the lock: %v", err)
			}

			// The whole point: while one holds it, another must not get it.
			if release2, err := acquire(ctx, second); err == nil {
				_ = release2(ctx)
				_ = release(ctx)
				t.Fatal("a second instance took the migration lock while it was held")
			}

			if err := release(ctx); err != nil {
				t.Fatalf("release: %v", err)
			}

			// And once released, the next instance must get it — a lock that
			// is never released is as broken as one that never excludes.
			release2, err := acquire(ctx, second)
			if err != nil {
				t.Fatalf("lock was not released: %v", err)
			}
			if err := release2(ctx); err != nil {
				t.Errorf("release by the second instance: %v", err)
			}
		})
	}
}

func TestSQLiteNeedsNoAdvisoryLock(t *testing.T) {
	db := dbtest.Open(t, "sqlite://"+t.TempDir()+"/lock.db")
	release, err := acquire(context.Background(), db)
	if err != nil {
		t.Fatalf("acquire on sqlite: %v", err)
	}
	if err := release(context.Background()); err != nil {
		t.Errorf("release on sqlite: %v", err)
	}
	_ = database.SQLite
}
