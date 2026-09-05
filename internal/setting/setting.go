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
	// DiscloseAfter is how long a finding nobody has announced stays that way
	// before the date arrives. It gives the embargo an end somebody outside
	// could hold us to, which is the point of having one at all.
	DiscloseAfter = "disclosure.after"
	// ExtensionThreshold is how much an embargo may be moved by, in total,
	// before a second person has to agree to moving it further. Cumulative for
	// the same reason the deferral one is: measured per extension, the
	// exception swallows the rule three weeks at a time.
	ExtensionThreshold = "disclosure.extension-threshold"
	// DeferralThreshold is how long something may be put off before a second
	// person has to agree. It ships with a starting point rather than a fixed
	// rule, because how long is too long is a judgment about a product.
	DeferralThreshold = "triage.deferral-threshold"
	// How long a finding may stay open before it is late, by how urgent it is.
	//
	// Being exploited has its own, and it is the shortest: severity is how bad
	// the flaw is, and being exploited is a fact about the world. Without a
	// separate one the deadline contradicts the ranking, which puts an
	// exploited medium above an unexploited critical — the list would say look
	// at this first while the clock said ninety days.
	// TriageFloor is what this deployment considers worth triaging: a severity
	// word, below which a finding is recorded and counted but kept out of the
	// working list (TRI-43). A product may state its own instead.
	TriageFloor = "triage.floor"

	DueExploited = "remediation.due.exploited"
	DueCritical  = "remediation.due.critical"
	DueHigh      = "remediation.due.high"
	DueMedium    = "remediation.due.medium"
	DueLow       = "remediation.due.low"
	// TogetherCap is how many findings one action may claim about at once.
	// A bound rather than none, because a single action writing an unbounded
	// number of rows is a denial of service somebody triggers by accident. How
	// generous it should be is a judgment about a product — a kernel's list is
	// long — so it is tuned here rather than compiled in.
	TogetherCap = "triage.together-cap"
	// QuietAfter is how long a declared build may go without a scan arriving
	// before it is reported as having gone quiet.
	//
	// It is a judgment about how often a deployment expects to be scanned —
	// nightly for some, on a release cadence for others — so it is tuned here
	// rather than compiled in. A build that stops being scanned reports no new
	// findings and fails nothing, which is why silence has to be looked for
	// rather than waited for.
	QuietAfter = "scanning.quiet-after"
	// ScanEvery is how often everything tracked is scanned again against the
	// vulnerability data of the day (ING-20).
	//
	// A judgment about a deployment rather than a constant: the data moves
	// daily, so scanning more often than that measures the same thing twice,
	// and scanning much less often means an advisory published this morning
	// waits to be noticed. It is tuned here rather than compiled in because
	// what a deployment's scanner costs to run over its whole estate is a
	// question about that estate.
	ScanEvery = "scanning.every"
	// UpstreamCurrency is whether this deployment asks public package indexes
	// what the newest version of a component is (ING-41).
	//
	// Off unless somebody turns it on, and the only setting here that decides
	// whether we talk to anyone. Every other outside answer arrives as a file
	// somebody imported deliberately, so that a deployment can run somewhere
	// sealed off; a deployment that cannot reach out loses this answer and
	// nothing else.
	UpstreamCurrency = "upstream.currency"
	// AttachmentMaxSize is the largest single file this deployment accepts, in
	// bytes, and AttachmentQuota is how much it will hold in total (ATT-09).
	//
	// Two limits rather than one because they answer different questions. The
	// first stops one upload being enormous; the second stops many ordinary
	// ones filling a disk somebody else pays for. Storage that another person
	// fills on our behalf needs a ceiling, and how high it should be is a
	// judgment about a deployment rather than a constant.
	AttachmentMaxSize = "attachment.max-size"
	AttachmentQuota   = "attachment.quota"
)

// DefaultAttachmentMaxSize is what one file may be where nobody has said.
//
// Screenshots and logs are what people attach, and both fit comfortably. It is
// deliberately not generous: an operator who wants to accept a core dump can
// say so, and the direction that needs a deliberate act is the one that fills
// a disk.
const DefaultAttachmentMaxSize = 25 << 20

// DefaultAttachmentQuota is what a deployment holds in total where nobody has
// said. Four hundred files at the default size, which is a working year for a
// team and small enough that filling it is noticed rather than invoiced.
const DefaultAttachmentQuota = 10 << 30

// DefaultQuietAfter is how long a build may go unscanned before it is reported
// as having gone quiet, where a deployment has not said otherwise.
//
// A week: long enough that a nightly build missing one night is not an alert,
// short enough that a pipeline switched off is noticed in the week it happened.
//
// It lives beside the name it defaults rather than in whichever package needed
// it first, because two now do — the endpoint that answers what has been
// scanned, and the pass that turns going quiet into something somebody is
// told. Two copies of a default is two policies that agree until one moves.
const DefaultQuietAfter = 7 * 24 * time.Hour

// DefaultScanEvery is how often everything tracked is scanned again, where a
// deployment has not said otherwise.
//
// A day, because the vulnerability databases the scanner reads are published
// daily: more often measures the same data twice, and less often means an
// advisory published this morning waits for the difference before anybody sees
// it against a release that has not been rebuilt in a year.
const DefaultScanEvery = 24 * time.Hour

// DefaultDiscloseAfter is how long an undisclosed finding has before its date
// arrives, where a deployment has not said otherwise.
//
// Ninety days is what coordinated disclosure practice converges on, and the
// number matters less than there being one: an embargo with no end is the
// indefinite secrecy the disclosure frameworks warn about, arrived at by
// nobody deciding anything.
//
// Reaching the date discloses nothing on its own (ACC-47). It is a date to
// answer, not a trigger.
const DefaultDiscloseAfter = 90 * 24 * time.Hour

// DefaultExtensionThreshold is how much an embargo may be moved by in total
// before a second person has to agree.
//
// Thirty days: enough that a fix slipping a sprint is ordinary triage, and not
// enough that a ninety-day embargo becomes a year without anybody else
// noticing. Like the deferral threshold it ships as a starting point rather
// than a rule, because how long is too long is a judgment about a product.
const DefaultExtensionThreshold = 30 * 24 * time.Hour

// On and Off are what a setting that is a switch may be set to.
//
// Words rather than true/false, because every setting is stored and returned
// as text and "on" reads the same in the store, in the API and on the screen.
const (
	On  = "on"
	Off = "off"
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

// Count reads a setting as a whole number of things, falling back where it is
// unset, unreadable or not a positive count.
func (s *Store) Count(ctx context.Context, name string, fallback int) (int, error) {
	raw, set, err := s.Get(ctx, name)
	if err != nil || !set {
		return fallback, err
	}
	parsed, err := strconv.Atoi(raw)
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
