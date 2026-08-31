package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Reach is how far a judgment travels, in the three parts somebody deciding
// needs told apart.
//
// The first two are consequences of the matching rules and are not choices —
// there is nothing to agree to, only something to be told. The third is the
// only choice, and it is the one worth slowing down for: ticking one is a
// claim about a version nobody has looked at.
//
// Presenting them as one number is what turns a considered judgment into a
// reflex, and it is also how a decision comes to reach builds the person
// making it never knew about.
type Reach struct {
	// Here is how many places in this build the judgment covers.
	Here int
	// Automatic are the other builds it reaches by matching: same upstream
	// versions, same chain. Nothing to tick.
	Automatic []Match
	// Differing hold the same issue at the same place at another version, so
	// each is asked separately.
	Differing []Match
}

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
func (s *Store) Reaching(ctx context.Context, subject access.Subject, at Deciding, exceptTargetID int64) (Reach, error) {
	if !subject.Sees(at.ProductID) {
		return Reach{}, access.Denied(fmt.Sprintf("read findings in product %d", at.ProductID))
	}
	visible := visibleTo(subject, at.ProductID)
	if len(visible) == 0 {
		return Reach{}, access.Denied(fmt.Sprintf("read findings in product %d", at.ProductID))
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
		ColumnExpr("st.display_name AS stream").
		ColumnExpr("va.display_name AS variant").
		ColumnExpr("c.upstream_version AS component_upstream").
		ColumnExpr("COALESCE(uc.upstream_version, '') AS consumer_upstream").
		ColumnExpr("COUNT(*) AS places").
		Where("st.product_id = ?", at.ProductID).
		Where("f.vulnerability_id = ?", at.VulnerabilityID).
		Where("f.place_identity = ?", at.PlaceIdentity).
		Where("f.target_id <> ?", exceptTargetID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.target_id, st.display_name, va.display_name, c.upstream_version, uc.upstream_version").
		OrderExpr("st.display_name, va.display_name").
		Scan(ctx, &rows)
	if err != nil {
		return Reach{}, fmt.Errorf("look for the same issue elsewhere: %w", err)
	}

	reach := Reach{Here: at.Places}
	for _, row := range rows {
		match := Match{
			TargetID: row.TargetID, Stream: row.Stream, Variant: row.Variant,
			ComponentUpstream: row.ComponentUpstream, ConsumerUpstream: row.ConsumerUpstream,
			Places: row.Places,
		}
		// Where the versions are identical the decision reaches it by
		// matching, so there is nothing to agree to — but somebody deciding
		// should still be told, because it is how far their judgment travels.
		if row.ComponentUpstream == at.ComponentUpstream &&
			row.ConsumerUpstream == at.ConsumerUpstream {
			reach.Automatic = append(reach.Automatic, match)
			continue
		}
		reach.Differing = append(reach.Differing, match)
	}
	return reach, nil
}
