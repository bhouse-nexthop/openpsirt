package ingest

import (
	"context"
	"fmt"
	"strconv"
	"time"

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

	// Which upload each run is attributed to, worked out over *every* scan
	// rather than over this page.
	//
	// A page is a window on the same history, so deciding "the newest upload
	// this run covered" from the rows in front of us gave a different answer
	// on page two: a run claimed on page one was claimed again by the first
	// older upload it also covered, and the same opened and closed counts
	// rendered twice. The claim has to be a property of the scan, not of the
	// page it appears on.
	claimed, err := s.claims(ctx, targetID, credential, runs)
	if err != nil {
		return nil, 0, err
	}

	receipts := make([]Receipt, 0, len(scans))
	for _, sc := range scans {
		state, failure, run := progressOf(sc, read[strconv.FormatInt(sc.ID, 10)], runs)
		receipt := Receipt{Scan: sc, State: state, Failure: failure}
		if run != nil && claimed[*run] == sc.ID {
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
	// The run that answers this upload is the earliest *successful* one to
	// finish after it was parsed. Runs arrive newest first, so the last
	// assignment in each branch below is the earliest of its kind.
	//
	// **A run that failed only answers an upload while nothing has succeeded
	// since.** A scan run covers a build rather than an upload, and this
	// upload is the newest document the build holds, so a later run that
	// finished cleanly did read this document — saying otherwise makes one bad
	// night permanent. That was the first version of this: the earliest run
	// after parsing was taken whatever became of it, so a scanner that fell
	// over once poisoned every receipt already waiting on it, for ever,
	// however many green runs came afterwards, and the scans screen got
	// steadily more wrong the longer a deployment ran.
	//
	// Still Refused while every run since has failed, which is the honest
	// answer to "did anything come of my upload" at that point.
	var answered, failed *finding.Run
	for i := range runs {
		if runs[i].FinishedAt == nil || runs[i].FinishedAt.Before(parsed) {
			continue
		}
		if runs[i].Failure != "" {
			failed = &runs[i]
			continue
		}
		answered = &runs[i]
	}
	if answered != nil {
		return Scanned, "", &answered.ID
	}
	if failed != nil {
		return Refused, failed.Failure, nil
	}
	return Scanning, "", nil
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

// claims says which upload each run's numbers belong to: the newest one it
// covered, decided over the whole history rather than over one page of it.
//
// Two columns for every scan filed here, which the target index answers
// directly. Arrival stands in for the moment a document was parsed — they are
// seconds apart, and what is being decided is only which of two uploads a run
// came after, so the distinction cannot change the answer without the two
// having arrived either side of the run finishing, in which case arrival is
// the more truthful of the two anyway.
func (s *Store) claims(ctx context.Context, targetID int64, credential string,
	runs []finding.Run) (map[int64]int64, error) {

	var filed []struct {
		ID         int64     `bun:"id"`
		ReceivedAt time.Time `bun:"received_at"`
	}
	q := s.db.NewSelect().TableExpr("scan AS sc").
		Column("sc.id", "sc.received_at").
		Where("sc.target_id = ?", targetID)
	if credential != "" {
		q = q.Where("sc.credential = ?", credential)
	}
	if err := q.Order("sc.id DESC").Scan(ctx, &filed); err != nil {
		return nil, fmt.Errorf("read what has been filed here: %w", err)
	}

	claimed := make(map[int64]int64, len(runs))
	for i := range runs {
		run := runs[i]
		if run.FinishedAt == nil {
			continue
		}
		// Scans are newest first, so the first one this run finished after is
		// the newest upload it covered.
		for _, sc := range filed {
			if !sc.ReceivedAt.After(*run.FinishedAt) {
				claimed[run.ID] = sc.ID
				break
			}
		}
	}
	return claimed, nil
}
