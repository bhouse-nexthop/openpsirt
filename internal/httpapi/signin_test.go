package httpapi_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/catalog"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/httpapi"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
	"github.com/bhouse-nexthop/openpsirt/internal/signin"
)

// stubProvider stands in for a real one, so the paths that decide who gets in
// can be tested without an identity provider to sign in to.
type stubProvider struct {
	says *signin.Identity
	fail error
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Begin(_ context.Context, _ string) (string, signin.Pending, error) {
	return "https://provider.example/authorize", signin.Pending{
		State: "the-state", Nonce: "the-nonce", Verifier: "the-verifier",
	}, nil
}

func (s *stubProvider) Complete(_ context.Context, _ string, _ signin.Pending, _ string) (*signin.Identity, error) {
	if s.fail != nil {
		return nil, s.fail
	}
	return s.says, nil
}

// signInReach is a server with one provider and one person who was granted
// something.
type signInReach struct {
	handler  http.Handler
	provider *stubProvider
	rights   *access.Store
}

func eachSignIn(t *testing.T, fn func(t *testing.T, r *signInReach)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		product, err := catalog.NewStore(db.DB).DeclareProduct(ctx, "mine", "Mine")
		if err != nil {
			t.Fatal(err)
		}
		rights := access.NewStore(db.DB)
		granted, err := rights.Ensure(ctx, "granted", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := rights.Claim(ctx, granted.ID, "stub", "granted"); err != nil {
			t.Fatal(err)
		}
		if err := rights.GrantRole(ctx, granted.ID, product.ID, access.PublicRead); err != nil {
			t.Fatal(err)
		}
		// Somebody recorded and granted nothing, who must be refused exactly
		// as a stranger is.
		ungranted, err := rights.Ensure(ctx, "ungranted", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if err := rights.Claim(ctx, ungranted.ID, "stub", "ungranted"); err != nil {
			t.Fatal(err)
		}

		provider := &stubProvider{says: &signin.Identity{Subject: "1", Username: "granted"}}
		handler, _ := httpapi.New(quiet, nil, httpapi.Ingest{
			DB: db, Queue: queue.New(db, queue.DefaultOptions()),
			Access:    access.NewResolver(rights, access.Trust{}),
			Providers: map[string]signin.Provider{"stub": provider},
			PlainHTTP: true,
		})
		fn(t, &signInReach{handler: handler, provider: provider, rights: rights})
	})
}

// callback is what the provider sends the browser back with.
func callback(t *testing.T, r *signInReach, state, code string, pending bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/sign-in/stub/callback?state="+state+"&code="+code, nil)
	if pending {
		// What begin would have left behind.
		start := httptest.NewRequest(http.MethodGet, "/v1/sign-in/stub", nil)
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, start)
		for _, cookie := range rec.Result().Cookies() {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func TestSigningInSendsTheBrowserToTheProvider(t *testing.T) {
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		req := httptest.NewRequest(http.MethodGet, "/v1/sign-in/stub", nil)
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusFound {
			t.Fatalf("starting a sign-in answered %d, want 302", rec.Code)
		}
		if where := rec.Header().Get("Location"); !strings.HasPrefix(where, "https://provider.example/") {
			t.Errorf("sent the browser to %q", where)
		}
		// What has to survive the round trip is left with the browser, and
		// left where a script cannot read it.
		var held *http.Cookie
		for _, cookie := range rec.Result().Cookies() {
			if cookie.Name == "openpsirt_pending" {
				held = cookie
			}
		}
		if held == nil {
			t.Fatal("nothing was remembered for the round trip")
		}
		if !held.HttpOnly {
			t.Error("what the sign-in remembered is readable by script")
		}
		if held.Value == "" || strings.Contains(held.Value, "the-verifier") {
			t.Errorf("the proof-key secret is sitting in the clear: %q", held.Value)
		}
	})
}

func TestAProviderNobodyConfiguredIsNotThere(t *testing.T) {
	// The same answer a name that was never a provider gets, so guessing does
	// not enumerate which are in use.
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		for _, path := range []string{"/v1/sign-in/github", "/v1/sign-in/okta", "/v1/sign-in/okta/callback"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Errorf("%s answered %d, want 404", path, rec.Code)
			}
		}
	})
}

