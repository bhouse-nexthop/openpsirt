package finding

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
)

// place is one finding's position: the component, and what pulled it in.
type place struct {
	componentID int64
	consumerID  int64 // zero where the thing above is the product itself
}

// key identifies a finding within a variant.
type key struct {
	vulnerabilityID int64
	place           place
}

// Apply records what a run found, writing only the difference.
//
// A scanner reports a package at a version. Fanning that out across the places
// the package occupies is done here, from the graph the inventory described —
// it is the step where one line in a report becomes the number of decisions
// somebody actually has to make.
//
// Re-running against a database that moved slightly must write only what
// changed, for the same reason the graph does: a nightly re-scan that found
// the same things should cost nothing.
func (s *Store) Apply(ctx context.Context, targetID, runID int64, reported []Reported) (Applied, error) {
	var applied Applied

	err := database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// Taken first, before anything is read, exactly as applying a graph
		// does. Two runs against one target can be in flight at once — the
		// queue hands different jobs to different workers by design — and
		// without this both read the same open findings, both compute the same
		// difference, and both write it, leaving two open rows where
		// everything downstream assumes one. An ordinary row update is a lock
		// every engine honors, so the second worker waits instead of racing.
		if _, err := tx.NewUpdate().Table("target").
			Set("last_run_id = ?", runID).
			Where("id = ?", targetID).Exec(ctx); err != nil {
			return fmt.Errorf("take the target: %w", err)
		}

		issues := make([]Named, 0, len(reported))
		for _, r := range reported {
			issues = append(issues, r.Issue)
		}
		vulnerabilities, err := NewVulnerabilities(tx).Intern(ctx, issues)
		if err != nil {
			return err
		}

		// Whether this build reaches customers, read once. A critical in
		// something only the build system runs matters less than a medium in
		// what people install, and that is a property of the build rather than
		// of any finding in it.
		shipped, err := shippedToCustomers(ctx, tx, targetID)
		if err != nil {
			return err
		}

		present, err := openComponents(ctx, tx, targetID)
		if err != nil {
			return err
		}
		places, err := openPlaces(ctx, tx, targetID)
		if err != nil {
			return err
		}
		// What the build has already argued about what it ships. Applied here
		// rather than upstream of us, so a suppressed finding is something
		// that can be seen and accounted for instead of one that never
		// arrived.
		claims, err := openClaims(ctx, tx, targetID)
		if err != nil {
			return err
		}
		// Which of them reached anything is worked out against what the target
		// contains, not against what was reported: a claim covering a
		// component nothing was found in has still done its job, while one
		// covering nothing the build ships has not.
		applied.ClaimsReaching, applied.ClaimsReachingNothing = claimsReaching(claims, present)

		wanted := map[key]Finding{}
		for _, r := range reported {
			component, held := present.byIdentity[r.Component.Identity()]
			if !held {
				// Reported against something this variant does not contain.
				// It has no place, so it cannot become a finding at one, and
				// silently dropping it would hide a report that does not match
				// the inventory it was produced from.
				applied.Unplaced++
				continue
			}
			vulnerabilityID, known := vulnerabilities[normalize(r.Issue.Identifier)]
			if !known {
				return fmt.Errorf("issue %q was not recorded", r.Issue.Identifier)
			}

			covering := coveringClaim(claims, r.Issue, component)
			for _, consumerID := range places.of(component.ID) {
				at := place{componentID: component.ID, consumerID: consumerID}
				wanted[key{vulnerabilityID, at}] = Finding{
					TargetID: targetID, Kind: Vulnerable,
					// What a scanner found in a shipped component is public
					// knowledge by the time it reaches us: the advisory it
					// matched is published. What is not disclosed is a finding
					// somebody entered here.
					Visibility:      access.Public,
					VulnerabilityID: vulnerabilityID,
					ComponentID:     component.ID,
					ConsumerID:      optional(consumerID),
					PlaceIdentity:   PlaceIdentity(component.Name, present.nameOf(consumerID)),
					FixState:        r.FixState, FixedIn: r.FixedIn, FixedAt: r.FixedAt,
					SuppressedBy: covering,
					OpenedRunID:  runID,
				}
				ranked := Ranked{
					Exploited: r.Issue.Exploited, Shipped: shipped,
					LikelihoodPPM: int(r.Issue.Likelihood * 1_000_000),
					ScoreCenti:    scoreOf(r.Issue),
				}
				entry := wanted[key{vulnerabilityID, at}]
				entry.Urgency = int64(ranked.Rank())
				entry.RankExploited, entry.RankShipped = ranked.Exploited, ranked.Shipped
				wanted[key{vulnerabilityID, at}] = entry
			}
		}

		var open []Finding
		err = tx.NewSelect().Model(&open).
			Where("target_id = ?", targetID).Where("closed_run_id IS NULL").Scan(ctx)
		if err != nil {
			return fmt.Errorf("read what is already open: %w", err)
		}

		held := map[key]Finding{}
		for _, f := range open {
			held[key{f.VulnerabilityID, place{f.ComponentID, value(f.ConsumerID)}}] = f
		}

		now := s.now().UTC().Truncate(time.Microsecond)

		var opening []Finding
		for k, f := range wanted {
			if f.SuppressedBy != nil {
				applied.Suppressed++
			}
			already, open := held[k]
			if !open {
				f.LastChangedAt = now
				opening = append(opening, f)
				continue
			}
			// A finding that is already open still moves. A fix appears, or
			// the build answers it — and somebody waiting for a fix is waiting
			// for exactly that. Leaving the row as first written would report
			// last month's answer indefinitely.
			if same(already, f) {
				continue
			}
			_, err := tx.NewUpdate().Model((*Finding)(nil)).
				Set("fix_state = ?", f.FixState).
				Set("fixed_in = ?", f.FixedIn).
				Set("fixed_at = ?", f.FixedAt).
				Set("suppressed_by = ?", f.SuppressedBy).
				Set("last_changed_at = ?", now).
				Where("id = ?", already.ID).Exec(ctx)
			if err != nil {
				return fmt.Errorf("update a finding that moved: %w", err)
			}
			applied.Updated++
		}
		if len(opening) > 0 {
			if err := database.InBatches(ctx, tx, opening); err != nil {
				return fmt.Errorf("open %d findings: %w", len(opening), err)
			}
			applied.Opened = len(opening)
		}

		var closing []Finding
		for k, f := range held {
			if _, still := wanted[k]; still {
				continue
			}
			closing = append(closing, f)
		}
		// What a closing finding was about is read from the component
		// catalog rather than from what the variant currently contains — the
		// whole reason it is closing is usually that it is no longer there.
		departed, err := componentsByID(ctx, tx, closing)
		if err != nil {
			return err
		}
		byReason := map[Closure][]int64{}
		for _, f := range closing {
			reason := present.why(departed[f.ComponentID])
			byReason[reason] = append(byReason[reason], f.ID)
			if reason == Unexplained {
				applied.Unexplained++
			}
			applied.Closed++
		}
		for reason, ids := range byReason {
			err := database.IDsInBatches(ctx, ids, func(ctx context.Context, batch []int64) error {
				_, err := tx.NewUpdate().Model((*Finding)(nil)).
					Set("closed_run_id = ?", runID).
					Set("closed_because = ?", reason).
					Where("id IN (?)", bun.List(batch)).Exec(ctx)
				return err
			})
			if err != nil {
				return fmt.Errorf("close %d findings: %w", len(ids), err)
			}
		}
		return nil
	})
	return applied, err
}

