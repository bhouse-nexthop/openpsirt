package ingest

import (
	"context"
	"strconv"

	"github.com/uptrace/bun"

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
}

// Receipts reports what became of the scans filed against a target, newest
// first, with how many there are in total.
//
// A credential narrows it to what that credential sent. The narrowing belongs
// here rather than in the caller: filtering a page after it has been read
// returns short pages and a total counting rows the reader was not shown,
// which is both wrong and a count of somebody else's uploads.
func (s *Store) Receipts(ctx context.Context, targetID int64, credential string, limit, offset int) ([]Receipt, int, error) {
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
	// run finished after this upload was parsed.
	var runs []finding.Run
	if err := s.db.NewSelect().Model(&runs).
		Where("target_id = ?", targetID).
		Where("finished_at IS NOT NULL").
		Order("id DESC").Limit(1).Scan(ctx); err != nil {
		return nil, 0, err
	}

	receipts := make([]Receipt, 0, len(scans))
	for _, sc := range scans {
		state, failure := progressOf(sc, read[strconv.FormatInt(sc.ID, 10)], runs)
		receipts = append(receipts, Receipt{Scan: sc, State: state, Failure: failure})
	}
	return receipts, total, nil
}

// progressOf folds a scan, the job that read it and the target's latest
// finished run into one answer.
func progressOf(sc Scan, job queue.Job, runs []finding.Run) (Progress, string) {
	if sc.Status == Failed {
		return Refused, sc.Failure
	}
	switch job.State {
	case queue.Dead:
		// The scan row is marked too, but a job that died before it could mark
		// anything would otherwise read as still being worked on.
		if job.LastError != nil {
			return Refused, *job.LastError
		}
		return Refused, "the upload could not be read"
	case queue.Pending, queue.Running:
		return Reading, ""
	}
	if job.State != queue.Done {
		// No job row at all. An upload that matched one already held is
		// answered by the scan it matched, which has its own job.
		return Reading, ""
	}

	parsed := job.UpdatedAt
	if parsed.IsZero() {
		parsed = sc.ReceivedAt
	}
	for _, run := range runs {
		if run.FinishedAt == nil || run.FinishedAt.Before(parsed) {
			continue
		}
		if run.Failure != "" {
			return Refused, run.Failure
		}
		return Scanned, ""
	}
	return Scanning, ""
}
