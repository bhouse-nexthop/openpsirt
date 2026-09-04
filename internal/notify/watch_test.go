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
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/graph"
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

func TestAnEmbargoPastItsDateIsToldToAdminsAndWhoeverHoldsIt(t *testing.T) {
	// ACC-47. Reaching the date discloses nothing — it escalates. So this is a
	// condition rather than an event: it holds while the date has passed and
	// nothing has been decided, and it clears when somebody moves the date or
	// discloses, because both of those are answering it.
	//
	// Who hears about it is the careful part. Every one of these is a finding
	// nobody has announced, so the alert is a disclosure in its own right.
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
		owner, err := rights.Ensure(ctx, "owner@example.com", "Owner", false)
		if err != nil {
			t.Fatal(err)
		}
		outsider, err := rights.Ensure(ctx, "public@example.com", "Public", false)
		if err != nil {
			t.Fatal(err)
		}

		cat := catalog.NewStore(db.DB)
		product, err := cat.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		if err := rights.GrantRole(ctx, owner.ID, product.ID, access.PrivateTriage); err != nil {
			t.Fatal(err)
		}
		// Holds the ordinary right and no more, so undisclosed work — and any
		// alert about it — is not theirs to see.
		if err := rights.GrantRole(ctx, outsider.ID, product.ID, access.PublicTriage); err != nil {
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
		// New, so it is not also quiet — this test is about one condition.
		if _, err := db.DB.NewUpdate().Table("target").
			Set("created_at = ?", time.Now().UTC()).
			Where("id = ?", target.ID).Exec(ctx); err != nil {
			t.Fatal(err)
		}

		embargoed(t, db, target.ID, "SONIC-2026-0001", owner.ID,
			time.Now().UTC().Add(-24*time.Hour))

		// A second one, held by somebody who may not read undisclosed work
		// here. An assignment can outlive the role that allowed it, and
		// delivering this to them would hand over the thing the role was
		// withdrawn to stop.
		embargoed(t, db, target.ID, "SONIC-2026-0002", outsider.ID,
			time.Now().UTC().Add(-24*time.Hour))

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

		if _, _, err := watch.Once(ctx); err != nil {
			t.Fatal(err)
		}
		if n := seeing(admin); n != 2 {
			t.Errorf("the administrator was told %d things about passed dates, want 2", n)
		}
		if n := seeing(owner); n != 1 {
			t.Errorf("whoever holds one was told %d things, want 1", n)
		}
		// Holding it is not enough. They may not read undisclosed work here,
		// and the alert says an undisclosed finding exists.
		if n := seeing(outsider); n != 0 {
			t.Errorf("somebody who may not read undisclosed work was told %d things", n)
		}

		// Moving the date answers it, and the alert goes without anybody
		// dismissing anything — which is the whole reason this is a condition.
		if _, err := db.DB.NewUpdate().Table("finding").
			Set("disclose_at = ?", time.Now().UTC().Add(30*24*time.Hour)).
			Where("kind = ?", finding.Entered).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		_, cleared, err := watch.Once(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if cleared == 0 {
			t.Error("the date moved and the alert stayed")
		}
		if n := seeing(admin); n != 0 {
			t.Errorf("the administrator still sees %d after the dates moved", n)
		}
		if n := seeing(owner); n != 0 {
			t.Errorf("whoever holds it still sees %d after the date moved", n)
		}
	})
}

// embargoed writes one undisclosed finding past its disclosure date, held by
// somebody.
func embargoed(t *testing.T, db *database.DB, targetID int64, identifier string,
	owner int64, at time.Time) {
	t.Helper()
	ctx := t.Context()
	ids, err := finding.NewVulnerabilities(db.DB).Intern(ctx,
		[]finding.Named{{Identifier: identifier, Severity: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	components := graph.NewComponents(db.DB)
	interned, err := components.Intern(ctx, []graph.Described{
		{Purl: "pkg:generic/sonic@1.0", Name: "sonic", Version: "1.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var componentID int64
	for _, id := range interned {
		componentID = id
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	row := &finding.Finding{
		TargetID: targetID, Kind: finding.Entered, Visibility: access.Private,
		VulnerabilityID: ids[identifier], ComponentID: componentID,
		PlaceIdentity: "place-of-" + identifier, LastChangedAt: now, OpenedAt: now,
		DiscloseAt: &at, AssignedTo: &owner, AssignedAt: &now,
	}
	if _, err := db.DB.NewInsert().Model(row).Exec(ctx); err != nil {
		t.Fatal(err)
	}
}
