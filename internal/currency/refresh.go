package currency

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/setting"
)

// StaleAfter is how long an answer stands before it is asked again.
//
// A day. The question is whether upstream has released since a flaw was
// disclosed, which is a thing that changes on the order of days, and asking
// every hour would put a deployment's whole component list through four public
// indexes for an answer that had not moved.
const StaleAfter = 24 * time.Hour

// mostPerPass bounds one cycle.
//
// A first run against a real image has thousands of components in these
// ecosystems and no answer for any of them. Asking all of them at once is a
// burst at somebody else's index that looks exactly like abuse, so a pass takes
// a slice and the next pass takes the next — a backlog drains over hours, and
// nothing here needs it sooner.
const mostPerPass = 200

// betweenAsks is the pause between one index request and the next.
//
// Deliberate politeness rather than a rate limit we were given: these are free
// public services, and a tool that walks them as fast as it can is the reason
// they end up needing one.
const betweenAsks = 250 * time.Millisecond

// Refresher fills in what upstream has released, for components we build
// ourselves.
type Refresher struct {
	db bun.IDB
	// Index returns the asker for an ecosystem, or nil where there is none.
	// A field rather than a call so a test can answer without a network, which
	// is the only way to test this at all: the real ones are somebody else's
	// service and their answers change.
	Index  func(ecosystem string) Asker
	logger *slog.Logger
	// Pause is how long to wait between one request and the next. A field so a
	// test does not spend a real second being polite to a fake.
	Pause time.Duration
	now   func() time.Time
}

// NewRefresher returns a refresher over db, asking the real public indexes.
func NewRefresher(db bun.IDB, logger *slog.Logger) *Refresher {
	client := New()
	return &Refresher{
		db: db, Index: client.For, logger: logger,
		Pause: betweenAsks, now: time.Now,
	}
}

// Run asks, forever, until the context ends.
//
// The setting is read each cycle rather than at startup, so turning this on
// takes effect without a restart — and, more to the point, so does turning it
// off. This is the one thing here that reaches the network (ING-41), and an
// operator who decides that was a mistake should not have to redeploy to stop
// it.
func (r *Refresher) Run(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		on, err := r.enabled(ctx)
		if err != nil {
			r.logger.Error("reading whether to ask upstream", "error", err)
		} else if on {
			if asked, err := r.Once(ctx); err != nil {
				// Logged and carried on. An index having a bad day is not a
				// reason to stop asking about everything else, and this
				// answer is an extra rather than something the rest depends
				// on.
				r.logger.Error("asking what upstream has released", "error", err)
			} else if asked > 0 {
				r.logger.Info("asked upstream what it has released", "components", asked)
			}
		}
		timer.Reset(interval)
	}
}

// enabled reports whether this deployment has turned asking on.
func (r *Refresher) enabled(ctx context.Context) (bool, error) {
	value, set, err := setting.NewStore(r.db).Get(ctx, setting.UpstreamCurrency)
	if err != nil {
		return false, err
	}
	return set && value == setting.On, nil
}

// stale is one component due a question.
type stale struct {
	ID   int64  `bun:"id"`
	Purl string `bun:"purl"`
}

// Once asks about one slice of components and records what came back.
//
// Returns how many were asked about, which is how the caller knows whether a
// backlog is still draining.
func (r *Refresher) Once(ctx context.Context) (int, error) {
	due, err := r.due(ctx)
	if err != nil {
		return 0, err
	}
	asked := 0
	for _, component := range due {
		select {
		case <-ctx.Done():
			return asked, nil
		default:
		}

		ecosystem, name, ok := Asked(component.Purl)
		if !ok {
			continue
		}
		index := r.Index(ecosystem)
		if index == nil {
			continue
		}
		latest, err := index.Latest(ctx, name)
		switch {
		case err == nil:
		case errors.Is(err, ErrUnknown):
			// The index has never heard of it, which a private module and a
			// vendored fork both look like. Recorded as asked so it is not
			// asked again tomorrow and every day after: an answer we will
			// never get is still an answer about this component.
			latest = Latest{}
		case ctx.Err() != nil:
			return asked, nil
		default:
			// One index failing is not the pass failing. Nothing is written,
			// so it stays due and the next pass tries again.
			r.logger.Warn("an index did not answer",
				"ecosystem", ecosystem, "package", name, "error", err)
			time.Sleep(r.Pause)
			continue
		}

		if err := r.record(ctx, component.ID, latest); err != nil {
			return asked, err
		}
		asked++
		time.Sleep(r.Pause)
	}
	return asked, nil
}

// due reads the components whose answer is missing or old.
//
// Ordered oldest first, with never-asked before everything, so a first run
// works through the list rather than circling the same slice of it.
func (r *Refresher) due(ctx context.Context) ([]stale, error) {
	var rows []stale
	err := r.db.NewSelect().
		TableExpr("component AS c").
		ColumnExpr("c.id AS id").
		ColumnExpr("c.purl AS purl").
		Where("c.purl <> ''").
		// Only what we build ourselves. For a distribution package the
		// distribution is the maintainer, and the date it released says
		// nothing about the age of the software inside it — Debian shipping a
		// security update today does not mean upstream is moving.
		Where("c.purl NOT LIKE 'pkg:deb/%'").
		WhereGroup(" AND ", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.WhereOr("c.latest_checked_at IS NULL").
				WhereOr("c.latest_checked_at < ?", r.now().Add(-StaleAfter).UTC())
		}).
		OrderExpr("c.latest_checked_at IS NULL DESC").
		OrderExpr("c.latest_checked_at ASC").
		Limit(mostPerPass).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read what has not been asked about lately: %w", err)
	}
	return rows, nil
}

// record writes what an index said.
//
// The time of asking is written whatever came back, because "we asked and
// there is nothing" and "we have not asked" are different states and only one
// of them should be retried tomorrow. A version we did not get is stored as
// nothing rather than as an empty string standing in for an answer, and a date
// we did not get is left null — the beginning of time is not a release date.
func (r *Refresher) record(ctx context.Context, id int64, latest Latest) error {
	q := r.db.NewUpdate().
		Table("component").
		Set("latest_checked_at = ?", r.now().UTC()).
		Set("latest_version = ?", nullable(latest.Version)).
		Where("id = ?", id)
	if latest.Released.IsZero() {
		q = q.Set("latest_released_at = NULL")
	} else {
		q = q.Set("latest_released_at = ?", latest.Released.UTC())
	}
	if _, err := q.Exec(ctx); err != nil {
		return fmt.Errorf("record what upstream has released: %w", err)
	}
	return nil
}

// nullable keeps an absent answer out of the column as null rather than as an
// empty string, so a reader does not have to know which of the two this build
// happened to write.
func nullable(text string) any {
	if text == "" {
		return nil
	}
	return text
}
