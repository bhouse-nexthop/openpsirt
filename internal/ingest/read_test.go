package ingest_test

import (
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/catalogue"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/sbom"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

const anInventory = `{
  "bomFormat": "CycloneDX", "specVersion": "1.6",
  "metadata": {"timestamp": "2026-08-01T00:00:00Z",
    "component": {"bom-ref": "root", "name": "sonic-broadcom.bin", "version": "1.0"}},
  "components": [
    {"bom-ref": "a", "name": "libc6", "version": "2.41", "purl": "pkg:deb/debian/libc6@2.41"},
    {"bom-ref": "b", "name": "zlib1g", "version": "1.3", "purl": "pkg:deb/debian/zlib1g@1.3"}],
  "dependencies": [{"ref": "root", "dependsOn": ["a", "b"]}, {"ref": "a", "dependsOn": ["b"]}]
}`

const aSuppression = `{"@context": "https://openvex.dev/ns/v0.2.0", "@id": "urn:x", "version": 1,
 "statements": [{"vulnerability": {"name": "CVE-2026-1"}, "status": "not_affected",
 "products": [{"@id": "pkg:deb/debian/libc6"}]}]}`

// readerFixture is a migrated database with a declared branch and tag, a
// queue, and a reader over both.
type readerFixture struct {
	db     *database.DB
	queue  *queue.Queue
	reader *ingest.Reader
	branch int64
	tag    int64
}

// accept records a scan and stores what a build sent with it, the way an
// upload does, and leaves the work behind.
func (f *readerFixture) accept(t *testing.T, variantID int64, builtAt time.Time, inventory string, suppressions ...string) int64 {
	t.Helper()
	ctx := t.Context()
	scan, outcome, err := ingest.NewStore(f.db.DB).Record(ctx, ingest.Arriving{
		VariantID: variantID, ContentHash: inventory[:16] + builtAt.String(),
		BuiltAt: builtAt, ParserVersion: "test",
	})
	if err != nil || outcome != ingest.Accept {
		t.Fatalf("record: %v %v", outcome, err)
	}
	docs := ingest.NewDocuments(f.db.DB)
	if _, err := docs.Write(ctx, scan.ID, ingest.InventoryKind, 0, strings.NewReader(inventory)); err != nil {
		t.Fatal(err)
	}
	for i, s := range suppressions {
		if _, err := docs.Write(ctx, scan.ID, ingest.SuppressionsKind, i, strings.NewReader(s)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.queue.Add(ctx, ingest.JobKind, strconv.FormatInt(scan.ID, 10)); err != nil {
		t.Fatal(err)
	}
	return scan.ID
}

func eachReader(t *testing.T, fn func(t *testing.T, f *readerFixture)) {
	t.Helper()
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
		branch, err := cat.DeclareStream(ctx, product.ID, "master", catalogue.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		branchVariant, err := cat.DeclareVariant(ctx, branch.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}
		tag, err := cat.DeclareStream(ctx, product.ID, "2.4.0", catalogue.Tag, &branch.ID)
		if err != nil {
			t.Fatal(err)
		}
		tagVariant, err := cat.DeclareVariant(ctx, tag.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}

		q := queue.New(db, queue.DefaultOptions())
		fn(t, &readerFixture{
			db: db, queue: q, branch: branchVariant.ID, tag: tagVariant.ID,
			reader: ingest.NewReader(db, q, sbom.Limits{}, quiet, "test"),
		})
	})
}

func TestAnAcceptedScanBecomesStoredGraph(t *testing.T) {
	eachReader(t, func(t *testing.T, f *readerFixture) {
		scanID := f.accept(t, f.branch, time.Now().UTC().Add(-time.Hour), anInventory, aSuppression)

		result, err := f.reader.Once(t.Context())
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if result == nil {
			t.Fatal("there was work waiting and nothing was done")
		}
		if result.ScanID != scanID {
			t.Errorf("read scan %d, want %d", result.ScanID, scanID)
		}
		// The root is stored like any other node, so three components plus it.
		if result.Applied.NodesOpened != 3 || result.Applied.EdgesOpened != 3 {
			t.Errorf("applied %+v, want 3 nodes and 3 edges", result.Applied)
		}
		if result.Suppressions != 1 {
			t.Errorf("read %d suppressions, want 1", result.Suppressions)
		}

		nodes, err := graph.NewStore(f.db.DB).CurrentNodes(t.Context(), f.branch)
		if err != nil || len(nodes) != 3 {
			t.Errorf("%d nodes are present, want 3 (%v)", len(nodes), err)
		}
		if depth, _ := f.queue.Depth(t.Context()); depth != 0 {
			t.Errorf("%d jobs still waiting", depth)
		}
	})
}

func TestABranchScanDoesNotKeepWhatItWasSent(t *testing.T) {
	// The next night supersedes it, so keeping it grows storage with the
	// calendar rather than with what is being tracked.
	eachReader(t, func(t *testing.T, f *readerFixture) {
		scanID := f.accept(t, f.branch, time.Now().UTC().Add(-time.Hour), anInventory, aSuppression)
		result, err := f.reader.Once(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if result.Retained {
			t.Error("a branch scan kept what it was sent")
		}
		held, err := ingest.NewDocuments(f.db.DB).List(t.Context(), scanID)
		if err != nil || len(held) != 0 {
			t.Errorf("%d documents survived a branch scan (%v)", len(held), err)
		}
	})
}

func TestATaggedReleaseKeepsWhatItWasSent(t *testing.T) {
	// Re-scanning a release years from now needs both what it contained and
	// what the build had already argued about its own patches. Keeping only
	// the first would undo every one of those arguments on the next re-scan.
	eachReader(t, func(t *testing.T, f *readerFixture) {
		scanID := f.accept(t, f.tag, time.Now().UTC().Add(-time.Hour), anInventory, aSuppression)
		result, err := f.reader.Once(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !result.Retained {
			t.Error("a tagged release threw away what it was sent")
		}
		held, err := ingest.NewDocuments(f.db.DB).List(t.Context(), scanID)
		if err != nil {
			t.Fatal(err)
		}
		if len(held) != 2 {
			t.Errorf("%d documents kept, want the inventory and its suppressions", len(held))
		}
	})
}

func TestAScanThatCannotBeReadSaysSoOnTheScan(t *testing.T) {
	// A producer sending files nothing can read has to be visible as that,
	// rather than as a scan that was accepted and then quietly did nothing.
	eachReader(t, func(t *testing.T, f *readerFixture) {
		scanID := f.accept(t, f.branch, time.Now().UTC().Add(-time.Hour),
			`{"bomFormat": "CycloneDX", "specVersion": "1.6", "components": [{"version": "1"}]}`)

		if _, err := f.reader.Once(t.Context()); err == nil {
			t.Fatal("an unreadable inventory was read successfully")
		}
		scan, err := ingest.NewStore(f.db.DB).ByID(t.Context(), scanID)
		if err != nil {
			t.Fatal(err)
		}
		if scan.Status != ingest.Failed {
			t.Errorf("scan status is %q, want %q", scan.Status, ingest.Failed)
		}
		if !strings.Contains(scan.Failure, "no name") {
			t.Errorf("the scan does not say why it failed: %q", scan.Failure)
		}
		// Nothing partial: a half-applied graph is indistinguishable from
		// components having been removed.
		nodes, _ := graph.NewStore(f.db.DB).CurrentNodes(t.Context(), f.branch)
		if len(nodes) != 0 {
			t.Errorf("%d nodes were stored from a scan that failed", len(nodes))
		}
	})
}

func TestThereIsNothingToDoWhenNothingIsWaiting(t *testing.T) {
	eachReader(t, func(t *testing.T, f *readerFixture) {
		result, err := f.reader.Once(t.Context())
		if err != nil || result != nil {
			t.Errorf("an empty queue produced %+v (%v)", result, err)
		}
	})
}
