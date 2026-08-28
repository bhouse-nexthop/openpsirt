// Package queue is a durable work queue held in the application's own
// database.
//
// It exists rather than a library because the mature Go queues are tied to one
// engine or need a separate service, and neither suits software an operator
// installs against whatever database they already run.
//
// Work survives a restart: a job is a row, and a worker that dies mid-job
// leaves it claimed until the claim goes stale, after which another worker
// takes it. Losing an ingest because a pod was rescheduled would mean a scan
// silently never arriving.
package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// State is where a job has got to.
type State string

const (
	// Pending work is waiting to be claimed.
	Pending State = "pending"
	// Running work is claimed by a worker.
	Running State = "running"
	// Done work succeeded.
	Done State = "done"
	// Dead work failed more times than it is allowed to.
	Dead State = "dead"
)

// Job is one piece of work.
type Job struct {
	bun.BaseModel `bun:"table:job,alias:j"`

	ID   int64  `bun:"id,pk,autoincrement"`
	Kind string `bun:"kind,notnull"`
	// Reference points at what the work is about — a scan, say. The payload
	// itself is never carried here: a job row holds a pointer, so the queue
	// stays small however large the thing being worked on is.
	Reference   string     `bun:"reference,notnull"`
	State       State      `bun:"state,notnull"`
	Attempts    int        `bun:"attempts,notnull"`
	MaxAttempts int        `bun:"max_attempts,notnull"`
	RunAfter    time.Time  `bun:"run_after,notnull"`
	ClaimedBy   *string    `bun:"claimed_by"`
	ClaimedAt   *time.Time `bun:"claimed_at"`
	LastError   *string    `bun:"last_error"`
	CreatedAt   time.Time  `bun:"created_at,notnull"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull"`
}

// ErrBacklogFull is returned when the queue is too deep to take more.
var ErrBacklogFull = errors.New("queue backlog is full")

// Options tune a queue.
type Options struct {
	// MaxAttempts is how many times a job may be tried before it is set aside.
	// Without a limit, a job that can never succeed retries for ever and
	// crowds out work that could.
	MaxAttempts int
	// MaxBacklog caps how much work may be waiting. Beyond it, new work is
	// refused so a runaway producer cannot push everyone else's work behind
	// its own.
	MaxBacklog int
	// ClaimTimeout is how long a claim is honoured before another worker may
	// take the job. It must exceed the longest a job legitimately takes, or
	// two workers end up running the same work.
	ClaimTimeout time.Duration
	// Backoff is how long to wait before retrying, multiplied by the attempt.
	Backoff time.Duration
}

// DefaultOptions suit ingest: work measured in seconds to minutes, and a
// producer that will retry on its own if we refuse.
func DefaultOptions() Options {
	return Options{
		MaxAttempts:  5,
		MaxBacklog:   1000,
		ClaimTimeout: 30 * time.Minute,
		Backoff:      30 * time.Second,
	}
}

// Queue hands out work and records what happened to it.
type Queue struct {
	db   *database.DB
	opts Options
	now  func() time.Time
}

// New returns a queue over db.
func New(db *database.DB, opts Options) *Queue {
	return &Queue{db: db, opts: opts, now: func() time.Time { return time.Now().UTC() }}
}

// Add puts work on the queue, refusing it when the backlog is already too deep.
func (q *Queue) Add(ctx context.Context, kind, reference string) (*Job, error) {
	depth, err := q.Depth(ctx)
	if err != nil {
		return nil, err
	}
	if depth >= q.opts.MaxBacklog {
		return nil, fmt.Errorf("%w: %d waiting, limit is %d", ErrBacklogFull, depth, q.opts.MaxBacklog)
	}

	now := q.now().Truncate(time.Microsecond)
	job := &Job{
		Kind: kind, Reference: reference, State: Pending,
		MaxAttempts: q.opts.MaxAttempts, RunAfter: now,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := q.db.NewInsert().Model(job).Exec(ctx); err != nil {
		return nil, fmt.Errorf("add job: %w", err)
	}
	return job, nil
}

// Depth counts work waiting to be done.
func (q *Queue) Depth(ctx context.Context) (int, error) {
	n, err := q.db.NewSelect().Model((*Job)(nil)).Where("state = ?", Pending).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("measure backlog: %w", err)
	}
	return n, nil
}

