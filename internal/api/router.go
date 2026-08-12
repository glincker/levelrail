// Package api implements TASKS.md 1.9: the HTTP API the web frontend
// (1.10) and, later, the MCP layer (CLAUDE.md 4.11) both build on. Scope
// for this pass, per TASKS.md 1.9 literally: GET /api/v1/brand, apps
// CRUD, deploy trigger and deploy history, single-admin-user session
// auth. No teams, no RBAC: explicitly out of scope until Phase 4.
//
// Routing uses the standard library's Go 1.22+ pattern-based
// http.ServeMux ("GET /api/v1/apps/{name}") rather than a router
// framework like Chi. CLAUDE.md 4.1 says "do not pull in a heavy
// framework"; the one thing a framework would earn its weight on here is
// auth middleware, and requireAuth below is a plain
// http.HandlerFunc-wrapping closure that stdlib's mux composes with
// directly, so there is no ergonomics gap stdlib doesn't already close.
//
// "App" here is layered directly on store.DesiredService
// (internal/store/service.go), the closest existing resource, rather
// than a new, speculative domain type. That surfaces real gaps instead
// of papering over them with a fuller model TASKS.md 1.9 doesn't
// actually ask this package to build yet:
//
//   - No domains, replicas, or strategy fields on an app: internal/spec's
//     app.yaml Service has them, store.DesiredService doesn't yet, so
//     this API can't expose what the store can't hold. Adding them is a
//     store-schema and deploy-pipeline change (1.3/1.4/1.5), not
//     something this package should invent on its own.
//   - Deploy trigger (deploys.go) only updates desired_services.image; it
//     doesn't build anything. TASKS.md 1.4's internal/build and
//     internal/deploy already exist and do that, but they're owned by a
//     different concurrent session as this package was written, so this
//     endpoint takes an already-built image tag as input, the same
//     mechanism TASKS.md 1.3 documents for rollback, run forward instead
//     of backward. Wiring an actual "build then deploy" endpoint is a
//     follow-up once internal/deploy's API is stable.
//   - Deploy history (deploys.go) returns the latest reconcile condition
//     per (controller, condition type) pair, which is genuinely all
//     internal/store.UpsertConditions persists today, not a row-per-
//     deploy-attempt log. A real deploy history table is a store-schema
//     addition this package deliberately didn't invent speculatively.
//   - cmd/levelrail/main.go's reconcile engine still only runs the Phase
//     0 hardcoded nginxdemo controller, not a dynamic
//     internal/reconcile/application.Controller per app. A deploy
//     triggered through this API updates desired state correctly, but
//     nothing reconciles that particular app until the engine is wired
//     to spawn per-app controllers. Also not this package's job to fix.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AppStore is the store surface the apps and deploys handlers need.
// *store.DB satisfies this structurally; tests use a real temp-file
// store the same way internal/store's own tests do, not a mock.
type AppStore interface {
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	DeleteDesiredService(ctx context.Context, name string) error
}

// DeployStore is the store surface the deploy-history handler needs.
type DeployStore interface {
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
}

// AuthStore is the store surface the auth handlers need.
type AuthStore interface {
	GetAdminUser(ctx context.Context) (*store.AdminUser, error)
	UpsertAdminUser(ctx context.Context, username, passwordHash string) error
}

// Store is the full surface NewRouter needs. *store.DB satisfies it
// structurally, same pattern internal/reconcile/application.ServiceStore
// already established for this codebase.
type Store interface {
	AppStore
	DeployStore
	AuthStore
}

// Router wires every internal/api handler onto one http.Handler.
type Router struct {
	logger   *slog.Logger
	brand    *brand.Brand
	apps     AppStore
	deploys  DeployStore
	auth     AuthStore
	sessions *sessionStore
}

// NewRouter builds a Router. logger defaults to slog.Default() if nil.
func NewRouter(logger *slog.Logger, b *brand.Brand, s Store) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	return &Router{
		logger:   logger,
		brand:    b,
		apps:     s,
		deploys:  s,
		auth:     s,
		sessions: newSessionStore(),
	}
}

// Handler builds the *http.ServeMux with every route registered. Called
// once at startup; the returned handler is what gets wrapped in an
// *http.Server.
func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public: the frontend needs branding before a session exists (e.g.
	// on the login screen itself).
	mux.HandleFunc("GET /api/v1/brand", rt.handleBrand)

	// Auth. Login is necessarily public; everything else requires an
	// existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireAuth(rt.handleLogout))

	// Apps CRUD.
	mux.HandleFunc("GET /api/v1/apps", rt.requireAuth(rt.handleListApps))
	mux.HandleFunc("POST /api/v1/apps", rt.requireAuth(rt.handleCreateApp))
	mux.HandleFunc("GET /api/v1/apps/{name}", rt.requireAuth(rt.handleGetApp))
	mux.HandleFunc("PUT /api/v1/apps/{name}", rt.requireAuth(rt.handleUpdateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{name}", rt.requireAuth(rt.handleDeleteApp))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAuth(rt.handleTriggerDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAuth(rt.handleDeployHistory))

	return mux
}
