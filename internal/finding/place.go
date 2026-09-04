package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// The upstream version a decision is keyed on, as SQL, so that everything
// asking the question asks it the same way.
//
// The stated upstream version where there is one, and the component's own
// where there is not. Most packages are not forks and state no upstream at all
// — measured on a real image, 88% of them — so reading only the stated one
// leaves the version half of the key empty for almost everything, and a key
// that never changes is a decision that never lapses.
//
// Exported because a decision is written against these versions in one place
// and compared against them in another. Two spellings of the same expression
// is how a decision starts lapsing on one path and standing on the other.
const (
	ComponentUpstreamExpr = "COALESCE(NULLIF(c.upstream_version, ''), c.version, '')"
	ConsumerUpstreamExpr  = "COALESCE(NULLIF(uc.upstream_version, ''), uc.version, '')"
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
	// FixedIn is the version the report says this was fixed in, where it says
	// one. Somebody deciding to defer should see whether there is anywhere to
	// go, and somebody claiming a whole component is unreachable should see
	// which of it is already fixable.
	FixedIn string
	// Consumer is what pulls the component in here, for naming the place to
	// somebody choosing which of them a judgment covers. Empty where that is
	// the product itself.
	Consumer string
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
	visible := access.Visible(subject, productID)
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
		ColumnExpr(ComponentUpstreamExpr+" AS component_upstream").
		ColumnExpr(ConsumerUpstreamExpr+" AS consumer_upstream").
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

// PlacesFor resolves every place one issue occupies in one component.
//
// This is what a judgment made on the finding covers: all of them by default,
// with a narrower set chosen deliberately (TRI-29, TRI-37). Deciding one place
// at a time was the only thing on offer, and a finding sitting at thirty
// places then meant thirty judgments — which is how somebody stops reading.
//
// Read in one statement and grouped here rather than asked per place. A place
// is a pair of names and the rows under one are ordered the same way PlaceFor
// orders them, so which version a decision is keyed on does not depend on
// what the database happened to return first.
func (s *Store) PlacesFor(ctx context.Context, subject access.Subject, targetID int64,
	vulnerabilityID, componentID int64) ([]Deciding, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	visible := access.Visible(subject, productID)
	if len(visible) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}

	var rows []struct {
		placeRow
		PlaceIdentity string `bun:"place_identity"`
		Consumer      string `bun:"consumer"`
		FixedIn       string `bun:"fixed_in"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join(`JOIN component AS c ON c.id = f.component_id`).
		Join(`LEFT JOIN component AS uc ON uc.id = f.consumer_id`).
		Join(`JOIN vulnerability AS v ON v.id = f.vulnerability_id`).
		ColumnExpr("f.place_identity AS place_identity").
		ColumnExpr("COALESCE(uc.name, '') AS consumer").
		ColumnExpr("f.visibility AS visibility").
		ColumnExpr(ComponentUpstreamExpr+" AS component_upstream").
		ColumnExpr(ConsumerUpstreamExpr+" AS consumer_upstream").
		ColumnExpr("COALESCE(v.score_centi, 0) AS severity_centi").
		ColumnExpr("COALESCE(f.fixed_in, '') AS fixed_in").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.component_id = ?", componentID).
		Where("f.closed_run_id IS NULL").
		Where("f.visibility IN (?)", bun.List(visible)).
		OrderExpr("consumer, place_identity, component_upstream, consumer_upstream").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read where this sits: %w", err)
	}

	order := []string{}
	grouped := map[string][]placeRow{}
	consumers := map[string]string{}
	fixes := map[string]string{}
	for _, row := range rows {
		if _, seen := grouped[row.PlaceIdentity]; !seen {
			order = append(order, row.PlaceIdentity)
			consumers[row.PlaceIdentity] = row.Consumer
			fixes[row.PlaceIdentity] = row.FixedIn
		}
		grouped[row.PlaceIdentity] = append(grouped[row.PlaceIdentity], row.placeRow)
	}

	places := make([]Deciding, 0, len(order))
	for _, identity := range order {
		at := grouped[identity]
		places = append(places, Deciding{
			ProductID: productID, VulnerabilityID: vulnerabilityID, PlaceIdentity: identity,
			Visibility:        access.AsVisibility(at[0].Visibility),
			ComponentUpstream: at[0].ComponentUpstream,
			ConsumerUpstream:  at[0].ConsumerUpstream,
			SeverityCenti:     at[0].Severity,
			FixedIn:           fixes[identity],
			Consumer:          consumers[identity],
			Places:            len(at),
			distinctVersions:  distinctVersions(at),
		})
	}
	return places, nil
}
