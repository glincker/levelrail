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
//   - No replicas or strategy fields on an app: internal/spec's app.yaml
//     Service has them, store.DesiredService doesn't yet, so this API
//     can't expose what the store can't hold. Adding them is a
//     store-schema and deploy-pipeline change, not something this
//     package should invent on its own. Domains closed once TASKS.md 1.6
//     added the column: appResource now carries Domains too.
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
//
// cmd/levelrail/main.go's reconcile engine closed the gap noted above in
// an earlier draft of this comment: it now derives a dynamic controller
// set from the store every pass (reconcile.Engine.Source), so a deploy
// triggered through this API does reconcile on the next pass.
//
// Secrets (TASKS.md 1.7): PUT /api/v1/apps/{name}/secrets/{key} sets a
// value, encrypted at rest via internal/secrets.Manager. Deliberately
// set-only, no GET: this package never decrypts a value for a response
// body, only internal/reconcile/application does, immediately before
// container creation. Available only when the control plane was started
// with a master key (WithSecretSetter); without one, the route returns
// 501.
//
// Auth foundation ("Dashboard & auth", TASKS.md): POST
// /api/v1/auth/register is the interactive first-run counterpart to
// BootstrapAdmin's env-var path, gated on "no admin row exists yet" at
// both the route and the mutation layer. Session auth stays exactly what
// TASKS.md 1.9 scoped it as (single admin user, no teams, no RBAC); API
// tokens (POST/GET /api/v1/auth/tokens, DELETE .../{id}) are a separate,
// additive credential type for non-interactive callers (a future CLI,
// an MCP server per CLAUDE.md 4.11), scoped to abilities (abilities.go:
// read, read:sensitive, write, deploy, root) checked fresh on every
// call by requireAbility, never a cached decision. Token management
// itself is session-only via requireAuth: a token can never mint or
// revoke another token on its own behalf. See docs-local/research/
// theauth-go-fit-assessment.md and competitor-onboarding-auth-ux.md for
// why this shape (not theauth-go, not an all-or-nothing key) was chosen.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

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
	CreateAdminUser(ctx context.Context, username, passwordHash string) error
}

// TokenStore is the store surface the API-token handlers and the
// ability-aware auth middleware need (TASKS.md "Backend auth
// foundation").
type TokenStore interface {
	SaveAPIToken(ctx context.Context, t store.APIToken) error
	GetAPITokenByHash(ctx context.Context, hash string) (*store.APIToken, error)
	ListAPITokens(ctx context.Context) ([]store.APIToken, error)
	RevokeAPIToken(ctx context.Context, id string) error
	TouchAPITokenLastUsed(ctx context.Context, id string) error
}

// Store is the full surface NewRouter needs. *store.DB satisfies it
// structurally, same pattern internal/reconcile/application.ServiceStore
// already established for this codebase.
type Store interface {
	AppStore
	DeployStore
	AuthStore
	TokenStore
}

// SecretSetter is the surface the secrets handler needs from
// internal/secrets.Manager (TASKS.md 1.7): set a value, never read one
// back. *secrets.Manager satisfies this structurally.
type SecretSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// Router wires every internal/api handler onto one http.Handler.
type Router struct {
	logger     *slog.Logger
	brand      *brand.Brand
	apps       AppStore
	deploys    DeployStore
	auth       AuthStore
	tokens     TokenStore
	secrets    SecretSetter     // nil is valid: a control plane with no master key configured serves everything except secret-setting
	telemetry  TelemetryQuerier // nil is valid: metrics/logs query routes return 501, same shape as secrets above
	sessions   *sessionStore
	logins     *loginLimiter
	sessionTTL time.Duration // 0 means "use defaultSessionTTL", set via WithSessionTTL
}

// Option configures optional Router behavior.
type Option func(*Router)

// WithSecretSetter enables PUT /api/v1/apps/{name}/secrets/{key}.
// Without one configured (the default), that route returns 501: an
// operator running Levelrail without APP_MASTER_KEY set gets a clear
// "not configured" response instead of the route not existing at all or
// silently discarding the value.
func WithSecretSetter(s SecretSetter) Option {
	return func(rt *Router) { rt.secrets = s }
}

