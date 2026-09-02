package notify_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/notify"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// each gives every engine a migrated database and two people to tell things to.
func each(t *testing.T, fn func(t *testing.T, s *notify.Store, me, them access.Subject)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		rights := access.NewStore(db.DB)
		mine, err := rights.Ensure(ctx, "me@example.com", "Me", false)
		if err != nil {
			t.Fatal(err)
		}
		theirs, err := rights.Ensure(ctx, "them@example.com", "Them", false)
		if err != nil {
			t.Fatal(err)
		}
		fn(t, notify.NewStore(db.DB),
			access.NewPerson(mine.ID, "me@example.com", false, nil),
			access.NewPerson(theirs.ID, "them@example.com", false, nil))
	})
}

func TestAConditionOpensOnceAndClearsItself(t *testing.T) {
	// The whole of NTF-09. A build that stopped being scanned is one thing to
	// be told however many times the pass runs, and when it is scanned again
	// the alert goes without anybody dismissing it — otherwise the count fills
	// with problems that already went away and nobody reads it.
	each(t, func(t *testing.T, s *notify.Store, me, _ access.Subject) {
		ctx := t.Context()
		quiet := []notify.Holds{
			{About: "sonic/master/broadcom", Body: "not scanned for 9 days"},
			{About: "sonic/master/mellanox", Body: "not scanned for 30 days"},
		}

		opened, cleared, err := s.Reconcile(ctx, me.ID, notify.BuildQuiet, quiet)
		if err != nil {
			t.Fatal(err)
		}
		if opened != 2 || cleared != 0 {
			t.Fatalf("first pass opened %d cleared %d, want 2 and 0", opened, cleared)
		}

		// Running it again says the same thing, so nothing changes. A pass
		// that opened a second row every time is the failure this prevents.
		opened, cleared, err = s.Reconcile(ctx, me.ID, notify.BuildQuiet, quiet)
		if err != nil {
			t.Fatal(err)
		}
		if opened != 0 || cleared != 0 {
			t.Errorf("running the same pass again opened %d cleared %d, want nothing",
				opened, cleared)
		}
		if _, total, err := s.Waiting(ctx, me, 50, 0); err != nil || total != 2 {
			t.Errorf("waiting on %d after two passes (err %v), want 2", total, err)
		}

		// One build gets scanned again, so its alert stops being true.
		opened, cleared, err = s.Reconcile(ctx, me.ID, notify.BuildQuiet, quiet[:1])
		if err != nil {
			t.Fatal(err)
		}
		if opened != 0 || cleared != 1 {
			t.Errorf("after one recovered: opened %d cleared %d, want 0 and 1", opened, cleared)
		}
		rows, total, err := s.Waiting(ctx, me, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(rows) != 1 || rows[0].About != "sonic/master/broadcom" {
			t.Errorf("the one still true should be alone: %d rows %+v", total, rows)
		}

		// And when it goes quiet again it is a new thing to be told, not an
		// edit of the old one — which is what keeps "this cleared" answerable.
		opened, cleared, err = s.Reconcile(ctx, me.ID, notify.BuildQuiet, quiet)
		if err != nil {
			t.Fatal(err)
		}
		if opened != 1 || cleared != 0 {
			t.Errorf("returning: opened %d cleared %d, want 1 and 0", opened, cleared)
		}
	})
}

func TestAnEventIsNotCollapsedIntoTheOneBeforeIt(t *testing.T) {
	// Being assigned the same finding twice is two things that happened, and
	// collapsing them loses the second — the one they have not seen.
	each(t, func(t *testing.T, s *notify.Store, me, _ access.Subject) {
		ctx := t.Context()
		for range 2 {
			if err := s.Tell(ctx, notify.Telling{
				PersonID: me.ID, Kind: notify.Assigned,
				Body: "CVE-2026-1 in openssl", Link: "/findings/1",
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, total, err := s.Waiting(ctx, me, 50, 0); err != nil || total != 2 {
			t.Errorf("two things happened and %d are waiting (err %v)", total, err)
		}

		// An event that named a subject would be silently deduplicated against
		// an unrelated row, so it is refused rather than accepted and reshaped.
		if err := s.Tell(ctx, notify.Telling{
			PersonID: me.ID, Kind: notify.Assigned, About: "something",
			Body: "CVE-2026-2",
		}); err == nil {
			t.Error("an event was accepted with a subject to clear against")
		}
	})
}

func TestNobodyReadsAnybodyElsesNotifications(t *testing.T) {
	each(t, func(t *testing.T, s *notify.Store, me, them access.Subject) {
		ctx := t.Context()
		if err := s.Tell(ctx, notify.Telling{
			PersonID: them.ID, Kind: notify.Mentioned, Body: "named you",
		}); err != nil {
			t.Fatal(err)
		}
		rows, total, err := s.Waiting(ctx, me, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if total != 0 || len(rows) != 0 {
			t.Errorf("somebody else's notification was listed: %d %+v", total, rows)
		}

		// And the identifier is a number a caller supplies, so acknowledging
		// has to check whose it is rather than trusting it.
		theirs, _, err := s.Waiting(ctx, them, 50, 0)
		if err != nil || len(theirs) != 1 {
			t.Fatalf("the owner should see it: %d (err %v)", len(theirs), err)
		}
		if err := s.Acknowledge(ctx, me, theirs[0].ID); err == nil {
			t.Error("one person acknowledged another's notification")
		}
		if _, total, err := s.Waiting(ctx, them, 50, 0); err != nil || total != 1 {
			t.Errorf("it should still be waiting on its owner: %d (err %v)", total, err)
		}
	})
}

func TestAcknowledgingTakesItOutOfTheCount(t *testing.T) {
	each(t, func(t *testing.T, s *notify.Store, me, _ access.Subject) {
		ctx := t.Context()
		for _, body := range []string{"one", "two", "three"} {
			if err := s.Tell(ctx, notify.Telling{
				PersonID: me.ID, Kind: notify.SentBack, Body: body,
			}); err != nil {
				t.Fatal(err)
			}
		}
		rows, _, err := s.Waiting(ctx, me, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Acknowledge(ctx, me, rows[0].ID); err != nil {
			t.Fatal(err)
		}
		if _, total, err := s.Waiting(ctx, me, 50, 0); err != nil || total != 2 {
			t.Errorf("after acknowledging one, %d wait (err %v), want 2", total, err)
		}
		n, err := s.AcknowledgeAll(ctx, me)
		if err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("acknowledging the rest reported %d, want 2", n)
		}
		if _, total, err := s.Waiting(ctx, me, 50, 0); err != nil || total != 0 {
			t.Errorf("nothing should wait: %d (err %v)", total, err)
		}
	})
}

func TestAPipelineKeyIsToldNothing(t *testing.T) {
	// A key is not a person. It has no notification area, and asking for one
	// answers empty rather than failing — there is nothing to refuse.
	//
	// **The key is given the same identifier as the person**, which is the
	// case worth testing rather than the easy one: a key's identifier comes
	// from one table and a person's from another, so the two collide as a
	// matter of course. Without asking what kind of subject this is, a key
	// numbered 3 reads and acknowledges the notifications of person 3.
	each(t, func(t *testing.T, s *notify.Store, me, _ access.Subject) {
		ctx := t.Context()
		if err := s.Tell(ctx, notify.Telling{
			PersonID: me.ID, Kind: notify.Assigned, Body: "yours, not a key's",
		}); err != nil {
			t.Fatal(err)
		}
		mine, _, err := s.Waiting(ctx, me, 50, 0)
		if err != nil || len(mine) != 1 {
			t.Fatalf("the person should have one: %d (err %v)", len(mine), err)
		}

		key := access.NewPipeline(me.ID, "nightly", access.Scope{ProductID: 1})
		rows, total, err := s.Waiting(ctx, key, 50, 0)
		if err != nil || total != 0 || len(rows) != 0 {
			t.Errorf("a key sharing the person's number was told %d things (err %v)",
				total, err)
		}
		if err := s.Acknowledge(ctx, key, mine[0].ID); err == nil {
			t.Error("a key acknowledged a person's notification")
		}
		if _, total, err := s.Waiting(ctx, me, 50, 0); err != nil || total != 1 {
			t.Errorf("it should still be waiting on the person: %d (err %v)", total, err)
		}
		if n, err := s.AcknowledgeAll(ctx, key); err == nil || n != 0 {
			t.Errorf("a key acknowledged everything: %d (err %v)", n, err)
		}
	})
}
