package ingest_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
)

// What became of an upload, and which run's numbers belong to it.
//
// None of this had a test. The runs query went from "the newest finished one"
// to every finished one, a receipt gained the run that answered it, and both
// design documents assert behaviour that would regress with nothing saying so.
func TestAFailedRunDoesNotPoisonTheUploadsBeforeIt(t *testing.T) {
	// One bad night used to be permanent. The earliest run to finish after an
	// upload was taken as the one that answered it whatever became of that
	// run, so a scanner that fell over once made every receipt already waiting
	// on it report that failure for ever — and the screen got steadily more
	// wrong the longer a deployment ran.
	scanned(t, func(t *testing.T, s *ingest.Store, reader access.Subject, ours, _ int64) {
		ctx := t.Context()
		target := quietTarget(t, s, ours)
		now := time.Now().UTC()

		// An upload, then a run that fails, then a run that succeeds.
		file(t, s, target, "first", now.Add(-3*time.Hour))
		finishRun(t, s, target, now.Add(-2*time.Hour), "the scanner fell over")
		finishRun(t, s, target, now.Add(-time.Hour), "")

		receipts, _, err := s.Receipts(ctx, reader, target, "", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(receipts) == 0 {
			t.Fatal("wanted the upload back")
		}
		if receipts[0].Scan.ContentHash != "first" {
			t.Fatalf("newest receipt is %q, want the upload just filed",
				receipts[0].Scan.ContentHash)
		}
		if receipts[0].State != ingest.Scanned {
			t.Errorf("after a later run succeeded the upload reads %q (%q), want scanned",
				receipts[0].State, receipts[0].Failure)
		}
	})
}

func TestAnUploadReadsAsFailedWhileEveryRunSinceHasFailed(t *testing.T) {
	// The other half of the rule above: tolerating a failure that a later run
	// made irrelevant must not turn into never reporting one at all.
	scanned(t, func(t *testing.T, s *ingest.Store, reader access.Subject, ours, _ int64) {
		ctx := t.Context()
		target := quietTarget(t, s, ours)
		now := time.Now().UTC()

		file(t, s, target, "only", now.Add(-3*time.Hour))
		finishRun(t, s, target, now.Add(-2*time.Hour), "the scanner fell over")

		receipts, _, err := s.Receipts(ctx, reader, target, "", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if receipts[0].State != ingest.Refused {
			t.Errorf("the upload reads %q, want the failure reported", receipts[0].State)
		}
		if receipts[0].Failure != "the scanner fell over" {
			t.Errorf("the failure reads %q, want the scanner's own words", receipts[0].Failure)
		}
	})
}

func TestARunIsAttributedToOneUploadHoweverThePageFalls(t *testing.T) {
	// A page is a window on one history, so which upload a run belongs to
	// cannot be decided from the rows in front of us: page two would claim it
	// again, and the same opened-and-closed numbers would render twice.
	scanned(t, func(t *testing.T, s *ingest.Store, reader access.Subject, ours, _ int64) {
		ctx := t.Context()
		target := quietTarget(t, s, ours)
		now := time.Now().UTC()

		// Three uploads, then one run covering all of them.
		for i, hash := range []string{"one", "two", "three"} {
			file(t, s, target, hash, now.Add(-time.Duration(9-i)*time.Hour))
		}
		finishRun(t, s, target, now.Add(-time.Hour), "")

		claimed := 0
		for offset := range 3 {
			page, _, err := s.Receipts(ctx, reader, target, "", 1, offset)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) != 1 {
				t.Fatalf("page at %d returned %d rows", offset, len(page))
			}
			if page[0].RunID != nil {
				claimed++
			}
		}
		if claimed != 1 {
			t.Errorf("one run was attributed to %d uploads across three pages of one, want 1",
				claimed)
		}
	})
}

// quietTarget is the fixture's build whose only scan is a month old, so an
// upload filed now is newer than what it holds.
//
// The build the fixture scanned an hour ago cannot be used: a scan older than
// the one a variant already holds is refused, which is the monotonicity rule
// doing its job rather than something to work around.
func quietTarget(t *testing.T, s *ingest.Store, productID int64) int64 {
	t.Helper()
	var id int64
	if err := s.DB().NewSelect().TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS v ON v.id = tg.variant_id").
		Column("tg.id").
		Where("st.product_id = ?", productID).
		Where("v.name = ?", "mellanox").
		Scan(t.Context(), &id); err != nil {
		t.Fatal(err)
	}
	return id
}

// file records an upload that has been read, arriving and parsed at the given
// moment.
//
// The read job is marked done because that is what says an upload was parsed:
// an upload still sitting in the queue reads as "reading" whatever the runs
// around it did, so leaving it pending would make every assertion below pass
// for the wrong reason.
func file(t *testing.T, s *ingest.Store, target int64, hash string, at time.Time) {
	t.Helper()
	ctx := t.Context()
	scan, _, err := s.Record(ctx, ingest.Arriving{
		TargetID: target, ContentHash: hash, BuiltAt: at, ParserVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().NewUpdate().Model((*ingest.Scan)(nil)).
		Set("received_at = ?", at).Where("id = ?", scan.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	// The upload endpoint enqueues the read in the same transaction as the
	// scan; recording one directly does not, so the job is written here.
	job := map[string]any{
		"kind": ingest.JobKind, "reference": strconv.FormatInt(scan.ID, 10),
		"state": queue.Done, "attempts": 1, "max_attempts": 5,
		"run_after": at, "created_at": at, "updated_at": at,
	}
	if _, err := s.DB().NewInsert().Model(&job).TableExpr("job").
		Exec(ctx); err != nil {
		t.Fatal(err)
	}
}

// finishRun records a scan run that has already ended, failing where a cause
// is given.
func finishRun(t *testing.T, s *ingest.Store, target int64, at time.Time, failure string) {
	t.Helper()
	row := map[string]any{
		"target_id": target, "scanner": "test", "ran_here": true,
		"started_at": at.Add(-time.Minute), "finished_at": at, "failure": failure,
	}
	if _, err := s.DB().NewInsert().Model(&row).TableExpr("scan_run").
		Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
}
