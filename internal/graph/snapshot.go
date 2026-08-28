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

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
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

// CurrentEdges returns the dependencies present in a variant now.
func (s *Store) CurrentEdges(ctx context.Context, targetID int64) ([]Edge, error) {
	var edges []Edge
	err := s.db.NewSelect().Model(&edges).
		Where("target_id = ?", targetID).
		Where("closed_scan_id IS NULL").
		Scan(ctx)
	return edges, err
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
