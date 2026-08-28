// Package catalog holds what a scan can be filed against: the products, the
// branches and tags within them, and the variants each of those is built as.
//
// Everything here is declared before it can be targeted. A scan naming
// something undeclared is rejected, because a mistyped stream name would
// otherwise create one that looks entirely real — its own findings, its own
// counts, its own place in every report — while the real one appears to have
// stopped being scanned.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// Kind distinguishes a moving line of development from a frozen point.
type Kind string

const (
	// Branch moves: it is rebuilt, and its current state changes.
	Branch Kind = "branch"
	// Tag never moves. It was built once and is what someone received.
	Tag Kind = "tag"
)

// Valid reports whether k is a kind we recognize.
func (k Kind) Valid() bool { return k == Branch || k == Tag }

// ErrNotFound is returned when something named has not been declared.
var ErrNotFound = errors.New("not declared")

// ErrExists is returned when a declaration would duplicate one already made.
var ErrExists = errors.New("already declared")

// Product is a thing that gets shipped.
type Product struct {
	bun.BaseModel `bun:"table:product,alias:p"`

	ID          int64      `bun:"id,pk,autoincrement"`
	Name        string     `bun:"name,notnull"`
	DisplayName string     `bun:"display_name,notnull"`
	EOLOn       *time.Time `bun:"eol_on"`
	CreatedAt   time.Time  `bun:"created_at,notnull"`
}

// Stream is a branch or a tag of a product.
//
// Both live here because they differ in one respect only — a branch moves and
// a tag does not — and share everything else.
type Stream struct {
	bun.BaseModel `bun:"table:stream,alias:s"`

	ID        int64  `bun:"id,pk,autoincrement"`
	ProductID int64  `bun:"product_id,notnull"`
	Name      string `bun:"name,notnull"`
	Kind      Kind   `bun:"kind,notnull"`
	// ParentID is the branch a tag was cut from, where that is known. It is
	// what lets a branch be compared against its last release.
	ParentID  *int64     `bun:"parent_id"`
	EOLOn     *time.Time `bun:"eol_on"`
	CreatedAt time.Time  `bun:"created_at,notnull"`
}

// Variant is one of the ways a stream is built — a chip variant, an operating
// system, an architecture.
//
// It belongs to the stream rather than the product, so one introduced in a
// later release does not appear to have existed in earlier ones.
type Variant struct {
	bun.BaseModel `bun:"table:variant,alias:v"`

	ID       int64  `bun:"id,pk,autoincrement"`
	StreamID int64  `bun:"stream_id,notnull"`
	Name     string `bun:"name,notnull"`
	// CustomerFacing says whether this ships to customers or exists only
	// internally. It feeds ranking: a critical in a test-only artifact matters
	// less than a medium in something a customer runs.
	CustomerFacing bool      `bun:"customer_facing,notnull"`
	CreatedAt      time.Time `bun:"created_at,notnull"`
}

// Store reads and writes the catalog.
type Store struct{ db bun.IDB }

// NewStore returns a store over db.
func NewStore(db bun.IDB) *Store { return &Store{db: db} }

// maxNameLength matches the column width, which is bounded so a unique index
// on it stays inside every engine's key-length limit.
const maxNameLength = 191

// validName rejects what would be confusing or unusable as an identifier.
func validName(what, name string) error {
	trimmed := strings.TrimSpace(name)
	switch {
	case trimmed == "":
		return fmt.Errorf("%s name is empty", what)
	case trimmed != name:
		return fmt.Errorf("%s name %q has leading or trailing spaces", what, name)
	case len(name) > maxNameLength:
		return fmt.Errorf("%s name is %d characters; the limit is %d", what, len(name), maxNameLength)
	}
	return nil
}

// DeclareProduct records a product so scans may be filed against it.
func (s *Store) DeclareProduct(ctx context.Context, name, displayName string) (*Product, error) {
	if err := validName("product", name); err != nil {
		return nil, err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = name
	}
	if _, err := s.ProductByName(ctx, name); err == nil {
		return nil, fmt.Errorf("product %q: %w", name, ErrExists)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	p := &Product{Name: name, DisplayName: displayName, CreatedAt: now()}
	if _, err := s.db.NewInsert().Model(p).Exec(ctx); err != nil {
		return nil, fmt.Errorf("declare product %q: %w", name, err)
	}
	return p, nil
}

// ProductByName finds a product, or reports that it was never declared.
func (s *Store) ProductByName(ctx context.Context, name string) (*Product, error) {
	p := new(Product)
	err := s.db.NewSelect().Model(p).Where("name = ?", name).Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("product %q: %w", name, ErrNotFound)
		}
		return nil, err
	}
	return p, nil
}

