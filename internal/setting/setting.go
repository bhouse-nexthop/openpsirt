// Package setting holds what an administrator changes from inside the
// application, as opposed to what an operator sets when deploying it.
//
// The split matters: a configuration file is edited by whoever can reach the
// filesystem and restart the process, and an administrator is generally not
// that person. Anything an administrator is expected to tune belongs here so
// that tuning it is an action in the application with a record of who did it,
// rather than a deployment.
package setting

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/uptrace/bun"
)

// Setting is one named value.
type Setting struct {
	bun.BaseModel `bun:"table:application_setting,alias:as"`

	Name      string    `bun:"name,pk"`
	Value     string    `bun:"value,notnull"`
	UpdatedAt time.Time `bun:"updated_at,notnull"`
}

// The names in use. They are spelled once here rather than at each call site,
// because a misspelling would read as a setting nobody has changed yet and so
// silently take the default.
const (
	// SessionLifetime bounds how long a sign-in lasts. It is also the window
	// in which somebody who moved out of a team still holds what the team gave
	// them, because group membership is only read at sign-in.
	SessionLifetime = "session.lifetime"
)

// Store reads and writes settings.
type Store struct {
	db  bun.IDB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db bun.IDB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Get reads a setting, returning whether it has been set at all.
func (s *Store) Get(ctx context.Context, name string) (string, bool, error) {
	row := new(Setting)
	if err := s.db.NewSelect().Model(row).Where("name = ?", name).Scan(ctx); err != nil {
		// Unset is not an error. Every setting has a default, and a deployment
		// that has never been tuned is the ordinary case rather than a fault.
		return "", false, nil
	}
	return row.Value, true, nil
}

// Set records a setting, whether or not anybody has set it before.
//
// Written as an update and then an insert rather than as one upsert statement,
// because there is no portable spelling of an upsert: two of the four engines
// want ON CONFLICT and the other two want ON DUPLICATE KEY UPDATE, and
// engine-specific SQL is confined to migration data-definition and the queue's
// locking (DAT-02).
//
// The order matters. Updating first means the common case — a setting somebody
// has changed before — is one statement, and the insert is only reached the
// first time. Two administrators setting the same thing at once resolve
// against the primary key: one insert wins, the loser retries as an update.
func (s *Store) Set(ctx context.Context, name, value string) error {
	now := s.now().Truncate(time.Microsecond)

	present, err := s.update(ctx, name, value, now)
	if err != nil {
		return err
	}
	if present {
		return nil
	}

	row := &Setting{Name: name, Value: value, UpdatedAt: now}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err == nil {
		return nil
	}

	// Somebody inserted it between the update and here, so what failed is the
	// race rather than the setting.
	present, err = s.update(ctx, name, value, now)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("record the %q setting: it was neither updated nor inserted", name)
	}
	return nil
}

// update writes a setting and reports whether there was one to write.
//
// Whether a row exists is asked separately rather than read from the number of
// rows the update touched. Two of the four engines report nothing touched when
// an update writes a value identical to the one already stored, so a count of
// zero means either "no such setting" or "that setting already said exactly
// this" — and treating the second as the first would send the caller on to an
// insert that cannot succeed.
func (s *Store) update(ctx context.Context, name, value string, now time.Time) (bool, error) {
	if _, err := s.db.NewUpdate().Model((*Setting)(nil)).
		Set("value = ?", value).Set("updated_at = ?", now).
		Where("name = ?", name).Exec(ctx); err != nil {
		return false, fmt.Errorf("record the %q setting: %w", name, err)
	}
	n, err := s.db.NewSelect().Model((*Setting)(nil)).Where("name = ?", name).Count(ctx)
	if err != nil {
		return false, fmt.Errorf("record the %q setting: %w", name, err)
	}
	return n > 0, nil
}

// Duration reads a setting as a length of time, falling back to fallback where
// it is unset or unreadable.
//
// A value nobody can parse is treated as one nobody set. The alternative is a
// deployment that will not start because a setting somebody typed by hand in
// the database is malformed, which turns a tuning mistake into an outage.
func (s *Store) Duration(ctx context.Context, name string, fallback time.Duration) (time.Duration, error) {
	raw, set, err := s.Get(ctx, name)
	if err != nil || !set {
		return fallback, err
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback, nil
	}
	return parsed, nil
}

// Bool reads a setting as a flag, falling back where it is unset or
// unreadable.
func (s *Store) Bool(ctx context.Context, name string, fallback bool) (bool, error) {
	raw, set, err := s.Get(ctx, name)
	if err != nil || !set {
		return fallback, err
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback, nil
	}
	return parsed, nil
}
