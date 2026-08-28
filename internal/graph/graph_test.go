package graph_test

import (
	"fmt"
	"io"
	"log/slog"
	"maps"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalogue"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// fixture is one migrated database with a variant to file scans against, and a
// way to mint scans in order.
type fixture struct {
	store     *graph.Store
	scans     *ingest.Store
	variantID int64
	built     time.Time
	seq       int
}

// scan records a new scan, each newer than the last, and returns its
// identifier.
func (f *fixture) scan(t *testing.T) int64 {
	t.Helper()
	f.seq++
	f.built = f.built.Add(time.Hour)
	rec, outcome, err := f.scans.Record(t.Context(), ingest.Arriving{
		VariantID: f.variantID, ContentHash: fmt.Sprintf("hash-%d", f.seq),
		BuiltAt: f.built, ParserVersion: "test",
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record scan %d: outcome %v, err %v", f.seq, outcome, err)
	}
	return rec.ID
}

func each(t *testing.T, fn func(t *testing.T, f *fixture)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		cat := catalogue.NewStore(db.DB)
		p, err := cat.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		br, err := cat.DeclareStream(ctx, p.ID, "release-2.4", catalogue.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		v, err := cat.DeclareVariant(ctx, br.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}
		fn(t, &fixture{
			store: graph.NewStore(db.DB), scans: ingest.NewStore(db.DB),
			variantID: v.ID, built: time.Now().UTC().Add(-48 * time.Hour),
		})
	})
}

func at(name, version string) graph.Described {
	return graph.Described{
		Purl: "pkg:generic/" + name + "@" + version, Name: name, Version: version,
	}
}

var (
	root    = at("sonic", "2.4.0")
	openssl = at("openssl", "3.0.11")
	curl    = at("curl", "8.4.0")
	zlib    = at("zlib", "1.3")
)

// tree is the graph used by most of these tests: the product depends on curl
// and openssl, and curl depends on openssl too. openssl sits at two places and
// is still one node — the graph is a graph.
func tree() graph.Snapshot {
	return graph.Snapshot{
		Root:       root,
		Components: []graph.Described{openssl, curl},
		Dependencies: []graph.Dependency{
			{Parent: root, Child: curl},
			{Parent: root, Child: openssl},
			{Parent: curl, Child: openssl},
		},
	}
}

func TestFirstSnapshotIsStored(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		applied, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree())
		if err != nil {
			t.Fatal(err)
		}
		if applied.NodesOpened != 3 || applied.EdgesOpened != 3 {
			t.Errorf("opened %+v, want 3 nodes and 3 edges", applied)
		}
		if applied.NodesClosed != 0 || applied.EdgesClosed != 0 {
			t.Errorf("closed something on a first snapshot: %+v", applied)
		}

		nodes, err := f.store.CurrentNodes(t.Context(), f.variantID)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 3 {
			t.Fatalf("%d nodes present, want 3", len(nodes))
		}
		roots := 0
		for _, n := range nodes {
			if n.IsRoot {
				roots++
			}
		}
		if roots != 1 {
			t.Errorf("%d roots, want exactly 1", roots)
		}
	})
}

// TestAnUnchangedRebuildWritesNothing is the claim the whole interval design
// exists for. A nightly build that changed nothing must cost nothing: not a
// row, not a re-stamped timestamp. Without it, storage grows with the calendar
// rather than with change, and a product tracked for a year costs the same
// whether or not anything happened to it.
func TestAnUnchangedRebuildWritesNothing(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree()); err != nil {
			t.Fatal(err)
		}
		before := rowCounts(t, f)

		applied, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree())
		if err != nil {
			t.Fatal(err)
		}
		if !applied.Unchanged() {
			t.Errorf("an identical rebuild wrote %+v", applied)
		}
		if after := rowCounts(t, f); !maps.Equal(after, before) {
			t.Errorf("row counts moved from %v to %v on an identical rebuild", before, after)
		}
	})
}

