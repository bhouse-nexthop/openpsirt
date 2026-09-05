package attach

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Store reads and writes attachments, and owns the bytes as well as the rows.
//
// Both together, deliberately: an attachment is a row that describes a file,
// and a caller able to write one without the other could leave a reference to
// bytes that never arrived, or bytes nothing will ever reach.
type Store struct {
	db    *bun.DB
	files Storage
	now   func() time.Time
}

// NewStore returns a store over db, keeping bytes in files.
//
// A nil Storage is the ordinary case for a deployment that configured none:
// attachments are off and everything else works (ATT-04).
func NewStore(db *bun.DB, files Storage) *Store {
	return &Store{db: db, files: files, now: func() time.Time { return time.Now().UTC() }}
}

// Configured reports whether this deployment can hold files at all.
func (s *Store) Configured() bool { return s.files != nil }

// ErrNotConfigured is what every path answers where no store is configured.
var ErrNotConfigured = fmt.Errorf("this deployment holds no attachments")

// ErrTooLarge, ErrNoRoom and ErrGone are the three refusals a caller has to
// tell apart: one file is too big, the deployment is full, and the bytes were
// taken back out on purpose.
var (
	ErrTooLarge = fmt.Errorf("that file is larger than this deployment accepts")
	ErrNoRoom   = fmt.Errorf("this deployment has no room for more attachments")
	ErrGone     = fmt.Errorf("that attachment was removed")
)

// visibilityOf is how disclosed an issue is in one product.
//
// **The most careful row governs.** An issue with one undisclosed finding
// against it is undisclosed here, whatever else is open beside it — the same
// rule that decides what may be said about a group outside the application
// (NTF-15). Asked at the moment of the request rather than copied onto the
// attachment, so an embargo ending carries the file with the words.
func visibilityOf(ctx context.Context, db bun.IDB, productID, vulnerabilityID int64) (access.Visibility, error) {
	var private int
	err := db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("COUNT(*)").
		Where("st.product_id = ?", productID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.visibility = ?", access.Private).
		Scan(ctx, &private)
	if err != nil {
		return access.Private, fmt.Errorf("read how disclosed an issue is: %w", err)
	}
	if private > 0 {
		return access.Private, nil
	}
	return access.Public, nil
}

// mayReach reports whether a subject may read this issue's text, which is
// exactly the question of whether they may read what the text refers to.
func mayReach(ctx context.Context, db bun.IDB, subject access.Subject,
	productID, vulnerabilityID int64) error {

	if subject.Kind != access.Person {
		return access.Denied("reach an attachment without being a person")
	}
	visibility, err := visibilityOf(ctx, db, productID, vulnerabilityID)
	if err != nil {
		return err
	}
	if !subject.Reads(visibility, productID) {
		// The same answer as an issue that is not there. Telling somebody that
		// a file exists but is not theirs is telling them the issue exists
		// (ACC-08).
		return access.Denied(fmt.Sprintf("reach attachments in product %d", productID))
	}
	return nil
}

// Upload stores a file against an issue and records it.
//
// The bytes are streamed rather than held (ATT-09 bounds one file; holding
// each would mean every upload happening at once is resident at once), and
// hashed on the way through so that a redaction can say later what it removed.
//
// **`hangsOffTheIssue` says nothing is going to point at this from text.** A
// file attached while somebody is composing a justification is pointed at by
// words that are not saved yet, so it waits, and the sweep collects it if they
// abandon the form (ATT-11). A file attached to the issue itself — evidence, a
// test case that proves the flaw — is pointed at by the issue the moment it
// arrives, and waiting for text that will never be written would mean the
// sweep took it a day later.
func (s *Store) Upload(ctx context.Context, subject access.Subject,
	productID, vulnerabilityID int64, filename string, body io.Reader, size int64,
	maxSize, quota int64, hangsOffTheIssue bool) (*Attachment, error) {

	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	if err := mayReach(ctx, s.db, subject, productID, vulnerabilityID); err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, fmt.Errorf("an attachment has to have something in it")
	}
	if maxSize > 0 && size > maxSize {
		return nil, ErrTooLarge
	}
	// Asked before anything is carried, so an upload that cannot be kept is
	// refused rather than transferred and then thrown away.
	if err := roomIn(ctx, s.db, size, quota); err != nil {
		return nil, err
	}

	token, err := mintToken()
	if err != nil {
		return nil, err
	}
	key := keyFor(token)

	// The type is decided from the bytes and never from what the uploader
	// called it (ATT-07), which means reading the head before the rest goes to
	// the store — and then putting it back in front, so the store still
	// receives the whole file.
	head := make([]byte, 512)
	read, err := io.ReadFull(body, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("read what was uploaded: %w", err)
	}
	head = head[:read]
	contentType := TypeOf(head)

	digest := sha256.New()
	whole := io.MultiReader(strings.NewReader(string(head)), body)
	counted := &counting{}
	if err := s.files.Put(ctx, key, io.TeeReader(io.TeeReader(whole, digest), counted),
		size, contentType); err != nil {
		return nil, err
	}
	if counted.n != size {
		_ = s.files.Delete(ctx, key)
		return nil, fmt.Errorf("%d bytes arrived of the %d declared", counted.n, size)
	}

	now := s.now().Truncate(time.Microsecond)
	row := &Attachment{
		Token: token, ProductID: productID, VulnerabilityID: vulnerabilityID,
		Filename: SafeName(filename), ContentType: contentType, SizeBytes: size,
		Digest: hex.EncodeToString(digest.Sum(nil)), ObjectKey: key,
		UploadedBy: subject.ID, UploadedAt: now,
	}
	if hangsOffTheIssue {
		row.AttachedAt = &now
	}
	err = database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// Asked again inside the transaction, because the first answer was
		// read before the bytes were carried and the deployment may have
		// filled up while they were (DAT-31).
		if err := roomIn(ctx, tx, size, quota); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(row).Exec(ctx)
		return err
	})
	if err != nil {
		// The row is what the sweep can see, so a failure here leaves bytes
		// nothing knows about. Removed now rather than left for a reaper that
		// has no record to work from.
		_ = s.files.Delete(ctx, key)
		return nil, fmt.Errorf("record an attachment: %w", err)
	}
	return row, nil
}

// counting counts what passed through it, so that a declared size that does
// not match what arrived is caught rather than trusted.
type counting struct{ n int64 }

func (c *counting) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// roomIn refuses an upload the deployment has no space for (ATT-09).
//
// Takes the handle rather than reading the store's own, so that the question
// can be asked again inside the transaction that writes.
func roomIn(ctx context.Context, db bun.IDB, size, quota int64) error {
	if quota <= 0 {
		return nil
	}
	var held int64
	if err := db.NewSelect().
		TableExpr("attachment AS at").
		ColumnExpr("COALESCE(SUM(at.size_bytes), 0)").
		Where("at.redacted_at IS NULL").
		Scan(ctx, &held); err != nil {
		return fmt.Errorf("read how much is stored: %w", err)
	}
	if held+size > quota {
		return ErrNoRoom
	}
	return nil
}
