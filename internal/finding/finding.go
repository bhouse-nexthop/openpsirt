package finding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
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
	// Superseded means the component's version moved and the issue came with
	// it: this row closed, and the same issue is open against the new version.
	//
	// Told apart from Upgraded because they are opposite answers to "was this
	// fixed". Without it a bump that resolved nothing was recorded as a fix,
	// and the same issue appeared as fixed and as newly present in one
	// release comparison — a document that goes to customers.
	Superseded Closure = "superseded"
	// Unexplained means the component is present and unchanged and the scanner
	// stopped reporting it. It is always flagged and never suppressed.
	Unexplained Closure = "unexplained"
)

// Run is one execution of a scanner over one variant.
type Run struct {
	bun.BaseModel `bun:"table:scan_run,alias:sr"`

	ID       int64 `bun:"id,pk,autoincrement"`
	TargetID int64 `bun:"target_id,notnull"`
	// Scanner, ScannerVersion and DatabaseVersion are what produced this, and
	// RanHere says whether we ran it. Counts are only comparable between
	// products measured the same way, so a report that mixed the two without
	// saying would be a rumor rather than a report.
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
	TargetID        int64 `bun:"target_id,notnull"`
	Kind            Kind  `bun:"kind,notnull"`
	VulnerabilityID int64 `bun:"vulnerability_id,notnull"`
	// Visibility says whether this has been disclosed. A vulnerability a
	// scanner found in a shipped component is public knowledge by the time we
	// hear about it; what is not is a finding somebody entered here.
	Visibility  access.Visibility `bun:"visibility,notnull"`
	ComponentID int64             `bun:"component_id,notnull"`
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
	// FixedAt is when that version became available. How long a fix has
	// existed is a different question from which version carries it, and it is
	// the one that says whether an upgrade is overdue or fresh.
	FixedAt *time.Time `bun:"fixed_at"`
	// DueAt is when this has to be answered by: when it was first seen, plus
	// how long something of this urgency may stay open (REM-25). Stored rather
	// than derived (REM-26) — derived, it costs a pass over every open finding
	// per urgency band, since each band allows a different number of days.
	//
	// It is set when the finding opens and does not move as the finding ages —
	// nothing else would be a deadline. It is recounted on two events, both of
	// which make the stored answer wrong rather than merely old: the policy
	// that sets it changing, and the issue becoming known to be exploited,
	// which is the one signal that decides how long there is (REM-25, RNK-07).
	// A recount runs from when the change was learned, never from when the
	// finding opened, or a fact arriving late would land a deadline in the
	// past.
	//
	// Null on a finding recorded before this was stored, which reads as "not
	// known" rather than "not due" — a finding with no deadline is left out of
	// what is running out rather than treated as overdue.
	DueAt *time.Time `bun:"due_at"`
	// SuppressedBy is the claim the build made that covers this, where it made
	// one. A covered finding is kept and marked rather than dropped: a finding
	// that simply stopped appearing is indistinguishable from a scanner fault,
	// and that is the bucket nothing is allowed to explain away.
	SuppressedBy *int64 `bun:"suppressed_by"`
	// Rank is how urgent this is, as one sortable number, and the two flags
	// are what it was made of that is not already on the row. The rest — the
	// likelihood and the score — belong to the issue and are read from there.
	Urgency       int64 `bun:"urgency,notnull"`
	RankExploited bool  `bun:"urgency_exploited,notnull"`
	RankShipped   bool  `bun:"urgency_shipped,notnull"`
	// AssignedTo is who is dealing with this, and AssignedAt is when they were
	// given it. Absent means nobody, which is a state worth being able to ask
	// about rather than an empty column: work nobody owns is the thing that
	// falls between people.
	//
	// Held per finding rather than per group, because the rows are what
	// everything else is keyed on — but it is set for a whole group at once,
	// since assigning one place of an issue and not another is not something
	// anybody means to do.
	AssignedTo *int64     `bun:"assigned_to"`
	AssignedAt *time.Time `bun:"assigned_at"`
	// LastChangedAt is when anything about this finding last moved — a fix
	// appearing, or the build answering it. A finding open for years outlives
	// any record of the change kept elsewhere, so it carries its own.
	LastChangedAt time.Time `bun:"last_changed_at,notnull"`
	// ArrivedFrom is the upstream version this place held before, recorded
	// only where the version moved and the issue came with it. Its presence is
	// the statement that somebody bumped this and the bump did not resolve it;
	// its value is what they bumped from, so saying so needs no second query.
	ArrivedFrom string `bun:"arrived_from"`
	// OpenedAt is when this became true. Carried here rather than reached
	// through the run, because not every finding has a run: one somebody
	// recorded by hand was opened by a person, and everything that asks when a
	// finding opened has to be able to answer for it.
	OpenedAt time.Time `bun:"opened_at,notnull"`
	// OpenedRunID is the run that opened it, where one did. Nil is a finding a
	// person opened.
	OpenedRunID   *int64  `bun:"opened_run_id"`
	ClosedRunID   *int64  `bun:"closed_run_id"`
	ClosedBecause Closure `bun:"closed_because"`
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
	// FixedAt is when the fixing version became available, where the report
	// says. "Fixed upstream fourteen months ago" is a different conversation
	// from "fixed in 0.17.0", and it is the one that decides whether an
	// upgrade is overdue or fresh.
	FixedAt *time.Time
}

// Applied describes what a run changed.
type Applied struct {
	Opened int
	Closed int
	// Unexplained counts findings that closed with the component present and
	// unchanged. Always reported, never suppressed.
	Unexplained int
	// Updated counts findings that were already open and whose details moved —
	// a fix becoming available, or the build answering them. Somebody waiting
	// for a fix is waiting for exactly this.
	Updated int
	// Suppressed counts findings the build has already argued about. They are
	// open and visible; they are not work anybody has to do.
	Suppressed int
	// ClaimsReaching and ClaimsReachingNothing say how many of the build's
	// arguments landed on something it ships. One that reached nothing means a
	// finding the build believes it answered comes back as noise, and nothing
	// distinguishes that from a finding nobody has looked at.
	ClaimsReaching        int
	ClaimsReachingNothing int
	// Unplaced counts issues reported against something the target does not
	// contain. A report that does not match the inventory it was produced from
	// is worth seeing rather than quietly discarding.
	Unplaced int
}

// Unchanged reports whether the run changed nothing.
func (a Applied) Unchanged() bool { return a.Opened == 0 && a.Closed == 0 && a.Updated == 0 }

// PlaceIdentity keys a component under the thing that pulled it in.
//
// Names only, never versions: a version in the key would lapse every decision
// the next time anything was rebuilt. Where a component sits directly under
// the product, its name stands alone, because the product's name differs per
// variant and including it would stop the same place being recognized across
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

// Finish records that a run ended, what produced it, and why it went wrong if
// it did.
//
// The versions arrive here rather than at the start because they are the
// scanner's answer, not our question: what it says it is and what data it
// matched against are known once it has run. A finding that appeared or
// vanished because either moved is unexplainable without them.
func (s *Store) Finish(ctx context.Context, runID int64, version, databaseVersion string, cause error) error {
	done := s.now().Truncate(time.Microsecond)
	failure := ""
	if cause != nil {
		failure = cause.Error()
	}
	_, err := s.db.NewUpdate().Model((*Run)(nil)).
		Set("finished_at = ?", done).Set("failure = ?", failure).
		Set("scanner_version = ?", version).Set("database_version = ?", databaseVersion).
		Where("id = ?", runID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("record the end of scan run %d: %w", runID, err)
	}
	return nil
}
