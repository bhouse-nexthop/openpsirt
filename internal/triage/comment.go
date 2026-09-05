package triage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/attach"
	"github.com/bhouse-nexthop/openpsirt/internal/markdown"
)

// Comment is discussion on a decision.
//
// Not the reasoning. The obvious mistake is treating all text on a finding as
// one thing, and the two behave differently on purpose: revising the reasoning
// takes back the approval standing on it, and a comment never disturbs
// anything. Annotating an approved decision months later — "re-checked, still
// true" — is ordinary, and an approval that fell over each time somebody added
// a note would teach people not to add notes.
type Comment struct {
	bun.BaseModel `bun:"table:decision_comment,alias:dc"`

	ID         int64     `bun:"id,pk,autoincrement"`
	DecisionID int64     `bun:"decision_id,notnull"`
	Body       string    `bun:"body,notnull"`
	WrittenBy  int64     `bun:"written_by,notnull"`
	WrittenAt  time.Time `bun:"written_at,notnull"`
	// EditedAt marks that the author changed it. What it said before is not
	// kept: discussion is not the record a decision rests on, and that record
	// — the revisions and the approvals — is kept in full.
	EditedAt *time.Time `bun:"edited_at"`
}

// Say adds a comment to a decision.
//
// Allowed at any point, including long after an approval, and it disturbs
// nothing.
func (s *Store) Say(ctx context.Context, subject access.Subject, decisionID int64, body string) (*Comment, error) {
	if _, err := s.reaching(ctx, subject, decisionID, readable); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("a comment has to say something")
	}
	if err := markdown.Check(body); err != nil {
		return nil, err
	}

	comment := &Comment{
		DecisionID: decisionID, Body: body,
		WrittenBy: subject.ID, WrittenAt: s.now().Truncate(time.Microsecond),
	}
	if _, err := s.db.NewInsert().Model(comment).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record a comment: %w", err)
	}
	if err := noting(ctx, s.db, body, comment.WrittenAt); err != nil {
		return nil, err
	}
	return comment, nil
}

// Reword changes a comment, which only its author may do.
//
// Overwritten rather than revised, and marked as edited. Nobody else may
// change somebody's words — an edit that could be made by another person is
// not a correction, it is a forgery with a timestamp.
//
// **Whether the asker may reach the decision is settled before anything about
// the comment is said back** (ACC-56). The row is read first, because the
// decision it hangs off is not knowable otherwise, but no answer turns on what
// was in it until the asker has been let in: refusing on authorship first told
// anybody holding triage anywhere that a comment with this identifier exists,
// one request at a time, including on findings nobody has disclosed. A comment
// that is not there and one on a decision this person may not reach answer
// identically.
// It answers with the decision the comment hangs off, because whoever edited
// it may have named somebody the first version did not, and telling them needs
// to know what the text is about.
func (s *Store) Reword(ctx context.Context, subject access.Subject, commentID int64,
	body string) (int64, error) {
	comment := new(Comment)
	if err := s.db.NewSelect().Model(comment).
		Where("id = ?", commentID).Scan(ctx); err != nil {
		// The bare sentinel, not a wrapped one. The identifier in the message
		// is the difference a caller counts: "change comment 10000: not
		// authorized" against "not authorized" separates the two answers this
		// is written to make identical.
		return 0, ErrNotTheirs
	}
	if _, err := s.reaching(ctx, subject, comment.DecisionID, readable); err != nil {
		return 0, err
	}
	if comment.WrittenBy != subject.ID {
		return 0, fmt.Errorf("only the person who wrote a comment may change it")
	}
	if strings.TrimSpace(body) == "" {
		return 0, fmt.Errorf("a comment has to say something")
	}
	if err := markdown.Check(body); err != nil {
		return 0, err
	}

	edited := s.now().Truncate(time.Microsecond)
	if _, err := s.db.NewUpdate().Model((*Comment)(nil)).
		Set("body = ?", body).
		Set("edited_at = ?", edited).
		Where("id = ?", commentID).Exec(ctx); err != nil {
		return 0, fmt.Errorf("change a comment: %w", err)
	}
	// An edit can add a reference the first version did not have. It can also
	// take one away, and that does not un-attach the file: the revision that
	// referred to it is still on record.
	return comment.DecisionID, noting(ctx, s.db, body, edited)
}

// Discussion returns what has been said about a decision, oldest first.
func (s *Store) Discussion(ctx context.Context, subject access.Subject, decisionID int64) ([]Comment, error) {
	if _, err := s.reaching(ctx, subject, decisionID, readable); err != nil {
		return nil, err
	}
	var comments []Comment
	if err := s.db.NewSelect().Model(&comments).
		Where("decision_id = ?", decisionID).
		Order("id ASC").Scan(ctx); err != nil {
		return nil, fmt.Errorf("read the discussion: %w", err)
	}
	return comments, nil
}

// noting records that saved text refers to attachments (ATT-11).
//
// Called wherever text is stored, and only after it is stored: an upload
// becomes attached when something points at it, and something points at it
// once the words containing the reference are on record. Until then it is an
// upload somebody may yet abandon, which is what the sweep collects.
//
// **Silent about references it cannot match.** The text has already been
// accepted, and a reference to nothing is a broken link in a document rather
// than a reason to refuse somebody's justification after the fact.
func noting(ctx context.Context, db bun.IDB, body string, now time.Time) error {
	return attach.Attached(ctx, db, markdown.References(body), now)
}