func TestACallbackWithoutTheSignInThatStartedItIsRefused(t *testing.T) {
	// The state is what stops somebody handing a signed-in person a callback
	// of their own making and having the session come back as theirs.
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		for _, c := range []struct {
			what    string
			state   string
			code    string
			pending bool
		}{
			{"no sign-in in progress", "the-state", "a-code", false},
			{"a state that was never sent", "somebody-elses-state", "a-code", true},
			{"no state at all", "", "a-code", true},
			{"no code", "the-state", "", true},
		} {
			rec := callback(t, r, c.state, c.code, c.pending)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("a callback with %s answered %d, want 401", c.what, rec.Code)
			}
			if cookies := rec.Result().Cookies(); hasSession(cookies) {
				t.Errorf("a callback with %s handed out a session", c.what)
			}
		}
	})
}

func TestSomebodyWhoAuthenticatesButWasGrantedNothingGetsInNowhere(t *testing.T) {
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		for _, who := range []string{"ungranted", "a-stranger"} {
			r.provider.says = &signin.Identity{Subject: "2", Username: who}
			rec := callback(t, r, "the-state", "a-code", true)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%q answered %d, want 401", who, rec.Code)
			}
			if hasSession(rec.Result().Cookies()) {
				t.Errorf("%q was handed a session", who)
			}
		}
	})
}

func TestAuthenticatingCreatesNobodyOnASignInPath(t *testing.T) {
	// Access is granted in advance or not at all. A sign-in path that records
	// somebody is a sign-in path that admits whoever the provider vouches for.
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		r.provider.says = &signin.Identity{Subject: "3", Username: "a-stranger"}
		callback(t, r, "the-state", "a-code", true)

		if _, err := r.rights.ByIdentity(t.Context(), "a-stranger"); err == nil {
			t.Error("signing in recorded somebody nobody had granted anything")
		}
	})
}

func TestSigningInGrantsTheSessionSomebodyWasAlreadyOwed(t *testing.T) {
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		rec := callback(t, r, "the-state", "a-code", true)
		if rec.Code != http.StatusFound {
			t.Fatalf("signing in answered %d, want 302: %s", rec.Code, rec.Body.String())
		}

		cookies := rec.Result().Cookies()
		if !hasSession(cookies) {
			t.Fatal("signing in handed out no session")
		}
		for _, cookie := range cookies {
			switch cookie.Name {
			case access.SessionCookie:
				if !cookie.HttpOnly {
					t.Error("the session cookie is readable by script")
				}
			case "openpsirt_csrf":
				if cookie.HttpOnly {
					t.Error("the value a page has to echo cannot be read by the page")
				}
			}
		}

		// And it works.
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		reading := httptest.NewRecorder()
		r.handler.ServeHTTP(reading, req)
		if reading.Code != http.StatusOK {
			t.Errorf("the session handed out did not work: %d", reading.Code)
		}
	})
}

func TestAProviderThatFailedSaysNothingAboutWhy(t *testing.T) {
	// What went wrong between us and a provider is an operator's problem.
	// Describing it to whoever is at the browser describes our configuration.
	eachSignIn(t, func(t *testing.T, r *signInReach) {
		r.provider.fail = errClientSecretRejected
		rec := callback(t, r, "the-state", "a-code", true)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("a failed exchange answered %d, want 401", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret") {
			t.Errorf("the refusal described the fault: %q", rec.Body.String())
		}
	})
}

func hasSession(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.Name == access.SessionCookie && cookie.Value != "" {
			return true
		}
	}
	return false
}

// errClientSecretRejected stands for the sort of fault a provider reports:
// specific, useful to an operator, and nobody else's business.
var errClientSecretRejected = errors.New("the client secret was rejected")
