package queue

import "time"

// SetClock replaces the queue's idea of now, for tests that decide when a
// claim has gone stale rather than waiting for it to.
func SetClock(q *Queue, now func() time.Time) {
	q.now = now
}
