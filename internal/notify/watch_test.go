package notify_test

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/ingest"
	"github.com/bhouse-nexthop/openpsirt/internal/notify"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

func TestTheWatchTellsAdministratorsWhatHasGoneQuiet(t *testing.T) {
	// The pass that makes a condition real. A build nothing has been filed
	// against is something an administrator is told; when a scan arrives the
	// alert goes without anybody dismissing it, which is the whole of NTF-09
	// and the reason these are not events.
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		rights := access.NewStore(db.DB)
		admin, err := rights.Ensure(ctx, "admin@example.com", "Admin", true)
		if err != nil {
			t.Fatal(err)
		}
		// Somebody who is not an administrator, to check that the tool's own
		// health is not everybody's business (NTF-07).
		reader, err := rights.Ensure(ctx, "reader@example.com", "Reader", false)
		if err != nil {
			t.Fatal(err)
		}

		cat := catalog.NewStore(db.DB)
		product, err := cat.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		branch, err := cat.DeclareStream(ctx, product.ID, "master", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		variant, err := cat.DeclareVariant(ctx, product.ID, "broadcom", true)
		if err != nil {
			t.Fatal(err)
		}
		target, err := cat.TargetFor(ctx, branch.ID, variant.ID)
		if err != nil {
			t.Fatal(err)
		}

		// Declared a month ago. A build declared a moment ago is not quiet —
		// it is new — and the threshold is measured from when it was declared
		// precisely so that the two are told apart.
		if _, err := db.DB.NewUpdate().Table("target").
			Set("created_at = ?", time.Now().UTC().Add(-30*24*time.Hour)).
			Where("id = ?", target.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		watch := notify.NewWatch(db.DB, quiet)
		seeing := func(who *access.Account) int {
			t.Helper()
			_, total, err := notify.NewStore(db.DB).Waiting(ctx,
				access.NewPerson(who.ID, who.Identity, who.IsAdmin, nil), 50, 0)
			if err != nil {
				t.Fatal(err)
			}
			return total
		}

		// Declared and never filed against, which is the same failure as
		// having stopped — caught earlier.
		opened, cleared, err := watch.Once(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if opened != 1 || cleared != 0 {
			t.Fatalf("a build nothing was filed against: opened %d cleared %d, want 1 and 0",
				opened, cleared)
		}
		if n := seeing(admin); n != 1 {
			t.Errorf("the administrator was told %d things, want 1", n)
		}
		if n := seeing(reader); n != 0 {
			t.Errorf("somebody who administers nothing was told %d things", n)
		}

		// Running it again says the same thing, so nothing changes.
		if opened, cleared, err = watch.Once(ctx); err != nil || opened != 0 || cleared != 0 {
			t.Errorf("a second sweep opened %d cleared %d (err %v), want nothing",
				opened, cleared, err)
		}

		// A scan arrives, so it is no longer quiet and the alert clears itself.
		if _, _, err := ingest.NewStore(db.DB).Record(ctx, ingest.Arriving{
			TargetID: target.ID, ContentHash: "fresh",
			BuiltAt: time.Now().UTC().Add(-time.Hour), ParserVersion: "test",
		}); err != nil {
			t.Fatal(err)
		}
		opened, cleared, err = watch.Once(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if opened != 0 || cleared != 1 {
			t.Errorf("after a scan arrived: opened %d cleared %d, want 0 and 1", opened, cleared)
		}
		if n := seeing(admin); n != 0 {
			t.Errorf("the alert should have cleared itself, %d still waiting", n)
		}
	})
}
