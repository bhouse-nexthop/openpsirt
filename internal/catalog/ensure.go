package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// ErrDiffers is returned when something has been declared before, with
// something else.
//
// It is a separate answer from having declared it twice identically. A
// pipeline that declares before every build must not fail on the second one,
// and a pipeline that has quietly changed what it means by a name must not
// pass — telling those apart is the whole point of a declaration step.
var ErrDiffers = errors.New("already declared, differently")

// EnsureProduct declares a product, or confirms one already declared.
//
// The returned flag says which happened, because a caller scripting this into
// whatever cuts a branch needs to know whether anything changed, and a person
// reading the answer needs to know whether they created something.
func (s *Store) EnsureProduct(ctx context.Context, name, displayName string) (*Product, bool, error) {
	existing, err := s.ProductByName(ctx, name)
	switch {
	case err == nil:
		if displayName != "" && displayName != existing.DisplayName {
			return nil, false, fmt.Errorf("product %q: %w: it is displayed as %q, not %q",
				name, ErrDiffers, existing.DisplayName, displayName)
		}
		return existing, false, nil
	case !errors.Is(err, ErrNotFound):
		return nil, false, err
	}

	created, err := s.DeclareProduct(ctx, name, displayName)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// EnsureStream declares a branch or tag, or confirms one already declared.
func (s *Store) EnsureStream(ctx context.Context, productID int64, name string, kind Kind, parentID *int64) (*Stream, bool, error) {
	existing, err := s.StreamByName(ctx, productID, name)
	switch {
	case err == nil:
		// Whether a line moves is not something that can quietly change. A tag
		// that became a branch would make everything filed against it as a
		// frozen point into something that is rebuilt nightly.
		if existing.Kind != kind {
			return nil, false, fmt.Errorf("%q: %w: it was declared as a %s, not a %s",
				name, ErrDiffers, existing.Kind, kind)
		}
		if parentID != nil && (existing.ParentID == nil || *existing.ParentID != *parentID) {
			return nil, false, fmt.Errorf("%q: %w: it was not cut from the branch now being named",
				name, ErrDiffers)
		}
		return existing, false, nil
	case !errors.Is(err, ErrNotFound):
		return nil, false, err
	}

	created, err := s.DeclareStream(ctx, productID, name, kind, parentID)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// EnsureVariant declares a way a product is built, or confirms one already
// declared.
func (s *Store) EnsureVariant(ctx context.Context, productID int64, name string, customerFacing bool) (*Variant, bool, error) {
	existing, err := s.VariantByName(ctx, productID, name)
	switch {
	case err == nil:
		// Whether something reaches customers feeds how its findings rank, so
		// a change here changes what people are told to work on first. It is a
		// decision somebody should make deliberately rather than a field a
		// pipeline overwrites on its next run.
		if existing.CustomerFacing != customerFacing {
			return nil, false, fmt.Errorf("variant %q: %w: it was declared as %s",
				name, ErrDiffers, facing(existing.CustomerFacing))
		}
		return existing, false, nil
	case !errors.Is(err, ErrNotFound):
		return nil, false, err
	}

	created, err := s.DeclareVariant(ctx, productID, name, customerFacing)
	if err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// facing names the two states in the words somebody reading an error would.
func facing(customerFacing bool) string {
	if customerFacing {
		return "customer-facing"
	}
	return "internal"
}

// Products lists what this subject may know about.
//
// A product somebody holds nothing on is not listed and not counted. The list
// itself is a statement about what an organization ships, so filtering it is
// not a nicety — an unfiltered list tells somebody the names of things they
// were never granted.
func (s *Store) Products(ctx context.Context, subject access.Subject) ([]Product, error) {
	visible, all := subject.Products()
	if !all && len(visible) == 0 {
		return nil, nil
	}

	var rows []Product
	query := s.db.NewSelect().Model(&rows).Order("name")
	if !all {
		query = query.Where("id IN (?)", bun.List(visible))
	}
	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return rows, nil
}

// Streams lists the branches and tags of a product.
//
// Guarded here rather than by whoever asked. Somebody who cannot see a product
// cannot see what releases it has either, and finding that out endpoint by
// endpoint is how the second one gets forgotten.
func (s *Store) Streams(ctx context.Context, subject access.Subject, productID int64) ([]Stream, error) {
	if !subject.Sees(productID) {
		return nil, access.Denied("list the releases of a product")
	}
	var rows []Stream
	err := s.db.NewSelect().Model(&rows).
		Where("product_id = ?", productID).Order("kind", "name").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	return rows, nil
}

// Variants lists the ways a product is built.
func (s *Store) Variants(ctx context.Context, subject access.Subject, productID int64) ([]Variant, error) {
	if !subject.Sees(productID) {
		return nil, access.Denied("list the variants of a product")
	}
	var rows []Variant
	err := s.db.NewSelect().Model(&rows).
		Where("product_id = ?", productID).Order("name").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	return rows, nil
}

// BuiltAs lists the variants a release has actually been built as, which is a
// subset of what the product builds: a release predating a variant has no row
// for it, and one that stopped being built as something keeps its history.
func (s *Store) BuiltAs(ctx context.Context, subject access.Subject, productID, streamID int64) ([]Variant, error) {
	if !subject.Sees(productID) {
		return nil, access.Denied("list what a release is built as")
	}
	var rows []Variant
	err := s.db.NewSelect().Model(&rows).
		Join("JOIN target AS tg ON tg.variant_id = v.id").
		Where("tg.stream_id = ?", streamID).Order("v.name").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list what a release is built as: %w", err)
	}
	return rows, nil
}
