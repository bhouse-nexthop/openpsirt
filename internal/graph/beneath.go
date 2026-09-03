package graph

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// beneath counts the distinct issues open under each of these components,
// including on them.
//
// Worked out when the tree is read rather than stored after a scan. Storing it
// was the first attempt and it is the wrong shape: the number is derived from
// findings, and findings move between scans — something dismissed, something
// assigned, a rating reconsidered. A stored total is right at the moment a
// scan ends and drifts from the screen beside it thereafter, which is a screen
// quoting yesterday's answer with nothing saying so.
//
// Affordable because the graph is small. Measured on a real image: 19,192
// edges, 6,836 placed components, three levels deep. Two queries and a walk in
// memory, on a screen that already reads thousands of rows.
//
// **Distinct issues, not finding rows.** A finding is one issue at one place,
// and a library at thirty-six places with two issues is seventy-two rows —
// which is what every parent it sat beneath used to read, where somebody who
// drilled down one path is looking at one place and expects two. The issues
// open against a component are the same at every place it sits, so counting
// them once per component answers per path without a walk per path. And
// **counted over distinct components, not summed along edges**: a library
// reached by twenty containers is one thing inside each of them.
func (s *Store) beneath(ctx context.Context, targetID int64, visible []access.Visibility,
	of []int64) (map[int64]int, error) {

	totals := map[int64]int{}
	if len(of) == 0 {
		return totals, nil
	}
	kids, err := s.edges(ctx, targetID)
	if err != nil {
		return nil, err
	}

	var pairs []struct {
		ComponentID     int64 `bun:"component_id"`
		VulnerabilityID int64 `bun:"vulnerability_id"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("f.vulnerability_id AS vulnerability_id").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		// Narrowed exactly as every other count here is. A total is as much a
		// disclosure as a row: "there are six under this" tells a reader there
		// are six.
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.component_id, f.vulnerability_id").
		Scan(ctx, &pairs)
	if err != nil {
		return nil, fmt.Errorf("read what is open against each component: %w", err)
	}
	own := map[int64][]int64{}
	for _, pair := range pairs {
		own[pair.ComponentID] = append(own[pair.ComponentID], pair.VulnerabilityID)
	}
	for _, id := range of {
		issues := map[int64]bool{}
		for _, component := range subtree(id, kids) {
			for _, issue := range own[component] {
				issues[issue] = true
			}
		}
		totals[id] = len(issues)
	}
	return totals, nil
}

// edges reads the build's open edges as parent to children, by component.
func (s *Store) edges(ctx context.Context, targetID int64) (map[int64][]int64, error) {
	var edges []struct {
		Parent int64 `bun:"parent"`
		Child  int64 `bun:"child"`
	}
	err := s.db.NewSelect().
		TableExpr("graph_edge AS e").
		Join("JOIN graph_node AS pn ON pn.id = e.parent_id").
		Join("JOIN graph_node AS cn ON cn.id = e.child_id").
		ColumnExpr("pn.component_id AS parent").
		ColumnExpr("cn.component_id AS child").
		Where("e.target_id = ?", targetID).
		Where("e.closed_scan_id IS NULL").
		Scan(ctx, &edges)
	if err != nil {
		return nil, fmt.Errorf("read the build's edges: %w", err)
	}
	kids := map[int64][]int64{}
	for _, edge := range edges {
		if edge.Parent != edge.Child {
			kids[edge.Parent] = append(kids[edge.Parent], edge.Child)
		}
	}
	return kids, nil
}

// Subtree lists a component and everything beneath it in a build, each once
// however many ways it is reached, in one pass over the build's edges.
//
// What the findings list narrows by when asked for everything the tree's
// number counts, so the two agree.
func (s *Store) Subtree(ctx context.Context, targetID, componentID int64) ([]int64, error) {
	kids, err := s.edges(ctx, targetID)
	if err != nil {
		return nil, err
	}
	return subtree(componentID, kids), nil
}

// subtree walks down from a component, listing it and everything under it
// once.
//
// A cycle would be a fault in the document rather than something to reason
// about, so it is survived rather than diagnosed: a component already seen on
// the way down is not descended into again.
func subtree(from int64, kids map[int64][]int64) []int64 {
	seen := map[int64]bool{from: true}
	stack := []int64{from}
	out := []int64{}
	for len(stack) > 0 {
		at := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, at)
		for _, kid := range kids[at] {
			if !seen[kid] {
				seen[kid] = true
				stack = append(stack, kid)
			}
		}
	}
	return out
}
