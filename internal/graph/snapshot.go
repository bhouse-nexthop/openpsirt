package graph

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/uptrace/bun"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
)

// Node is a component's presence in a variant.
//
// The graph is a graph, not a tree. A component reached by several parents is
// one node with several edges, not one node per route. Enumerating routes is a
// separate question, left until real data can say how much a real graph
// shares.
type Node struct {
	bun.BaseModel `bun:"table:graph_node,alias:n"`

	ID           int64  `bun:"id,pk,autoincrement"`
	TargetID     int64  `bun:"target_id,notnull"`
	ComponentID  int64  `bun:"component_id,notnull"`
	IsRoot       bool   `bun:"is_root,notnull"`
	OpenedScanID int64  `bun:"opened_scan_id,notnull"`
	ClosedScanID *int64 `bun:"closed_scan_id"`
}

// Edge is one component depending on another.
type Edge struct {
	bun.BaseModel `bun:"table:graph_edge,alias:e"`

	ID           int64  `bun:"id,pk,autoincrement"`
	TargetID     int64  `bun:"target_id,notnull"`
	ParentID     int64  `bun:"parent_id,notnull"`
	ChildID      int64  `bun:"child_id,notnull"`
	OpenedScanID int64  `bun:"opened_scan_id,notnull"`
	ClosedScanID *int64 `bun:"closed_scan_id"`
}

// Snapshot is the graph a single scan describes.
type Snapshot struct {
	// Components are every component the scan mentions.
	Components []Described
	// Root is the component the scan is about — the product itself. It is
	// marked so it can be excluded from anything that walks upwards: its
	// version changes on every build and its name differs per variant, so
	// letting it into an identity or an expiry rule would invalidate
	// everything below it on every rebuild.
	Root Described
	// Dependencies say which component depends on which, by identity.
	Dependencies []Dependency
}

// Dependency is one edge, named by component identity rather than by row.
type Dependency struct{ Parent, Child Described }

// Applied describes what a snapshot changed.
type Applied struct {
	NodesOpened int
	NodesClosed int
	EdgesOpened int
	EdgesClosed int
}

// Unchanged reports whether the snapshot changed nothing.
func (a Applied) Unchanged() bool {
	return a.NodesOpened == 0 && a.NodesClosed == 0 && a.EdgesOpened == 0 && a.EdgesClosed == 0
}

// Store writes graphs.
type Store struct {
	db         *bun.DB
	components *Components
}

// NewStore returns a graph store over db.
func NewStore(db *bun.DB) *Store {
	return &Store{db: db, components: NewComponents(db)}
}

// Apply records a scan's graph against a variant.
//
// Only differences are written. A nightly rebuild that changed nothing writes
// nothing at all — no rows, no history, no growth. That is what keeps the
// stored volume tracking real change rather than the calendar, and it is
// checked by a test rather than assumed.
//
// Everything happens in one transaction: a half-applied graph is
// indistinguishable from components having been removed, which would close
// findings that are still present.
func (s *Store) Apply(ctx context.Context, targetID, scanID int64, snap Snapshot) (Applied, error) {
	var applied Applied

	err := database.InTransaction(ctx, s.db, func(ctx context.Context, tx bun.Tx) error {
		// Taken first, before anything is read. Two scans of one target can be
		// in flight at once — the queue hands different jobs to different
		// workers by design — and without this both would read the same open
		// rows, both compute the same difference, and both write it, leaving
		// two open rows where everything downstream assumes one. This is an
		// ordinary row update, so every engine takes the lock and the second
		// worker waits rather than racing.
		if _, err := tx.NewUpdate().Table("target").
			Set("last_scan_id = ?", scanID).
			Where("id = ?", targetID).Exec(ctx); err != nil {
			return fmt.Errorf("take the target: %w", err)
		}

		components := NewComponents(tx)

		// The product itself is stored without the version that moves every
		// build. Everything the document said about it — including every edge
		// hanging off it — has to resolve to that one row, or the version
		// arrives again through the edges and the churn it causes with it.
		root := snap.Root.AsRoot()
		described := snap.Root.Identity()
		asStored := func(d Described) Described {
			if d.Identity() == described {
				return root
			}
			return d
		}

		all := append([]Described{root}, snap.Components...)
		for _, dep := range snap.Dependencies {
			all = append(all, asStored(dep.Parent), asStored(dep.Child))
		}
		ids, err := components.Intern(ctx, all)
		if err != nil {
			return err
		}

		wanted := map[int64]bool{} // component id -> is root
		wanted[ids[root.Identity()]] = true
		for _, d := range snap.Components {
			id := ids[d.Identity()]
			if _, already := wanted[id]; !already {
				wanted[id] = false
			}
		}

		nodeIDs, opened, closed, err := reconcileNodes(ctx, tx, targetID, scanID, wanted)
		if err != nil {
			return err
		}
		applied.NodesOpened, applied.NodesClosed = opened, closed

		wantedEdges := map[[2]int64]bool{}
		for _, dep := range snap.Dependencies {
			parent, okP := nodeIDs[ids[asStored(dep.Parent).Identity()]]
			child, okC := nodeIDs[ids[asStored(dep.Child).Identity()]]
			if !okP || !okC {
				return fmt.Errorf("dependency names a component the snapshot does not list: %s -> %s",
					dep.Parent.Name, dep.Child.Name)
			}
			wantedEdges[[2]int64{parent, child}] = true
		}

		applied.EdgesOpened, applied.EdgesClosed, err = reconcileEdges(ctx, tx, targetID, scanID, wantedEdges)
		return err
	})
	return applied, err
}

