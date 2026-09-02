package ingest

import (
	"context"
	"fmt"
	"strconv"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"

	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
)

// Progress is how far an upload has got, in the terms the sender asked in.
//
// A producer that was told its upload was accepted wants to know two things:
// whether the file parsed, and whether anything came of it. Which of our two
// queues the work is sitting in is not a producer's business and would tie
// their build scripts to our internals, so the queues are collapsed into one
// sequence here.
type Progress string

const (
	// Reading means taken, not yet parsed.
	Reading Progress = "reading"
	// Scanning means parsed and stored, with the vulnerability scan still to
	// finish.
	Scanning Progress = "scanning"
	// Scanned means done — the graph is stored and the findings are recorded.
	Scanned Progress = "scanned"
	// Refused means it was taken and could not be used. Failure says why, and
	// is the whole reason this endpoint exists: a producer emitting files
	// nothing can read would otherwise see a successful upload every night.
	Refused Progress = "failed"
)

// Receipt is what became of one upload.
type Receipt struct {
	Scan    Scan
	State   Progress
	Failure string
	// RunID is the scan run that reflects this upload, where one has finished.
	//
	// A run covers a target rather than an upload, so several uploads can be
	// answered by one run. It is attributed to the newest upload it covered
	// and left off the rest: what a run opened and closed is one fact, and
	// repeating it down three rows reads as three separate changes.
	RunID *int64
}

// Receipts reports what became of the scans filed against a target, newest
// first, with how many there are in total.
//
// A credential narrows it to what that credential sent. The narrowing belongs
// here rather than in the caller: filtering a page after it has been read
// returns short pages and a total counting rows the reader was not shown,
// which is both wrong and a count of somebody else's uploads.
func (s *Store) Receipts(ctx context.Context, subject access.Subject, targetID int64, credential string, limit, offset int) ([]Receipt, int, error) {
	// Asked here rather than only in the handler. A check beside the query
	// cannot be skipped by adding another endpoint, which is the whole reason
	// visibility is decided in this layer — and receipts carry a producer's
	// own failure text, so reaching them is reaching something.
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, 0, err
	}
	if !subject.Sees(productID) {
		return nil, 0, fmt.Errorf("no build is declared there")
	}

	sent := func(q *bun.SelectQuery) *bun.SelectQuery {
		q = q.Where("target_id = ?", targetID)
		if credential != "" {
			q = q.Where("credential = ?", credential)
		}
		return q
	}

	total, err := sent(s.db.NewSelect().Model((*Scan)(nil))).Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var scans []Scan
	if err := sent(s.db.NewSelect().Model(&scans)).
		Order("id DESC").Limit(limit).Offset(offset).Scan(ctx); err != nil {
		return nil, 0, err
	}
	if len(scans) == 0 {
		return nil, total, nil
	}

	// The read job carries the outcome of parsing, and it is kept after it
	// finishes rather than deleted, so it stays answerable for as long as the
	// scan it belongs to.
	references := make([]string, 0, len(scans))
	for _, sc := range scans {
		references = append(references, strconv.FormatInt(sc.ID, 10))
	}
	var jobs []queue.Job
	if err := s.db.NewSelect().Model(&jobs).
		Where("kind = ?", JobKind).
		Where("reference IN (?)", bun.List(references)).Scan(ctx); err != nil {
		return nil, 0, err
	}
	read := make(map[string]queue.Job, len(jobs))
	for _, j := range jobs {
		read[j.Reference] = j
	}

	// One run covers a target rather than a scan, so what matters is whether a
	// run finished after this upload was parsed. Every finished run rather
	// than only the newest, because a page of receipts spans several and each
	// upload is answered by the run that came after it rather than by the last
	// one to happen.
	var runs []finding.Run
	if err := s.db.NewSelect().Model(&runs).
		Where("target_id = ?", targetID).
		Where("finished_at IS NOT NULL").
		Order("id DESC").Scan(ctx); err != nil {
		return nil, 0, err
	}

	receipts := make([]Receipt, 0, len(scans))
	claimed := make(map[int64]bool, len(runs))
	for _, sc := range scans {
		state, failure, run := progressOf(sc, read[strconv.FormatInt(sc.ID, 10)], runs)
		receipt := Receipt{Scan: sc, State: state, Failure: failure}
		// Scans arrive newest first, so the first upload a run answers is the
		// newest one it covered.
		if run != nil && !claimed[*run] {
			claimed[*run] = true
			receipt.RunID = run
		}
		receipts = append(receipts, receipt)
	}
	return receipts, total, nil
}

// progressOf folds a scan, the job that read it and the target's latest
// finished run into one answer.
func progressOf(sc Scan, job queue.Job, runs []finding.Run) (Progress, string, *int64) {
	if sc.Status == Failed {
		return Refused, sc.Failure, nil
	}
	switch job.State {
	case queue.Dead:
		// The scan row is marked too, but a job that died before it could mark
		// anything would otherwise read as still being worked on.
		if job.LastError != nil {
			return Refused, *job.LastError, nil
		}
		return Refused, "the upload could not be read", nil
	case queue.Pending, queue.Running:
		return Reading, "", nil
	}
	if job.State != queue.Done {
		// No job row at all. An upload that matched one already held is
		// answered by the scan it matched, which has its own job.
		return Reading, "", nil
	}

	parsed := job.UpdatedAt
	if parsed.IsZero() {
		parsed = sc.ReceivedAt
	}
	// Runs arrive newest first, so the last one still finishing after this
	// upload was parsed is the earliest that reflects it — and that one is the
	// run that answers this upload.
	//
	// **Whether it failed is asked of that run alone.** Asking it of every run
	// in the loop made one bad night permanent: a scanner that fell over once
	// finished after every upload parsed before it, so every one of those
	// receipts reported that failure for ever, however many green runs came
	// afterwards. The scans screen got steadily more wrong the longer a
	// deployment ran.
	var answered *finding.Run
	for i := range runs {
		if runs[i].FinishedAt == nil || runs[i].FinishedAt.Before(parsed) {
			continue
		}
		answered = &runs[i]
	}
	if answered == nil {
		return Scanning, "", nil
	}
	if answered.Failure != "" {
		return Refused, answered.Failure, nil
	}
	return Scanned, "", &answered.ID
}

// productOf reads which product a build belongs to, so that reaching it can be
// decided from the row rather than from what a caller said about it.
func productOf(ctx context.Context, db bun.IDB, targetID int64) (int64, error) {
	var productID int64
	err := db.NewSelect().
		TableExpr("target AS t").
		Join("JOIN stream AS st ON st.id = t.stream_id").
		Column("st.product_id").
		Where("t.id = ?", targetID).
		Scan(ctx, &productID)
	if err != nil {
		return 0, fmt.Errorf("no build is declared there")
	}
	return productID, nil
}
