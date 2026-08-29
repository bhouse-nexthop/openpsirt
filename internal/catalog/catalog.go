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

	"github.com/bhouse-nexthop/openpsirt/internal/access"
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
// It belongs to the product rather than to any one release, so one introduced in a
// later release does not appear to have existed in earlier ones.
type Variant struct {
	bun.BaseModel `bun:"table:variant,alias:v"`

	ID        int64  `bun:"id,pk,autoincrement"`
	ProductID int64  `bun:"product_id,notnull"`
	Name      string `bun:"name,notnull"`
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

// DeclareVariant records a way a product is built.
//
// Once per product, not once per release. A variant is a chip, an
// architecture, an operating system — a property of the product that does not
// change because a new release came out. Restating it per release is how one
// release ends up with a name spelled differently from the last, and three
// spellings are three sets of findings with nothing saying they belong
// together.
func (s *Store) DeclareVariant(ctx context.Context, productID int64, name string, customerFacing bool) (*Variant, error) {
	if err := validName("variant", name); err != nil {
		return nil, err
	}
	if _, err := s.VariantByName(ctx, productID, name); err == nil {
		return nil, fmt.Errorf("variant %q: %w", name, ErrExists)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	v := &Variant{ProductID: productID, Name: name, CustomerFacing: customerFacing, CreatedAt: now()}
	if _, err := s.db.NewInsert().Model(v).Exec(ctx); err != nil {
		return nil, fmt.Errorf("declare variant %q: %w", name, err)
	}
	return v, nil
}

// VariantByName finds one of a product's variants.
func (s *Store) VariantByName(ctx context.Context, productID int64, name string) (*Variant, error) {
	v := new(Variant)
	err := s.db.NewSelect().Model(v).
		Where("product_id = ?", productID).Where("name = ?", name).Scan(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, fmt.Errorf("variant %q: %w", name, ErrNotFound)
		}
		return nil, err
	}
	return v, nil
}

// Resolve turns the names a scan supplies into the target it is filed against.
//
// Every part must already be declared. The error names exactly which part is
// missing, because whoever sees the failed upload needs to know what to add
// rather than that something, somewhere, was wrong.
//
// The pair itself is not declared. Once the product, the release and the
// variant all exist, a scan saying this release was built as that variant is
// reporting a fact rather than naming something new, so the row is recorded on
// first use. It is also why a variant introduced later stays out of earlier
// releases: nothing ever filed a scan for it there.
func (s *Store) Resolve(ctx context.Context, product, stream, variant string) (*Target, error) {
	named, err := s.Locate(ctx, product, stream, variant)
	if err != nil {
		return nil, err
	}
	return s.TargetFor(ctx, named.StreamID, named.VariantID)
}

// Named is what an upload said it was for, resolved to rows but not yet
// recorded as a target.
type Named struct {
	ProductID int64
	StreamID  int64
	VariantID int64
}

// LocateVisible is Locate for one sender, reporting anything they may not file
// against as not declared.
//
// The same answer as a name nobody ever declared, deliberately. A key holds one
// product; without this, presenting it and guessing at names elsewhere would
// return a different error for a name that exists — which turns a stolen build
// credential into a reader of the whole shipping catalog.
func (s *Store) LocateVisible(ctx context.Context, subject access.Subject, product, stream, variant string) (*Named, error) {
	p, err := s.ProductByName(ctx, product)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(p.ID) {
		return nil, fmt.Errorf("product %q: %w", product, ErrNotFound)
	}
	return s.Locate(ctx, product, stream, variant)
}

// Locate turns the names an upload states into the things they refer to,
// without recording anything.
//
// Separate from Resolve so that whether the sender is allowed to file against
// this can be decided before the pair is written down. Recording first and
// checking afterwards leaves a row created by a request that was refused.
func (s *Store) Locate(ctx context.Context, product, stream, variant string) (*Named, error) {
	p, err := s.ProductByName(ctx, product)
	if err != nil {
		return nil, err
	}
	st, err := s.StreamByName(ctx, p.ID, stream)
	if err != nil {
		return nil, fmt.Errorf("product %q: %w", product, err)
	}
	v, err := s.VariantByName(ctx, p.ID, variant)
	if err != nil {
		return nil, fmt.Errorf("product %q: %w", product, err)
	}
	return &Named{ProductID: p.ID, StreamID: st.ID, VariantID: v.ID}, nil
}

// TargetFor returns the row for a release built as a variant, recording it the
// first time.
func (s *Store) TargetFor(ctx context.Context, streamID, variantID int64) (*Target, error) {
	target := new(Target)
	err := s.db.NewSelect().Model(target).
		Where("stream_id = ?", streamID).Where("variant_id = ?", variantID).Scan(ctx)
	if err == nil {
		return target, nil
	}
	if !isNoRows(err) {
		return nil, fmt.Errorf("look up what a scan is filed against: %w", err)
	}

	target = &Target{StreamID: streamID, VariantID: variantID, CreatedAt: now()}
	if _, err := s.db.NewInsert().Model(target).Exec(ctx); err != nil {
		return nil, fmt.Errorf("record that this release is built as this variant: %w", err)
	}
	return target, nil
}

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func isNoRows(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows")
}

// Target is one release built as one variant. It is what a scan is filed
// against, and what everything downstream points at, so a single identifier
// runs from a scan through to a finding.
type Target struct {
	bun.BaseModel `bun:"table:target,alias:tg"`

	ID        int64     `bun:"id,pk,autoincrement"`
	StreamID  int64     `bun:"stream_id,notnull"`
	VariantID int64     `bun:"variant_id,notnull"`
	CreatedAt time.Time `bun:"created_at,notnull"`
	// LastScanID is which scan last wrote here, and taking it is how two
	// workers applying two scans of one target are kept apart.
	LastScanID *int64 `bun:"last_scan_id"`
}

// Placement is a target with everything above it named: what it is of, whether
// that line moves, and what to call the thing a scan is about when the scan
// does not name it.
type Placement struct {
	Product string
	Stream  string
	Kind    Kind
	Variant string
	// Moves says whether the line this was filed against is one that advances.
	// A branch is superseded by the next build; a tag never is, so what it
	// shipped has to be answerable years later.
	Moves bool
}

// Describe reads back what a target is.
func (s *Store) Describe(ctx context.Context, targetID int64) (*Placement, error) {
	var t Target
	if err := s.db.NewSelect().Model(&t).Where("id = ?", targetID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up target %d: %w", targetID, err)
	}
	var v Variant
	if err := s.db.NewSelect().Model(&v).Where("id = ?", t.VariantID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up the variant target %d is built as: %w", targetID, err)
	}
	var st Stream
	if err := s.db.NewSelect().Model(&st).Where("id = ?", t.StreamID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up the release target %d belongs to: %w", targetID, err)
	}
	var p Product
	if err := s.db.NewSelect().Model(&p).Where("id = ?", st.ProductID).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up the product release %d belongs to: %w", st.ID, err)
	}
	return &Placement{
		Product: p.Name, Stream: st.Name, Kind: st.Kind, Variant: v.Name,
		Moves: st.Kind == Branch,
	}, nil
}

// ProductByID finds a product by its row.
//
// Used where something already holds an identifier and needs the name to show:
// a credential says which product it may send for, and an operator reading the
// list wants the name they declared rather than a number.
func (s *Store) ProductByID(ctx context.Context, id int64) (*Product, error) {
	p := new(Product)
	if err := s.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("look up product %d: %w", id, err)
	}
	return p, nil
}

// ExistingTarget finds a release built as a variant, without recording one.
//
// Reading is not filing. Asking what is open against a build that has never
// been scanned should not create the record that says it was.
func (s *Store) ExistingTarget(ctx context.Context, streamID, variantID int64) (*Target, error) {
	target := new(Target)
	err := s.db.NewSelect().Model(target).
		Where("stream_id = ?", streamID).Where("variant_id = ?", variantID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("nothing has been filed against this build: %w", err)
	}
	return target, nil
}