// DB exposes the underlying handle for queries this package does not wrap.
func (s *Store) DB() *bun.DB { return s.db }

// CurrentNodes returns the components present in a variant now.
func (s *Store) CurrentNodes(ctx context.Context, targetID int64) ([]Node, error) {
	var nodes []Node
	err := s.db.NewSelect().Model(&nodes).
		Where("target_id = ?", targetID).
		Where("closed_scan_id IS NULL").
		Scan(ctx)
	return nodes, err
}

// CurrentComponents returns what a target contains now, as the scanner needs
// to be given it.
//
// The scanner is fed from here rather than from the file a build sent, because
// that file is not kept for a moving line: a nightly scan is superseded the
// next night. Re-scanning a year-old release against today's vulnerability
// data works from this, which is why what is not stored can never be scanned.
func (s *Store) CurrentComponents(ctx context.Context, targetID int64) ([]Described, error) {
	var rows []Component
	err := s.db.NewSelect().Model(&rows).
		Join("JOIN graph_node AS n ON n.component_id = c.id").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Where("n.is_root = ?", false).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("read what a target contains: %w", err)
	}

	described := make([]Described, 0, len(rows))
	for _, row := range rows {
		described = append(described, Described{
			Purl: row.Purl, CPE: row.CPE, Name: row.Name, Version: row.Version,
			UpstreamName: row.UpstreamName, UpstreamVersion: row.UpstreamVersion,
		})
	}
	return described, nil
}

// ComponentAt resolves a component by name within one build.
//
// By name, because that is what a findings list gives out and what somebody
// composing a request has. Scoped to the build so the name means what it means
// there: two products can ship different things under one name, and a lookup
// across everything would answer with whichever was interned first.
func (s *Store) ComponentAt(ctx context.Context, targetID int64, name string) (int64, error) {
	return s.ComponentVersionAt(ctx, targetID, name, "")
}

// ErrAmbiguous says a name matched more than one component and no version was
// given to tell them apart.
var ErrAmbiguous = errors.New("this build contains that name at more than one version")

// ComponentVersionAt resolves a component by name and, where one is given,
// version.
//
// **A name is not unique within a build**, and not rarely. This was written
// assuming it nearly always was, resolving a collision by taking the lowest
// identifier — stable between requests, which was the property being protected.
// A real switch image then shipped three vendored versions of one library, and
// every one of them resolved to the first: two of the three findings answered
// "no such finding" for a row the list had just drawn, and the third answered
// about a version nobody asked about.
//
// So the version narrows it where the caller has one, and an ambiguous name
// with no version is an error rather than a guess. A caller that guesses on
// behalf of somebody is worse than one that says it cannot tell.
func (s *Store) ComponentVersionAt(ctx context.Context, targetID int64, name, version string) (int64, error) {
	query := s.db.NewSelect().
		TableExpr("graph_node AS n").
		Join("JOIN component AS c ON c.id = n.component_id").
		ColumnExpr("c.id").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Where("c.name = ?", name).
		OrderExpr("c.id")
	if version != "" {
		query = query.Where("c.version = ?", version)
	}

	var ids []int64
	if err := query.Scan(ctx, &ids); err != nil {
		return 0, fmt.Errorf("look up component %q: %w", name, err)
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("this build contains no component called %q", name)
	}
	if len(ids) > 1 {
		// Only reachable without a version: with one, the name and version
		// together identify a component.
		return 0, fmt.Errorf("%w: %q", ErrAmbiguous, name)
	}
	return ids[0], nil
}

