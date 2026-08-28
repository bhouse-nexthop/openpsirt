package graph

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
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
	VariantID    int64  `bun:"variant_id,notnull"`
	ComponentID  int64  `bun:"component_id,notnull"`
	IsRoot       bool   `bun:"is_root,notnull"`
	OpenedScanID int64  `bun:"opened_scan_id,notnull"`
	ClosedScanID *int64 `bun:"closed_scan_id"`
}

// Edge is one component depending on another.
type Edge struct {
	bun.BaseModel `bun:"table:graph_edge,alias:e"`

	ID           int64  `bun:"id,pk,autoincrement"`
	VariantID    int64  `bun:"variant_id,notnull"`
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
func (s *Store) Apply(ctx context.Context, variantID, scanID int64, snap Snapshot) (Applied, error) {
	var applied Applied

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		components := NewComponents(tx)

		all := append([]Described{snap.Root}, snap.Components...)
		for _, dep := range snap.Dependencies {
			all = append(all, dep.Parent, dep.Child)
		}
		ids, err := components.Intern(ctx, all)
		if err != nil {
			return err
		}

		wanted := map[int64]bool{} // component id -> is root
		wanted[ids[snap.Root.Identity()]] = true
		for _, d := range snap.Components {
			id := ids[d.Identity()]
			if _, already := wanted[id]; !already {
				wanted[id] = false
			}
		}

		nodeIDs, opened, closed, err := reconcileNodes(ctx, tx, variantID, scanID, wanted)
		if err != nil {
			return err
		}
		applied.NodesOpened, applied.NodesClosed = opened, closed

		wantedEdges := map[[2]int64]bool{}
		for _, dep := range snap.Dependencies {
			parent, okP := nodeIDs[ids[dep.Parent.Identity()]]
			child, okC := nodeIDs[ids[dep.Child.Identity()]]
			if !okP || !okC {
				return fmt.Errorf("dependency names a component the snapshot does not list: %s -> %s",
					dep.Parent.Name, dep.Child.Name)
			}
			wantedEdges[[2]int64{parent, child}] = true
		}

		applied.EdgesOpened, applied.EdgesClosed, err = reconcileEdges(ctx, tx, variantID, scanID, wantedEdges)
		return err
	})
	return applied, err
}

// DB exposes the underlying handle for queries this package does not wrap.
func (s *Store) DB() *bun.DB { return s.db }

// CurrentNodes returns the components present in a variant now.
func (s *Store) CurrentNodes(ctx context.Context, variantID int64) ([]Node, error) {
	var nodes []Node
	err := s.db.NewSelect().Model(&nodes).
		Where("variant_id = ?", variantID).
		Where("closed_scan_id IS NULL").
		Scan(ctx)
	return nodes, err
}

// CurrentEdges returns the dependencies present in a variant now.
func (s *Store) CurrentEdges(ctx context.Context, variantID int64) ([]Edge, error) {
	var edges []Edge
	err := s.db.NewSelect().Model(&edges).
		Where("variant_id = ?", variantID).
		Where("closed_scan_id IS NULL").
		Scan(ctx)
	return edges, err
}
