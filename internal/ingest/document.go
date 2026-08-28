package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Kind says what a document a build sent is.
type Kind string

const (
	// InventoryKind is what the build shipped: components and the edges
	// between them.
	InventoryKind Kind = "inventory"
	// SuppressionsKind is what the build has already argued does not apply to
	// it, usually because it carries a patch.
	SuppressionsKind Kind = "suppressions"
)

// chunkSize is how much of a document one row holds.
//
// Two engines cap how large a single statement may be, the cap is server
// configuration rather than anything a client can discover, and the lowest
// default in circulation is sixteen megabytes. A bounded row stays far inside
// every default and lets a document be read as a stream rather than held
// whole.
const chunkSize = 512 << 10

// Document is one of the files a build sent, as we hold it.
type Document struct {
	bun.BaseModel `bun:"table:scan_document,alias:sd"`

	ID     int64 `bun:"id,pk,autoincrement"`
	ScanID int64 `bun:"scan_id,notnull"`
	Kind   Kind  `bun:"kind,notnull"`
	// Ordinal separates several documents of one kind. A build's suppressions
	// arrive as a directory rather than a file, so there is rarely one.
	Ordinal int `bun:"ordinal,notnull"`
	// ContentHash is over the bytes as received, which is what makes a
	// re-upload recognizable without reading it again.
	ContentHash string    `bun:"content_hash,notnull"`
	SizeBytes   int64     `bun:"size_bytes,notnull"`
	CreatedAt   time.Time `bun:"created_at,notnull"`
}

// chunk is a bounded piece of a document.
type chunk struct {
	bun.BaseModel `bun:"table:scan_document_chunk,alias:sdc"`

	ID         int64  `bun:"id,pk,autoincrement"`
	DocumentID int64  `bun:"document_id,notnull"`
	Seq        int    `bun:"seq,notnull"`
	Body       []byte `bun:"body,notnull"`
}

// Documents holds what a build sent, from arrival until it has been read.
type Documents struct {
	db  bun.IDB
	now func() time.Time
}

// NewDocuments returns a store over db.
//
// It takes the narrower handle deliberately: writing a document has to be able
// to join the transaction that records the scan, since a scan whose documents
// did not land is a scan nothing can ever read.
func NewDocuments(db bun.IDB) *Documents {
	return &Documents{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// Write stores a document, reading it as a stream.
//
// The content hash is computed here rather than taken from the caller: a hash
// somebody else calculated says nothing about the bytes that actually arrived.
func (d *Documents) Write(ctx context.Context, scanID int64, kind Kind, ordinal int, r io.Reader) (*Document, error) {
	doc := &Document{
		ScanID: scanID, Kind: kind, Ordinal: ordinal,
		CreatedAt: d.now().UTC().Truncate(time.Microsecond),
	}
	if _, err := d.db.NewInsert().Model(doc).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record document: %w", err)
	}

	digest := sha256.New()
	buf := make([]byte, chunkSize)
	for seq := 0; ; seq++ {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			body := buf[:n]
			digest.Write(body)
			doc.SizeBytes += int64(n)
			row := &chunk{DocumentID: doc.ID, Seq: seq, Body: body}
			if _, err := d.db.NewInsert().Model(row).Exec(ctx); err != nil {
				return nil, fmt.Errorf("store part %d of document: %w", seq, err)
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read document: %w", err)
		}
	}

	doc.ContentHash = hex.EncodeToString(digest.Sum(nil))
	if _, err := d.db.NewUpdate().Model(doc).
		Column("content_hash", "size_bytes").WherePK().Exec(ctx); err != nil {
		return nil, fmt.Errorf("record document size and hash: %w", err)
	}
	return doc, nil
}

// List returns a scan's documents, in the order they were sent.
func (d *Documents) List(ctx context.Context, scanID int64) ([]Document, error) {
	var docs []Document
	err := d.db.NewSelect().Model(&docs).
		Where("scan_id = ?", scanID).
		Order("kind", "ordinal").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return docs, nil
}

// Open reads a document back as a stream.
//
// A document is tens of megabytes and there is no reason to hold one in memory
// to hand it to a reader that only ever moves forwards.
func (d *Documents) Open(ctx context.Context, documentID int64) io.Reader {
	return &chunkReader{ctx: ctx, db: d.db, documentID: documentID}
}

// Discard removes a scan's documents.
//
// What a nightly build sent is superseded the next night, so keeping it costs
// storage that grows with the calendar. What a tagged release sent is kept,
// because re-scanning it years from now needs both what it contained and what
// the build had already argued about its own patches — so this is called for
// one and not the other.
func (d *Documents) Discard(ctx context.Context, scanID int64) error {
	docs, err := d.List(ctx, scanID)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
	}
	err = database.IDsInBatches(ctx, ids, func(ctx context.Context, batch []int64) error {
		_, err := d.db.NewDelete().Model((*chunk)(nil)).
			Where("document_id IN (?)", bun.List(batch)).Exec(ctx)
		return err
	})
	if err != nil {
		return fmt.Errorf("discard document content: %w", err)
	}
	if _, err := d.db.NewDelete().Model((*Document)(nil)).
		Where("scan_id = ?", scanID).Exec(ctx); err != nil {
		return fmt.Errorf("discard documents: %w", err)
	}
	return nil
}

// chunkReader walks a document's rows in order.
type chunkReader struct {
	ctx        context.Context
	db         bun.IDB
	documentID int64
	next       int
	held       []byte
	done       bool
	err        error
}

func (c *chunkReader) Read(p []byte) (int, error) {
	for len(c.held) == 0 {
		if c.err != nil {
			return 0, c.err
		}
		if c.done {
			return 0, io.EOF
		}
		var row chunk
		err := c.db.NewSelect().Model(&row).
			Where("document_id = ?", c.documentID).
			Where("seq = ?", c.next).
			Limit(1).Scan(c.ctx)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			c.done = true
			return 0, io.EOF
		case err != nil:
			c.err = fmt.Errorf("read part %d of document: %w", c.next, err)
			return 0, c.err
		}
		c.held = row.Body
		c.next++
	}
	n := copy(p, c.held)
	c.held = c.held[n:]
	return n, nil
}