// same reports whether what a scan found about a finding matches what is
// already recorded. Only the parts that can move are compared: everything else
// is what makes it that finding rather than another.
func same(held, found Finding) bool {
	return held.FixState == found.FixState &&
		held.FixedIn == found.FixedIn &&
		held.Urgency == found.Urgency &&
		sameDate(held.FixedAt, found.FixedAt) &&
		equalRef(held.SuppressedBy, found.SuppressedBy)
}

// equalRef compares two references that may be absent.
func equalRef(a, b *int64) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// claimsReaching counts how many of a build's arguments land on something it
// actually ships.
//
// A claim that reached nothing is the ordinary case rather than the
// exceptional one — a producer's automatically-extracted claims name source
// trees rather than packages — and it means a finding the build believes it
// has answered will come back as noise. Nothing distinguishes that from a
// finding nobody has looked at, so the count is reported rather than left to
// be inferred.
func claimsReaching(claims []Claim, present inventory) (reaching, reachingNothing int) {
	for _, claim := range claims {
		found := false
		for _, component := range present.byID {
			if claim.covers(describedOf(component)) {
				found = true
				break
			}
		}
		if found {
			reaching++
		} else {
			reachingNothing++
		}
	}
	return reaching, reachingNothing
}

// describedOf reads a stored component back into the shape a claim matches
// against.
func describedOf(c graph.Component) graph.Described {
	return graph.Described{
		Purl: c.Purl, CPE: c.CPE, Name: c.Name, Version: c.Version,
		UpstreamName: c.UpstreamName, UpstreamVersion: c.UpstreamVersion,
	}
}

