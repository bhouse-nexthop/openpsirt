package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/database"
	"github.com/bhouse-nexthop/openpsirt/internal/dbtest"
	"github.com/bhouse-nexthop/openpsirt/internal/httpapi"
	"github.com/bhouse-nexthop/openpsirt/internal/queue"
	"github.com/bhouse-nexthop/openpsirt/internal/schema"
)

// testHeader is what the fixture's proxy would set. Requests from the test
// client arrive from the address httptest uses, which the fixture trusts.
const testHeader = "X-Test-User"

// declaring is a server over an empty catalog, with an administrator to speak
// as and a way to speak as anybody else.
type declaring struct {
	handler http.Handler
	access  *access.Store
	admin   string
}

func (d *declaring) post(t *testing.T, path, body string) (int, map[string]any) {
	t.Helper()
	return d.postAs(t, d.admin, path, body)
}

func (d *declaring) postAs(t *testing.T, who, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	fromOurOwnPage(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(testHeader, who)
	return d.do(t, req)
}

func (d *declaring) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	return d.getAs(t, d.admin, path)
}

func (d *declaring) getAs(t *testing.T, who, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(testHeader, who)
	return d.do(t, req)
}

func (d *declaring) do(t *testing.T, req *http.Request) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	d.handler.ServeHTTP(rec, req)
	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s: %v (%s)", req.URL, err, rec.Body.String())
		}
	}
	return rec.Code, body
}

// The four-engine form and the two-engine form of the same fixture. Which
// one a test uses is decided by the rule at dbtest.Two: a test that pins what
// a query does — what a list contains, what a filter hides, what a conflict
// looks like, what text comes back — runs on every engine, and a test that
// pins routing, who may reach what, or the shape of a response runs on two.
func eachCatalog(t *testing.T, fn func(t *testing.T, d *declaring)) {
	t.Helper()
	catalogOn(t, dbtest.Each, fn)
}

func twoCatalog(t *testing.T, fn func(t *testing.T, d *declaring)) {
	t.Helper()
	catalogOn(t, dbtest.Two, fn)
}

func catalogOn(t *testing.T, on engines, fn func(t *testing.T, d *declaring)) {
	t.Helper()
	on(t, func(t *testing.T, db *database.DB) {
		quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := schema.Up(t.Context(), db, quiet); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		dbtest.Reset(t, db)

		rights := access.NewStore(db.DB)
		administrator, err := rights.Ensure(t.Context(), "admin", "Administrator", true)
		if err != nil {
			t.Fatal(err)
		}
		if err := rights.Claim(t.Context(), administrator.ID, access.ProxyProvider, "admin"); err != nil {
			t.Fatal(err)
		}
		// httptest sends from a documentation address; the fixture's proxy is
		// taken to sit there.
		sources, err := access.ParseSources("192.0.2.1")
		if err != nil {
			t.Fatal(err)
		}
		handler, _ := httpapi.New(quiet, nil, httpapi.Ingest{
			DB: db, Queue: queue.New(db, queue.DefaultOptions()),
			Access: access.NewResolver(rights, access.Trust{Header: testHeader, From: sources}),
		})
		fn(t, &declaring{handler: handler, access: rights, admin: "admin"})
	})
}

func TestDeclaringTheSameThingTwiceSucceeds(t *testing.T) {
	// A pipeline declares before every build. Failing the second one makes the
	// step something everybody works around, and then a scan arrives against
	// something nobody declared.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		code, body := d.post(t, "/v1/products", `{"name": "sonic", "display_name": "SONiC"}`)
		if code != http.StatusCreated {
			t.Fatalf("first declaration returned %d, want 201: %v", code, body)
		}
		if body["created"] != true {
			t.Errorf("first declaration reported created=%v", body["created"])
		}

		code, body = d.post(t, "/v1/products", `{"name": "sonic", "display_name": "SONiC"}`)
		if code != http.StatusOK {
			t.Fatalf("declaring it again returned %d, want 200: %v", code, body)
		}
		if body["created"] != false {
			t.Errorf("declaring it again reported created=%v", body["created"])
		}
	})
}

func TestRedeclaringSomethingDifferentlyIsRefused(t *testing.T) {
	// The other half of being able to declare repeatedly. A pipeline that has
	// quietly changed what it means by a name must not pass, or the name stops
	// meaning anything.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		d.post(t, "/v1/products", `{"name": "sonic"}`)
		d.post(t, "/v1/products/sonic/streams", `{"name": "2.4.0", "kind": "tag"}`)

		code, body := d.post(t, "/v1/products/sonic/streams", `{"name": "2.4.0", "kind": "branch"}`)
		if code != http.StatusConflict {
			t.Fatalf("a tag redeclared as a branch returned %d, want 409", code)
		}
		if detail, _ := body["detail"].(string); detail == "" {
			t.Errorf("the refusal says nothing: %v", body)
		}
	})
}

