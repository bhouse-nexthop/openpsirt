package catalog_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// each runs fn against every available engine, with the schema applied and the
// catalog emptied first so a persistent server behaves like a fresh one.
func each(t *testing.T, fn func(t *testing.T, db *database.DB, s *catalog.Store)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		fn(t, db, catalog.NewStore(db.DB))
	})
}

func TestDeclareAndResolve(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, err := s.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatalf("declare product: %v", err)
		}
		br, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil)
		if err != nil {
			t.Fatalf("declare branch: %v", err)
		}
		// A tag records the branch it was cut from, which is what lets a
		// branch be compared against its last release.
		tag, err := s.DeclareStream(ctx, p.ID, "v2.4.1", catalog.Tag, &br.ID)
		if err != nil {
			t.Fatalf("declare tag: %v", err)
		}
		if tag.ParentID == nil || *tag.ParentID != br.ID {
			t.Errorf("tag lost its parent branch: %+v", tag.ParentID)
		}
		v, err := s.DeclareVariant(ctx, p.ID, "broadcom", true)
		if err != nil {
			t.Fatalf("declare variant: %v", err)
		}

		// Resolving records that this release is built as this variant. The
		// pair is not declared: the product, the release and the variant all
		// were, so a scan saying they go together reports a fact rather than
		// naming something new.
		got, err := s.Resolve(ctx, "sonic", "release-2.4", "broadcom")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.StreamID != br.ID || got.VariantID != v.ID {
			t.Errorf("resolved to %+v", got)
		}
		// Resolving again is the same target, not a second one.
		again, err := s.Resolve(ctx, "sonic", "release-2.4", "broadcom")
		if err != nil || again.ID != got.ID {
			t.Errorf("resolving twice gave %v and %v (%v)", got.ID, again.ID, err)
		}

		// The same variant in another release is a different target over the
		// same variant, which is what stops the name being typed twice.
		other, err := s.Resolve(ctx, "sonic", "v2.4.1", "broadcom")
		if err != nil {
			t.Fatalf("resolve the tag: %v", err)
		}
		if other.VariantID != v.ID {
			t.Error("a release built as the same variant got a second variant")
		}
		if other.ID == got.ID {
			t.Error("two releases share one target")
		}
	})
}

func TestResolveNamesTheMissingPart(t *testing.T) {
	// Whoever sees the failed upload needs to know what to declare, not that
	// something somewhere was wrong.
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		_, _ = s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil)
		if _, err := s.DeclareVariant(ctx, p.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}

		for _, tc := range []struct{ product, stream, variant, want string }{
			{"nope", "release-2.4", "broadcom", `product "nope"`},
			{"sonic", "relase-2.4", "broadcom", `stream "relase-2.4"`},
			{"sonic", "release-2.4", "brodcom", `variant "brodcom"`},
		} {
			_, err := s.Resolve(ctx, tc.product, tc.stream, tc.variant)
			if err == nil {
				t.Errorf("%v resolved, but should not have", tc)
				continue
			}
			if !errors.Is(err, catalog.ErrNotFound) {
				t.Errorf("error is not ErrNotFound: %v", err)
			}
			if !contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %s", err, tc.want)
			}
		}
	})
}

func TestNamesAreUniqueWithinTheirParent(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, err := s.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.DeclareProduct(ctx, "sonic", "again"); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate product accepted: %v", err)
		}
		a, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate stream accepted: %v", err)
		}
		if _, err := s.DeclareVariant(ctx, p.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}
		// A variant is declared once for the product. Declaring it again is
		// the same variant, not a second one — which is the whole point: a
		// release cannot introduce a second spelling of something the product
		// already builds.
		if _, err := s.DeclareVariant(ctx, p.ID, "broadcom", false); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate variant accepted: %v", err)
		}

		// Every release is built as it without anyone restating the name.
		b, err := s.DeclareStream(ctx, p.ID, "release-2.5", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		// An administrator, because what this test is about is the catalog
		// rather than who may read it.
		admin := access.NewPerson(1, "admin", true, nil)
		built, err := s.Variants(ctx, admin, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(built) != 1 {
			t.Errorf("the product builds %d variants, want 1", len(built))
		}
		// And a release only appears to be built as something once a scan has
		// been filed for it, which is what keeps a variant introduced later
		// out of earlier releases.
		for _, stream := range []int64{a.ID, b.ID} {
			was, err := s.BuiltAs(ctx, admin, p.ID, stream)
			if err != nil {
				t.Fatal(err)
			}
			if len(was) != 0 {
				t.Errorf("a release nothing was filed against reports %d variants", len(was))
			}
		}
		if _, err := s.Resolve(ctx, "sonic", "release-2.5", "broadcom"); err != nil {
			t.Fatal(err)
		}
		was, err := s.BuiltAs(ctx, admin, p.ID, b.ID)
		if err != nil || len(was) != 1 {
			t.Errorf("after a scan was filed the release reports %d variants (%v)", len(was), err)
		}
	})
}

func TestBadNamesAreRejected(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		for _, name := range []string{"", "   ", " leading", "trailing ", string(make([]byte, 200))} {
			if _, err := s.DeclareProduct(ctx, name, ""); err == nil {
				t.Errorf("product name %q was accepted", name)
			}
		}
	})
}

func TestStreamKindIsChecked(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		if _, err := s.DeclareStream(ctx, p.ID, "x", catalog.Kind("release"), nil); err == nil {
			t.Error("an unrecognised stream kind was accepted")
		}
	})
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
