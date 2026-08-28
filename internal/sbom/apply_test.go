package sbom_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalogue"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// TestAProducerDocumentBecomesTheStoredGraph reads a document the way an
// arriving scan is read and stores it, on every engine. The two halves are
// written separately and have to agree: a reader that produced a graph the
// store refuses, or one whose edges named components it did not list, would
// pass every test in either package alone.
func TestAProducerDocumentBecomesTheStoredGraph(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		cat := catalogue.NewStore(db.DB)
		product, err := cat.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		stream, err := cat.DeclareStream(ctx, product.ID, "master", catalogue.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		variant, err := cat.DeclareVariant(ctx, stream.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}

		doc, err := sbom.Read(fixture(t, "image.cdx.json"), sbom.Limits{})
		if err != nil {
			t.Fatalf("read: %v", err)
		}

		scans := ingest.NewStore(db.DB)
		store := graph.NewStore(db.DB)
		built := time.Now().UTC().Add(-24 * time.Hour)

		record := func(hash string, at time.Time) int64 {
			t.Helper()
			rec, outcome, err := scans.Record(ctx, ingest.Arriving{
				VariantID: variant.ID, ContentHash: hash, BuiltAt: at, ParserVersion: "test",
			})
			if err != nil || outcome != ingest.Accept {
				t.Fatalf("record: outcome %v, err %v", outcome, err)
			}
			return rec.ID
		}

		applied, err := store.Apply(ctx, variant.ID, record("first", built), doc.Snapshot())
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		// The root is stored as a node like any other, so the count is the
		// components plus it.
		if want := len(doc.Components) + 1; applied.NodesOpened != want {
			t.Errorf("opened %d nodes, want %d", applied.NodesOpened, want)
		}
		if applied.EdgesOpened != len(doc.Dependencies) {
			t.Errorf("opened %d edges, want %d", applied.EdgesOpened, len(doc.Dependencies))
		}

		// The same build, sent again the next night. Nothing changed, so
		// nothing is written — not a row, not a re-stamped timestamp.
		again, err := sbom.Read(fixture(t, "image.cdx.json"), sbom.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		applied, err = store.Apply(ctx, variant.ID, record("second", built.Add(time.Hour)), again.Snapshot())
		if err != nil {
			t.Fatalf("re-apply: %v", err)
		}
		if !applied.Unchanged() {
			t.Errorf("an unchanged build wrote %+v", applied)
		}
	})
}
