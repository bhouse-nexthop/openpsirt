package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// betweenSweeps is how often the conditions are re-derived.
//
// They are about a threshold measured in days, so asking every few minutes
// would be answering a question nobody asked more often than the answer can
// change. Long enough to be cheap, short enough that somebody who fixes a
// pipeline sees the alert go while they are still looking at it.
const betweenSweeps = 15 * time.Minute

// Watch derives the conditions that are true and tells the people who should
// know.
//
// Operational alerts are their own category (NTF-07): they are about the
// tool's own health rather than about anybody's work, they are not an explicit
// human action so they are never immediate mail, and a digest that is off by
// default would carry them to nobody. They go to administrators, who are the
// people who can act on them.
//
// Everything here is a condition rather than an event (NTF-09). A build that
// stopped being scanned is true until it is scanned again, and then it is not
// — which is a thing the pass discovers rather than a thing anybody dismisses.
type Watch struct {
	db     *bun.DB
	logger *slog.Logger
}

// NewWatch returns a watch over db.
func NewWatch(db *bun.DB, logger *slog.Logger) *Watch {
	return &Watch{db: db, logger: logger}
}

// Run sweeps until the context ends.
func (w *Watch) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = betweenSweeps
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if opened, cleared, err := w.Once(ctx); err != nil {
			// Logged and carried on, like every other background pass here: a
			// sweep that cannot run is not a reason to stop the process, and
			// the next one is fifteen minutes away.
			w.logger.Error("working out what is worth saying", "error", err)
		} else if opened > 0 || cleared > 0 {
			w.logger.Info("operational alerts changed",
				"opened", opened, "cleared", cleared)
		}
		if gone, err := w.Tidy(ctx); err != nil {
			w.logger.Error("clearing expired sessions", "error", err)
		} else if gone > 0 {
			w.logger.Info("cleared expired sessions", "sessions", gone)
		}
		timer.Reset(interval)
	}
}

// Once derives every condition and reconciles it against what is being said.
func (w *Watch) Once(ctx context.Context) (opened, cleared int, err error) {
	admins, err := w.administrators(ctx)
	if err != nil {
		return 0, 0, err
	}
	if len(admins) == 0 {
		// Nothing to say and nobody to say it to. Not an error: a deployment
		// with no administrator recorded cannot start, so this is the window
		// between the table existing and the first sign-in.
		return 0, 0, nil
	}

	quiet, err := w.quietBuilds(ctx)
	if err != nil {
		return 0, 0, err
	}

	// The same conditions to each of them. An alert about the tool's health is
	// not somebody's personal work item, and the first administrator to look
	// should not be the only one who ever sees it.
	for _, admin := range admins {
		o, c, err := NewStore(w.db).Reconcile(ctx, admin, BuildQuiet, quiet)
		if err != nil {
			return opened, cleared, fmt.Errorf("tell %d what has gone quiet: %w", admin, err)
		}
		opened += o
		cleared += c
	}
	return opened, cleared, nil
}

// Tidy clears what has run out: sessions past their lifetime.
//
// Sessions are the one table that grows with use rather than with what is
// tracked, and an expired one is refused on sight, so nothing but the table's
// size changes when they go. Nothing else here runs on a clock — the readers
// wait on work and the upstream pass on a setting — so the clearing rides on
// the sweep that already runs every quarter of an hour on every replica. Two
// replicas clearing at once delete the same rows or disjoint ones, and either
// way nothing is lost.
func (w *Watch) Tidy(ctx context.Context) (int64, error) {
	return access.NewStore(w.db).PurgeExpiredSessions(ctx)
}

// quietBuilds is every build nothing has been filed against for longer than
// this deployment allows.
//
// It reads the same answer the front page and the scans screen read, rather
// than asking a question of its own: two queries about when something was last
// scanned are two numbers that can disagree, and the one nobody is looking at
// would be the one that drifts.
func (w *Watch) quietBuilds(ctx context.Context) ([]Holds, error) {
	after, err := setting.NewStore(w.db).Duration(ctx, setting.QuietAfter, setting.DefaultQuietAfter)
	if err != nil {
		return nil, fmt.Errorf("read how long counts as quiet: %w", err)
	}

	// Asked as somebody who can see everything, because that is what this is:
	// the tool reporting on itself rather than answering a person. What it
	// produces is then told only to administrators.
	everything := access.NewPerson(0, "the watch", true, nil)
	rows, err := ingest.NewStore(w.db).Scanning(ctx, everything, finding.Scope{}, after)
	if err != nil {
		return nil, err
	}

	holding := make([]Holds, 0, len(rows))
	for _, row := range rows {
		if !row.Quiet {
			continue
		}
		where := row.Product + " " + row.Stream + " " + row.Variant
		days := int(row.Since.Hours() / 24)
		body := fmt.Sprintf("%s has not been scanned for %d days. Nothing has failed — "+
			"nothing has arrived.", where, days)
		if row.LastReceivedAt == nil {
			body = fmt.Sprintf("%s was declared %d days ago and nothing has ever been "+
				"filed against it.", where, days)
		}
		holding = append(holding, Holds{
			// Hashed rather than the three names joined.
			//
			// Each of them may be 191 characters and the column holds 191, so
			// the obvious key does not fit — and what happens then depends on
			// the engine: three of them refuse the write and abort the sweep,
			// one truncates and silently collides. Joining them also collides
			// on its own, because a name may contain the separator: product
			// "a/b" branch "c" and product "a" branch "b/c" are one key.
			//
			// A hash is a fixed width, so it fits by construction, and it is
			// only ever compared for equality — nothing reads it back. The
			// names people read are in the body.
			About: identify(where),
			Body:  body,
			Link: "/products/" + url.PathEscape(row.Product) +
				"/streams/" + url.PathEscape(row.Stream) +
				"/variants/" + url.PathEscape(row.Variant) + "/scans",
		})
	}
	return holding, nil
}

// administrators is who hears about the tool's own health.
func (w *Watch) administrators(ctx context.Context) ([]int64, error) {
	people, _, err := access.NewStore(w.db).People(ctx)
	if err != nil {
		return nil, fmt.Errorf("read who administers this: %w", err)
	}
	var admins []int64
	for _, person := range people {
		if person.IsAdmin {
			admins = append(admins, person.ID)
		}
	}
	return admins, nil
}

// identify is the key a condition is recognized by between sweeps.
//
// Hashed for the reason every other identity here is: it is compared for
// equality and never read, and a fixed width fits a column whatever the names
// were.
func identify(what string) string {
	sum := sha256.Sum256([]byte(what))
	return hex.EncodeToString(sum[:])
}
