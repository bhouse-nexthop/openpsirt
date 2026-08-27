package schema_test

import (
	"sync"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

func TestConcurrentStartsDoNotCollide(t *testing.T) {
	// Every instance migrates at startup, so a rolling deploy runs this
	// several times at once. Without the lock they race on the same tables and
	// one of them fails with a duplicate-object error.
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		const instances = 4

		var wg sync.WaitGroup
		errs := make([]error, instances)
		start := make(chan struct{})

		for i := range instances {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start // let them all try at the same moment
				errs[i] = schema.Up(t.Context(), db, quiet())
			}()
		}
		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("instance %d failed to migrate: %v", i, err)
			}
		}
		version, err := schema.Version(t.Context(), db)
		if err != nil {
			t.Fatalf("read version: %v", err)
		}
		if version == 0 {
			t.Fatal("nothing was applied")
		}
	})
}
