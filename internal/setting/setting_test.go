package setting_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

func eachSetting(t *testing.T, fn func(t *testing.T, s *setting.Store)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)
		fn(t, setting.NewStore(db.DB))
	})
}

func TestASettingNobodyHasChangedReadsAsItsDefault(t *testing.T) {
	// Every setting has a default and most deployments change none of them, so
	// unset is the ordinary case rather than a fault.
	eachSetting(t, func(t *testing.T, s *setting.Store) {
		got, err := s.Duration(t.Context(), setting.SessionLifetime, 12*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if got != 12*time.Hour {
			t.Errorf("unset read as %v, want the default", got)
		}
		if _, set, err := s.Get(t.Context(), setting.SessionLifetime); err != nil || set {
			t.Errorf("unset reported as set=%v (%v)", set, err)
		}
	})
}

func TestSettingSomethingTwiceRecordsTheSecondValue(t *testing.T) {
	// Written as an update and then an insert, because no upsert spelling is
	// portable across the four engines. Setting twice is what exercises both
	// halves.
	eachSetting(t, func(t *testing.T, s *setting.Store) {
		ctx := t.Context()
		if err := s.Set(ctx, setting.SessionLifetime, "1h"); err != nil {
			t.Fatal(err)
		}
		if err := s.Set(ctx, setting.SessionLifetime, "30m"); err != nil {
			t.Fatal(err)
		}
		got, err := s.Duration(ctx, setting.SessionLifetime, 12*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if got != 30*time.Minute {
			t.Errorf("read back %v, want 30m", got)
		}
	})
}

func TestSettingSomethingToWhatItAlreadySaysIsNotAFailure(t *testing.T) {
	// The ordinary shape of setting something back to what it already said.
	// The timestamp still moves, so this does not by itself reach the case
	// where an engine reports nothing touched — the frozen-clock test beside
	// this one is what pins that.
	eachSetting(t, func(t *testing.T, s *setting.Store) {
		ctx := t.Context()
		if err := s.Set(ctx, setting.SessionLifetime, "8h"); err != nil {
			t.Fatal(err)
		}
		if err := s.Set(ctx, setting.SessionLifetime, "8h"); err != nil {
			t.Fatalf("setting a value to what it already said: %v", err)
		}
		got, err := s.Duration(ctx, setting.SessionLifetime, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if got != 8*time.Hour {
			t.Errorf("read back %v, want 8h", got)
		}
	})
}

func TestAValueNobodyCanParseReadsAsTheDefault(t *testing.T) {
	// A setting typed by hand into the database is a tuning mistake. Refusing
	// to start over it would make that mistake an outage.
	eachSetting(t, func(t *testing.T, s *setting.Store) {
		ctx := t.Context()
		for _, bad := range []string{"soon", "", "-5m", "0"} {
			if err := s.Set(ctx, setting.SessionLifetime, bad); err != nil {
				t.Fatal(err)
			}
			got, err := s.Duration(ctx, setting.SessionLifetime, 12*time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			if got != 12*time.Hour {
				t.Errorf("%q read as %v, want the default", bad, got)
			}
		}
	})
}
