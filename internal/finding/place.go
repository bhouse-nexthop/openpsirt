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
	// distinctVersions is how many versions of this component sit at this
	// place. More than one means a decision cannot cover all of them.
	distinctVersions int
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

	var rows []placeRow
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join(`JOIN component AS c ON c.id = f.component_id`).
		Join(`LEFT JOIN component AS uc ON uc.id = f.consumer_id`).
		Join(`JOIN vulnerability AS v ON v.id = f.vulnerability_id`).
		ColumnExpr("f.visibility AS visibility").
		// The upstream version where one is stated, and the component's own
		// where none is. Most packages are not forks and state no upstream at
		// all — measured on a real image, 88% of them — so reading only the
		// stated one left the version half of the key empty for almost
		// everything, and a key that never changes is a decision that never
		// lapses. A dismissal written about one version would suppress the
		// same issue in every later one, forever.
		//
		// Falling back to the shipped version asks again on a packaging
		// revision that changed no code, which TRI-11 would rather avoid. That
		// is the safe direction to be wrong in: asking twice costs somebody a
		// minute, and not asking costs a vulnerability nobody looked at. The
		// case TRI-11 is actually about — a fork carrying its own version
		// while the issue lives upstream — is exactly the case that states an
		// upstream, so it is unaffected.
		ColumnExpr("COALESCE(NULLIF(c.upstream_version, ''), c.version) AS component_upstream").
		ColumnExpr("COALESCE(NULLIF(uc.upstream_version, ''), uc.version, '') AS consumer_upstream").
		ColumnExpr("COALESCE(v.score_centi, 0) AS severity_centi").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.place_identity = ?", placeIdentity).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		OrderExpr("component_upstream, consumer_upstream").
		Scan(ctx, &rows)
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("no open finding is recorded there")
	}

	// A place is a pair of names, so one place can hold the same package at
	// two versions — a build shipping both is unusual and not impossible. The
	// rows are ordered so that which version a decision is keyed on is the
	// same on every engine and every run, rather than whichever the database
	// happened to return first.
	return &Deciding{
		ProductID: productID, VulnerabilityID: vulnerabilityID, PlaceIdentity: placeIdentity,
		Visibility:        access.AsVisibility(rows[0].Visibility),
		ComponentUpstream: rows[0].ComponentUpstream,
		ConsumerUpstream:  rows[0].ConsumerUpstream,
		SeverityCenti:     rows[0].Severity,
		Places:            len(rows),
		distinctVersions:  distinctVersions(rows),
	}, nil
}

// placeRow is one open finding at the place being decided about.
type placeRow struct {
	Visibility        string `bun:"visibility"`
	ComponentUpstream string `bun:"component_upstream"`
	ConsumerUpstream  string `bun:"consumer_upstream"`
	Severity          int    `bun:"severity_centi"`
}

// Versions reports how many distinct versions this place holds.
//
// One is the ordinary answer. More than one means a build ships the same
// package twice at different versions under the same consumer, and a single
// decision cannot honestly cover both — so whoever is deciding is told rather
// than being given a judgment about one version silently applied to another.
func (d Deciding) Versions() int { return d.distinctVersions }

// distinctVersions counts the version pairs a place holds.
func distinctVersions(rows []placeRow) int {
	seen := map[[2]string]bool{}
	for _, row := range rows {
		seen[[2]string{row.ComponentUpstream, row.ConsumerUpstream}] = true
	}
	return len(seen)
}
