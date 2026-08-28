package finding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/graph"
)

// FixState is what upstream has done about an issue.
//
// The three mean different things to whoever is triaging: no fix existing yet,
// upstream having declined to fix it, and a fix being available are separate
// situations, and "upstream will not fix this" is a permanent condition that
// changes the outcome somebody should reach.
type FixState string

const (
	// NoFix means no fix is available.
	NoFix FixState = "none"
	// WontFix means upstream declined to fix it.
	WontFix FixState = "wont-fix"
	// FixedUpstream means a fixed version exists.
	FixedUpstream FixState = "fixed"
)

// Closure says why a finding stopped being present.
//
// A finding that closes without a reason is a finding nobody can account for,
// and there is no volume at which "we cannot explain this" stops mattering.
type Closure string

const (
	// Removed means the component is no longer in the build at all.
	Removed Closure = "removed"
	// Upgraded means the component's upstream version moved.
	Upgraded Closure = "upgraded"
	// Revised means the shipped version changed while the upstream version did
	// not, which is what a carried patch looks like from the outside.
	Revised Closure = "revised"
	// Unexplained means the component is present and unchanged and the scanner
	// stopped reporting it. It is always flagged and never suppressed.
	Unexplained Closure = "unexplained"
)

// Run is one execution of a scanner over one variant.
type Run struct {
	bun.BaseModel `bun:"table:scan_run,alias:sr"`

	ID        int64 `bun:"id,pk,autoincrement"`
	VariantID int64 `bun:"variant_id,notnull"`
	// Scanner, ScannerVersion and DatabaseVersion are what produced this, and
	// RanHere says whether we ran it. Counts are only comparable between
	// products measured the same way, so a report that mixed the two without
	// saying would be a rumour rather than a report.
	Scanner         string     `bun:"scanner,notnull"`
	ScannerVersion  string     `bun:"scanner_version"`
	DatabaseVersion string     `bun:"database_version"`
	RanHere         bool       `bun:"ran_here,notnull"`
	StartedAt       time.Time  `bun:"started_at,notnull"`
	FinishedAt      *time.Time `bun:"finished_at"`
	Failure         string     `bun:"failure"`
}

// Finding is a vulnerability at a place.
type Finding struct {
	bun.BaseModel `bun:"table:finding,alias:f"`

	ID              int64 `bun:"id,pk,autoincrement"`
	VariantID       int64 `bun:"variant_id,notnull"`
	Kind            Kind  `bun:"kind,notnull"`
	VulnerabilityID int64 `bun:"vulnerability_id,notnull"`
	ComponentID     int64 `bun:"component_id,notnull"`
	// ConsumerID is what pulled the component in. Empty where that is the
	// product itself: the root's name differs per variant, so keying on it
	// would break grouping the same finding across variants.
	ConsumerID *int64 `bun:"consumer_id"`
	// PlaceIdentity is the hashed pair of names. It is what a triage decision
	// is keyed on, so it is stored rather than derived — a decision has to be
	// findable without walking the graph of every variant it might reach.
	PlaceIdentity string   `bun:"place_identity,notnull"`
	FixState      FixState `bun:"fix_state"`
	FixedIn       string   `bun:"fixed_in"`
	OpenedRunID   int64    `bun:"opened_run_id,notnull"`
	ClosedRunID   *int64   `bun:"closed_run_id"`
	ClosedBecause Closure  `bun:"closed_because"`
}

// Reported is one issue a scanner reported against one component.
//
// It names a package at a version and stops there. Where that package sits is
// not something a scanner can know, because it never saw the graph.
type Reported struct {
	Issue     Named
	Component graph.Described
	FixState  FixState
	FixedIn   string
}

// Applied describes what a run changed.
type Applied struct {
	Opened int
	Closed int
	// Unexplained counts findings that closed with the component present and
	// unchanged. Always reported, never suppressed.
	Unexplained int
	// Unplaced counts issues reported against something the variant does not
	// contain. A report that does not match the inventory it was produced from
	// is worth seeing rather than quietly discarding.
	Unplaced int
}

// Unchanged reports whether the run changed nothing.
func (a Applied) Unchanged() bool { return a.Opened == 0 && a.Closed == 0 }

// PlaceIdentity keys a component under the thing that pulled it in.
//
// Names only, never versions: a version in the key would lapse every decision
// the next time anything was rebuilt. Where a component sits directly under
// the product, its name stands alone, because the product's name differs per
// variant and including it would stop the same place being recognised across
// them.
func PlaceIdentity(component, consumer string) string {
	basis := strings.TrimSpace(component)
	if c := strings.TrimSpace(consumer); c != "" {
		basis = c + "\x00" + basis
	}
	sum := sha256.Sum256([]byte(basis))
	return hex.EncodeToString(sum[:])
}

// Store records what runs find.
type Store struct {
	db  *bun.DB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db *bun.DB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Begin records that a scanner is about to run.
func (s *Store) Begin(ctx context.Context, run Run) (*Run, error) {
	run.StartedAt = s.now().Truncate(time.Microsecond)
	if _, err := s.db.NewInsert().Model(&run).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record the start of a scan run: %w", err)
	}
	return &run, nil
}

// Finish records that it ended, and why if it went wrong.
func (s *Store) Finish(ctx context.Context, runID int64, cause error) error {
	done := s.now().Truncate(time.Microsecond)
	failure := ""
	if cause != nil {
		failure = cause.Error()
	}
	_, err := s.db.NewUpdate().Model((*Run)(nil)).
		Set("finished_at = ?", done).Set("failure = ?", failure).
		Where("id = ?", runID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("record the end of scan run %d: %w", runID, err)
	}
	return nil
}