// Neighbour is one component next to another in the graph.
type Neighbour struct {
	Name    string `bun:"name"`
	Version string `bun:"version"`
	// Findings is what is open against this component itself, and Beneath is
	// that plus everything under it.
	//
	// Both, because they answer different questions and a container answers
	// zero to the first: it holds no findings of its own, so a tree showing
	// only that says every container is clean while the packages inside them
	// hold thousands. What makes a branch worth opening is what is in it.
	Findings int `bun:"findings"`
	Beneath  int `bun:"beneath,scanonly"`
	Children int `bun:"children"`
	// ComponentID is what the rollup is worked out from. Read rather than
	// shown.
	ComponentID int64 `bun:"component_id"`
}

// Around reports what sits directly above and below one component in a build.
//
// A neighbourhood rather than a tree. Eight thousand components will not draw
// and would not be readable if they did, so what is asked for is one step at a
// time — and the counts come with it, because descending a graph without them
// is exploring rather than following anything.
//
// Naming a component rather than an identifier: it is what a findings list
// gives out and what somebody composing a request has.
func (s *Store) Around(ctx context.Context, subject access.Subject, targetID int64,
	name string) ([]Neighbour, []Neighbour, error) {

	componentID, err := s.ComponentAt(ctx, targetID, name)
	if err != nil {
		return nil, nil, err
	}
	below, err := s.step(ctx, subject, targetID, componentID, true)
	if err != nil {
		return nil, nil, err
	}
	above, err := s.step(ctx, subject, targetID, componentID, false)
	if err != nil {
		return nil, nil, err
	}
	return above, below, nil
}

// Roots reports the build's own component and what it pulls in directly.
//
// The root comes back with its children because a tree drawn without it is
// drawn without the thing being explored: every path shown starts one step in,
// and the indentation has nothing to hang from. It is nil where the document
// named no root of its own.
func (s *Store) Roots(ctx context.Context, subject access.Subject, targetID int64) (
	*Neighbour, []Neighbour, error) {

	var rootID int64
	err := s.db.NewSelect().
		TableExpr("graph_node AS n").
		ColumnExpr("n.component_id").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Where("n.is_root = ?", true).
		Limit(1).Scan(ctx, &rootID)
	if database.IsNoRows(err) {
		// A build whose document named no root of its own. Nothing is wrong
		// and there is simply nothing above the components.
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("look up what this build is: %w", err)
	}

	kids, err := s.step(ctx, subject, targetID, rootID, true)
	if err != nil {
		return nil, nil, err
	}
	root, err := s.describe(ctx, subject, targetID, rootID, len(kids))
	if err != nil {
		return nil, nil, err
	}
	return root, kids, nil
}

// describe reads one component as a neighbour of nothing, for the root.
//
// The child count is passed in rather than counted again: the caller has just
// walked that edge and holds the answer, and asking the database a second time
// for a number already in hand is how the same walk ends up costing twice.
func (s *Store) describe(ctx context.Context, subject access.Subject, targetID, componentID int64,
	children int) (*Neighbour, error) {

	readable, err := s.visibleIn(ctx, subject, targetID)
	if err != nil {
		return nil, err
	}

	row := &Neighbour{Children: children, ComponentID: componentID}
	err = s.db.NewSelect().
		TableExpr("component AS c").
		ColumnExpr("c.name AS name").
		ColumnExpr("c.version AS version").
		// Narrowed exactly as the neighbours are. The root is one row, but a
		// count that is not narrowed the same way is still a count of what the
		// reader may not see.
		ColumnExpr(`(SELECT COUNT(*) FROM "finding" AS f
			WHERE f.target_id = ? AND f.component_id = c.id
			  AND f.closed_run_id IS NULL AND f.visibility IN (?)) AS findings`,
			targetID, bun.List(readable)).
		Where("c.id = ?", componentID).
		Scan(ctx, row)
	if database.IsNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read what this build is: %w", err)
	}
	totals, err := s.beneath(ctx, targetID, readable, []int64{componentID})
	if err != nil {
		return nil, err
	}
	row.Beneath = totals[componentID]
	return row, nil
}

