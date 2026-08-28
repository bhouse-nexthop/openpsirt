package catalogue

import (
	"context"
	"errors"
	"fmt"
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

// EnsureVariant declares a way a stream is built, or confirms one already
// declared.
func (s *Store) EnsureVariant(ctx context.Context, streamID int64, name string, customerFacing bool) (*Variant, bool, error) {
	existing, err := s.VariantByName(ctx, streamID, name)
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

	created, err := s.DeclareVariant(ctx, streamID, name, customerFacing)
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

// Products lists what has been declared.
func (s *Store) Products(ctx context.Context) ([]Product, error) {
	var rows []Product
	if err := s.db.NewSelect().Model(&rows).Order("name").Scan(ctx); err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return rows, nil
}

// Streams lists the branches and tags of a product.
func (s *Store) Streams(ctx context.Context, productID int64) ([]Stream, error) {
	var rows []Stream
	err := s.db.NewSelect().Model(&rows).
		Where("product_id = ?", productID).Order("kind", "name").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	return rows, nil
}

// Variants lists the ways a stream is built.
func (s *Store) Variants(ctx context.Context, streamID int64) ([]Variant, error) {
	var rows []Variant
	err := s.db.NewSelect().Model(&rows).
		Where("stream_id = ?", streamID).Order("name").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	return rows, nil
}
