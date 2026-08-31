// Package httpapi builds the HTTP surface.
//
// The OpenAPI document is generated from the operations registered here — it is
// never written by hand, so it cannot drift from what the server actually does.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bhouse-nexthop/openpsirt/internal/access"
	"github.com/bhouse-nexthop/openpsirt/internal/finding"
	"github.com/bhouse-nexthop/openpsirt/internal/setting"
	"github.com/bhouse-nexthop/openpsirt/internal/version"
)

// New returns the HTTP handler and the API description it was built from.
//
// The description is returned so the OpenAPI document can be written out
// without starting a server.
// Ready reports whether the service can do its job. A nil Ready means the
// readiness probe only reflects the process being up.
type Ready func(context.Context) error

// changesSomething reports whether a method is one that writes.
//
// Named as a list of what is safe rather than of what is not: a method absent
// from the list is treated as changing something, so an unusual one is guarded
// by default rather than by having been thought of.
func changesSomething(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

func New(logger *slog.Logger, ready Ready, in Ingest) (http.Handler, huma.API) {
	in.Logger = logger
	router := chi.NewMux()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	// One resolution step, before any handler. Doing it here rather than in
	// each handler is the whole point: a handler that forgets is a handler
	// answering for everybody, and the forgetting is invisible until somebody
	// reads the one that did.
	//
	// It attaches whoever was recognized and refuses nobody. What a subject
	// may reach is decided further down, where the query is, so that a route
	// added later cannot be less careful than the one beside it.
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Named exceptions rather than a guarded prefix. Guarding one
			// prefix leaves everything else open by default, and the
			// framework registers routes of its own: the API document and
			// the schemas it references were served to anybody who asked,
			// including the running version that the endpoint reporting it
			// is authenticated to withhold.
			//
			// The probes are the exception, because a container probe cannot
			// sign in and they report nothing beyond whether this process can
			// serve.
			if open[r.URL.Path] || strings.HasPrefix(r.URL.Path, openPrefix) {
				next.ServeHTTP(w, r)
				return
			}

			// A path this server has no route for belongs to the interface,
			// which does its own routing — and the page has to load for
			// somebody holding nothing, because the sign-in screen is the
			// page. Asked of the router rather than by matching a prefix, so
			// this cannot shadow a route: anything registered, including the
			// framework's own document and schema routes, still goes through
			// the check below. What is served here is a compiled page and its
			// assets, which carry no data.
			if in.Interface.Files != nil && !reserved(r.URL.Path) &&
				!router.Match(chi.NewRouteContext(), r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			var subject access.Subject
			var session *access.Session
			var err error
			if in.Access == nil {
				err = access.ErrDenied
			} else {
				subject, session, err = in.Access.Resolve(r.Context(), r)
			}
			if err != nil {
				// Refused here rather than in a handler, so that nothing
				// about the request is examined first. Otherwise an
				// unauthenticated caller learns whether their body was
				// well-formed, which is a small thing to hand somebody who
				// has not identified themselves at all.
				refuse(w)
				return
			}
			// A request a browser made by itself is one somebody else's page
			// may have caused, because the credential goes along without
			// anybody asking — our cookie, or the proxy's. Requests carrying a
			// key are exempt: nothing sends those automatically, so the guard
			// would protect nothing and break every build (ACC-18).
			if !meantToBeSent(r, session, in.BaseURL) {
				refuse(w)
				return
			}

			ctx := access.With(r.Context(), subject)
			if session != nil {
				ctx = access.WithSession(ctx, session)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	// Sign-in is mounted before the API description is built, because these
	// are browser redirects rather than operations: nothing calls them with a
	// generated client, and the answer is a 302 carrying cookies.
	registerSignIn(router, in)

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
	registerFindings(api, in)
	registerReceipts(api, in)
	registerSession(api, in)
	registerTokens(api, in)
	registerFindingDetail(api, in)
	registerAssignment(api, in)
	registerDue(api, in)
	registerGraph(api, in)
	registerSettings(api, in)
	registerProviders(api, in)
	registerWhoAmI(api, in)
	registerBulk(api, in)
	registerReports(api, in)
	registerCarry(api, in)
	registerTriage(api, in)
	registerTriageReading(api, in)
	registerSendBack(api, in)
	registerProposing(api, in)
	registerPlaceDecisions(api, in)
	registerElsewhere(api, in)
	registerBindings(api, Administering{
		Access: in.rights, Catalog: in.catalog, Logger: logger, Mode: in.Mode,
	}, func() *setting.Store {
		if in.DB == nil {
			return nil
		}
		return setting.NewStore(in.DB.DB)
	})
	registerCatalog(api, Declaring{Store: in.catalog, Logger: logger})
	registerAdministration(api, Administering{
		Access: in.rights, Catalog: in.catalog, Logger: logger, Mode: in.Mode,
		Findings: func() *finding.Store {
			if in.DB == nil {
				return nil
			}
			return finding.NewStore(in.DB.DB)
		},
	})
	registerRevocation(api, Administering{
		Access: in.rights, Catalog: in.catalog, Logger: logger, Mode: in.Mode,
	})

	// Last, so it claims only what nothing above it did.
	mountInterface(router, in.Interface)

	return router, api
}

// wentWrong reports a fault to the caller without describing it to them.
//
// The framework serializes an error passed alongside the message, so handing
// it one hands the caller the query text and whatever the driver put in its
// message — which for a connection failure is the address and the user it
// tried. Whoever operates this deployment needs that; whoever is asking does
// not.
func wentWrong(logger *slog.Logger, what string, err error) error {
	if logger != nil {
		logger.Error(what, "error", err)
	}
	return huma.Error500InternalServerError(what)
}

// open is every path served without a credential. It is a list rather than a
// rule so that adding a route never quietly adds an exception.
var open = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	// What somebody sees before they have a credential. It lists the
	// providers an operator configured and nothing else — the sign-in page
	// has to draw a button per provider, and it cannot ask for that list
	// while holding nothing. Everything under openPrefix already answers
	// without a credential for the same reason; this is the list of what is
	// down there.
	"/v1/sign-in": true,
}

// openPrefix is the one place a prefix is used rather than a named route,
// because sign-in cannot name its routes in advance: the provider is part of
// the path and the set of providers is configuration.
//
// It is narrow on purpose. Everything under it either redirects to a provider
// or refuses, and nothing under it reads anything — so a route added here by
// mistake can leak a redirect and not data.
const openPrefix = "/v1/sign-in/"

// refuse answers somebody unrecognized.
//
// The same answer whoever they are: unknown, known and granted nothing, or
// holding a credential that has been revoked. Saying which would tell an
// outsider whether a name or a key is real.
func refuse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"title":"Unauthorized","status":401,"detail":"not authorized"}` + "\n"))
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
		Summary:     "Get the server version",
		Description: "Identifies the build that is answering, so an operator can tell which version they are looking at.",
		Tags:        []string{"Meta"},
	}, func(ctx context.Context, _ *struct{}) (*VersionOutput, error) {
		// A person, not a pipeline. A build server has no business asking
		// what is deployed, and "nothing else" has to mean this too or it
		// means whatever each new endpoint remembers.
		if _, err := reading(ctx); err != nil {
			return nil, err
		}
		return &VersionOutput{Body: version.Get()}, nil
	})
}
