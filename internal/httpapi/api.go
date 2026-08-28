// Package httpapi builds the HTTP surface.
//
// The OpenAPI document is generated from the operations registered here — it is
// never written by hand, so it cannot drift from what the server actually does.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bhouse-nexthop/openpsirt/internal/version"
)

// New returns the HTTP handler and the API description it was built from.
//
// The description is returned so the OpenAPI document can be written out
// without starting a server.
// Ready reports whether the service can do its job. A nil Ready means the
// readiness probe only reflects the process being up.
type Ready func(context.Context) error

func New(logger *slog.Logger, ready Ready, in Ingest) (http.Handler, huma.API) {
	router := chi.NewMux()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	// RealIP is deliberately absent. It rewrites the client address from
	// X-Forwarded-For and similar, which any caller can set, so it is only
	// safe behind a proxy known to overwrite them. Trusting proxy-supplied
	// headers happens in one place, guarded by a trusted-source check.

	// Liveness and readiness are deliberately outside the documented API and
	// carry no authentication: a container probe cannot sign in, and these
	// report nothing beyond whether the process can serve.
	//
	// Liveness answers as long as the process is running. Readiness also needs
	// the database, because a process that cannot reach its database is up but
	// useless, and sending it traffic helps nobody.
	router.Get("/healthz", plainOK)
	router.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			if err := ready(r.Context()); err != nil {
				logger.Warn("not ready", "error", err)
				http.Error(w, "not ready\n", http.StatusServiceUnavailable)
				return
			}
		}
		plainOK(w, r)
	})

	cfg := huma.DefaultConfig("OpenPSIRT", version.Get().Version)
	cfg.Info.Description = "Track vulnerabilities in the products you ship."
	// Documentation is published separately, so the server does not serve it.
	cfg.DocsPath = ""

	api := humachi.New(router, cfg)
	registerVersion(api)
	registerScans(api, in)
	registerCatalogue(api, Declaring{Store: in.catalogue})

	return router, api
}

func plainOK(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// VersionOutput is the body of a version response.
type VersionOutput struct {
	Body version.Info
}

func registerVersion(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-version",
		Method:      http.MethodGet,
		Path:        "/v1/version",
		Summary:     "Report the running build",
		Description: "Identifies the build that is answering, so an operator can tell which version they are looking at.",
		Tags:        []string{"Meta"},
	}, func(_ context.Context, _ *struct{}) (*VersionOutput, error) {
		return &VersionOutput{Body: version.Get()}, nil
	})
}
