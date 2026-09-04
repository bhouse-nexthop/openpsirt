package notify

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/uptrace/bun"
)

// betweenPosts is how often what is unsent is carried out of the application.
//
// A minute, because the categories that leave here at all are the ones NTF-02
// calls worth interrupting somebody for, and an hour's delay on "this is now
// yours" makes the message a record of something rather than a prompt.
const betweenPosts = time.Minute

// tries is how many times one message is attempted before it is left alone.
//
// A mailbox that refuses five times has gone, and a sweep that keeps trying it
// is a sweep that eventually does nothing else. The row stays unsent and
// stays readable, which is the honest end state: the notification area still
// has it, and nobody is told a message arrived that did not.
const tries = 5

// Post carries what is unsent to whatever channel a deployment configured.
//
// A sweep rather than a queue of its own. What is unsent is the work list, so
// a message that failed needs no state to say it should be tried again — it is
// simply still unsent — and a deployment that configures mail after running
// for a week finds the last week's messages waiting rather than lost.
type Post struct {
	db      *bun.DB
	channel Channel
	baseURL string
	logger  *slog.Logger
	now     func() time.Time
}

// NewPost returns a sweep over db, or nil where no channel is configured.
//
// Nil because a deployment without mail is ordinary: the notification area is
// the channel that always exists, and it needs nothing set up (NTF-08).
func NewPost(db *bun.DB, channel Channel, baseURL string, logger *slog.Logger) *Post {
	if channel == nil {
		return nil
	}
	return &Post{
		db: db, channel: channel, baseURL: baseURL, logger: logger,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Run sweeps until the context ends.
func (p *Post) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = betweenPosts
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if sent, failed, err := p.Once(ctx); err != nil {
			// Logged and carried on, like every other background pass here.
			p.logger.Error("carrying notifications out of the application", "error", err)
		} else if sent > 0 || failed > 0 {
			p.logger.Info("notifications sent",
				"channel", p.channel.Name(), "sent", sent, "failed", failed)
		}
		timer.Reset(interval)
	}
}

// waiting is one notification to carry, with where it goes.
type waiting struct {
	Notification `bun:",extend"`
	Email        string `bun:"email"`
}

// Once carries everything that is waiting.
//
// Ordered oldest first, so a backlog drains in the order things happened
// rather than in the order a query felt like.
func (p *Post) Once(ctx context.Context) (sent, failed int, err error) {
	var rows []waiting
	err = p.db.NewSelect().
		Model((*Notification)(nil)).
		ColumnExpr("nt.*").
		ColumnExpr("pe.email AS email").
		Join(`JOIN "person" AS pe ON pe.id = nt.person_id`).
		Where("nt.sent_at IS NULL").
		Where("nt.attempts < ?", tries).
		// Somebody with no address is told nothing outside the application
		// and keeps the area inside it (ACC-60). Excluded in the query rather
		// than skipped after reading, so a deployment where nobody has an
		// address does no work at all.
		Where("pe.email IS NOT NULL AND pe.email <> ?", "").
		OrderExpr("nt.created_at ASC, nt.id ASC").
		Limit(200).
		Scan(ctx, &rows)
	if err != nil {
		return 0, 0, fmt.Errorf("read what is waiting to be sent: %w", err)
	}

	for _, row := range rows {
		message := Compose(row.Notification, p.baseURL)
		sendErr := p.channel.Send(ctx, row.Email, message)
		if sendErr != nil {
			failed++
			// Counted rather than recorded as a failure: the row is still
			// unsent, which is already the whole of "try this again".
			if _, err := p.db.NewUpdate().Model((*Notification)(nil)).
				Set("attempts = attempts + 1").
				Where("id = ?", row.ID).Exec(ctx); err != nil {
				return sent, failed, fmt.Errorf("record a failed send: %w", err)
			}
			p.logger.Warn("could not carry a notification out of the application",
				"channel", p.channel.Name(), "notification", row.ID, "error", sendErr)
			continue
		}
		now := p.now()
		if _, err := p.db.NewUpdate().Model((*Notification)(nil)).
			Set("sent_at = ?", now).
			Set("attempts = attempts + 1").
			Where("id = ?", row.ID).Exec(ctx); err != nil {
			// Sent and not recorded as sent, which the next sweep will send
			// again. Reported rather than swallowed: a duplicate message is a
			// small harm and an unexplained one is a confusing one.
			return sent, failed, fmt.Errorf("record that a notification was sent: %w", err)
		}
		sent++
	}
	return sent, failed, nil
}
