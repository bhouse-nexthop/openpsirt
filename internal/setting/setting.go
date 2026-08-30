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

	"github.com/bhouse-nexthop/openpsirt/internal/database"
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
	// RoleMode is where roles come from: assigned by an administrator, or
	// derived from provider groups. One mode for the whole deployment.
	RoleMode = "roles.mode"
	// MaxTokenLifetime is the longest a person's own credential may last.
	// Expiry is mandatory; this is how far out it may be set.
	MaxTokenLifetime = "token.max-lifetime"
	// DeferralThreshold is how long something may be put off before a second
	// person has to agree. It ships with a starting point rather than a fixed
	// rule, because how long is too long is a judgment about a product.
	DeferralThreshold = "triage.deferral-threshold"
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
//
// Unset is not an error. Every setting has a default, and a deployment that
// has never been tuned is the ordinary case rather than a fault.
//
// **A failure to read is not "unset", though**, and treating the two as one
// was worse than it looks. Every caller falls back to a default when a setting
// is unset, so a database that could not answer would silently swap the
// deployment's configuration for the shipped one — including the threshold
// deciding which deferrals need a second person. A policy that quietly becomes
// a different policy under load is the kind of failure nobody finds, because
// nothing anywhere reports it.
func (s *Store) Get(ctx context.Context, name string) (string, bool, error) {
	row := new(Setting)
	switch err := s.db.NewSelect().Model(row).Where("name = ?", name).Scan(ctx); {
	case database.IsNoRows(err):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("read the %q setting: %w", name, err)
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
	db, ok := s.db.(*bun.DB)
	if !ok {
		return fmt.Errorf("this store is already inside a transaction")
	}

	// Inside one transaction, and retried whole. Written as two statements it
	// could report success having stored nothing: the update matches no row,
	// another writer inserts one, and a separate existence check then sees a
	// row that the caller's value never reached. Whether the row exists and
	// what it says have to be decided in the same view.
	return database.InTransaction(ctx, db, func(ctx context.Context, tx bun.Tx) error {
		now := s.now().Truncate(time.Microsecond)

		if _, err := tx.NewUpdate().Model((*Setting)(nil)).
			Set("value = ?", value).Set("updated_at = ?", now).
			Where("name = ?", name).Exec(ctx); err != nil {
			return fmt.Errorf("record the %q setting: %w", name, err)
		}

		// Asked inside the transaction, so what it sees is what the update
		// just wrote against. Rows touched is not the question: two of the
		// four engines report nothing touched when an update writes a value
		// identical to the one already stored, which is the same number "no
		// such setting" reports.
		n, err := tx.NewSelect().Model((*Setting)(nil)).Where("name = ?", name).Count(ctx)
		if err != nil {
			return fmt.Errorf("record the %q setting: %w", name, err)
		}
		if n > 0 {
			return nil
		}

		row := &Setting{Name: name, Value: value, UpdatedAt: now}
		if _, err := tx.NewInsert().Model(row).Exec(ctx); err != nil {
			return fmt.Errorf("record the %q setting: %w", name, err)
		}
		return nil
	})
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