// coveringClaim finds the build's argument that covers a reported issue, if it
// made one.
//
// A claim that arrived attached to the component is preferred over one that
// named something we had to match: the first knows exactly what it is about,
// while the second may name a whole source tree.
func coveringClaim(claims []Claim, issue Named, component graph.Component) *int64 {
	names := map[string]bool{normalize(issue.Identifier): true}
	for _, alias := range issue.Aliases {
		names[normalize(alias)] = true
	}

	described := describedOf(component)

	var found *int64
	for _, claim := range claims {
		if !names[normalize(claim.Vulnerability)] || !claim.suppresses() || !claim.covers(described) {
			continue
		}
		id := claim.ID
		if claim.Origin == string(sbom.FromPedigree) {
			return &id
		}
		if found == nil {
			// The first in a stable order, so the answer does not move
			// between runs.
			found = &id
		}
	}
	return found
}

// inventory is what a target currently contains, as far as findings care.
type inventory struct {
	byID       map[int64]graph.Component
	byIdentity map[string]graph.Component
	byName     map[string][]graph.Component
}

// nameOf names a consumer, or nothing where the consumer is the product.
func (i inventory) nameOf(consumerID int64) string {
	if consumerID == 0 {
		return ""
	}
	return i.byID[consumerID].Name
}

// why explains a finding that is no longer reported.
//
// The component being gone, its upstream version having moved, and a
// downstream revision having landed are three different things to whoever
// reads the report later. What is left over — the component still present and
// unchanged, and the scanner no longer reporting it — is the case that must
// never be quietly dropped.
func (i inventory) why(gone graph.Component) Closure {
	if _, present := i.byID[gone.ID]; present {
		return Unexplained
	}
	for _, now := range i.byName[gone.Name] {
		switch {
		case upstreamOf(now) != upstreamOf(gone):
			return Upgraded
		case now.Version != gone.Version:
			return Revised
		}
	}
	// Something of that name is still here, unchanged in both the version that
	// ships and the version vulnerabilities are matched against. Nothing about
	// the component explains the finding going away, so it is not explained.
	if len(i.byName[gone.Name]) > 0 {
		return Unexplained
	}
	return Removed
}

// upstreamOf is the version a vulnerability is actually matched against: what
// a fork was made from, where there is one, and otherwise what shipped.
func upstreamOf(c graph.Component) string {
	if c.UpstreamVersion != "" {
		return c.UpstreamVersion
	}
	return c.Version
}

