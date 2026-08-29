package access_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// fixture is a migrated database with two products, so that holding something
// on one says nothing about the other.
type fixture struct {
	store    *access.Store
	products map[string]int64
	streams  map[string]int64
	variants map[string]int64
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

		cat := catalog.NewStore(db.DB)
		f := &fixture{
			store:    access.NewStore(db.DB),
			products: map[string]int64{}, streams: map[string]int64{}, variants: map[string]int64{},
		}
		for _, name := range []string{"sonic", "onie"} {
			product, err := cat.DeclareProduct(ctx, name, name)
			if err != nil {
				t.Fatal(err)
			}
			f.products[name] = product.ID
			stream, err := cat.DeclareStream(ctx, product.ID, "master", catalog.Branch, nil)
			if err != nil {
				t.Fatal(err)
			}
			f.streams[name] = stream.ID
			variant, err := cat.DeclareVariant(ctx, product.ID, "broadcom", true)
			if err != nil {
				t.Fatal(err)
			}
			f.variants[name] = variant.ID
		}
		fn(t, f)
	})
}

func TestSomebodyUnknownAndSomebodyUngrantedGetTheSameAnswer(t *testing.T) {
	// Telling an outsider which of the two applies is free reconnaissance:
	// one answer says the name is wrong, the other says the name is right.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		if _, err := f.store.Ensure(ctx, "known", "Known Person", false); err != nil {
			t.Fatal(err)
		}

		_, unknownErr := f.store.Resolve(ctx, "nobody")
		_, ungrantedErr := f.store.Resolve(ctx, "known")

		if !errors.Is(unknownErr, access.ErrDenied) || !errors.Is(ungrantedErr, access.ErrDenied) {
			t.Fatalf("unknown: %v; granted nothing: %v", unknownErr, ungrantedErr)
		}
		if unknownErr.Error() != ungrantedErr.Error() {
			t.Errorf("the two are distinguishable: %q and %q", unknownErr, ungrantedErr)
		}
	})
}

func TestAuthenticatingCreatesNobody(t *testing.T) {
	// Authenticating proves who somebody is and says nothing about whether
	// they should be here. The first person to arrive gains nothing by being
	// first.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		if _, err := f.store.Resolve(ctx, "first-to-arrive"); !errors.Is(err, access.ErrDenied) {
			t.Fatalf("resolving an unknown identity: %v", err)
		}
		if _, err := f.store.ByIdentity(ctx, "first-to-arrive"); err == nil {
			t.Error("signing in created an account")
		}
	})
}

func TestARoleOnOneProductSaysNothingAboutAnother(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		person, err := f.store.Ensure(ctx, "reader", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.GrantRole(ctx, person.ID, f.products["sonic"], access.PublicRead); err != nil {
			t.Fatal(err)
		}

		subject, err := f.store.Resolve(ctx, "reader")
		if err != nil {
			t.Fatal(err)
		}
		if !subject.Reads(access.Public, f.products["sonic"]) {
			t.Error("a public reader cannot read the product they were granted")
		}
		if subject.Reads(access.Public, f.products["onie"]) {
			t.Error("a grant on one product reached another")
		}
		// A product held nothing on is invisible, not merely unreadable.
		if subject.Sees(f.products["onie"]) {
			t.Error("a product held nothing on is visible")
		}
		if !subject.Sees(f.products["sonic"]) {
			t.Error("a product held something on is invisible")
		}
	})
}

func TestReadingPublicIsNotReadingPrivate(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		person, _ := f.store.Ensure(ctx, "public-only", "", false)
		if err := f.store.GrantRole(ctx, person.ID, f.products["sonic"], access.PublicRead); err != nil {
			t.Fatal(err)
		}
		subject, err := f.store.Resolve(ctx, "public-only")
		if err != nil {
			t.Fatal(err)
		}
		if subject.Reads(access.Private, f.products["sonic"]) {
			t.Error("a public reader reached something undisclosed")
		}
	})
}

func TestACapabilityHandsOverNoVisibility(t *testing.T) {
	// Reporting and approving are things somebody may do, bounded by what they
	// may read. Otherwise granting the ability to approve would quietly grant
	// everything there is to approve.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		person, _ := f.store.Ensure(ctx, "approver", "", false)
		for _, role := range []access.Role{access.Approver, access.Reporting} {
			if err := f.store.GrantRole(ctx, person.ID, f.products["sonic"], role); err != nil {
				t.Fatal(err)
			}
		}
		subject, err := f.store.Resolve(ctx, "approver")
		if err != nil {
			t.Fatal(err)
		}
		if subject.Reads(access.Public, f.products["sonic"]) || subject.Reads(access.Private, f.products["sonic"]) {
			t.Error("a capability granted visibility on its own")
		}
		if !subject.Holds(access.Approver, f.products["sonic"]) {
			t.Error("the capability itself was not granted")
		}
	})
}

