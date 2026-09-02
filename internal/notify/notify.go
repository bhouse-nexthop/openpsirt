// Package notify records what somebody is told, and reads it back.
//
// It is the in-app notification area (NTF-08, NTF-10) — the one channel that
// works with nothing configured. Everyone has one: a triager sees work
// arriving, a proposer sees a dismissal sent back, an approver sees what waits
// on them, an administrator sees that the tool itself is unwell. What differs
// by role is the content, not the mechanism.
//
// Mail and chat are separate channels behind their own interface (NTF-01), and
// nothing here assumes either exists.
package notify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Lifetime is how long a notification is worth showing.
//
// The two are not a tidiness distinction, they are the design (NTF-09). An
// Event happened once and is acknowledged by the person it happened to. A
// Condition is true while something is true and clears itself when that stops
// — a build that resumes being scanned should leave the list without anybody
// dismissing it. Treating a condition as an event fills the count with
// problems that already went away, and then nobody reads the count.
type Lifetime string

const (
	// Event is a thing that happened: you were assigned this, your dismissal
	// was sent back, somebody named you.
	Event Lifetime = "event"
	// Condition is a state that holds: this build stopped being scanned, this
	// scan will not parse, this person holds work and has not been seen.
	Condition Lifetime = "condition"
)

// Kind is what happened, as a word the interface knows how to draw. Spelled
// here so a producer and a screen cannot disagree about it.
type Kind string

const (
	// Assigned is work arriving, which is what a triager most wants to notice.
	Assigned Kind = "assigned"
	// SentBack is a dismissal an approver asked more of. It goes straight back
	// into the proposer's queue, so silence would leave it sitting (NTF-05).
	SentBack Kind = "sent-back"
	// BuildQuiet is a build nothing has been filed against for longer than
	// this deployment allows. A condition: it clears when a scan arrives.
	BuildQuiet Kind = "build-quiet"
)

// Notification is one thing somebody was told.
type Notification struct {
	bun.BaseModel `bun:"table:notification,alias:nt"`

	ID       int64    `bun:"id,pk,autoincrement"`
	PersonID int64    `bun:"person_id,notnull"`
	Kind     Kind     `bun:"kind,notnull"`
	Lifetime Lifetime `bun:"lifetime,notnull"`
	// About names what a condition is about. Empty for an event.
	About string `bun:"about,notnull"`
	// AboutOpen carries the uniqueness: the same value while the condition
	// holds, null once it has cleared. See the migration for why it is a
	// column rather than a partial index.
	AboutOpen *string `bun:"about_open"`
	// Body and Link describe a moment. They are stored rather than derived on
	// read because the finding a line names may since have been decided,
	// closed or reopened, and re-deriving would describe the world now instead
	// of the world somebody was told about.
	Body      string     `bun:"body,notnull"`
	Link      string     `bun:"link,notnull"`
	CreatedAt time.Time  `bun:"created_at,notnull"`
	ReadAt    *time.Time `bun:"read_at"`
	ClearedAt *time.Time `bun:"cleared_at"`
}

// Store records and reads notifications.
type Store struct {
	db  *bun.DB
	now func() time.Time
}

