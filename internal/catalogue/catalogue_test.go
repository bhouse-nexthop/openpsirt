package catalogue_test

import (
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/catalogue"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// each runs fn against every available engine, with the schema applied and the
// catalogue emptied first so a persistent server behaves like a fresh one.
func each(t *testing.T, fn func(t *testing.T, db *database.DB, s *catalogue.Store)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		fn(t, db, catalogue.NewStore(db.DB))
	})
}

func TestDeclareAndResolve(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalogue.Store) {
		ctx := t.Context()
		p, err := s.DeclareProduct(ctx, "sonic", "SONiC")
		if err != nil {
			t.Fatalf("declare product: %v", err)
		}
		br, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalogue.Branch, nil)
		if err != nil {
			t.Fatalf("declare branch: %v", err)
		}
		// A tag records the branch it was cut from, which is what lets a
		// branch be compared against its last release.
		tag, err := s.DeclareStream(ctx, p.ID, "v2.4.1", catalogue.Tag, &br.ID)
		if err != nil {
			t.Fatalf("declare tag: %v", err)
		}
		if tag.ParentID == nil || *tag.ParentID != br.ID {
			t.Errorf("tag lost its parent branch: %+v", tag.ParentID)
		}
		if _, err := s.DeclareVariant(ctx, br.ID, "broadcom", true); err != nil {
			t.Fatalf("declare variant: %v", err)
		}

		got, err := s.Resolve(ctx, "sonic", "release-2.4", "broadcom")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.StreamID != br.ID || !got.CustomerFacing {
			t.Errorf("resolved to %+v", got)
		}
	})
}

func TestResolveNamesTheMissingPart(t *testing.T) {
	// Whoever sees the failed upload needs to know what to declare, not that
	// something somewhere was wrong.
	each(t, func(t *testing.T, _ *database.DB, s *catalogue.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		br, _ := s.DeclareStream(ctx, p.ID, "release-2.4", catalogue.Branch, nil)
		if _, err := s.DeclareVariant(ctx, br.ID, "broadcom", true); err != nil {
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
			if !errors.Is(err, catalogue.ErrNotFound) {
				t.Errorf("error is not ErrNotFound: %v", err)
			}
			if !contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name %s", err, tc.want)
			}
		}
	})
}

func TestNamesAreUniqueWithinTheirParent(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalogue.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		if _, err := s.DeclareProduct(ctx, "sonic", "again"); !errors.Is(err, catalogue.ErrExists) {
			t.Errorf("duplicate product accepted: %v", err)
		}
		a, _ := s.DeclareStream(ctx, p.ID, "release-2.4", catalogue.Branch, nil)
		if _, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalogue.Tag, nil); !errors.Is(err, catalogue.ErrExists) {
			t.Errorf("duplicate stream accepted: %v", err)
		}
		if _, err := s.DeclareVariant(ctx, a.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DeclareVariant(ctx, a.ID, "broadcom", false); !errors.Is(err, catalogue.ErrExists) {
			t.Errorf("duplicate variant accepted: %v", err)
		}

		// The same variant name in a different stream is a different variant:
		// variants belong to a stream, not to the product.
		b, _ := s.DeclareStream(ctx, p.ID, "release-2.5", catalogue.Branch, nil)
		if _, err := s.DeclareVariant(ctx, b.ID, "broadcom", true); err != nil {
			t.Errorf("the same variant name in another stream was rejected: %v", err)
		}
	})
}

func TestBadNamesAreRejected(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalogue.Store) {
		ctx := t.Context()
		for _, name := range []string{"", "   ", " leading", "trailing ", string(make([]byte, 200))} {
			if _, err := s.DeclareProduct(ctx, name, ""); err == nil {
				t.Errorf("product name %q was accepted", name)
			}
		}
	})
}

func TestStreamKindIsChecked(t *testing.T) {
	each(t, func(t *testing.T, _ *database.DB, s *catalogue.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		if _, err := s.DeclareStream(ctx, p.ID, "x", catalogue.Kind("release"), nil); err == nil {
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
