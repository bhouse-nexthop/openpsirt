package finding

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// Release is one build of a product and how much stands open against it.
type Release struct {
	Stream  string
	Kind    string
	Variant string
	// Open is every open finding at this build, whatever its severity — the
	// same population the findings list counts before any triage line is
	// applied, so the two agree.
	Open int
	// BySeverity is that total split the four ways everything else here ranks
	// by, folded through the one expression the line and the deadline use so a
	// chart cannot disagree with a list about what "high" means.
	BySeverity map[string]int
}

// Releases reports what is open against every build of one product.
//
// The number a release-over-release chart is drawn from, which nothing
// reported before: the comparison screen could say what changed between two
// builds and could not say whether the estate was getting better or worse
// across all of them.
//
// One statement rather than one per build. A product with thirty tags would
// otherwise be thirty round trips to draw one chart, and the answer would be
// assembled from thirty moments rather than one.
func (s *Store) Releases(ctx context.Context, subject access.Subject,
	productID int64) ([]Release, error) {

	products, all := subject.Products()
	if subject.Kind != access.Person || !subject.Sees(productID) {
		return nil, nil
	}

	var rows []struct {
		Stream  string `bun:"stream"`
		Kind    string `bun:"kind"`
		Variant string `bun:"variant"`
		Band    string `bun:"band"`
		Open    int    `bun:"open"`
	}
	query := s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN target AS tg ON tg.id = f.target_id").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		Join("JOIN variant AS va ON va.id = tg.variant_id").
		Join("JOIN vulnerability AS v ON v.id = f.vulnerability_id").
		ColumnExpr("st.name AS stream").
		ColumnExpr("st.kind AS kind").
		ColumnExpr("va.name AS variant").
		ColumnExpr(BandExpr+" AS band").
		ColumnExpr("COUNT(*) AS open").
		Where("st.product_id = ?", productID).
		Where("f.closed_run_id IS NULL").
		GroupExpr("st.name, st.kind, va.name, " + BandExpr).
		// Ordered here rather than in the caller, so every reader of this gets
		// the same sequence. A chart whose points move between requests is
		// worse than one that is wrong in a fixed way.
		OrderExpr("st.name, va.name")
	query = onlyVisible(query, subject, products, all)

	if err := query.Scan(ctx, &rows); err != nil {
		return nil, fmt.Errorf("read what is open against each build: %w", err)
	}

	// Folded in order, so the sequence the database returned is the sequence
	// the caller sees.
	var releases []Release
	at := map[string]int{}
	for _, row := range rows {
		key := row.Stream + "\x00" + row.Variant
		index, seen := at[key]
		if !seen {
			index = len(releases)
			at[key] = index
			releases = append(releases, Release{
				Stream: row.Stream, Kind: row.Kind, Variant: row.Variant,
				BySeverity: map[string]int{},
			})
		}
		releases[index].Open += row.Open
		releases[index].BySeverity[row.Band] += row.Open
	}
	return releases, nil
}

// LatestRun is the most recent finished scan run against a build.
//
// What the numbers on a screen were measured against: which scanner, at which
// version, reading which vulnerability database. The scan run has always
// carried it and nothing showed it, so a reader had no way to tell a build
// with nothing wrong from one last measured against a database from March.
func (s *Store) LatestRun(ctx context.Context, subject access.Subject,
	targetID int64) (*Run, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	if !subject.Sees(productID) {
		return nil, nil
	}
	var runs []Run
	err = s.db.NewSelect().Model(&runs).
		Where("target_id = ?", targetID).
		Where("finished_at IS NOT NULL").
		Order("id DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the last run against this build: %w", err)
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return &runs[0], nil
}

// VersionsWithIssue lists the versions of a named component in one build that
// this issue is actually open against.
//
// The component lookup raises an ambiguity before it knows which issue is
// being asked about, so on its own it can only offer every version of the
// name. A real image ships one library at fifteen, of which three carry a
// given issue — offering all fifteen is a list where four in five choices lead
// to "no such finding", which is a worse answer than the refusal it replaced.
func (s *Store) VersionsWithIssue(ctx context.Context, subject access.Subject,
	targetID, vulnerabilityID int64, name string) ([]string, error) {

	productID, err := productOf(ctx, s.db, targetID)
	if err != nil {
		return nil, err
	}
	visible := visibleTo(subject, productID)
	if !subject.Sees(productID) || len(visible) == 0 {
		return nil, nil
	}

	var versions []string
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		Join("JOIN component AS c ON c.id = f.component_id").
		ColumnExpr("DISTINCT c.version").
		Where("f.target_id = ?", targetID).
		Where("f.vulnerability_id = ?", vulnerabilityID).
		Where("f.closed_run_id IS NULL").
		Where("c.name = ?", name).
		Where("f.visibility IN (?)", bun.List(visible)).
		OrderExpr("c.version").
		Scan(ctx, &versions)
	if err != nil {
		return nil, fmt.Errorf("read which versions carry this issue: %w", err)
	}
	return versions, nil
}