// componentsByID reads what a set of findings was about.
func componentsByID(ctx context.Context, db bun.IDB, findings []Finding) (map[int64]graph.Component, error) {
	if len(findings) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.ComponentID)
	}
	byID := map[int64]graph.Component{}
	err := database.IDsInBatches(ctx, ids, func(ctx context.Context, batch []int64) error {
		var rows []graph.Component
		if err := db.NewSelect().Model(&rows).Where("id IN (?)", bun.List(batch)).Scan(ctx); err != nil {
			return err
		}
		for _, row := range rows {
			byID[row.ID] = row
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read what closing findings were about: %w", err)
	}
	return byID, nil
}

// openComponents reads what a variant currently contains.
func openComponents(ctx context.Context, db bun.IDB, targetID int64) (inventory, error) {
	var rows []graph.Component
	err := db.NewSelect().Model(&rows).
		Join("JOIN graph_node AS n ON n.component_id = c.id").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Scan(ctx)
	if err != nil {
		return inventory{}, fmt.Errorf("read what the variant contains: %w", err)
	}

	held := inventory{
		byID:       make(map[int64]graph.Component, len(rows)),
		byIdentity: make(map[string]graph.Component, len(rows)),
		byName:     map[string][]graph.Component{},
	}
	for _, row := range rows {
		held.byID[row.ID] = row
		held.byIdentity[row.Identity] = row
		held.byName[row.Name] = append(held.byName[row.Name], row)
	}
	return held, nil
}

// consumers maps a component to everything that directly pulled it in.
type consumers map[int64][]int64

// of returns where a component sits. A component nothing leads to still has a
// place — itself — because it ships whether or not the producer could say what
// pulled it in.
func (c consumers) of(componentID int64) []int64 {
	if at, held := c[componentID]; held {
		return at
	}
	return []int64{0}
}

// openPlaces reads every place in a variant.
//
// A component under the product itself is recorded as being under nothing: the
// product's name differs per variant, so keying on it would stop the same
// place being recognized across them.
func openPlaces(ctx context.Context, db bun.IDB, targetID int64) (consumers, error) {
	var edges []struct {
		ChildComponentID  int64 `bun:"child_component_id"`
		ParentComponentID int64 `bun:"parent_component_id"`
		ParentIsRoot      bool  `bun:"parent_is_root"`
	}
	err := db.NewSelect().
		TableExpr("graph_edge AS e").
		Join("JOIN graph_node AS child ON child.id = e.child_id").
		Join("JOIN graph_node AS parent ON parent.id = e.parent_id").
		ColumnExpr("child.component_id AS child_component_id").
		ColumnExpr("parent.component_id AS parent_component_id").
		ColumnExpr("parent.is_root AS parent_is_root").
		Where("e.target_id = ?", targetID).
		Where("e.closed_scan_id IS NULL").
		Scan(ctx, &edges)
	if err != nil {
		return nil, fmt.Errorf("read where things sit: %w", err)
	}

	at := consumers{}
	seen := map[[2]int64]bool{}
	for _, e := range edges {
		consumer := e.ParentComponentID
		if e.ParentIsRoot {
			consumer = 0
		}
		pair := [2]int64{e.ChildComponentID, consumer}
		if seen[pair] {
			continue
		}
		seen[pair] = true
		at[e.ChildComponentID] = append(at[e.ChildComponentID], consumer)
	}
	return at, nil
}

// optional turns a consumer into something that can be absent.
func optional(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

// value reads a consumer that may be absent.
func value(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// sameDate compares two dates that may be absent.
//
// Absent and present are different, and two absences are the same — the
// distinction matters because this decides whether a finding changed, and a
// finding that appears to change every scan writes rows every night.
func sameDate(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Equal(*b)
	}
}

// shippedToCustomers reports whether the build a scan is for reaches
// customers.
//
// Read from the variant, because that is where it is recorded: what a product
// is built as is what decides whether anybody outside runs it.
func shippedToCustomers(ctx context.Context, tx bun.Tx, targetID int64) (bool, error) {
	var shipped bool
	err := tx.NewSelect().
		TableExpr("target AS t").
		Join("JOIN variant AS v ON v.id = t.variant_id").
		Column("v.customer_facing").
		Where("t.id = ?", targetID).
		Scan(ctx, &shipped)
	if err != nil {
		return false, fmt.Errorf("read whether this build reaches customers: %w", err)
	}
	return shipped, nil
}

// scoreOf reads the severity of an issue as a number.
//
// The number where a report gives one, and the word standing in where it does
// not — so a finding rated only in words does not sort below everything rated
// at all.
func scoreOf(issue Named) int {
	if issue.Score > 0 {
		return int(issue.Score * 100)
	}
	return SeverityScore(issue.Severity)
}
