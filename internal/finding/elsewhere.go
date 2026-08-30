package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Match is the same issue at the same place in another build.
type Match struct {
	TargetID int64
	Stream   string
	Variant  string
	// ComponentUpstream and ConsumerUpstream are what that build has. They are
	// why this is a separate question: where they are identical the decision
	// already applies there and nobody is asked, and where they differ it is a
	// different claim about different code.
	ComponentUpstream string
	ConsumerUpstream  string
	Places            int
}

// Elsewhere finds the same issue at the same place in other builds of a
// product, where a decision made here would not already cover it.
//
// A decision is keyed on the combination of code rather than on the release it
// was made in, so a build running the same versions picks it up by looking it
// up — nothing is copied, nothing syncs, and nobody is asked. What this returns
// is the remainder: builds where the versions differ, so the decision does not
// reach them and somebody has to say whether the same reasoning holds.
//
// Offered one at a time rather than as one answer. A component may be used in
// a later release and not an earlier one, and the reasoning that made something
// harmless in one build is not automatically true in another.
func (s *Store) Elsewhere(ctx context.Context, subject access.Subject, at Deciding, exceptTargetID int64) ([]Match, error) {
	if !subject.Sees(at.ProductID) {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", at.ProductID))
	}
	visible := visibleTo(subject, at.ProductID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", at.ProductID))
	}

	var rows []struct {
		TargetID          int64  `bun:"target_id"`
		Stream            string `bun:"stream"`
		Variant           string `bun:"variant"`
		ComponentUpstream string `bun:"component_upstream"`
		ConsumerUpstream  string `bun:"consumer_upstream"`
		Places            int    `bun:"places"`
	}
	err := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS t ON t.id = f.target_id").
		Join("JOIN stream AS st ON st.id = t.stream_id").
		Join("JOIN variant AS va ON va.id = t.variant_id").
		Join("JOIN component AS c ON c.id = f.component_id").
		Join("LEFT JOIN component AS uc ON uc.id = f.consumer_id").
		ColumnExpr("f.target_id AS target_id").
		ColumnExpr("st.name AS stream").
		ColumnExpr("va.name AS variant").
		ColumnExpr("c.upstream_version AS component_upstream").
		ColumnExpr("COALESCE(uc.upstream_version, '') AS consumer_upstream").
		ColumnExpr("COUNT(*) AS places").
		Where("st.product_id = ?", at.ProductID).
		Where("f.vulnerability_id = ?", at.VulnerabilityID).
		Where("f.place_identity = ?", at.PlaceIdentity).
		Where("f.target_id <> ?", exceptTargetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.target_id, st.name, va.name, c.upstream_version, uc.upstream_version").
		OrderExpr("st.name, va.name").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("look for the same issue elsewhere: %w", err)
	}

	matches := make([]Match, 0, len(rows))
	for _, row := range rows {
		// Where the versions are identical the decision already reaches it,
		// and offering it again would ask somebody to agree to something that
		// has already happened.
		if row.ComponentUpstream == at.ComponentUpstream &&
			row.ConsumerUpstream == at.ConsumerUpstream {
			continue
		}
		matches = append(matches, Match{
			TargetID: row.TargetID, Stream: row.Stream, Variant: row.Variant,
			ComponentUpstream: row.ComponentUpstream, ConsumerUpstream: row.ConsumerUpstream,
			Places: row.Places,
		})
	}
	return matches, nil
}
