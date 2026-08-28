package catalog_test

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

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
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		br, _ := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil)
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
		p, _ := s.DeclareProduct(ctx, "sonic", "SONiC")
		if _, err := s.DeclareProduct(ctx, "sonic", "again"); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate product accepted: %v", err)
		}
		a, _ := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Branch, nil)
		if _, err := s.DeclareStream(ctx, p.ID, "release-2.4", catalog.Tag, nil); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate stream accepted: %v", err)
		}
		if _, err := s.DeclareVariant(ctx, a.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DeclareVariant(ctx, a.ID, "broadcom", false); !errors.Is(err, catalog.ErrExists) {
			t.Errorf("duplicate variant accepted: %v", err)
		}

		// A new release is built as the same things the product already is,
		// so it arrives already carrying them rather than needing them
		// restated.
		b, _ := s.DeclareStream(ctx, p.ID, "release-2.5", catalog.Branch, nil)
		carried, err := s.Variants(ctx, b.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(carried) != 1 || carried[0].Name != "broadcom" {
			t.Errorf("a new release carried %+v, want the product's variants", carried)
		}
		// The row is its own, per release: the same name in two releases is
		// two rows, which is what keeps a variant introduced later from
		// appearing in earlier ones.
		if carried[0].StreamID != b.ID {
			t.Error("a carried variant belongs to the wrong release")
		}
	})
}

func TestAVariantNameTheProductDoesNotUseIsRefused(t *testing.T) {
	// The typo that would invent a stream invents a variant once per release.
	// "win", "windows" and "win32" across three releases are three sets of
	// findings and three sets of decisions, and nothing in the data says they
	// were meant to be one.
	each(t, func(t *testing.T, _ *database.DB, s *catalog.Store) {
		ctx := t.Context()
		p, err := s.DeclareProduct(ctx, "windows-agent", "")
		if err != nil {
			t.Fatal(err)
		}
		first, err := s.DeclareStream(ctx, p.ID, "2024", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		// A vocabulary has to start somewhere: the first release can say
		// anything, because there is nothing to have misspelled.
		if _, _, err := s.EnsureVariant(ctx, first.ID, "windows", true, false); err != nil {
			t.Fatalf("the first variant of a product was refused: %v", err)
		}

		next, err := s.DeclareStream(ctx, p.ID, "2025", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = s.EnsureVariant(ctx, next.ID, "win32", true, false)
		if !errors.Is(err, catalog.ErrUnknownVariant) {
			t.Errorf("a variant the product does not build was accepted: %v", err)
		}
		if err != nil && !strings.Contains(err.Error(), "windows") {
			t.Errorf("the refusal does not say what it does build: %v", err)
		}

		// Something genuinely new still gets in, said deliberately.
		if _, _, err := s.EnsureVariant(ctx, next.ID, "linux", true, true); err != nil {
			t.Errorf("a genuinely new variant was refused: %v", err)
		}
		// And from then on it is one of the product's own.
		later, err := s.DeclareStream(ctx, p.ID, "2026", catalog.Branch, nil)
		if err != nil {
			t.Fatal(err)
		}
		carried, err := s.Variants(ctx, later.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(carried) != 2 {
			t.Errorf("the next release carried %d variants, want both", len(carried))
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
