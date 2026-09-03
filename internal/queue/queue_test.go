package queue_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

func each(t *testing.T, opts queue.Options, fn func(t *testing.T, db *database.DB, q *queue.Queue)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)
		fn(t, db, queue.New(db, opts))
	})
}

func TestWorkIsHandedOutOnce(t *testing.T) {
	// The reason this package has engine-specific SQL in it. Two workers
	// reading the same row and both updating it means the same scan ingested
	// twice — and on a duplicate ingest that would look like real change.
	each(t, queue.DefaultOptions(), func(t *testing.T, db *database.DB, q *queue.Queue) {
		ctx := t.Context()
		const jobs = 12
		for i := range jobs {
			if _, err := q.Add(ctx, "ingest", fmt.Sprintf("scan-%d", i)); err != nil {
				t.Fatal(err)
			}
		}

		const workers = 6
		var mu sync.Mutex
		claimed := map[int64]string{}
		duplicates := 0

		var wg sync.WaitGroup
		start := make(chan struct{})
		for w := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				name := fmt.Sprintf("worker-%d", w)
				<-start
				for {
					job, err := q.Claim(ctx, name, "ingest")
					if err != nil {
						t.Errorf("%s: claim: %v", name, err)
						return
					}
					if job == nil {
						return
					}
					mu.Lock()
					if prior, seen := claimed[job.ID]; seen {
						duplicates++
						t.Errorf("job %d claimed by %s and again by %s", job.ID, prior, name)
					}
					claimed[job.ID] = name
					mu.Unlock()
					if err := q.Succeed(ctx, job.ID, name); err != nil {
						t.Errorf("succeed: %v", err)
					}
				}
			}()
		}
		close(start)
		wg.Wait()

		if duplicates > 0 {
			t.Errorf("%d jobs were handed out more than once", duplicates)
		}
		if len(claimed) != jobs {
			t.Errorf("%d of %d jobs were done", len(claimed), jobs)
		}
	})
}

func TestNothingToDoReturnsNothing(t *testing.T) {
	each(t, queue.DefaultOptions(), func(t *testing.T, _ *database.DB, q *queue.Queue) {
		job, err := q.Claim(t.Context(), "worker", "ingest")
		if err != nil {
			t.Fatalf("claim on an empty queue: %v", err)
		}
		if job != nil {
			t.Errorf("got %+v from an empty queue", job)
		}
	})
}

func TestFailedWorkComesBackLater(t *testing.T) {
	each(t, queue.DefaultOptions(), func(t *testing.T, _ *database.DB, q *queue.Queue) {
		ctx := t.Context()
		if _, err := q.Add(ctx, "ingest", "x"); err != nil {
			t.Fatal(err)
		}
		job, err := q.Claim(ctx, "worker", "ingest")
		if err != nil || job == nil {
			t.Fatalf("claim: %v %+v", err, job)
		}
		if err := q.Fail(ctx, job.ID, "worker", errors.New("upstream unavailable")); err != nil {
			t.Fatal(err)
		}
		// Held back deliberately, so a dependency that is briefly unavailable
		// is not hammered while it recovers.
		again, err := q.Claim(ctx, "worker", "ingest")
		if err != nil {
			t.Fatal(err)
		}
		if again != nil {
			t.Error("failed work was handed straight back out with no delay")
		}
	})
}

func TestWorkThatKeepsFailingIsSetAside(t *testing.T) {
	// Without a limit, one job that can never succeed retries for ever and
	// crowds out work that could.
	opts := queue.DefaultOptions()
	opts.MaxAttempts = 3
	opts.Backoff = 0 // retry immediately, so the test does not wait
	each(t, opts, func(t *testing.T, db *database.DB, q *queue.Queue) {
		ctx := t.Context()
		if _, err := q.Add(ctx, "ingest", "doomed"); err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
			job, err := q.Claim(ctx, "worker", "ingest")
			if err != nil {
				t.Fatal(err)
			}
			if job == nil {
				t.Fatalf("nothing to claim on attempt %d", attempt)
			}
			if err := q.Fail(ctx, job.ID, "worker", errors.New("still broken")); err != nil {
				t.Fatal(err)
			}
		}
		if job, err := q.Claim(ctx, "worker", "ingest"); err != nil || job != nil {
			t.Errorf("work was retried past its limit: %+v %v", job, err)
		}
		var state string
		if err := db.QueryRowContext(ctx, "SELECT state FROM job WHERE reference = ?", "doomed").Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state != string(queue.Dead) {
			t.Errorf("state is %q, want %q", state, queue.Dead)
		}
	})
}

