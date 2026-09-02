package ingest_test

import (
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
)

// What became of an upload, and which run's numbers belong to it.
//
// None of this had a test. The runs query went from "the newest finished one"
// to every finished one, a receipt gained the run that answered it, and both
// design documents assert behaviour that would regress with nothing saying so.
func TestAFailedRunDoesNotPoisonTheUploadsBeforeIt(t *testing.T) {
	// One bad night used to be permanent. The failure was asked of every run
	// that finished after an upload rather than of the run that answers it, so
	// a scanner that fell over once made every earlier receipt report that
	// failure for ever — and the screen got steadily more wrong the longer a
	// deployment ran.
	scanned(t, func(t *testing.T, s *ingest.Store, reader access.Subject, ours, _ int64) {
		ctx := t.Context()
		target := onlyTarget(t, s, ours)

		// An upload, then a run that fails, then a run that succeeds.
		file(t, s, target, "first", time.Now().UTC().Add(-3*time.Hour))
		finishRun(t, s, target, time.Now().UTC().Add(-2*time.Hour), "the scanner fell over")
		finishRun(t, s, target, time.Now().UTC().Add(-time.Hour), "")

		receipts, _, err := s.Receipts(ctx, reader, target, "", 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(receipts) == 0 {
			t.Fatal("wanted the upload back")
		}
		if receipts[0].State != ingest.Scanned {
			t.Errorf("after a later run succeeded the upload reads %q (%q), want scanned",
				receipts[0].State, receipts[0].Failure)
		}
	})
}

func TestARunIsAttributedToOneUploadHoweverThePageFalls(t *testing.T) {
	// A page is a window on one history, so which upload a run belongs to
	// cannot be decided from the rows in front of us: page two would claim it
	// again, and the same opened-and-closed numbers would render twice.
	scanned(t, func(t *testing.T, s *ingest.Store, reader access.Subject, ours, _ int64) {
		ctx := t.Context()
		target := onlyTarget(t, s, ours)

		// Three uploads, then one run covering all of them.
		for i, hash := range []string{"one", "two", "three"} {
			file(t, s, target, hash, time.Now().UTC().Add(-time.Duration(9-i)*time.Hour))
		}
		finishRun(t, s, target, time.Now().UTC(), "")

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

// onlyTarget is the build the fixture filed against.
func onlyTarget(t *testing.T, s *ingest.Store, productID int64) int64 {
	t.Helper()
	var id int64
	if err := s.DB().NewSelect().TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Column("tg.id").Where("st.product_id = ?", productID).
		Order("tg.id").Limit(1).Scan(t.Context(), &id); err != nil {
		t.Fatal(err)
	}
	return id
}

func file(t *testing.T, s *ingest.Store, target int64, hash string, at time.Time) {
	t.Helper()
	if _, _, err := s.Record(t.Context(), ingest.Arriving{
		TargetID: target, ContentHash: hash, BuiltAt: at, ParserVersion: "test",
	}); err != nil {
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