// WithSessionTTL overrides how long a session cookie stays valid.
// Without one configured, defaultSessionTTL (24h) applies. CLAUDE.md 7's
// "no hardcoded thresholds, use env vars" rule is honored one layer up,
// at cmd/levelrail/main.go, which reads APP_SESSION_TTL and passes the
// parsed duration here; this package itself never reads the environment
// directly (NewRouter takes everything as constructor args, matching
// every other option here).
func WithSessionTTL(d time.Duration) Option {
	return func(rt *Router) { rt.sessionTTL = d }
}

// WithTelemetryQuerier enables GET /api/v1/apps/{name}/metrics and
// GET /api/v1/apps/{name}/logs (TASKS.md 2.3). Without one configured,
// both routes return 501, the same "not configured" shape
// WithSecretSetter's absence produces, rather than the routes not
// existing at all or panicking on a nil dereference.
func WithTelemetryQuerier(q TelemetryQuerier) Option {
	return func(rt *Router) { rt.telemetry = q }
}

// NewRouter builds a Router. logger defaults to slog.Default() if nil.
func NewRouter(logger *slog.Logger, b *brand.Brand, s Store, opts ...Option) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	rt := &Router{
		logger:  logger,
		brand:   b,
		apps:    s,
		deploys: s,
		auth:    s,
		tokens:  s,
		logins:  newLoginLimiter(),
	}
	for _, opt := range opts {
		opt(rt)
	}
	// sessions is built after options are applied, not in the struct
	// literal above, specifically so WithSessionTTL can influence it:
	// sessionStore reads its TTL once at construction, it isn't a field
	// re-read on every create() call.
	rt.sessions = newSessionStore(rt.sessionTTL)
	return rt
}

// Handler builds the *http.ServeMux with every route registered. Called
// once at startup; the returned handler is what gets wrapped in an
// *http.Server.
func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public: the frontend needs branding before a session exists (e.g.
	// on the login screen itself).
	mux.HandleFunc("GET /api/v1/brand", rt.handleBrand)

	// Auth. Login and first-run registration are necessarily public;
	// everything else requires an existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", rt.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireAuth(rt.handleLogout))

	// API tokens: session-only, deliberately never bearer-token
	// authenticated. A token cannot mint or revoke another token on its
	// own behalf; only an interactive human session can manage the
	// token set, the same boundary that stops a leaked scoped token from
	// escalating itself by minting a broader one.
	mux.HandleFunc("POST /api/v1/auth/tokens", rt.requireAuth(rt.handleCreateToken))
	mux.HandleFunc("GET /api/v1/auth/tokens", rt.requireAuth(rt.handleListTokens))
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{id}", rt.requireAuth(rt.handleRevokeToken))

	// Apps CRUD. requireAbility accepts either a session (implicitly
	// root, there is exactly one human identity in Phase 1) or a bearer
	// token scoped to at least the named ability, so a read-only
	// MCP-issued token is provably unable to reach a write/deploy route,
	// not just conventionally discouraged from calling it.
	mux.HandleFunc("GET /api/v1/apps", rt.requireAbility(AbilityRead, rt.handleListApps))
	mux.HandleFunc("POST /api/v1/apps", rt.requireAbility(AbilityWrite, rt.handleCreateApp))
	mux.HandleFunc("GET /api/v1/apps/{name}", rt.requireAbility(AbilityRead, rt.handleGetApp))
	mux.HandleFunc("PUT /api/v1/apps/{name}", rt.requireAbility(AbilityWrite, rt.handleUpdateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{name}", rt.requireAbility(AbilityWrite, rt.handleDeleteApp))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityDeploy, rt.handleTriggerDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityRead, rt.handleDeployHistory))

	// Secrets (TASKS.md 1.7). Set-only: there is deliberately no GET,
	// returning a value (even to its own owner over an authenticated
	// session) is exactly the kind of exposure envelope encryption
	// exists to avoid.
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleSetSecret))

	// Telemetry query (TASKS.md 2.3): metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryMetrics))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryLogs))

	// Prometheus remote read (TASKS.md 2.6). Gated by requireAbility the
	// same as every other read route, not left open: leaving a metrics
	// endpoint unauthenticated would let any caller pull every service's
	// resource usage. Prometheus's own remote_read config supports
	// bearer-token auth (authorization.credentials), which is exactly an
	// API token scoped to at least "read" (see the token management
	// routes above); requireAbility already accepts one the same way it
	// does for every JSON route, this isn't a special case.
	mux.HandleFunc("POST /api/v1/prometheus/read", rt.requireAbility(AbilityRead, rt.handlePrometheusRead))

	return mux
}
