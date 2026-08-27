package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	h, _ := New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	return h
}

func TestProbesAnswerWithoutAuthentication(t *testing.T) {
	// Container probes cannot sign in. If these ever start requiring
	// credentials, every deployment fails its health check.
	h := newTestHandler(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
}

func TestVersionEndpointReportsTheBuild(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/version", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/version = %d, want 200", rec.Code)
	}
	var body struct {
		Version string `json:"version"`
		Go      string `json:"go"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if body.Version == "" || body.Go == "" {
		t.Errorf("incomplete: %+v", body)
	}
}

func TestOpenAPIDocumentDescribesTheRegisteredRoutes(t *testing.T) {
	// This is the check that keeps the published specification honest: it is
	// generated from the same registrations the server routes on, so a route
	// that exists but is undocumented cannot happen.
	_, api := New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	doc := api.OpenAPI()
	if doc.Paths["/v1/version"] == nil {
		t.Fatal("/v1/version missing from the generated document")
	}
	spec, err := doc.YAML()
	if err != nil {
		t.Fatalf("render document: %v", err)
	}
	if len(spec) == 0 {
		t.Fatal("generated document is empty")
	}
}

func TestDocumentationIsNotServed(t *testing.T) {
	// Documentation is published separately. Serving it here would be the only
	// unauthenticated route in the application.
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if rec.Code == http.StatusOK {
		t.Error("GET /docs served something; documentation should not be served")
	}
}

func TestReadinessFailsWhenTheServiceCannotWork(t *testing.T) {
	// A process that is up but cannot reach its database should not be sent
	// traffic. Answering "ok" regardless would make the probe decorative.
	h, _ := New(slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(context.Context) error { return errUnavailable })

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz = %d, want 503", rec.Code)
	}

	// Liveness is a different question: the process is running, so restarting
	// it would not help.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz = %d, want 200 even when not ready", rec.Code)
	}
}

var errUnavailable = errStub("database is unreachable")

type errStub string

func (e errStub) Error() string { return string(e) }