func TestAnAdministratorHoldsEverythingEverywhere(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		if _, err := f.store.Ensure(ctx, "admin", "", true); err != nil {
			t.Fatal(err)
		}
		subject, err := f.store.Resolve(ctx, "admin")
		if err != nil {
			t.Fatal(err)
		}
		for _, product := range f.products {
			if !subject.Reads(access.Private, product) || !subject.Sees(product) {
				t.Error("an administrator was refused a product")
			}
		}
		if _, all := subject.Products(); !all {
			t.Error("an administrator's reach is reported as a list")
		}
	})
}

func TestAKeyMaySendAndNothingElse(t *testing.T) {
	// A build server has no business holding a person's permissions, which is
	// also what keeps the visibility rules out of its reach entirely.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		_, secret, err := f.store.NewKey(ctx, "nightly", access.Scope{ProductID: f.products["sonic"]})
		if err != nil {
			t.Fatal(err)
		}
		subject, err := f.store.ResolveKey(ctx, secret)
		if err != nil {
			t.Fatal(err)
		}
		if !subject.MaySend(f.products["sonic"], f.streams["sonic"], f.variants["sonic"]) {
			t.Error("a key cannot send against the product it is scoped to")
		}
		if subject.Reads(access.Public, f.products["sonic"]) ||
			subject.Reads(access.Private, f.products["sonic"]) {
			t.Error("a pipeline can read")
		}
		if subject.Holds(access.PublicRead, f.products["sonic"]) {
			t.Error("a pipeline holds a role")
		}
		// It does know the product it may send to exists, because it may send
		// there. Pretending otherwise would mean an upload to its own product
		// could not be told apart from one to a product that is not there,
		// and the sender needs that difference to fix a misconfigured
		// pipeline.
		if !subject.Sees(f.products["sonic"]) {
			t.Error("a pipeline cannot see the product it sends to")
		}
		if subject.Sees(f.products["onie"]) {
			t.Error("a pipeline can see a product it holds nothing for")
		}
		if ids, all := subject.Products(); all || len(ids) != 0 {
			t.Error("a pipeline appears in the list of what somebody may reach")
		}
	})
}

func TestAKeyIsRefusedWhereItsConstraintsDoNotMatch(t *testing.T) {
	// Every constraint present must match, and a mismatch is refused rather
	// than redirected: a key pinned to one release must not quietly accept a
	// scan of another.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		stream := f.streams["sonic"]
		_, secret, err := f.store.NewKey(ctx, "pinned", access.Scope{
			ProductID: f.products["sonic"], StreamID: &stream,
		})
		if err != nil {
			t.Fatal(err)
		}
		subject, err := f.store.ResolveKey(ctx, secret)
		if err != nil {
			t.Fatal(err)
		}
		if !subject.MaySend(f.products["sonic"], stream, f.variants["sonic"]) {
			t.Error("a pinned key was refused its own release")
		}
		if subject.MaySend(f.products["sonic"], f.streams["onie"], f.variants["sonic"]) {
			t.Error("a key pinned to one release accepted another")
		}
		if subject.MaySend(f.products["onie"], f.streams["onie"], f.variants["onie"]) {
			t.Error("a key reached another product entirely")
		}
	})
}

func TestASecretIsNeverStoredAndARevokedKeyStopsWorking(t *testing.T) {
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		key, secret, err := f.store.NewKey(ctx, "nightly", access.Scope{ProductID: f.products["sonic"]})
		if err != nil {
			t.Fatal(err)
		}
		if key.SecretHash == secret || key.SecretHash == "" {
			t.Error("the secret is recoverable from what is stored")
		}
		if _, err := f.store.ResolveKey(ctx, secret+"x"); !errors.Is(err, access.ErrDenied) {
			t.Errorf("a wrong secret: %v", err)
		}

		if err := f.store.Revoke(ctx, key.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.ResolveKey(ctx, secret); !errors.Is(err, access.ErrDenied) {
			t.Errorf("a revoked key still works: %v", err)
		}
	})
}