func TestOnlyTheChangeIsWritten(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree()); err != nil {
			t.Fatal(err)
		}

		// zlib arrives under curl. Nothing else moved.
		next := tree()
		next.Components = append(next.Components, zlib)
		next.Dependencies = append(next.Dependencies, graph.Dependency{Parent: curl, Child: zlib})

		applied, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), next)
		if err != nil {
			t.Fatal(err)
		}
		want := graph.Applied{NodesOpened: 1, EdgesOpened: 1}
		if applied != want {
			t.Errorf("applied %+v, want %+v", applied, want)
		}
	})
}

// TestAVersionBumpClosesTheOldComponent checks that a bump is a close and an
// open rather than an edit. Editing the row in place would silently rewrite
// what every past release contained.
func TestAVersionBumpClosesTheOldComponent(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		first := f.scan(t)
		if _, err := f.store.Apply(t.Context(), f.variantID, first, tree()); err != nil {
			t.Fatal(err)
		}

		bumped := at("openssl", "3.0.12")
		next := graph.Snapshot{
			Root:       root,
			Components: []graph.Described{bumped, curl},
			Dependencies: []graph.Dependency{
				{Parent: root, Child: curl},
				{Parent: root, Child: bumped},
				{Parent: curl, Child: bumped},
			},
		}
		second := f.scan(t)
		applied, err := f.store.Apply(t.Context(), f.variantID, second, next)
		if err != nil {
			t.Fatal(err)
		}
		want := graph.Applied{NodesOpened: 1, NodesClosed: 1, EdgesOpened: 2, EdgesClosed: 2}
		if applied != want {
			t.Errorf("applied %+v, want %+v", applied, want)
		}

		nodes, err := f.store.CurrentNodes(t.Context(), f.variantID)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 3 {
			t.Errorf("%d nodes present after a bump, want 3", len(nodes))
		}
		// The closed row is still there, stamped with the scan that ended it.
		var closed []graph.Node
		if err := f.store.DB().NewSelect().Model(&closed).
			Where("variant_id = ?", f.variantID).
			Where("closed_scan_id IS NOT NULL").Scan(t.Context()); err != nil {
			t.Fatal(err)
		}
		if len(closed) != 1 || *closed[0].ClosedScanID != second {
			t.Errorf("closed history is %+v, want one node closed by scan %d", closed, second)
		}
	})
}

// TestAComponentIsSharedAcrossVariants checks the deduplication that keeps the
// component table sized by the portfolio rather than by scans times variants.
func TestAComponentIsSharedAcrossVariants(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree()); err != nil {
			t.Fatal(err)
		}
		before := rowCounts(t, f)["component"]
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), tree()); err != nil {
			t.Fatal(err)
		}
		if after := rowCounts(t, f)["component"]; after != before {
			t.Errorf("%d components after a repeat, want %d", after, before)
		}
	})
}

func TestADependencyOnAnUnlistedComponentIsRefused(t *testing.T) {
	// A file that names an edge to something it never described is malformed.
	// Inventing the missing component would report a dependency nobody
	// declared; failing says so where it can be fixed.
	each(t, func(t *testing.T, f *fixture) {
		bad := tree()
		bad.Dependencies = append(bad.Dependencies, graph.Dependency{Parent: curl, Child: zlib})
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), bad); err == nil {
			t.Fatal("an edge to an undescribed component was accepted")
		}
	})
}

func TestAComponentWithoutAVersionIsRefused(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		bad := tree()
		bad.Components = append(bad.Components, graph.Described{Name: "mystery"})
		if _, err := f.store.Apply(t.Context(), f.variantID, f.scan(t), bad); err == nil {
			t.Fatal("a component with no version was accepted")
		}
	})
}

func rowCounts(t *testing.T, f *fixture) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range []string{"component", "graph_node", "graph_edge"} {
		n, err := f.store.DB().NewSelect().Table(table).Count(t.Context())
		if err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}
