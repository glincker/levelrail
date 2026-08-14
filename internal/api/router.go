// Package api implements TASKS.md 1.9: the HTTP API the web frontend
// (1.10) and, later, the MCP layer both build on. Scope
// for this pass, per TASKS.md 1.9 literally: GET /api/v1/brand, apps
// CRUD, deploy trigger and deploy history, single-admin-user session
// auth. No teams, no RBAC: explicitly out of scope until Phase 4.
//
// Routing uses the standard library's Go 1.22+ pattern-based
// http.ServeMux ("GET /api/v1/apps/{name}") rather than a router
// framework like Chi, per the project's rule against pulling in a heavy
// framework; the one thing a framework would earn its weight on here is
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
//     of backward. POST /api/v1/apps/{name}/builds (builds.go,
//     handleTriggerBuild) closes this gap: it invokes the same
//     internal/deploy.Pipeline the git webhook receiver uses, given a git
//     source (repo_url/ref) in the request body, since no app has a
//     stored git/build config anywhere in this codebase yet (see
//     specServiceFromDesired's own doc comment for what that means for
//     fidelity versus the original app.yaml).
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
// an MCP server), scoped to abilities (abilities.go:
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
	"github.com/GLINCKER/levelrail/internal/docker"
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
	// UpdateServiceNode is TASKS.md 3.3's placement mutation, separate
	// from SaveDesiredService on purpose: see store.DB.SaveDesiredService's
	// own doc comment for why an ordinary app update must never be able
	// to silently move a service between nodes.
	UpdateServiceNode(ctx context.Context, name, nodeID string) error
	// ListDesiredServicesByNode is TASKS.md 3.7's drain and
	// delete-guard primitive (handleDrainNode, handleDeleteNode): find
	// what's placed on a node without listing every service.
	ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]store.DesiredService, error)
}

// DeployStore is the store surface the deploy-history handler needs.
type DeployStore interface {
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
}

// DatabaseStore is the store surface the databases handlers need, the
// database-kind counterpart to AppStore.
type DatabaseStore interface {
	SaveDesiredDatabase(ctx context.Context, d store.DesiredDatabase) error
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	DeleteDesiredDatabase(ctx context.Context, name string) error
	// UpdateDatabaseNode is AppStore.UpdateServiceNode's counterpart,
	// same separation-from-ordinary-update reasoning. Originally added
	// unexposed by 3.3; handleDrainNode is the first caller with an
	// actual route to reuse it through.
	UpdateDatabaseNode(ctx context.Context, name, nodeID string) error
	// ListDesiredDatabasesByNode is the database-kind counterpart to
	// AppStore.ListDesiredServicesByNode.
	ListDesiredDatabasesByNode(ctx context.Context, nodeID string) ([]store.DesiredDatabase, error)
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
	DatabaseStore
	AuthStore
	TokenStore
	NodeStore
	CertStore
	StaticSiteStore
}

// SecretSetter is the surface the secrets handler needs from
// internal/secrets.Manager (TASKS.md 1.7): set a value, never read one
// back. *secrets.Manager satisfies this structurally.
type SecretSetter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// DockerPinger is the surface GET /api/v1/system/status needs to report
// Docker daemon connectivity: a liveness check, nothing else.
// *docker.Client satisfies this structurally via the Ping method added
// directly to that concrete type (internal/docker/client.go), not
// through docker.Runtime: Runtime has 6+ fake implementations across
// internal/reconcile's controllers and internal/agent's test files,
// plus the real client and the agent-side remote transport, so adding a
// method there for the sake of this one read-only health signal would
// ripple into all of them. This narrow interface is the actual
// consumer-defined boundary instead, the same shape SecretSetter and
// TelemetryQuerier already establish for their own single-purpose
// surfaces.
type DockerPinger interface {
	Ping(ctx context.Context) error
}

// ImageLister is the surface GET /api/v1/apps/{name}/images needs:
// discover previously-built tags under a repo, so the deploy trigger
// form (web/src/components/DeployTriggerForm.tsx) can offer a dropdown
// instead of forcing an operator to hand-type a full image reference
// every time. Unlike DockerPinger above, this mirrors an existing
// docker.Runtime method (internal/docker/runtime.go) exactly rather than
// inventing a narrower one: ListImages is already the consumer-defined
// boundary every reconciler controller depends on for rollback candidate
// discovery, so redeclaring it here (structurally satisfied by
// *docker.Client, same as Runtime) avoids a second, divergent surface
// for the same one capability. *docker.Client satisfies this
// structurally.
type ImageLister interface {
	ListImages(ctx context.Context, repo string) ([]docker.ImageInfo, error)
}