// step walks one edge, downward or upward.
//
// Edges join *nodes* rather than components — a node is a component's presence
// in one build — so both ends go through the node table. Reading the edge
// columns as component identifiers is the mistake this comment exists to stop
// somebody making twice.
func (s *Store) step(ctx context.Context, subject access.Subject, targetID, componentID int64,
	down bool) ([]Neighbour, error) {
	near, far := "parent_id", "child_id"
	if !down {
		near, far = far, near
	}

	readable, err := s.visibleIn(ctx, subject, targetID)
	if err != nil {
		return nil, err
	}

	var rows []Neighbour
	err = s.db.NewSelect().
		TableExpr("graph_edge AS e").
		Join("JOIN graph_node AS nn ON nn.id = e."+near).
		Join("JOIN graph_node AS fn ON fn.id = e."+far).
		Join("JOIN component AS c ON c.id = fn.component_id").
		Join(`LEFT JOIN (SELECT dp.component_id AS cid, COUNT(*) AS n
			FROM "graph_edge" AS d
			JOIN "graph_node" AS dp ON dp.id = d.parent_id
			WHERE d.target_id = ? AND d.closed_scan_id IS NULL
			GROUP BY dp.component_id) AS kids ON kids.cid = c.id`, targetID).
		ColumnExpr("c.id AS component_id").
		ColumnExpr("c.name AS name").
		ColumnExpr("c.version AS version").
		// What is open against it here, so descending follows the findings
		// rather than being exploration.
		// Narrowed like every other count. Without this a reader browsing the
		// tree gets an accurate count of the undisclosed findings under each
		// component and can bisect down to which one holds them — a leak that
		// needs no row to be shown.
		ColumnExpr(`(SELECT COUNT(*) FROM "finding" AS f
			WHERE f.target_id = ? AND f.component_id = c.id
			  AND f.closed_run_id IS NULL AND f.visibility IN (?)) AS findings`,
			targetID, bun.List(readable)).
		// Whether anything is under it, so a node that opens can be told from
		// one that does not before somebody clicks it.
		//
		// Counted once for the whole build and joined, rather than asked per
		// row. As a correlated subquery this has no index to take: it is bound
		// on the child's component, while the only way into the edge table is
		// the target, so each row scanned every edge in the build. Measured on
		// a switch operating-system image — 19,192 edges, 5,270 components
		// directly under the root — that column alone cost 5.06 s against
		// 0.106 s for one pass. An index on the node's component was tried
		// first and made it worse (5.4 s to 10.0 s), because the scan being
		// repeated is over the edges rather than the lookup it drives.
		ColumnExpr("COALESCE(kids.n, 0) AS children").
		Where("e.target_id = ?", targetID).
		Where("nn.component_id = ?", componentID).
		Where("e.closed_scan_id IS NULL").
		// kids.n is grouped on as well as selected. It is one value per c.id
		// and so adds nothing, but the engines that enforce the rule strictly
		// will not take a column from a joined subquery on the strength of a
		// primary key belonging to a different table.
		GroupExpr("c.id, c.name, c.version, kids.n").
		// What opens comes before what does not, and within each the most
		// findings first.
		//
		// Ordering by findings alone buries the structure: a container holds
		// no findings of its own, so on a real image the root's 5,270
		// children put the first thing that opens at position 546 and every
		// one of the 37 containers below that. A tree whose first screen
		// contains no branches is a list, and the reader never learns the
		// build has containers in it at all.
		OrderExpr("CASE WHEN COALESCE(kids.n, 0) > 0 THEN 0 ELSE 1 END, findings DESC, c.name").
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("walk the graph: %w", err)
	}

	// What is in each of them, not only what is on it. A container holds no
	// findings of its own, so without this every one of them reads zero while
	// the packages inside hold thousands — and a tree whose counts cannot tell
	// a full branch from an empty one is not something anybody can descend by.
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ComponentID)
	}
	totals, err := s.beneath(ctx, targetID, readable, ids)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Beneath = totals[rows[i].ComponentID]
	}

	// What opens comes first, then leaves by what is open against them.
	//
	// **Branches are ordered by name, not by what is beneath them**, and that
	// is deliberate. Ordering by the rollup was tried and made the screen
	// worse: an edge here means "contains or depends on", the two are not
	// distinguished in the document, and forty kernel-module packages each
	// depend on the one kernel — so every one of them reported the kernel's
	// 425,098 and they filled the first screen, putting the containers back
	// out of sight. That is the defect this ordering was changed to fix,
	// arriving from the other side.
	//
	// A leaf has nothing under it, so for leaves the rollup and the count are
	// the same number and ordering by it follows the findings as it should.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Children > 0) != (b.Children > 0) {
			return a.Children > 0
		}
		if a.Children == 0 && a.Findings != b.Findings {
			return a.Findings > b.Findings
		}
		return a.Name < b.Name
	})
	return rows, nil
}

