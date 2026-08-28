package httpapi_test

import (
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
)

// reach is a server with two products and people holding various things, so
// that every combination of who-asks and what-they-ask-for can be checked
// rather than a representative few.
type reach struct {
	handler http.Handler
	key     string
}

func (r *reach) as(t *testing.T, who, method, path string) int {
	t.Helper()
	// A well-formed body, so that what is being measured is the decision about
	// the asker rather than a complaint about the request.
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{"name": "declared-by-the-test"}`)
	}
	req := httptest.NewRequest(method, path, body)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if who != "" {
		req.Header.Set(testHeader, who)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Code
}

func (r *reach) asKey(t *testing.T, method, path string) int {
	t.Helper()
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{"name": "declared-by-a-pipeline"}`)
	}
	req := httptest.NewRequest(method, path, body)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+r.key)
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Code
}

func eachReach(t *testing.T, fn func(t *testing.T, r *reach)) {
	t.Helper()
	dbtest.Each(t, func(t *testing.T, db *database.DB) {
		ctx := t.Context()
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(ctx, db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		cat := catalog.NewStore(db.DB)
		mine, err := cat.DeclareProduct(ctx, "mine", "Mine")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cat.DeclareStream(ctx, mine.ID, "master", catalog.Branch, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := cat.DeclareVariant(ctx, mine.ID, "broadcom", true); err != nil {
			t.Fatal(err)
		}
		theirs, err := cat.DeclareProduct(ctx, "theirs", "Theirs")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cat.DeclareStream(ctx, theirs.ID, "master", catalog.Branch, nil); err != nil {
			t.Fatal(err)
		}

		rights := access.NewStore(db.DB)
		if _, err := rights.Ensure(ctx, "admin", "", true); err != nil {
			t.Fatal(err)
		}
		for who, role := range map[string]access.Role{
			"reader":   access.PublicRead,
			"private":  access.PrivateRead,
			"triager":  access.PublicTriage,
			"approver": access.Approver,
		} {
			person, err := rights.Ensure(ctx, who, "", false)
			if err != nil {
				t.Fatal(err)
			}
			if err := rights.GrantRole(ctx, person.ID, mine.ID, role); err != nil {
				t.Fatal(err)
			}
		}
		// Somebody who exists and was granted nothing at all.
		if _, err := rights.Ensure(ctx, "nothing", "", false); err != nil {
			t.Fatal(err)
		}
		_, secret, err := rights.NewKey(ctx, "nightly", access.Scope{ProductID: mine.ID})
		if err != nil {
			t.Fatal(err)
		}

		sources, err := access.ParseSources("192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		handler, _ := httpapi.New(quiet, nil, httpapi.Ingest{
			DB: db, Queue: queue.New(db, queue.DefaultOptions()),
			Access: access.NewResolver(rights, access.Trust{Header: testHeader, From: sources}),
		})
		fn(t, &reach{handler: handler, key: secret})
	})
}

func TestWhoMayReachWhat(t *testing.T) {
	// The matrix rather than a few representative cases. What leaks is never
	// the endpoint somebody thought about.
	eachReach(t, func(t *testing.T, r *reach) {
		for _, c := range []struct {
			who    string
			method string
			path   string
			want   int
		}{
			// Nobody at all.
			{"", http.MethodGet, "/v1/products", http.StatusUnauthorized},
			{"", http.MethodPost, "/v1/products", http.StatusUnauthorized},
			{"", http.MethodGet, "/v1/products/mine/streams", http.StatusUnauthorized},

			// Somebody real who was granted nothing. Refused, and refused the
			// same way as somebody who does not exist.
			{"nothing", http.MethodGet, "/v1/products", http.StatusUnauthorized},
			{"ghost", http.MethodGet, "/v1/products", http.StatusUnauthorized},

			// Reading what you hold a role on, and not what you do not.
			{"reader", http.MethodGet, "/v1/products", http.StatusOK},
			{"reader", http.MethodGet, "/v1/products/mine/streams", http.StatusOK},
			{"reader", http.MethodGet, "/v1/products/theirs/streams", http.StatusForbidden},
			{"reader", http.MethodGet, "/v1/products/mine/variants", http.StatusOK},
			{"reader", http.MethodGet, "/v1/products/theirs/variants", http.StatusForbidden},
			{"reader", http.MethodGet, "/v1/products/mine/streams/master/variants", http.StatusOK},
			{"reader", http.MethodGet, "/v1/products/theirs/streams/master/variants", http.StatusForbidden},

			// A capability is not a way in.
			{"approver", http.MethodGet, "/v1/products/mine/streams", http.StatusOK},

			// Declaring is administration, whoever else you are.
			{"reader", http.MethodPost, "/v1/products", http.StatusForbidden},
			{"triager", http.MethodPost, "/v1/products", http.StatusForbidden},
			{"private", http.MethodPost, "/v1/products", http.StatusForbidden},
			{"approver", http.MethodPost, "/v1/products", http.StatusForbidden},

			// An administrator reaches everything.
			{"admin", http.MethodGet, "/v1/products", http.StatusOK},
			{"admin", http.MethodGet, "/v1/products/theirs/streams", http.StatusOK},
		} {
			if got := r.as(t, c.who, c.method, c.path); got != c.want {
				who := c.who
				if who == "" {
					who = "nobody"
				}
				t.Errorf("%s %s as %s = %d, want %d", c.method, c.path, who, got, c.want)
			}
		}
	})
}

func TestAPipelineCanReachNothingButSending(t *testing.T) {
	// A build server has no business holding a person's permissions, and this
	// is what keeps the visibility rules out of its reach entirely rather than
	// relying on them being applied to it correctly.
	eachReach(t, func(t *testing.T, r *reach) {
		for _, path := range []string{
			"/v1/products",
			"/v1/products/mine/streams",
			"/v1/products/mine/variants",
			"/v1/products/mine/streams/master/variants",
		} {
			if got := r.asKey(t, http.MethodGet, path); got == http.StatusOK {
				t.Errorf("a pipeline read %s", path)
			}
		}
		if got := r.asKey(t, http.MethodPost, "/v1/products"); got == http.StatusCreated {
			t.Error("a pipeline declared a product")
		}
	})
}

func TestAListShowsOnlyWhatTheAskerHolds(t *testing.T) {
	// Counting is reading. A list that quietly includes what somebody may not
	// see is the same disclosure as showing it, and the product list is itself
	// a statement about what an organization ships.
	eachReach(t, func(t *testing.T, r *reach) {
		req := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
		req.Header.Set(testHeader, "reader")
		rec := httptest.NewRecorder()
		r.handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !contains(body, "mine") {
			t.Errorf("the product they hold something on is missing: %s", body)
		}
		if contains(body, "theirs") {
			t.Errorf("a product they hold nothing on was listed: %s", body)
		}
	})
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