func TestAVariantShipsUnlessItSaysOtherwise(t *testing.T) {
	// An unclassified artifact should rank as though it reaches customers, so
	// leaving the field out must not read as a denial.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		d.post(t, "/v1/products", `{"name": "sonic"}`)
		d.post(t, "/v1/products/sonic/streams", `{"name": "master", "kind": "branch"}`)

		_, body := d.post(t, "/v1/products/sonic/variants", `{"name": "broadcom"}`)
		item, _ := body["item"].(map[string]any)
		if item["customer_facing"] != true {
			t.Errorf("a variant declared without saying reported %v", item["customer_facing"])
		}

		_, body = d.post(t, "/v1/products/sonic/variants",
			`{"name": "test-only", "customer_facing": false}`)
		item, _ = body["item"].(map[string]any)
		if item["customer_facing"] != false {
			t.Errorf("a variant declared as internal reported %v", item["customer_facing"])
		}
	})
}

func TestDeclaringUnderSomethingUndeclaredSaysWhichPart(t *testing.T) {
	twoCatalog(t, func(t *testing.T, d *declaring) {
		code, body := d.post(t, "/v1/products/sonic/streams", `{"name": "master", "kind": "branch"}`)
		if code != http.StatusNotFound {
			t.Fatalf("a stream under an undeclared product returned %d, want 404", code)
		}
		if detail, _ := body["detail"].(string); !bytes.Contains([]byte(detail), []byte("sonic")) {
			t.Errorf("the refusal does not name what is missing: %v", body)
		}
	})
}

func TestATagRecordsTheBranchItWasCutFrom(t *testing.T) {
	// It is what lets a branch be compared against its last release.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		d.post(t, "/v1/products", `{"name": "sonic"}`)
		d.post(t, "/v1/products/sonic/streams", `{"name": "master", "kind": "branch"}`)

		code, _ := d.post(t, "/v1/products/sonic/streams",
			`{"name": "2.4.0", "kind": "tag", "parent": "master"}`)
		if code != http.StatusCreated {
			t.Fatalf("declaring a tag cut from a branch returned %d", code)
		}
		code, body := d.post(t, "/v1/products/sonic/streams",
			`{"name": "2.5.0", "kind": "tag", "parent": "nonexistent"}`)
		if code != http.StatusNotFound {
			t.Errorf("a tag cut from an undeclared branch returned %d, want 404: %v", code, body)
		}
	})
}

func TestWhatHasBeenDeclaredCanBeListed(t *testing.T) {
	// The first question after an upload is refused for naming something
	// undeclared.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		d.post(t, "/v1/products", `{"name": "sonic"}`)
		d.post(t, "/v1/products", `{"name": "onie"}`)
		d.post(t, "/v1/products/sonic/streams", `{"name": "master", "kind": "branch"}`)
		d.post(t, "/v1/products/sonic/variants", `{"name": "broadcom"}`)

		_, body := d.get(t, "/v1/products")
		items, _ := body["items"].([]any)
		if len(items) != 2 {
			t.Errorf("listed %d products, want 2", len(items))
		}
		_, body = d.get(t, "/v1/products/sonic/streams")
		items, _ = body["items"].([]any)
		if len(items) != 1 {
			t.Errorf("listed %d streams, want 1", len(items))
		}
		_, body = d.get(t, "/v1/products/sonic/variants")
		items, _ = body["items"].([]any)
		if len(items) != 1 {
			t.Errorf("listed %d variants, want 1", len(items))
		}
	})
}

func TestADeclaredTargetCanBeUploadedAgainst(t *testing.T) {
	// The two halves have to meet: what declaration writes is what an upload
	// resolves. Testing them apart would let the names diverge.
	eachCatalog(t, func(t *testing.T, d *declaring) {
		d.post(t, "/v1/products", `{"name": "sonic"}`)
		d.post(t, "/v1/products/sonic/streams", `{"name": "master", "kind": "branch"}`)
		d.post(t, "/v1/products/sonic/variants", `{"name": "broadcom"}`)

		// The administrator sends it by hand, which is triage work rather than
		// a pipeline's job — and an administrator holds every role there is.
		req := upload(t, "/v1/products/sonic/streams/master/variants/broadcom/scans",
			inventory(nowish(), "libc6"))
		req.Header.Set(testHeader, d.admin)
		fromOurOwnPage(req)
		rec := httptest.NewRecorder()
		d.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("an upload against a freshly declared target returned %d: %s", rec.Code, rec.Body)
		}
	})
}