// DeclareStream records a branch or tag of a product.
func (s *Store) DeclareStream(ctx context.Context, productID int64, name string, kind Kind, parentID *int64) (*Stream, error) {
	if err := validName("stream", name); err != nil {
		return nil, err
	}
	if !kind.Valid() {
		return nil, fmt.Errorf("stream kind %q: want %q or %q", kind, Branch, Tag)
	}
	if _, err := s.StreamByName(ctx, productID, name); err == nil {
		return nil, fmt.Errorf("stream %q: %w", name, ErrExists)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	st := &Stream{ProductID: productID, Name: name, Kind: kind, ParentID: parentID, CreatedAt: now()}
	if _, err := s.db.NewInsert().Model(st).Exec(ctx); err != nil {
		return nil, fmt.Errorf("declare stream %q: %w", name, err)
	}
	// A new release is built as the same things the product is already built
	// as. Making somebody restate that list is how one release ends up with a
	// variant spelled differently from the last.
	if err := s.seedVariants(ctx, productID, st.ID); err != nil {
		return nil, err
	}
	return st, nil
}

// StreamByName finds a stream within a product.
func (s *Store) StreamByName(ctx context.Context, productID int64, name string) (*Stream, error) {
	st := new(Stream)
	err := s.db.NewSelect().Model(st).
		Where("product_id = ?", productID).Where("name = ?", name).Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("stream %q: %w", name, ErrNotFound)
		}
		return nil, err
	}
	return st, nil
}

// DeclareVariant records a way a stream is built.
func (s *Store) DeclareVariant(ctx context.Context, streamID int64, name string, customerFacing bool) (*Variant, error) {
	if err := validName("variant", name); err != nil {
		return nil, err
	}
	if _, err := s.VariantByName(ctx, streamID, name); err == nil {
		return nil, fmt.Errorf("variant %q: %w", name, ErrExists)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	v := &Variant{StreamID: streamID, Name: name, CustomerFacing: customerFacing, CreatedAt: now()}
	if _, err := s.db.NewInsert().Model(v).Exec(ctx); err != nil {
		return nil, fmt.Errorf("declare variant %q: %w", name, err)
	}
	return v, nil
}

// VariantByName finds a variant within a stream.
func (s *Store) VariantByName(ctx context.Context, streamID int64, name string) (*Variant, error) {
	v := new(Variant)
	err := s.db.NewSelect().Model(v).
		Where("stream_id = ?", streamID).Where("name = ?", name).Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("variant %q: %w", name, ErrNotFound)
		}
		return nil, err
	}
	return v, nil
}

// Resolve turns the names a scan supplies into the variant it targets.
//
// Every part must already be declared. The error names exactly which part is
// missing, because whoever sees the failed upload needs to know what to add
// rather than that something, somewhere, was wrong.
func (s *Store) Resolve(ctx context.Context, product, stream, variant string) (*Variant, error) {
	p, err := s.ProductByName(ctx, product)
	if err != nil {
		return nil, err
	}
	st, err := s.StreamByName(ctx, p.ID, stream)
	if err != nil {
		return nil, fmt.Errorf("product %q: %w", product, err)
	}
	v, err := s.VariantByName(ctx, st.ID, variant)
	if err != nil {
		return nil, fmt.Errorf("product %q stream %q: %w", product, stream, err)
	}
	return v, nil
}

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows")
}

// Target is a variant with everything above it, which is what filing a scan
// needs: what it is of, whether that line moves, and what to call the thing
// the scan is about when the scan does not name it.
type Target struct {
	Product string
	Stream  string
	Kind    Kind
	Variant string
	// Moves says whether the line this was filed against is one that advances.
	// A branch is superseded by the next build; a tag never is, so what it
	// shipped has to be answerable years later.
	Moves bool
}

// Describe reads back what a variant belongs to.
func (s *Store) Describe(ctx context.Context, variantID int64) (*Target, error) {
	var v Variant
	if err := s.db.NewSelect().Model(&v).Where("id = ?", variantID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up variant %d: %w", variantID, err)
	}
	var st Stream
	if err := s.db.NewSelect().Model(&st).Where("id = ?", v.StreamID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up the stream variant %d belongs to: %w", variantID, err)
	}
	var p Product
	if err := s.db.NewSelect().Model(&p).Where("id = ?", st.ProductID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up the product stream %d belongs to: %w", st.ID, err)
	}
	return &Target{
		Product: p.Name, Stream: st.Name, Kind: st.Kind, Variant: v.Name,
		Moves: st.Kind == Branch,
	}, nil
}