func TestAQueryWithNobodyAttachedIsAFault(t *testing.T) {
	// Not a denial: it means a query was written that does not say who is
	// asking, and treating that as "show nothing" hides it until somebody
	// writes the one that treats it as "show everything".
	if _, err := access.From(context.Background()); !errors.Is(err, access.ErrNoSubject) {
		t.Errorf("a context with no subject gave %v", err)
	}
	ctx := access.With(context.Background(), access.NewPerson(1, "someone", true, nil))
	if _, err := access.From(ctx); err != nil {
		t.Errorf("a context with a subject gave %v", err)
	}
}

func TestTheTrustedHeaderIsOffUnlessBothHalvesAreSet(t *testing.T) {
	// Trusting it unconditionally would let anybody reaching this process
	// directly be anybody at all. Naming the header and naming what to trust
	// it from are two deliberate acts, and half of it is the dangerous state.
	sources, err := access.ParseSources("10.0.0.1, 192.168.0.0/16")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		trust access.Trust
		on    bool
		valid bool
	}{
		{"neither", access.Trust{}, false, true},
		{"header only", access.Trust{Header: "X-User"}, false, false},
		{"sources only", access.Trust{From: sources}, false, false},
		{"both", access.Trust{Header: "X-User", From: sources}, true, true},
	} {
		if got := c.trust.Enabled(); got != c.on {
			t.Errorf("%s: enabled is %v", c.name, got)
		}
		if got := c.trust.Configured() == nil; got != c.valid {
			t.Errorf("%s: configured reads as %v", c.name, got)
		}
	}
}

func TestTheHeaderIsRefusedFromSomewhereUntrusted(t *testing.T) {
	// Reaching the process directly bypasses the proxy that was supposed to
	// have authenticated somebody.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		person, err := f.store.Ensure(ctx, "someone", "", true)
		if err != nil {
			t.Fatal(err)
		}
		// The proxy asserts a username, and that is what it is matched on.
		if err := f.store.Claim(ctx, person.ID, access.ProxyProvider, "someone"); err != nil {
			t.Fatal(err)
		}

		sources, err := access.ParseSources("10.9.9.9")
		if err != nil {
			t.Fatal(err)
		}
		resolver := access.NewResolver(f.store, access.Trust{Header: "X-User", From: sources})

		trusted := httptest.NewRequest(http.MethodGet, "/", nil)
		trusted.Header.Set("X-User", "someone")
		trusted.RemoteAddr = "10.9.9.9:5555"
		if _, _, err := resolver.Resolve(ctx, trusted); err != nil {
			t.Errorf("a header from a trusted source: %v", err)
		}

		elsewhere := httptest.NewRequest(http.MethodGet, "/", nil)
		elsewhere.Header.Set("X-User", "someone")
		elsewhere.RemoteAddr = "203.0.113.7:5555"
		if _, _, err := resolver.Resolve(ctx, elsewhere); !errors.Is(err, access.ErrDenied) {
			t.Errorf("a header from anywhere else was honored: %v", err)
		}
	})
}

func TestAKeyIsAcceptedWhereverItComesFrom(t *testing.T) {
	// A pipeline holds a credential rather than being vouched for by position,
	// so where it connects from says nothing.
	each(t, func(t *testing.T, f *fixture) {
		ctx := t.Context()
		_, secret, err := f.store.NewKey(ctx, "nightly", access.Scope{ProductID: f.products["sonic"]})
		if err != nil {
			t.Fatal(err)
		}
		resolver := access.NewResolver(f.store, access.Trust{})

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Authorization", "Bearer "+secret)
		req.RemoteAddr = "203.0.113.7:5555"

		subject, _, err := resolver.Resolve(ctx, req)
		if err != nil {
			t.Fatalf("a key from an ordinary address: %v", err)
		}
		if subject.Kind != access.Pipeline {
			t.Errorf("resolved to %q", subject.Kind)
		}
	})
}

func TestTrustingEveryAddressIsRefused(t *testing.T) {
	// The guard that halts on a header with no sources would be decorative if
	// the setting meant to satisfy it could name every address instead.
	for _, sources := range []string{"0.0.0.0/0", "::/0", "10.0.0.0/8, 0.0.0.0/0"} {
		parsed, err := access.ParseSources(sources)
		if err != nil {
			t.Fatalf("%q: %v", sources, err)
		}
		trust := access.Trust{Header: "X-User", From: parsed}
		if err := trust.Configured(); err == nil {
			t.Errorf("%q was accepted as a set of trusted sources", sources)
		}
	}
	// A real range still works.
	parsed, err := access.ParseSources("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	if err := (access.Trust{Header: "X-User", From: parsed}).Configured(); err != nil {
		t.Errorf("an ordinary range was refused: %v", err)
	}
}