// Router wires every internal/api handler onto one http.Handler.
type Router struct {
	logger       *slog.Logger
	brand        *brand.Brand
	apps         AppStore
	deploys      DeployStore
	databases    DatabaseStore
	auth         AuthStore
	tokens       TokenStore
	nodes        NodeStore
	secrets      SecretSetter     // nil is valid: a control plane with no master key configured serves everything except secret-setting
	telemetry    TelemetryQuerier // nil is valid: metrics/logs query routes return 501, same shape as secrets above
	alertRules   AlertRules       // nil is valid: alert rule routes return 501, same shape as secrets/telemetry above
	sessions     *sessionStore
	logins       *loginLimiter
	sessionTTL   time.Duration // 0 means "use defaultSessionTTL", set via WithSessionTTL
	dataDir      string        // "" means "don't report disk usage", set via WithDataDir
	dockerPinger DockerPinger  // nil is valid: a control plane started without one reports DockerConnected: false, same shape as secrets/telemetry/alertRules above
	images       ImageLister   // nil is valid: GET /apps/{name}/images returns an empty list, same shape as dockerPinger above
	certs        CertStore     // always set, part of the core Store interface: unlike dockerPinger/images this isn't an optional plug-in, every *store.DB already has it
	// certExpiryWarningWindow overrides defaultCertExpiryWarningWindow
	// for GET /api/v1/certificates's "expiring_soon" threshold. 0 means
	// "use the default", set via WithCertExpiryWarningWindow.
	certExpiryWarningWindow time.Duration
	builder                 Builder         // nil is valid: POST /apps/{name}/builds returns 501, same shape as secrets/telemetry/alertRules above
	fetch                   fetchFunc       // git source fetcher for handleTriggerBuild; always non-nil, defaulted to gitCheckout in NewRouter, overridable in this package's own tests
	staticSites             StaticSiteStore // always set, same "core Store interface, not an optional plug-in" shape as certs above
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
// Without one configured, defaultSessionTTL (24h) applies. The project's
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

// WithAlertRules enables POST/GET /api/v1/apps/{name}/alerts and DELETE
// /api/v1/apps/{name}/alerts/{id} (TASKS.md 2.5/2.7). Without one
// configured (the default), all three routes return 501, the same
// "not configured" shape WithSecretSetter and WithTelemetryQuerier's
// absence already produce: this control plane still starts and serves
// everything else without an alerting.DB wired in.
func WithAlertRules(a AlertRules) Option {
	return func(rt *Router) { rt.alertRules = a }
}

// WithDataDir enables disk-usage reporting on GET /api/v1/system/status.
// path should be the same APP_DATA_DIR the control plane itself was
// started with. Without one configured (the default), the status
// response simply omits the two data-dir byte fields, the same
// "optional signal, absence is not an error" shape WithSecretSetter's
// own absence already has for secret-setting.
func WithDataDir(path string) Option {
	return func(rt *Router) { rt.dataDir = path }
}

// WithDockerPinger enables Docker daemon connectivity reporting on
// GET /api/v1/system/status. Without one configured (the default), the
// response's DockerConnected field simply stays false, the same
// "optional signal, absence is not an error" shape WithDataDir and
// WithSecretSetter's own absence already have.
func WithDockerPinger(p DockerPinger) Option {
	return func(rt *Router) { rt.dockerPinger = p }
}

// WithImageLister enables GET /api/v1/apps/{name}/images. Without one
// configured (the default), that route returns an empty list rather
// than 501 or 404: the deploy trigger form's manual text input always
// works regardless, so a missing image lister is "no suggestions
// available" (an empty array), not a request error, the same
// "optional signal, absence is not an error" shape WithDockerPinger's
// own absence already has.
func WithImageLister(l ImageLister) Option {
	return func(rt *Router) { rt.images = l }
}

// WithCertExpiryWarningWindow overrides how far ahead of a certificate's
// expiry GET /api/v1/certificates starts reporting "expiring_soon"
// instead of "healthy" (see statusForExpiry). Without one configured (or
// passed as 0), defaultCertExpiryWarningWindow (14 days) applies. Same
// "no hardcoded thresholds, use env vars" shape as WithSessionTTL: this
// package never reads the environment directly, cmd/levelrail/main.go
// reads APP_CERT_EXPIRY_WARNING_WINDOW and passes the parsed duration
// here.
func WithCertExpiryWarningWindow(d time.Duration) Option {
	return func(rt *Router) { rt.certExpiryWarningWindow = d }
}

// WithBuilder enables POST /api/v1/apps/{name}/builds: a manual build
// trigger for an operator with no working git webhook configured (see
// internal/webhook.Config's own doc comment on why that path is
// deliberately static, single-app configuration). Without one configured
// (the default), that route returns 501, the same "not configured" shape
// WithSecretSetter/WithTelemetryQuerier/WithAlertRules absence already
// share. cmd/levelrail/main.go passes the same *deploy.Pipeline the git
// webhook receiver uses, when a BuildKit connection was successfully
// established at startup.
func WithBuilder(b Builder) Option {
	return func(rt *Router) { rt.builder = b }
}

// NewRouter builds a Router. logger defaults to slog.Default() if nil.
func NewRouter(logger *slog.Logger, b *brand.Brand, s Store, opts ...Option) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	rt := &Router{
		logger:      logger,
		brand:       b,
		apps:        s,
		deploys:     s,
		databases:   s,
		auth:        s,
		tokens:      s,
		nodes:       s,
		certs:       s,
		staticSites: s,
		fetch:       gitCheckout,
		logins:      newLoginLimiter(),
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
	mux.HandleFunc("GET /api/v1/dev-mode", rt.handleDevMode)

	// System status (General settings page): configured/not-configured
	// signals plus disk usage, AbilityRead like everything else an
	// authenticated operator can passively view.
	mux.HandleFunc("GET /api/v1/system/status", rt.requireAbility(AbilityRead, rt.handleSystemStatus))

	// Auth. Login and first-run registration are necessarily public;
	// everything else requires an existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", rt.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireAuth(rt.handleLogout))
	mux.HandleFunc("PUT /api/v1/auth/password", rt.requireAuth(rt.handleChangePassword))
	mux.HandleFunc("GET /api/v1/auth/session", rt.requireAuth(rt.handleGetSession))
	mux.HandleFunc("POST /api/v1/auth/sessions/revoke-others", rt.requireAuth(rt.handleRevokeOtherSessions))

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

	// Placement (TASKS.md 3.3): AbilityRoot, not AbilityWrite, matching
	// the sensitivity of the standalone node routes above: moving a
	// service between physical machines is infrastructure placement,
	// not ordinary app config, even though it's reached through this
	// app-scoped URL.
	mux.HandleFunc("PUT /api/v1/apps/{name}/node", rt.requireAbility(AbilityRoot, rt.handleSetAppNode))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityDeploy, rt.handleTriggerDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityRead, rt.handleDeployHistory))

	// Manual build trigger (see Builder/WithBuilder above and
	// handleTriggerBuild's own doc comment): builds an image from a git
	// source through the same internal/deploy.Pipeline the webhook
	// receiver uses, for an operator with no working git webhook
	// configured. AbilityDeploy, the same boundary as the image-tag
	// trigger above: this also ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/builds", rt.requireAbility(AbilityDeploy, rt.handleTriggerBuild))

	// Previously-built image tags for this app's repo, so the deploy
	// trigger form can offer a dropdown instead of a hand-typed tag
	// (see ImageLister above). AbilityRead like every other passive
	// view of an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/images", rt.requireAbility(AbilityRead, rt.handleListImages))

	// Databases CRUD, the database-kind counterpart to apps CRUD above.
	// No PUT (full-replace update) yet: unlike a service's image/port/
	// domains, none of engine/version/name are meant to change in place
	// once created (an engine or major-version change is a migration,
	// not a config edit), so there is nothing for an update endpoint to
	// legitimately do yet.
	// Read-only, ahead of the CRUD routes below since it's not scoped to
	// any one database: every engine this control plane can create at
	// all, backed by the embedded database_engines.yaml registry rather
	// than a hardcoded list, see handleListDatabaseEngines' own doc
	// comment.
	mux.HandleFunc("GET /api/v1/database-engines", rt.requireAbility(AbilityRead, rt.handleListDatabaseEngines))

	mux.HandleFunc("GET /api/v1/databases", rt.requireAbility(AbilityRead, rt.handleListDatabases))
	mux.HandleFunc("POST /api/v1/databases", rt.requireAbility(AbilityWrite, rt.handleCreateDatabase))
	mux.HandleFunc("GET /api/v1/databases/{name}", rt.requireAbility(AbilityRead, rt.handleGetDatabase))
	mux.HandleFunc("DELETE /api/v1/databases/{name}", rt.requireAbility(AbilityWrite, rt.handleDeleteDatabase))
	mux.HandleFunc("GET /api/v1/databases/{name}/status", rt.requireAbility(AbilityRead, rt.handleDatabaseStatus))

	// Placement (TASKS.md 3.3), the database counterpart to
	// PUT /apps/{name}/node above: same AbilityRoot gating.
	mux.HandleFunc("PUT /api/v1/databases/{name}/node", rt.requireAbility(AbilityRoot, rt.handleSetDatabaseNode))

	// Secrets (TASKS.md 1.7). Set-only: there is deliberately no GET,
	// returning a value (even to its own owner over an authenticated
	// session) is exactly the kind of exposure envelope encryption
	// exists to avoid.
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}", rt.requireAbility(AbilityWriteSensitive, rt.handleSetSecret))

	// Telemetry query (TASKS.md 2.3): metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryMetrics))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryLogs))

	// Alerting (TASKS.md 2.5/2.7): threshold and crashloop rules scoped
	// to one app, fanned through a *alerting.DB when configured (see
	// WithAlertRules).
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityWrite, rt.handleCreateAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityRead, rt.handleListAlertRules))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/alerts/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteAlertRule))

	// Prometheus remote read (TASKS.md 2.6). Gated by requireAbility the
	// same as every other read route, not left open: leaving a metrics
	// endpoint unauthenticated would let any caller pull every service's
	// resource usage. Prometheus's own remote_read config supports
	// bearer-token auth (authorization.credentials), which is exactly an
	// API token scoped to at least "read" (see the token management
	// routes above); requireAbility already accepts one the same way it
	// does for every JSON route, this isn't a special case.
	mux.HandleFunc("POST /api/v1/prometheus/read", rt.requireAbility(AbilityRead, rt.handlePrometheusRead))

	// Nodes (TASKS.md 3.1): fleet-level infrastructure, not scoped to any
	// one app, so every route here requires AbilityRoot specifically
	// rather than AbilityRead/AbilityWrite: minting a join token or
	// removing a node is a materially more sensitive operation than
	// editing one app's config, the same reasoning secrets.go's
	// AbilityWriteSensitive already applies one level down at the
	// per-app layer.
	mux.HandleFunc("GET /api/v1/nodes", rt.requireAbility(AbilityRoot, rt.handleListNodes))
	mux.HandleFunc("GET /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNode))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteNode))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/workloads", rt.requireAbility(AbilityRoot, rt.handleSetNodeWorkloads))
	mux.HandleFunc("POST /api/v1/nodes/join-tokens", rt.requireAbility(AbilityRoot, rt.handleCreateNodeJoinToken))
	// Health, cordon, drain (TASKS.md 3.7), same AbilityRoot boundary as
	// every other node route above.
	mux.HandleFunc("GET /api/v1/nodes/{id}/health", rt.requireAbility(AbilityRoot, rt.handleGetNodeHealth))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cordon", rt.requireAbility(AbilityRoot, rt.handleCordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/uncordon", rt.requireAbility(AbilityRoot, rt.handleUncordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", rt.requireAbility(AbilityRoot, rt.handleDrainNode))

	// Certificates (TLS renewal visibility): this project treats
	// "a cert renewal fails silently at 3am" as its central
	// risk to catch before it bites a real user, and until now nothing
	// in the API surfaced certificate state at all. Read-only, so
	// AbilityRead like handleSystemStatus/handleGetNodeHealth, not
	// AbilityRoot: seeing whether a domain's certificate is about to
	// expire is ordinary operator visibility, not a fleet-admin
	// mutation the way the node routes above are.
	mux.HandleFunc("GET /api/v1/certificates", rt.requireAbility(AbilityRead, rt.handleListCertificates))

	// Static sites (build.type: static): read-only
	// dashboard visibility for sites served directly by embedded Caddy
	// with no container, closing the gap flagged when static-site
	// support (migration 0015) first landed. AbilityRead, the same
	// passive-visibility boundary as handleListCertificates above: no
	// create/update/delete route exists yet because the backend has no
	// mutation path for a static site beyond internal/deploy.Pipeline's
	// own git-push-triggered write.
	mux.HandleFunc("GET /api/v1/static-sites", rt.requireAbility(AbilityRead, rt.handleListStaticSites))

	return mux
}
