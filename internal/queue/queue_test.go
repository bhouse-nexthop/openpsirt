package queue_test

import (
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
					if err := q.Succeed(ctx, job.ID); err != nil {
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
		if err := q.Fail(ctx, job.ID, errors.New("upstream unavailable")); err != nil {
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
			if err := q.Fail(ctx, job.ID, errors.New("still broken")); err != nil {
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
		if err := q.Succeed(ctx, job.ID); err != nil {
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
			if err := q.Succeed(ctx, job.ID); err != nil {
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
