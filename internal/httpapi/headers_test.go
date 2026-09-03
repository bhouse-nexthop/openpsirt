package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEveryResponseCarriesTheBrowserHeaders(t *testing.T) {
	// On the probes, on a refusal and on a route nothing serves alike: the
	// headers are set before any handler, so a route added later cannot lack
	// them, and a JSON body opened in a browser is still a page (SEC-20).
	h := newTestHandler(t)
	for _, path := range []string{"/healthz", "/v1/version", "/nothing-here"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		for name, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "same-origin",
		} {
			if got := rec.Header().Get(name); got != want {
				t.Errorf("GET %s: %s = %q, want %q", path, name, got, want)
			}
		}
		policy := rec.Header().Get("Content-Security-Policy")
		// The directives the interface depends on and the ones that keep it
		// from loading anything else, rather than the string verbatim: what
		// is pinned is what the policy permits and refuses.
		for _, directive := range []string{
			"default-src 'self'", "script-src 'self'", "connect-src 'self'",
			"frame-ancestors 'none'", "img-src 'self' data:",
		} {
			if !strings.Contains(policy, directive) {
				t.Errorf("GET %s: the policy lacks %q: %q", path, directive, policy)
			}
		}
		if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") ||
			strings.Contains(policy, "'unsafe-eval'") {
			t.Errorf("GET %s: the policy lets inline script run: %q", path, policy)
		}
	}
}