// visibleIn reports the visibilities this subject may read in a build, and
// refuses where they may read none.
//
// The graph is browsed beside a findings list that is narrowed correctly, so a
// count here that is not narrowed the same way is the more dangerous of the
// two: nobody looking at it expects it to be a disclosure.
func (s *Store) visibleIn(ctx context.Context, subject access.Subject, targetID int64) ([]access.Visibility, error) {
	var productID int64
	err := s.db.NewSelect().
		TableExpr("target AS tg").
		Join("JOIN stream AS st ON st.id = tg.stream_id").
		ColumnExpr("st.product_id").
		Where("tg.id = ?", targetID).
		Scan(ctx, &productID)
	if err != nil {
		return nil, fmt.Errorf("look up which product this build belongs to: %w", err)
	}
	if !subject.Sees(productID) {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	readable := []access.Visibility{}
	if subject.Reads(access.Public, productID) {
		readable = append(readable, access.Public)
	}
	if subject.Reads(access.Private, productID) {
		readable = append(readable, access.Private)
	}
	if len(readable) == 0 {
		return nil, access.Denied(fmt.Sprintf("read findings in product %d", productID))
	}
	return readable, nil
}

// Step is one component on the way down to another.
type Step struct {
	Name    string
	Version string
}

// Chains returns the way down to each of these components, root first.
//
// `UIX-14` asks a finding to show the complete chain, root to component, with
// the version at each step, because that is where somebody judges whether the
// vulnerable code is reached. The immediate parent alone cannot answer it:
// where a component is reached several ways the parent is often the same word
// twice, and two identical rows are not two answers.
//
// Read as one pass over the build's edges rather than as a query per step.
// A walk upward is only two or three hops deep here — the graph an SBOM
// describes is mostly containment — but it is a hop per query per place, and a
// finding can sit at sixty. One pass over nineteen thousand edges is cheaper
// than that and does not grow with how widely a library is shared.
//
// Where a component is reached several ways, the shortest way down is the one
// returned. A path is being shown to explain a position rather than to
// enumerate the graph, and the shortest is the one somebody can hold in mind.
func (s *Store) Chains(ctx context.Context, subject access.Subject, targetID int64,
	componentIDs []int64) (map[int64][]Step, error) {

	chains := map[int64][]Step{}
	if len(componentIDs) == 0 {
		return chains, nil
	}
	if _, err := s.visibleIn(ctx, subject, targetID); err != nil {
		return nil, err
	}

	var rootID int64
	err := s.db.NewSelect().
		TableExpr("graph_node AS n").
		ColumnExpr("n.component_id").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Where("n.is_root = ?", true).
		Limit(1).Scan(ctx, &rootID)
	if database.IsNoRows(err) {
		return chains, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up what this build is: %w", err)
	}

	var edges []struct {
		Parent int64 `bun:"parent"`
		Child  int64 `bun:"child"`
	}
	err = s.db.NewSelect().
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

	parents := map[int64][]int64{}
	for _, edge := range edges {
		if edge.Parent != edge.Child {
			parents[edge.Child] = append(parents[edge.Child], edge.Parent)
		}
	}

	// Breadth-first upward, so the first way out is the shortest. Every
	// component asked about is walked separately because they rarely share a
	// route, and a shared answer would have to be recomputed anyway once the
	// paths diverge.
	routes := map[int64][]int64{}
	needed := map[int64]bool{rootID: true}
	for _, id := range componentIDs {
		if _, done := routes[id]; done {
			continue
		}
		route := climb(id, rootID, parents)
		if route == nil {
			continue
		}
		routes[id] = route
		for _, step := range route {
			needed[step] = true
		}
	}
	if len(routes) == 0 {
		return chains, nil
	}

	ids := make([]int64, 0, len(needed))
	for id := range needed {
		ids = append(ids, id)
	}
	var named []Component
	if err := s.db.NewSelect().Model(&named).
		Column("id", "name", "version").
		Where("id IN (?)", bun.List(ids)).Scan(ctx); err != nil {
		return nil, fmt.Errorf("read what is on the way down: %w", err)
	}
	words := map[int64]Step{}
	for _, one := range named {
		words[one.ID] = Step{Name: one.Name, Version: one.Version}
	}

	for id, route := range routes {
		chain := make([]Step, 0, len(route))
		for _, step := range route {
			if word, ok := words[step]; ok {
				chain = append(chain, word)
			}
		}
		chains[id] = chain
	}
	return chains, nil
}

// climb walks upward from one component to the root and returns the way back
// down, root first and the component itself last. Nil where the root is not
// reachable, which a build whose inventory left a component unplaced will have.
func climb(from, root int64, parents map[int64][]int64) []int64 {
	if from == root {
		return []int64{root}
	}
	came := map[int64]int64{from: 0}
	queue := []int64{from}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, up := range parents[at] {
			if _, seen := came[up]; seen {
				continue
			}
			came[up] = at
			if up == root {
				// Unwound downward: the map points at where each step was
				// reached from, which walking back is the way down.
				route := []int64{root}
				for step := at; ; step = came[step] {
					route = append(route, step)
					if step == from {
						break
					}
				}
				return route
			}
			queue = append(queue, up)
		}
	}
	return nil
}