func TestAStaleClaimIsTakenOverByAnotherWorker(t *testing.T) {
	// A worker that dies mid-job would otherwise strand the work for ever.
	opts := queue.DefaultOptions()
	opts.ClaimTimeout = time.Millisecond
	each(t, opts, func(t *testing.T, _ *database.DB, q *queue.Queue) {
		ctx := t.Context()
		if _, err := q.Add(ctx, "ingest", "abandoned"); err != nil {
			t.Fatal(err)
		}
		first, err := q.Claim(ctx, "worker-that-dies", "ingest")
		if err != nil || first == nil {
			t.Fatalf("first claim: %v", err)
		}
		time.Sleep(20 * time.Millisecond)

		second, err := q.Claim(ctx, "worker-that-lives", "ingest")
		if err != nil {
			t.Fatal(err)
		}
		if second == nil {
			t.Fatal("the abandoned job was never picked up")
		}
		if second.ID != first.ID {
			t.Errorf("picked up job %d, expected %d", second.ID, first.ID)
		}
		if second.Attempts != 2 {
			t.Errorf("attempts is %d after a retake, want 2", second.Attempts)
		}
	})
}

func TestABacklogThatIsTooDeepIsRefused(t *testing.T) {
	// A runaway producer must not be able to push everyone else's work behind
	// its own.
	opts := queue.DefaultOptions()
	opts.MaxBacklog = 3
	each(t, opts, func(t *testing.T, _ *database.DB, q *queue.Queue) {
		ctx := t.Context()
		for i := range opts.MaxBacklog {
			if _, err := q.Add(ctx, "ingest", fmt.Sprintf("scan-%d", i)); err != nil {
				t.Fatalf("adding %d: %v", i, err)
			}
		}
		if _, err := q.Add(ctx, "ingest", "one too many"); !errors.Is(err, queue.ErrBacklogFull) {
			t.Errorf("error is %v, want ErrBacklogFull", err)
		}
		// Doing some of the work must make room again.
		job, err := q.Claim(ctx, "worker", "ingest")
		if err != nil || job == nil {
			t.Fatal(err)
		}
		if err := q.Succeed(ctx, job.ID, "worker"); err != nil {
			t.Fatal(err)
		}
		if _, err := q.Add(ctx, "ingest", "now there is room"); err != nil {
			t.Errorf("still refused after work was done: %v", err)
		}
	})
}

