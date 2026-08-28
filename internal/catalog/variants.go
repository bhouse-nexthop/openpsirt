package catalog

import (
	"context"
	"errors"
	"fmt"
)

// ErrUnknownVariant is returned when a stream declares a variant this product
// has never used before, without saying that it is new.
//
// It exists because the same typo that would create a phantom stream will
// otherwise create a phantom variant, once per release. "win", "windows" and
// "win32" across three releases are three variants as far as everything
// downstream is concerned — three sets of findings, three sets of decisions,
// and three columns in every report — and nothing in the data says they were
// meant to be one.
var ErrUnknownVariant = errors.New("not a variant this product uses")

// Known is a variant name a product already builds, with what it means.
type Known struct {
	Name           string `bun:"name"`
	CustomerFacing bool   `bun:"customer_facing"`
}

// KnownVariants returns the variant names a product already uses, wherever
// they were first declared.
//
// A variant is a way a product is built — a chip, an architecture, an
// operating system. That is a property of the product, and it does not stop
// being one because each release records the ones it was actually built as.
func (s *Store) KnownVariants(ctx context.Context, productID int64) ([]Known, error) {
	// Folded here rather than aggregated in the query. Reducing a boolean
	// across rows has no portable spelling — one engine has no aggregate for
	// the type at all — and the set is one row per release per variant, which
	// is small enough that the database gains nothing by doing it.
	var rows []Known
	err := s.db.NewSelect().
		TableExpr("variant AS v").
		Join("JOIN stream AS st ON st.id = v.stream_id").
		ColumnExpr("v.name AS name").
		ColumnExpr("v.customer_facing AS customer_facing").
		Where("st.product_id = ?", productID).
		OrderExpr("v.name").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read the variants product %d builds: %w", productID, err)
	}

	facing := map[string]bool{}
	var known []Known
	for _, row := range rows {
		if _, seen := facing[row.Name]; !seen {
			known = append(known, Known{Name: row.Name})
		}
		// Customer-facing wins where releases disagree, for the same reason it
		// is the default: something unclassified should rank as though it
		// ships.
		facing[row.Name] = facing[row.Name] || row.CustomerFacing
	}
	for i := range known {
		known[i].CustomerFacing = facing[known[i].Name]
	}
	return known, nil
}

// seedVariants gives a new stream the variants its product already builds.
//
// Declaring a release should not mean restating what the product is built as.
// Someone made to retype a list will eventually retype it differently, and the
// difference is invisible until two releases cannot be compared.
//
// Seeding only ever runs forward. A variant introduced in a later release does
// not appear in earlier ones, which is the property that stops a new chip from
// looking like something the product shipped years ago.
func (s *Store) seedVariants(ctx context.Context, productID, streamID int64) error {
	known, err := s.KnownVariants(ctx, productID)
	if err != nil {
		return err
	}
	for _, k := range known {
		if _, err := s.DeclareVariant(ctx, streamID, k.Name, k.CustomerFacing); err != nil {
			return fmt.Errorf("carry variant %q into a new stream: %w", k.Name, err)
		}
	}
	return nil
}

// checkKnown refuses a variant name the product has never used.
//
// The first stream of a product has nothing to compare against, so anything it
// declares is accepted — a vocabulary has to start somewhere. Once one exists,
// adding to it is deliberate rather than accidental.
func (s *Store) checkKnown(ctx context.Context, productID int64, name string, introducing bool) error {
	if introducing {
		return nil
	}
	known, err := s.KnownVariants(ctx, productID)
	if err != nil {
		return err
	}
	if len(known) == 0 {
		return nil
	}
	for _, k := range known {
		if k.Name == name {
			return nil
		}
	}
	names := make([]string, 0, len(known))
	for _, k := range known {
		names = append(names, k.Name)
	}
	return fmt.Errorf("variant %q: %w. It builds %v. Say the variant is new if it really is",
		name, ErrUnknownVariant, names)
}