// Claim takes the oldest runnable job for this worker, or returns nil when
// there is nothing to do.
//
// Two workers must never get the same job. That is guaranteed by the update
// below, which only succeeds if the job is still claimable — portably, on
// every engine. The engine-specific row locking in claim_locking.go is about
// throughput, not correctness.
func (q *Queue) Claim(ctx context.Context, worker string) (*Job, error) {
	now := q.now().Truncate(time.Microsecond)

	// A job whose claim has gone stale is available again: the worker holding
	// it has died, and waiting for it forever would strand the work.
	staleBefore := now.Add(-q.opts.ClaimTimeout)

	var job *Job
	err := q.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		id, err := claimableID(ctx, tx, q.db.Server.Engine, now, staleBefore)
		if err != nil || id == 0 {
			return err
		}
		// The update repeats the conditions the select just checked, so it
		// only succeeds if the job is *still* claimable.
		//
		// This is what makes claiming correct, rather than the row locking
		// below it. Locking is a throughput matter: it stops workers queueing
		// behind one another on the same row. If it were the guarantee, then
		// getting it wrong for some future engine would mean handing the same
		// work out twice, which on an ingest looks like real change. This way
		// the worst case is slow.
		res, err := tx.NewUpdate().Model((*Job)(nil)).
			Set("state = ?", Running).
			Set("attempts = attempts + 1").
			Set("claimed_by = ?", worker).
			Set("claimed_at = ?", now).
			Set("updated_at = ?", now).
			Where("id = ?", id).
			WhereGroup(" AND ", func(u *bun.UpdateQuery) *bun.UpdateQuery {
				return u.
					WhereOr("state = ? AND run_after <= ?", Pending, now).
					WhereOr("state = ? AND claimed_at < ?", Running, staleBefore)
			}).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("claim job %d: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Someone else got there first. Nothing to do this round.
			return nil
		}
		job = new(Job)
		return tx.NewSelect().Model(job).Where("id = ?", id).Scan(ctx)
	})
	if err != nil {
		return nil, err
	}
	return job, nil
}

// Succeed marks work as finished.
func (q *Queue) Succeed(ctx context.Context, id int64) error {
	now := q.now().Truncate(time.Microsecond)
	_, err := q.db.NewUpdate().Model((*Job)(nil)).
		Set("state = ?", Done).
		Set("claimed_by = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id).Exec(ctx)
	return err
}

// Fail records that work did not succeed.
//
// It goes back on the queue with a delay, unless it has been tried as often as
// it is allowed to be, in which case it is set aside. Retrying for ever would
// let one job that can never succeed crowd out work that could.
func (q *Queue) Fail(ctx context.Context, id int64, cause error) error {
	job := new(Job)
	if err := q.db.NewSelect().Model(job).Where("id = ?", id).Scan(ctx); err != nil {
		return fmt.Errorf("load job %d: %w", id, err)
	}

	now := q.now().Truncate(time.Microsecond)
	message := cause.Error()
	update := q.db.NewUpdate().Model((*Job)(nil)).
		Set("last_error = ?", message).
		Set("claimed_by = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if job.Attempts >= job.MaxAttempts {
		update = update.Set("state = ?", Dead)
	} else {
		// Longer each time, so a dependency that is briefly unavailable is
		// not hammered while it recovers.
		delay := time.Duration(job.Attempts) * q.opts.Backoff
		update = update.Set("state = ?", Pending).Set("run_after = ?", now.Add(delay))
	}
	_, err := update.Exec(ctx)
	return err
}