func TestOldestWorkGoesFirst(t *testing.T) {
	each(t, queue.DefaultOptions(), func(t *testing.T, _ *database.DB, q *queue.Queue) {
		ctx := t.Context()
		for _, ref := range []string{"first", "second", "third"} {
			if _, err := q.Add(ctx, "ingest", ref); err != nil {
				t.Fatal(err)
			}
		}
		for _, want := range []string{"first", "second", "third"} {
			job, err := q.Claim(ctx, "worker", "ingest")
			if err != nil || job == nil {
				t.Fatalf("claim %s: %v", want, err)
			}
			if job.Reference != want {
				t.Errorf("got %q, want %q", job.Reference, want)
			}
			if err := q.Succeed(ctx, job.ID, "worker"); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func TestAWorkerDoesNotTakeAnotherKindOfWork(t *testing.T) {
	// Workers of different sorts share one queue. One taking another's work
	// would not fail: a job's reference means something different to each of
	// them, so the wrong worker would act on it, get an answer, and mark it
	// done. The work it was meant for simply never happens.
	each(t, queue.DefaultOptions(), func(t *testing.T, _ *database.DB, q *queue.Queue) {
		ctx := t.Context()
		if _, err := q.Add(ctx, "ingest", "42"); err != nil {
			t.Fatal(err)
		}

		other, err := q.Claim(ctx, "scanner", "vulnerability.scan")
		if err != nil {
			t.Fatal(err)
		}
		if other != nil {
			t.Fatalf("a scanner took %q work referring to %q", other.Kind, other.Reference)
		}

		mine, err := q.Claim(ctx, "reader", "ingest")
		if err != nil || mine == nil {
			t.Fatalf("the work was not there for the worker it was left for (%v)", err)
		}
		if mine.Reference != "42" {
			t.Errorf("claimed %q", mine.Reference)
		}
	})
}

func TestOnlyTheWorkerHoldingAJobMayFinishIt(t *testing.T) {
	// A worker that ran long enough for its claim to go stale is still running
	// when another takes the job over. Without the condition, whichever
	// finishes first records the ending of what the other is in the middle of
	// — a job marked done while a second worker is still working it, or
	// handed back for retry while it is being finished.
	// Long enough that the second retake below cannot find the first one's
	// claim already stale — under the race detector two claims take longer
	// than a millisecond, and the test then held one job twice.
	opts := queue.DefaultOptions()
	opts.ClaimTimeout = 50 * time.Millisecond
	each(t, opts, func(t *testing.T, db *database.DB, q *queue.Queue) {
		ctx := t.Context()
		for _, ref := range []string{"finished-late", "failed-late"} {
			if _, err := q.Add(ctx, "ingest", ref); err != nil {
				t.Fatal(err)
			}
		}
		var lost []*queue.Job
		for range 2 {
			job, err := q.Claim(ctx, "slow", "ingest")
			if err != nil || job == nil {
				t.Fatalf("first claim: %v", err)
			}
			lost = append(lost, job)
		}
		time.Sleep(100 * time.Millisecond)
		var taken []*queue.Job
		for range 2 {
			job, err := q.Claim(ctx, "fresh", "ingest")
			if err != nil || job == nil {
				t.Fatalf("retake: %v", err)
			}
			taken = append(taken, job)
		}
		if taken[0].ID == taken[1].ID {
			t.Fatalf("the retake claimed job %d twice", taken[0].ID)
		}

		// The slow worker finishes both, one way each. Neither is its to
		// finish any more.
		if err := q.Succeed(ctx, lost[0].ID, "slow"); !errors.Is(err, queue.ErrNoLongerHeld) {
			t.Errorf("a stale worker succeeding a retaken job got %v, want ErrNoLongerHeld", err)
		}
		if err := q.Fail(ctx, lost[1].ID, "slow", errors.New("gave up")); !errors.Is(err, queue.ErrNoLongerHeld) {
			t.Errorf("a stale worker failing a retaken job got %v, want ErrNoLongerHeld", err)
		}
		for _, job := range taken {
			var row queue.Job
			if err := db.DB.NewSelect().Model(&row).Where("id = ?", job.ID).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if row.State != queue.Running || row.ClaimedBy == nil || *row.ClaimedBy != "fresh" {
				t.Errorf("job %d is %s held by %v after a stale worker finished it, want running and held by fresh",
					row.ID, row.State, row.ClaimedBy)
			}
		}

		// And the worker that holds them finishes them as usual.
		if err := q.Succeed(ctx, taken[0].ID, "fresh"); err != nil {
			t.Errorf("the holder could not finish its job: %v", err)
		}
		if err := q.Fail(ctx, taken[1].ID, "fresh", errors.New("gave up")); err != nil {
			t.Errorf("the holder could not hand its job back: %v", err)
		}
	})
}

func TestSettlingOutlivesTheCancellationThatEndedTheWork(t *testing.T) {
	// The record of how a job ended is written after the work, which on a
	// shutdown means after the cancellation that interrupted it. Written with
	// the cancelled context it fails, and the job stays claimed by a process
	// that has gone until the claim goes stale.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	settled, done := queue.Settling(ctx)
	defer done()
	if settled.Err() != nil {
		t.Fatalf("a settling context is already over: %v", settled.Err())
	}
	if _, bounded := settled.Deadline(); !bounded {
		t.Error("a settling context has no bound, so a database that is not answering holds a shutdown for ever")
	}
}
