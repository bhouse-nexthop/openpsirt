package access

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// atClock is a store whose sense of now a test moves, because a session
// expires by time passing rather than by being born expired — the constructor
// deliberately refuses to issue one that has already run out.
func atClock(t *testing.T, db *database.DB, at *time.Time) (*Store, int64) {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := schema.Up(t.Context(), db, quiet); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dbtest.Reset(t, db)

	// An administrator, so that no product has to be declared here: what these
	// tests move is the clock, and a role grant would only add a table this
	// package cannot reach without importing something that imports it back.
	store := &Store{db: db.DB, now: func() time.Time { return *at }}
	person, err := store.Ensure(t.Context(), "someone", "", true)
	if err != nil {
		t.Fatal(err)
	}
	return store, person.ID
}

func TestASessionStopsWorkingOnceItsLifetimeHasPassed(t *testing.T) {
	// The lifetime is not decoration. Group membership is read at sign-in and
	// never again, so this is the window in which somebody who moved out of a
	// team still holds what the team gave them.
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		at := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
		store, person := atClock(t, db, &at)

		issued, err := store.StartSession(t.Context(), person, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ResolveSession(t.Context(), issued.Token); err != nil {
			t.Fatalf("a session refused inside its lifetime: %v", err)
		}

		at = at.Add(time.Hour + time.Second)
		if _, _, err := store.ResolveSession(t.Context(), issued.Token); err == nil {
			t.Error("a session outlived its lifetime")
		}
	})
}

func TestClearingExpiredSessionsLeavesTheLiveOnesAlone(t *testing.T) {
	// Sessions are the one table here that grows with use rather than with
	// what is being tracked.
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		at := time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
		store, person := atClock(t, db, &at)

		brief, err := store.StartSession(t.Context(), person, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		lasting, err := store.StartSession(t.Context(), person, 24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}

		at = at.Add(2 * time.Hour)
		cleared, err := store.PurgeExpiredSessions(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if cleared != 1 {
			t.Errorf("cleared %d sessions, want 1", cleared)
		}
		if _, _, err := store.ResolveSession(t.Context(), lasting.Token); err != nil {
			t.Errorf("the live session was cleared too: %v", err)
		}
		if _, _, err := store.ResolveSession(t.Context(), brief.Token); err == nil {
			t.Error("the expired session still resolves")
		}
	})
}
