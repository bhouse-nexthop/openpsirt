package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Open returns the findings currently open against a target that this subject
// may read.
//
// The filtering happens here rather than in whatever asked. A check in a
// handler is a check somebody forgets the first time they add another handler,
// and the thing being forgotten is not a blank screen — it is somebody seeing
// an issue that has not been disclosed.
//
// Which product the target belongs to is read here too, rather than accepted
// from the caller. A caller that could name the product could name a different
// one, and then the check would be answering a question nobody asked.
func (s *Store) Open(ctx context.Context, subject access.Subject, targetID int64) ([]Finding, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		// Not merely empty: a product somebody holds nothing on does not
		// exist as far as they are concerned, and an empty list is a
		// different statement from a refusal.
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []Finding
	err = s.db.NewSelect().Model(&rows).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Order("id").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read open findings: %w", err)
	}
	return rows, nil
}

// CountOpen counts what Open would return.
//
// A count is a read. Counting rows somebody may not see and reporting the
// total is the same disclosure as listing them, just compressed — and it is
// the path that leaks when only row reads are guarded.
func (s *Store) CountOpen(ctx context.Context, subject access.Subject, targetID int64) (int, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return 0, err
	}
	if !subject.Sees(productID) {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return 0, access.Denied(fmt.Sprintf("count findings in product %d", productID))
	}

	n, err := s.db.NewSelect().Model((*Finding)(nil)).
		Where("target_id = ?", targetID).
		Where("closed_run_id IS NULL").
		Where("visibility IN (?)", bun.List(visible)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count open findings: %w", err)
	}
	return n, nil
}

// visibleTo is what a subject may read in a product, as values to compare
// against. Empty means nothing, which is a refusal rather than a filter.
func visibleTo(subject access.Subject, productID int64) []access.Visibility {
	var visible []access.Visibility
	for _, v := range []access.Visibility{access.Public, access.Private} {
		if subject.Reads(v, productID) {
			visible = append(visible, v)
		}
	}
	return visible
}

// productOf reads which product a target belongs to.
func productOf(ctx context.Context, db *bun.DB, targetID int64) (int64, error) {
	var productID int64
	err := db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("st.product_id").
		Where("tg.id = ?", targetID).
		Scan(ctx, &productID)
	if err != nil {
		return 0, fmt.Errorf("look up which product target %d belongs to: %w", targetID, err)
	}
	return productID, nil
}
