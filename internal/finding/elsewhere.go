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
	// Version is what that build ships under the name, which is what a route
	// naming a component resolves. For anything that is not a patched fork it
	// is the same as the upstream version below; for a fork it is the fork's
	// own, and the route knows nothing else.
	Version string
	// ComponentUpstream and ConsumerUpstream are what that build has. They are
	// why this is a separate question: where they are identical the decision
	// already applies there and nobody is asked, and where they differ it is a
	// different claim about different code.
	ComponentUpstream string
	ConsumerUpstream  string
	Places            int
	// Here says this is in the build being decided in — another version of the
	// same component, sitting beside the one in hand rather than in another
	// release or variant.
	Here bool
}

// Reaching finds the same issue at the same place held at a version this
// decision would not already cover.
//
// A decision is keyed on the combination of code rather than on the release it
// was made in, so anything running the same versions picks it up by looking it
// up — nothing is copied, nothing syncs, and nobody is asked. What this returns
// is the remainder: the same issue at the same place at *another version*, so
// the decision does not reach it and somebody has to say whether the same
// reasoning holds.
//
// **The build this is being decided in is searched too**, and that is the
// point. A build commonly ships one name at several versions — the reference
// image carries the Go standard library at four — so the same issue at the
// same place sits at versions right beside the one being decided. Looking only
// at other builds asked about a second variant's other version and said
// nothing about this build's, which reads as a question about variants when
// every question here is about a version.
//
// What is skipped is the decision itself: the rows in this build at the very
// versions being decided are what `at.Places` already counts.
func (s *Store) Reaching(ctx context.Context, subject access.Subject, at Deciding, hereTargetID int64) (Reach, error) {
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
		Version           string `bun:"version"`
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
		ColumnExpr("c.version AS version").
		// The same expressions the decision is keyed on (place.go). Read raw,
		// the column is empty for everything that is not a patched fork, so
		// every other build read as "differing" from one whose key had
		// fallen back to the shipped version — including builds at the very
		// same version, which the decision already reached by lookup.
		ColumnExpr(ComponentUpstreamExpr+" AS component_upstream").
		// Exactly the grouped expression, not wrapped once more: MySQL's
		// only_full_group_by matches a selected expression to a grouped one by
		// text, and the expression already answers '' for no consumer.
		ColumnExpr(ConsumerUpstreamExpr+" AS consumer_upstream").
		ColumnExpr("COUNT(*) AS places").
		Where("st.product_id = ?", at.ProductID).
		Where("f.vulnerability_id = ?", at.VulnerabilityID).
		Where("f.place_identity = ?", at.PlaceIdentity).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.target_id, st.display_name, va.display_name, c.version, "+
			ComponentUpstreamExpr+", "+ConsumerUpstreamExpr).
		OrderExpr("st.display_name, va.display_name, c.version").
		Scan(ctx, &rows)
	if err != nil {
		return Reach{}, fmt.Errorf("look for the same issue elsewhere: %w", err)
	}

	reach := Reach{Here: at.Places}
	for _, row := range rows {
		match := Match{
			TargetID: row.TargetID, Stream: row.Stream, Variant: row.Variant, Version: row.Version,
			ComponentUpstream: row.ComponentUpstream, ConsumerUpstream: row.ConsumerUpstream,
			Places: row.Places,
			// Whether it is somewhere else or right here. A screen leads with
			// the version, because that is what differs, and says where as an
			// aside — but it still has to be able to say "here".
			Here: row.TargetID == hereTargetID,
		}
		matches := row.ComponentUpstream == at.ComponentUpstream &&
			row.ConsumerUpstream == at.ConsumerUpstream
		if matches {
			// In this build at these versions, this *is* what is being
			// decided: at.Places already counts it, and listing it as
			// somewhere the judgment travels to would count it twice.
			if match.Here {
				continue
			}
			// Elsewhere at these versions the decision reaches it by matching,
			// so there is nothing to agree to — but somebody deciding should
			// still be told, because it is how far their judgment travels.
			reach.Automatic = append(reach.Automatic, match)
			continue
		}
		reach.Differing = append(reach.Differing, match)
	}
	return reach, nil
}