// NewStore returns a store over db.
func NewStore(db *bun.DB) *Store {
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Telling is one thing to tell one person.
type Telling struct {
	PersonID int64
	Kind     Kind
	// About is required for a condition and refused for an event: a condition
	// that cannot say what it is about cannot be cleared when that thing
	// changes, and an event that names one would be silently deduplicated
	// against an unrelated row.
	About string
	Body  string
	Link  string
}

// Tell records something that happened, for one person.
//
// Events are not deduplicated. Being assigned the same finding twice is two
// things that happened, and collapsing them would lose the second — which is
// the one the person has not seen.
func (s *Store) Tell(ctx context.Context, t Telling) error {
	if t.PersonID == 0 || t.Kind == "" || t.Body == "" {
		return errors.New("a notification needs somebody, a kind and something to say")
	}
	if t.About != "" {
		return fmt.Errorf("%s: an event is about a moment rather than a state, "+
			"so it takes no subject to clear against", t.Kind)
	}
	row := &Notification{
		PersonID: t.PersonID, Kind: t.Kind, Lifetime: Event,
		Body: t.Body, Link: t.Link, CreatedAt: s.now(),
	}
	if _, err := s.db.NewInsert().Model(row).Exec(ctx); err != nil {
		return fmt.Errorf("record what happened: %w", err)
	}
	return nil
}

// Holds is one condition that is currently true.
type Holds struct {
	// About names the thing, and is what makes two runs of the same pass
	// recognize the same condition rather than opening it again.
	About string
	Body  string
	Link  string
}

// Reconcile makes the open conditions of one kind, for one person, exactly
// these — opening what is newly true and clearing what has stopped being true.
//
// This is the whole of NTF-09. The pass that derives a condition does not have
// to remember what it said last time, and nobody has to dismiss an alert about
// a problem that went away: the answer is recomputed and the difference is
// what gets written.
//
// Returns how many were opened and how many cleared, because a pass that
// reports "nothing changed" is how somebody notices it has stopped working.
func (s *Store) Reconcile(ctx context.Context, personID int64, kind Kind,
	holding []Holds) (opened, cleared int, err error) {

	if personID == 0 || kind == "" {
		return 0, 0, errors.New("a condition needs somebody and a kind")
	}
	wanted := make(map[string]Holds, len(holding))
	for _, h := range holding {
		if h.About == "" {
			return 0, 0, fmt.Errorf("%s: a condition has to say what it is about", kind)
		}
		wanted[h.About] = h
	}

	err = database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		opened, cleared = 0, 0

		var open []Notification
		if err := tx.NewSelect().Model(&open).
			Where("person_id = ?", personID).
			Where("kind = ?", kind).
			Where("cleared_at IS NULL").
			Scan(ctx); err != nil {
			return fmt.Errorf("read what is already being said: %w", err)
		}

		now := s.now()
		held := make(map[string]bool, len(open))
		for _, row := range open {
			held[row.About] = true
			if _, still := wanted[row.About]; still {
				continue
			}
			// It stopped being true. The row is kept and marked rather than
			// deleted, so "this cleared" stays answerable and a condition that
			// returns is a new row rather than an edit of an old one.
			if _, err := tx.NewUpdate().Model((*Notification)(nil)).
				Set("cleared_at = ?", now).
				Set("about_open = NULL").
				Where("id = ?", row.ID).Exec(ctx); err != nil {
				return fmt.Errorf("clear a condition that ended: %w", err)
			}
			cleared++
		}

		for about, h := range wanted {
			if held[about] {
				continue
			}
			key := about
			row := &Notification{
				PersonID: personID, Kind: kind, Lifetime: Condition,
				About: about, AboutOpen: &key,
				Body: h.Body, Link: h.Link, CreatedAt: now,
			}
			// Each insert stands on its own savepoint, because carrying on
			// after a failed write is not something every engine allows.
			//
			// Another process getting there first is the ordinary case rather
			// than a fault: a deployment runs more than one of these — the
			// chart ships two replicas and every one sweeps — so two saying
			// the same true thing at the same moment is expected, the unique
			// index is what makes it one row, and a duplicate is not
			// retryable, so aborting the sweep would leave every
			// administrator after this one told nothing.
			//
			// But on PostgreSQL a refused statement poisons the whole
			// transaction: everything after it fails and the commit turns
			// itself into a rollback. Simply continuing therefore threw away
			// the clears made above and reported counts for work that had not
			// happened — and it did so only on that one engine, which is why
			// this is a savepoint rather than a bare `continue`. `SAVEPOINT`
			// is plain SQL that all four engines take, so this stays engine
			// agnostic.
			sp, err := tx.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("hold a point to open a condition from: %w", err)
			}
			if _, err := sp.NewInsert().Model(row).Exec(ctx); err != nil {
				_ = sp.Rollback()
				if database.IsDuplicate(err) {
					continue
				}
				return fmt.Errorf("open a condition that became true: %w", err)
			}
			if err := sp.Commit(); err != nil {
				return fmt.Errorf("keep a condition that was opened: %w", err)
			}
			opened++
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return opened, cleared, nil
}

// Waiting is what one person has not dealt with, newest first, and how many
// there are.
//
// A cleared condition is not waiting on anybody: the thing it was about
// stopped being true, which is the answer rather than a task.
func (s *Store) Waiting(ctx context.Context, subject access.Subject,
	limit, offset int) ([]Notification, int, error) {

	if subject.Kind != access.Person || subject.ID == 0 {
		// A pipeline key is not a person and has nothing to be told.
		return nil, 0, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	mine := func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("person_id = ?", subject.ID).
			Where("read_at IS NULL").
			Where("cleared_at IS NULL")
	}

	total, err := mine(s.db.NewSelect().Model((*Notification)(nil))).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count what is waiting: %w", err)
	}
	var rows []Notification
	if err := mine(s.db.NewSelect().Model(&rows)).
		Order("created_at DESC", "id DESC").
		Limit(limit).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, fmt.Errorf("read what is waiting: %w", err)
	}
	return rows, total, nil
}

// Acknowledge marks one of somebody's notifications read.
//
// Their own only: the identifier is a number a caller supplies, and a store
// that took it at face value would let anybody mark anybody's list read.
func (s *Store) Acknowledge(ctx context.Context, subject access.Subject, id int64) error {
	if subject.Kind != access.Person || subject.ID == 0 {
		return access.Denied("acknowledge a notification")
	}
	res, err := s.db.NewUpdate().Model((*Notification)(nil)).
		Set("read_at = ?", s.now()).
		Where("id = ?", id).
		Where("person_id = ?", subject.ID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("acknowledge: %w", err)
	}
	// Matched rather than changed: the connection settings make an affected
	// count mean "the row was still there" on all four engines (DAT-35), so
	// zero here means it is not theirs or does not exist — which are the same
	// answer on purpose.
	//
	// Which is only true because the update does not also require it to be
	// unread. With that condition, acknowledging something twice reported zero
	// and was refused — so a second click, or a click racing the button that
	// clears everything, answered "no notification of yours by that number"
	// about one plainly theirs. Acknowledging is idempotent instead.
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return access.Denied("acknowledge a notification")
	}
	return nil
}

// AcknowledgeAll marks everything somebody is waiting on read.
func (s *Store) AcknowledgeAll(ctx context.Context, subject access.Subject) (int, error) {
	if subject.Kind != access.Person || subject.ID == 0 {
		return 0, access.Denied("acknowledge notifications")
	}
	res, err := s.db.NewUpdate().Model((*Notification)(nil)).
		Set("read_at = ?", s.now()).
		Where("person_id = ?", subject.ID).
		Where("read_at IS NULL").
		Where("cleared_at IS NULL").
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("acknowledge everything: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}
