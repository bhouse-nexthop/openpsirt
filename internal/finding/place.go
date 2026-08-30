package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Deciding is everything a decision needs to know about where it is being
// made, read from the findings rather than from whoever is making it.
//
// A caller naming a place freely would be choosing which decisions apply
// where, and the versions are what expiry compares — so they come from the
// rows. What a person supplies is which finding they are looking at.
type Deciding struct {
	ProductID       int64
	VulnerabilityID int64
	PlaceIdentity   string
	Visibility      access.Visibility
	// The upstream versions, which are what a decision is keyed on. A shipped
	// version moves whenever something is rebuilt, and a rebuild is not
	// somebody's reasoning becoming wrong.
	ComponentUpstream string
	ConsumerUpstream  string
	// SeverityCenti is how bad this is judged to be now, recorded with the
	// claim so a later re-affirmation can ask whether it has risen since.
	SeverityCenti int
	// Places is how many findings this decision would cover. One judgment
	// about one issue in one component applies everywhere that pair sits, and
	// somebody deciding should know whether that is one place or sixty.
	Places int
}

// PlaceFor resolves what somebody is deciding about.
//
// Authorized here, not by the caller: reaching a finding is what makes it
// yours to argue about, so a place is only returned where the subject can see
// findings of that visibility on that product.
func (s *Store) PlaceFor(ctx context.Context, subject access.Subject, targetID int64, vulnerabilityID int64, placeIdentity string) (*Deciding, error) {
	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := visibleTo(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []struct {
		Visibility        string `bun:"visibility"`
		ComponentUpstream string `bun:"component_upstream"`
		ConsumerUpstream  string `bun:"consumer_upstream"`
		Severity          int    `bun:"severity_centi"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join(`JOIN component AS c ON c.id = f.component_id`).
		Join(`LEFT JOIN component AS uc ON uc.id = f.consumer_id`).
		Join(`JOIN vulnerability AS v ON v.id = f.vulnerability_id`).
		ColumnExpr("f.visibility AS visibility").
		ColumnExpr("c.upstream_version AS component_upstream").
		ColumnExpr("COALESCE(uc.upstream_version, '') AS consumer_upstream").
		ColumnExpr("COALESCE(v.score_centi, 0) AS severity_centi").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.place_identity = ?", placeIdentity).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		Scan(ctx, &rows)
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("no open finding is recorded there")
	}

	// Every row at one place agrees about the component and its consumer —
	// that is what a place is — so the first answers, and the count is what
	// says how much one judgment would cover.
	return &Deciding{
		ProductID: productID, VulnerabilityID: vulnerabilityID, PlaceIdentity: placeIdentity,
		Visibility:        access.AsVisibility(rows[0].Visibility),
		ComponentUpstream: rows[0].ComponentUpstream,
		ConsumerUpstream:  rows[0].ConsumerUpstream,
		SeverityCenti:     rows[0].Severity,
		Places:            len(rows),
	}, nil
}