// Counts reports how much the build's graph holds, which is what the screen
// showing it says at the top.
//
// Two numbers rather than one because they answer different questions: how
// much was inventoried, and how much of it was placed. A build with many
// components and few edges is a document that listed everything and said
// where almost nothing went.
func (s *Store) Counts(ctx context.Context, subject access.Subject, targetID int64) (int, int, error) {
	if _, err := s.visibleIn(ctx, subject, targetID); err != nil {
		return 0, 0, err
	}
	components, err := s.db.NewSelect().
		TableExpr("graph_node AS n").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		Count(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("count what this build holds: %w", err)
	}
	edges, err := s.db.NewSelect().
		TableExpr("graph_edge AS e").
		Where("e.target_id = ?", targetID).
		Where("e.closed_scan_id IS NULL").
		Count(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("count what this build's edges are: %w", err)
	}
	return components, edges, nil
}

// Search finds components of a build by name.
//
// Opening nodes does not find anything in a graph this size — eight thousand
// components under a root with five thousand children — so searching is the
// way in and browsing is for answering "what else is under this" once somebody
// is already somewhere. A tree without it is a tree nobody reaches the middle
// of.
//
// Matched on a substring of the name, case-folded, because somebody looking
// for openssl types "ssl" and should not have to know whether the package is
// called openssl, libssl3t64 or symcrypt-openssl. Ordered by findings so the
// first answer is the one worth opening.
func (s *Store) Search(ctx context.Context, subject access.Subject, targetID int64,
	term string, limit int) ([]Neighbour, error) {

	readable, err := s.visibleIn(ctx, subject, targetID)
	if err != nil {
		return nil, err
	}
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var rows []Neighbour
	err = s.db.NewSelect().
		TableExpr("graph_node AS n").
		Join("JOIN component AS c ON c.id = n.component_id").
		Join(`LEFT JOIN (SELECT dp.component_id AS cid, COUNT(*) AS n
			FROM "graph_edge" AS d
			JOIN "graph_node" AS dp ON dp.id = d.parent_id
			WHERE d.target_id = ? AND d.closed_scan_id IS NULL
			GROUP BY dp.component_id) AS kids ON kids.cid = c.id`, targetID).
		ColumnExpr("c.name AS name").
		ColumnExpr("c.version AS version").
		ColumnExpr(`(SELECT COUNT(*) FROM "finding" AS f
			WHERE f.target_id = ? AND f.component_id = c.id
			  AND f.closed_run_id IS NULL AND f.visibility IN (?)) AS findings`,
			targetID, bun.List(readable)).
		ColumnExpr("COALESCE(kids.n, 0) AS children").
		Where("n.target_id = ?", targetID).
		Where("n.closed_scan_id IS NULL").
		// LOWER on both sides rather than a case-insensitive comparison,
		// which two of the four engines spell differently and one of them
		// decides by collation.
		Where("LOWER(c.name) LIKE ?", "%"+strings.ToLower(term)+"%").
		GroupExpr("c.id, c.name, c.version, kids.n").
		OrderExpr("findings DESC, c.name").
		Limit(limit).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("search the build: %w", err)
	}
	return rows, nil
}
