package graph

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
)

// beneath totals what is open under each of these components, including on
// them.
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
// **Counted over distinct components, not summed along edges.** A library
// reached by twenty containers is one thing inside each of them, and adding up
// the children of a node that reaches it several ways would report it several
// times.
func (s *Store) beneath(ctx context.Context, targetID int64, visible []access.Visibility,
	of []int64) (map[int64]int, error) {

	totals := map[int64]int{}
	if len(of) == 0 {
		return totals, nil
	}

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

	var counts []struct {
		ComponentID int64 `bun:"component_id"`
		Open        int   `bun:"open"`
	}
	err = s.db.NewSelect().
		TableExpr("finding AS f").
		ColumnExpr("f.component_id AS component_id").
		ColumnExpr("COUNT(*) AS open").
		Where("f.target_id = ?", targetID).
		Where("f.closed_run_id IS NULL").
		// Narrowed exactly as every other count here is. A total is as much a
		// disclosure as a row: "there are six under this" tells a reader there
		// are six.
		Where("f.visibility IN (?)", bun.List(visible)).
		GroupExpr("f.component_id").
		Scan(ctx, &counts)
	if err != nil {
		return nil, fmt.Errorf("read what is open against each component: %w", err)
	}

	kids := map[int64][]int64{}
	for _, edge := range edges {
		if edge.Parent != edge.Child {
			kids[edge.Parent] = append(kids[edge.Parent], edge.Child)
		}
	}
	own := make(map[int64]int, len(counts))
	for _, row := range counts {
		own[row.ComponentID] = row.Open
	}
	for _, id := range of {
		totals[id] = reach(id, kids, own)
	}
	return totals, nil
}

// reach totals what is open on a component and everything under it, counting
// each component once however many ways it is reached.
//
// A cycle would be a fault in the document rather than something to reason
// about, so it is survived rather than diagnosed: a component already counted
// on the way down is not descended into again.
func reach(from int64, kids map[int64][]int64, own map[int64]int) int {
	seen := map[int64]bool{from: true}
	stack := []int64{from}
	total := 0
	for len(stack) > 0 {
		at := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		total += own[at]
		for _, kid := range kids[at] {
			if !seen[kid] {
				seen[kid] = true
				stack = append(stack, kid)
			}
		}
	}
	return total
}
