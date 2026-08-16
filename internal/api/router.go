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

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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
	// RestartService is the only way to force a running container to be
	// recreated without an image change: a redeploy of the same image
	// tag is otherwise a genuine reconciler no-op (see
	// internal/reconcile/application.ContainerName's own doc comment).
	// See store.DB.RestartService's own doc comment for why this is
	// deliberately excluded from SaveDesiredService's full-record-replace
	// semantics, the same reasoning UpdateServiceNode already establishes
	// for NodeID.
	RestartService(ctx context.Context, name string) error
	// UpdateServiceProject is UpdateServiceNode's project-kind
	// counterpart (projects.go): same separation-from-ordinary-update
	// reasoning, see store.DB.UpdateServiceProject's own doc comment.
	UpdateServiceProject(ctx context.Context, name, projectID string) error
	// UpdateServiceStorageTarget backs PUT/DELETE
	// /api/v1/apps/{name}/storage (apps_storage.go): which store.BackupTarget
	// (already-established interface, BackupTargetStore below) this app's
	// own object-storage credentials resolve from. Same
	// separation-from-ordinary-update reasoning as UpdateServiceNode/
	// UpdateServiceProject, see store.DB.UpdateServiceStorageTarget's own
	// doc comment.
	UpdateServiceStorageTarget(ctx context.Context, name, storageTargetID string) error
	// UpdateServiceSuspended backs POST /api/v1/apps/{name}/stop and
	// .../start (handleStopApp/handleStartApp): same
	// separation-from-ordinary-update reasoning as UpdateServiceNode/
	// UpdateServiceProject/UpdateServiceStorageTarget, see
	// store.DB.UpdateServiceSuspended's own doc comment.
	UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error
}

// AppGroupStore is the store surface GET /api/v1/apps/{name}/group
// needs (apps_group.go): stage 1 of multi-service apps
// (migrations/0039_apps.sql), read path only. *store.DB satisfies this
// structurally.
type AppGroupStore interface {
	ListServicesByApp(ctx context.Context, appID string) ([]store.DesiredService, error)
}

// DeployStore is the store surface the deploy-history handler needs.
type DeployStore interface {
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
	// GetConditionsForControllers is handleListApps' batched status
	// source (apps.go): one query for every app's application
	// controller, not a GetConditions call per app.
	GetConditionsForControllers(ctx context.Context, controllerNames []string) (map[string][]reconcile.Condition, error)
}

// DeployAttemptStore is the store surface real deploy-attempt history
// needs: row-per-attempt CRUD, layered alongside DeployStore above
// rather than replacing it. See handleDeployHistory's own doc comment
// for why GET /api/v1/apps/{name}/deploys keeps returning reconcile
// conditions unchanged (an existing, real frontend consumer,
// web/src/queries/deploys.ts's useDeployStatus, already depends on that
// shape) while this interface backs a new, additional endpoint,
// GET /api/v1/apps/{name}/deploy-attempts, instead of overloading the
// old one. *store.DB satisfies this structurally.
type DeployAttemptStore interface {
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error
	GetDeployAttempt(ctx context.Context, id string) (*store.DeployAttempt, error)
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
}

// DeployLogStore is the telemetry-side read surface
// handleDeployLogStream needs to serve a full replay once an attempt has
// already finished (see that handler's own doc comment for the
// in-progress-vs-finished split). *telemetry.DB satisfies this
// structurally. Optional (nil is valid, the same "not configured" shape
// WithTelemetryQuerier's own absence already has): without one, a
// finished attempt's log route returns 501; an in-progress attempt's
// live tail (backed by deployRecorder below, a separate optional field)
// is unaffected by this one being unset.
type DeployLogStore interface {
	QueryDeployLog(ctx context.Context, attemptID string) ([]telemetry.DeployLogEntry, error)
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
	// UpdateDatabaseProject is AppStore.UpdateServiceProject's
	// counterpart (projects.go).
	UpdateDatabaseProject(ctx context.Context, name, projectID string) error
	// SetDatabaseBackupSchedule backs
	// PUT/DELETE /api/v1/databases/{name}/backup-schedule (backups.go):
	// wave-2 roadmap item 6, scheduled backups. Same "own endpoint, own
	// store method" separation UpdateDatabaseNode/UpdateDatabaseProject
	// already establish for their own single-purpose updates.
	SetDatabaseBackupSchedule(ctx context.Context, name, targetID, schedule string, retain int) error
	// SetDatabasePublicAccess backs
	// PUT/DELETE /api/v1/databases/{name}/public-access
	// (database_public_access.go): the same "own endpoint, own store
	// method" separation SetDatabaseBackupSchedule already establishes,
	// applied to whether this database's container port is bound to a
	// host port. Returns the port actually assigned (0 when disabling).
	SetDatabasePublicAccess(ctx context.Context, name string, enabled bool, requestedPort int) (int, error)
}

// ProjectStore is the store surface the projects handlers need
// (projects.go). See that file's own package doc comment for why a
// project is deliberately not the start of the deferred Phase 4 teams/
// RBAC work (repo-plan section 6's own scope note): no owner, no
// member list, no per-project ability, just create/list/get/delete.
type ProjectStore interface {
	SaveProject(ctx context.Context, p store.Project) error
	GetProject(ctx context.Context, id string) (store.Project, error)
	ListProjects(ctx context.Context) ([]store.Project, error)
	DeleteProject(ctx context.Context, id string) error
}

// AuthStore is the store surface the auth handlers need.
type AuthStore interface {
	GetUserByEmail(ctx context.Context, email string) (*store.User, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	CreateUser(ctx context.Context, u store.User) error
	UpdateUserPasswordHash(ctx context.Context, id string, hash *string) error
	UpdateUserLastLogin(ctx context.Context, id string, when time.Time) error
	CountUsers(ctx context.Context) (int, error)
	ListUsers(ctx context.Context) ([]store.User, error)
	DeleteUser(ctx context.Context, id string) error
}

// EmailSettingsStore is the store surface GET/PUT
// /api/v1/settings/email need: the single platform-wide row, always
// present, the same shape IngressSettingsStore has for its own row.
type EmailSettingsStore interface {
	GetEmailSettings(ctx context.Context) (store.EmailSettings, error)
	UpdateEmailSettings(ctx context.Context, s store.EmailSettings) error
}

// PasswordResetTokenStore is the store surface the forgot-password flow
// needs: always set, part of the core Store interface.
type PasswordResetTokenStore interface {
	SavePasswordResetToken(ctx context.Context, t store.PasswordResetToken) error
	GetPasswordResetTokenByHash(ctx context.Context, hash string) (*store.PasswordResetToken, error)
	ClaimPasswordResetToken(ctx context.Context, id string) error
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
	AppGroupStore
	DeployStore
	DeployAttemptStore
	DatabaseStore
	AuthStore
	TokenStore
	NodeStore
	CertStore
	StaticSiteStore
	BackupTargetStore
	BackupHistoryStore
	RestoreHistoryStore
	ProjectStore
	IngressSettingsStore
	DomainStore
	GitSourceStore
	GitHubAppStore
	OAuthSettingsStore
	OAuthIdentityStore
	EmailSettingsStore
	PasswordResetTokenStore
}

// SecretSetter is the surface the secrets handlers need from
// internal/secrets.Manager: set a value (with a reversible per-key lock
// guard), list which keys exist, toggle a key's lock, never read one
// back. Every other secret-backed feature in this file keeps using
// Manager's plain SetValue directly, unaffected by this narrower
// interface.
type SecretSetter interface {
	SetValueGuarded(ctx context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error
	ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	SetLocked(ctx context.Context, serviceName, envKey string, locked bool) error
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

// DockerDiskUsager is the surface GET /api/v1/system/status needs for
// Docker's own storage accounting (images/containers/volumes/build
// cache), a materially different number from DataDirTotalBytes/
// DataDirFreeBytes above: see docker.DiskUsage's own doc comment for
// why. Narrow and single-purpose like DockerPinger, not docker.Runtime,
// same "adding to Runtime would ripple into 6+ fakes for one read-only
// signal" reasoning DockerPinger's own doc comment gives. *docker.Client
// satisfies this structurally via the DiskUsage method added directly to
// that concrete type (internal/docker/prune.go).
type DockerDiskUsager interface {
	DiskUsage(ctx context.Context) (docker.DiskUsage, error)
}

// DockerPruner is the surface POST /api/v1/system/prune needs: run every
// cleanup stage internal/docker/prune.go supports and report what
// happened. Unlike DockerDiskUsager (a read), this is a destructive-ish
// action, gated at AbilityRoot the same way handleDrainNode is (see
// router.go's own route registration), so it gets its own interface
// rather than being folded into DockerDiskUsager: a read and a
// destructive action deserve independently swappable seams, the same
// split Builder/ImageLister already have. *docker.Client satisfies this
// structurally via the Prune method (internal/docker/prune.go).
type DockerPruner interface {
	Prune(ctx context.Context, keep []string) docker.PruneResult
}

// NodeRuntimeResolver picks the docker.Runtime that owns a given
// nodeID: the control plane's own local Docker daemon for "" (the
// store's established "empty NodeID means this control plane's own
// local node" convention, store.DesiredService.NodeID's own doc
// comment), or a connected remote agent's Transport for anything else.
// This is a func type, not an interface, the same pattern fetchFunc
// (builds.go) already establishes for a single-method seam: the real
// implementation is cmd/levelrail/main.go's own resolveNodeTransport
// (used identically by every reconciler controller to pick which node's
// Runtime it drives), passed in as a closure over that function's own
// registry rather than this package importing internal/agent.Registry
// directly.
//
// handleExecApp (exec.go) is the first internal/api handler that needs
// live, per-request node routing: DockerPinger/ImageLister/
// DockerDiskUsager/DockerPruner above are all fixed to this control
// plane's own local daemon, a deliberate choice their own doc comments
// explain (adding a method to docker.Runtime itself would ripple into
// every one of Runtime's 6+ fake implementations across
// internal/reconcile and internal/agent's test files, for the sake of
// one read-only or narrowly-scoped signal). Exec has the opposite
// requirement: it must reach whichever node the target app's container
// is actually running on, so it needs the resolver, not a fixed
// Runtime.
type NodeRuntimeResolver func(nodeID string) (docker.Runtime, error)

// Router wires every internal/api handler onto one http.Handler.
type Router struct {
	logger          *slog.Logger
	brand           *brand.Brand
	apps            AppStore
	appGroups       AppGroupStore
	deploys         DeployStore
	databases       DatabaseStore
	auth            AuthStore
	tokens          TokenStore
	nodes           NodeStore
	projects        ProjectStore
	secrets         SecretSetter     // nil is valid: a control plane with no master key configured serves everything except secret-setting
	telemetry       TelemetryQuerier // nil is valid: metrics/logs query routes return 501, same shape as secrets above
	alertRules      AlertRules       // nil is valid: alert rule routes return 501, same shape as secrets/telemetry above
	sessions        *sessionStore
	logins          *loginLimiter
	sessionTTL      time.Duration        // 0 means "use defaultSessionTTL", set via WithSessionTTL
	dataDir         string               // "" means "don't report disk usage", set via WithDataDir
	dockerPinger    DockerPinger         // nil is valid: a control plane started without one reports DockerConnected: false, same shape as secrets/telemetry/alertRules above
	images          ImageLister          // nil is valid: GET /apps/{name}/images returns an empty list, same shape as dockerPinger above
	dockerDiskUsage DockerDiskUsager     // nil is valid: GET /system/status omits its docker_disk_usage field, same "optional signal, absence is not an error" shape as dockerPinger above
	dockerPruner    DockerPruner         // nil is valid: POST /system/prune returns 501, same shape as builder/secrets above
	execRuntime     NodeRuntimeResolver  // nil is valid: POST /apps/{name}/exec returns 501, same shape as dockerPruner above
	certs           CertStore            // always set, part of the core Store interface: unlike dockerPinger/images this isn't an optional plug-in, every *store.DB already has it
	ingressSettings IngressSettingsStore // always set, same "core Store interface, not an optional plug-in" shape as certs above: the settings row always exists (migrations/0023's own seeded row)
	domains         DomainStore          // always set, same shape as ingressSettings above: service_domains is always queryable, empty is a valid, non-error result
	// publicHost is APP_PUBLIC_HOST: the IP or hostname operators should
	// point a DNS record at to reach this control plane's embedded
	// ingress. "" means "not configured", set via WithPublicHost;
	// handleCheckDomain (domain_check.go) falls back to the request's own
	// Host header in that case, see advertisedHost's own doc comment.
	publicHost string
	// lookupHost resolves a hostname's A/AAAA addresses for
	// handleCheckDomain; always non-nil, defaulted to defaultLookupHost
	// (a thin net.DefaultResolver.LookupHost wrapper) in NewRouter,
	// overridable in this package's own tests the same way fetch and
	// listBranches already are.
	lookupHost lookupHostFunc
	// domainChecks rate-limits handleCheckDomain's real DNS lookups per
	// domain; always non-nil, constructed in NewRouter.
	domainChecks *domainCheckCache
	// certExpiryWarningWindow overrides defaultCertExpiryWarningWindow
	// for GET /api/v1/certificates's "expiring_soon" threshold. 0 means
	// "use the default", set via WithCertExpiryWarningWindow.
	certExpiryWarningWindow   time.Duration
	builder                   Builder                     // nil is valid: POST /apps/{name}/builds returns 501, same shape as secrets/telemetry/alertRules above
	fetch                     fetchFunc                   // git source fetcher for handleTriggerBuild; always non-nil, defaulted to gitCheckout in NewRouter, overridable in this package's own tests
	listBranches              listBranchesFunc            // remote branch lister for handleListGitBranches; always non-nil, defaulted to listRemoteBranches in NewRouter, overridable in this package's own tests
	staticSites               StaticSiteStore             // always set, same "core Store interface, not an optional plug-in" shape as certs above
	backupTargets             BackupTargetStore           // always set, same "core Store interface" shape as certs/staticSites above: listing/getting/deleting a backup target needs no secrets configuration, only creating one does
	backupSecrets             BackupSecretsSetter         // nil is valid: POST /api/v1/backup-targets returns 501, same shape as secrets above
	backupHistory             BackupHistoryStore          // always set, same "core Store interface" shape as backupTargets above: listing backup history needs no runner configuration, only triggering a new one does
	backupRunner              BackupRunner                // nil is valid: POST /api/v1/databases/{name}/backups returns 501, same shape as backupSecrets above
	backupDownloader          BackupDownloader            // nil is valid: GET .../backups/{historyId}/download returns 501, same shape as backupRunner above
	restoreHistory            RestoreHistoryStore         // always set, same "core Store interface" shape as backupHistory above
	restoreRunner             RestoreRunner               // nil is valid: POST /api/v1/databases/{name}/restore returns 501, same shape as backupRunner above
	deployAttempts            DeployAttemptStore          // always set, same "core Store interface" shape as certs/staticSites above
	deployLogStore            DeployLogStore              // nil is valid: a finished attempt's log route returns 501, same shape as secrets/telemetry/alertRules above
	deployRecorder            *deploylog.Recorder         // nil is valid: an in-progress attempt's live tail returns 501, and handleTriggerBuild falls back to build.SlogProgress with no persisted log, same "not configured" shape as builder/telemetry above
	logBroadcaster            *telemetry.LogBroadcaster   // nil is valid: GET /apps/{name}/logs/stream returns 501, same "not configured" shape as deployRecorder above
	deployNotifyTargets       DeployNotifyTargets         // nil is valid: deploy-notify-target routes return 501, same shape as alertRules above
	deployNotifier            DeployNotifier              // nil is valid: recordPlainDeployAttempt/beginBuildDeployAttempt simply don't dispatch a deploy-outcome notification, same "optional signal, absence is not an error" shape as dockerPinger above
	notificationChannels      NotificationChannels        // nil is valid: notification-channel routes return 501, same shape as deployNotifyTargets above
	notificationChannelTester NotificationChannelTester   // nil is valid: the test-send routes return 501, same shape as deployNotifier above
	gitSources                GitSourceStore              // always set, same "core Store interface" shape as backupTargets above: listing/getting/deleting a git source needs no secrets configuration, only connecting one does
	gitSourceSecrets          GitSourceSecrets            // nil is valid: PUT /apps/{name}/git-source and the git-push webhook route both return 501, same shape as backupSecrets above
	gitSourceFetch            gitSourceFetchFunc          // git-source fetcher for handleGitPushWebhook; always non-nil, defaulted to gitCheckoutWithToken in NewRouter, overridable in this package's own tests, the same "seam, not an interface" shape fetch/listBranches above already use
	githubApp                 GitHubAppStore              // always set, same "core Store interface" shape as backupTargets/certs above: the connection row/its absence is always queryable, no secrets configuration needed just to read status
	githubAppSecrets          GitHubAppSecrets            // nil is valid: every github-app route that needs it (register/start, callback, installed, repos, branches) returns 501, same shape as backupSecrets above
	githubAppClient           GitHubAppClient             // always set (NewRouter defaults it to a real *githubapp.Client, which needs no configuration to construct), overridable in this package's own tests the same way fetch is
	githubAppState            *githubAppRegistrationState // always set (NewRouter constructs one unconditionally); purely in-memory bookkeeping, see its own doc comment
	oauthSettings             OAuthSettingsStore          // always set, same "core Store interface" shape as ingressSettings above: both provider rows always exist (migrations/0035's own seeded rows)
	oauthIdentities           OAuthIdentityStore          // always set, same shape as oauthSettings above
	oauthSecrets              OAuthSecrets                // nil is valid: every /auth/oauth/... sign-in route and PUT /settings/oauth/{provider} return 501/501, same "not configured" shape as gitSourceSecrets above
	oauthState                *oauthStateStore            // built in NewRouter unconditionally, the same "always present, not an Option" shape sessions itself has
	oauthClientFactory        oauthClientFactory          // defaulted to defaultOAuthClientFactory in NewRouter, overridable in this package's own tests, the same "seam, not an interface" shape fetch/listBranches/gitSourceFetch above already use
	emailSettings             EmailSettingsStore          // always set, same shape as ingressSettings above
	emailSecrets              EmailSecretsStore           // nil is valid: PUT /api/v1/settings/email returns 501
	emailSender               email.Sender                // nil is valid: forgot-password still returns its generic success response
	passwordResetTokens       PasswordResetTokenStore     // always set, same shape as backupTargets above
	forgotPasswordByIP        *loginLimiter               // per-IP forgot-password budget, distinct from logins above
	forgotPasswordByEmail     *loginLimiter               // per-(IP,email) forgot-password budget; both this and forgotPasswordByIP must allow a request
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

// WithBackupSecrets enables POST /api/v1/backup-targets. Without one
// configured (the default), that route returns 501, the same
// "not configured" shape WithSecretSetter's absence produces; listing,
// getting, and deleting an already-connected backup target work
// regardless, since none of those need to write a credential.
func WithBackupSecrets(s BackupSecretsSetter) Option {
	return func(rt *Router) { rt.backupSecrets = s }
}

// WithEmailSecrets enables PUT /api/v1/settings/email. Without one
// configured (the default), that route returns 501; GET works regardless.
func WithEmailSecrets(s EmailSecretsStore) Option {
	return func(rt *Router) { rt.emailSecrets = s }
}

// WithEmailSender enables the actual send behind
// POST /api/v1/auth/forgot-password. Without one, that route still
// returns its generic success response, it just never sends anything.
func WithEmailSender(s email.Sender) Option {
	return func(rt *Router) { rt.emailSender = s }
}

// WithGitSourceSecrets enables PUT /api/v1/apps/{name}/git-source and
// the git-push webhook route. Without one configured (the default),
// both return 501; listing/getting/deleting a git source works
// regardless, since neither needs to resolve a credential.
func WithGitSourceSecrets(s GitSourceSecrets) Option {
	return func(rt *Router) { rt.gitSourceSecrets = s }
}

// WithGitHubAppSecrets enables the GitHub App routes that read or write
// a credential (manifest callback, installation callback, repo/branch
// listing). Without one configured, those return 501; GET/DELETE
// /api/v1/github-app work regardless.
func WithGitHubAppSecrets(s GitHubAppSecrets) Option {
	return func(rt *Router) { rt.githubAppSecrets = s }
}

// WithBackupRunner enables POST /api/v1/databases/{name}/backups.
// Without one configured (the default), that route returns 501, the
// same "not configured" shape WithBackupSecrets' absence produces;
// listing backup history (GET .../backups) works regardless, since it
// only reads store.BackupHistory rows a runner already wrote in the
// past, nothing about it needs a live runner today.
func WithBackupRunner(r BackupRunner) Option {
	return func(rt *Router) { rt.backupRunner = r }
}

// WithBackupDownloader enables
// GET /api/v1/databases/{name}/backups/{historyId}/download. Without one
// configured (the default), that route returns 501, the same
// "not configured" shape WithBackupRunner's absence produces: both need a
// live secretsManager to resolve a target's credentials, WithBackupRunner
// to upload with them, this to download with them.
func WithBackupDownloader(d BackupDownloader) Option {
	return func(rt *Router) { rt.backupDownloader = d }
}

// WithRestoreRunner enables POST /api/v1/databases/{name}/restore.
// Without one configured (the default), that route returns 501, the same
// "not configured" shape WithBackupRunner's absence produces; listing
// restore history (GET .../restores) works regardless, the same
// "listing needs no live runner" reasoning WithBackupRunner's own doc
// comment gives for backup history.
func WithRestoreRunner(r RestoreRunner) Option {
	return func(rt *Router) { rt.restoreRunner = r }
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

// DeployNotifier is the narrow surface recordPlainDeployAttempt
// (deploys.go) and beginBuildDeployAttempt (deploy_attempts.go) need to
// fire a deploy-outcome notification once a deploy attempt reaches a
// terminal state. *alerting.DeployDispatcher satisfies this
// structurally. resourceID is resourceIDForApp(ev.AppName), computed by
// the caller (see alerting.DeployDispatcher.Dispatch's own doc comment
// for why it takes resourceID as a parameter rather than importing
// resourceIDForApp itself).
type DeployNotifier interface {
	Dispatch(ctx context.Context, resourceID string, ev alerting.DeployOutcome)
}

// WithDeployNotifyTargets enables POST/GET
// /api/v1/apps/{name}/deploy-notify-targets and DELETE
// .../deploy-notify-targets/{id}: CRUD for deploy-outcome notification
// destinations, the sibling surface to WithAlertRules for TASKS.md
// wave-2 deploy-outcome notifications. Without one configured (the
// default), all three routes return 501, the same "not configured"
// shape WithAlertRules' own absence produces.
func WithDeployNotifyTargets(t DeployNotifyTargets) Option {
	return func(rt *Router) { rt.deployNotifyTargets = t }
}

// WithDeployNotifier enables the actual send: without one configured
// (the default), a deploy attempt still finishes and is still recorded
// exactly as it is today, it just never dispatches a notification, even
// if WithDeployNotifyTargets has targets configured. Kept as a separate
// option from WithDeployNotifyTargets on purpose, the same "listing
// doesn't need a live runner" split WithBackupRunner/WithRestoreRunner
// already establish: a control plane can serve deploy-notify-target CRUD
// without necessarily having a dispatcher wired (e.g. in a test), and
// vice versa cmd/levelrail/main.go always wires both together from the
// same *alerting.DeployDispatcher, which itself embeds the
// *alerting.DB that WithDeployNotifyTargets is given.
func WithDeployNotifier(n DeployNotifier) Option {
	return func(rt *Router) { rt.deployNotifier = n }
}

// WithNotificationChannels enables the notification-channel CRUD routes.
// Without one configured (the default), they return 501.
func WithNotificationChannels(c NotificationChannels) Option {
	return func(rt *Router) { rt.notificationChannels = c }
}

// WithNotificationChannelTester enables the test-send routes: a real
// send, not just a format check. Without one, they return 501.
func WithNotificationChannelTester(t NotificationChannelTester) Option {
	return func(rt *Router) { rt.notificationChannelTester = t }
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

// WithDockerDiskUsager enables the docker_disk_usage field on
// GET /api/v1/system/status. Without one configured (the default), that
// field is simply omitted, the same "optional signal, absence is not an
// error" shape WithDockerPinger's own absence already has.
func WithDockerDiskUsager(u DockerDiskUsager) Option {
	return func(rt *Router) { rt.dockerDiskUsage = u }
}

// WithDockerPruner enables POST /api/v1/system/prune. Without one
// configured (the default), that route returns 501, the same
// "not configured" shape WithBuilder's own absence produces.
func WithDockerPruner(p DockerPruner) Option {
	return func(rt *Router) { rt.dockerPruner = p }
}

// WithExecRuntime enables POST /apps/{name}/exec (exec.go's
// handleExecApp). Without one configured (the default), that route
// returns 501, the same "not configured" shape WithDockerPruner's
// absence produces: a control plane can run every other route with no
// resolver wired in, exec is the one action that needs live node
// routing (NodeRuntimeResolver's own doc comment).
func WithExecRuntime(r NodeRuntimeResolver) Option {
	return func(rt *Router) { rt.execRuntime = r }
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

// WithPublicHost sets the IP or hostname GET
// /api/v1/apps/{name}/domains/{domain}/check tells an operator to point
// their DNS record at (domain_check.go's advertisedHost). Without one
// configured (the default), that handler falls back to a best-effort
// guess from the request's own Host header instead of failing: this
// package never reads the environment directly, cmd/levelrail/main.go
// reads APP_PUBLIC_HOST and passes it here, the same shape
// WithCertExpiryWarningWindow already follows for its own env var.
func WithPublicHost(host string) Option {
	return func(rt *Router) { rt.publicHost = host }
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

// WithDeployLogStore enables a finished deploy attempt's full log replay
// on GET /api/v1/apps/{name}/deploys/{deployId}/logs. Without one
// configured (the default), that route returns 501 for an attempt that
// has already finished; an in-progress attempt's live tail is unaffected
// (see WithDeployRecorder). cmd/levelrail/main.go passes the same
// *telemetry.DB internal/deploylog.Recorder writes to.
func WithDeployLogStore(s DeployLogStore) Option {
	return func(rt *Router) { rt.deployLogStore = s }
}

// WithDeployRecorder enables live build-log fan-out for the manual build
// trigger (handleTriggerBuild) and the in-progress side of
// GET /api/v1/apps/{name}/deploys/{deployId}/logs. Without one
// configured (the default), handleTriggerBuild falls back to
// build.SlogProgress (no persisted log, matching this package's
// pre-existing behavior) and the SSE log route returns 501 for any
// attempt still in progress. r must be the exact same *deploylog.Recorder
// instance passed to internal/webhook.New when a webhook handler is also
// wired: see that package's own doc comment for why a shared instance,
// not two independent ones, is required for a webhook-triggered
// attempt's live log to be visible through this router's SSE route.
func WithDeployRecorder(r *deploylog.Recorder) Option {
	return func(rt *Router) { rt.deployRecorder = r }
}

// WithLogBroadcaster enables GET /api/v1/apps/{name}/logs/stream, the
// live-tailing counterpart to WithTelemetryQuerier's historical
// GET .../logs. Without one configured (the default), that route
// returns 501, the same "not configured" shape WithDeployRecorder's
// absence produces for the in-progress side of a deploy attempt's log.
// Deliberately a separate option from WithTelemetryQuerier even though
// both are telemetry-flavored and cmd/levelrail/main.go always
// configures them together in practice: a control plane could in
// principle want historical log search without live tailing (or the
// reverse), and keeping them as two options leaves that possible instead
// of one silently implying the other. b must be the exact same
// *telemetry.LogBroadcaster instance passed to
// telemetry.NewLogCollector: see that constructor's own doc comment for
// why a shared instance, not two independent ones, is required for a
// line StreamOne receives from Docker to ever reach a viewer connected
// through this router's SSE route.
func WithLogBroadcaster(b *telemetry.LogBroadcaster) Option {
	return func(rt *Router) { rt.logBroadcaster = b }
}

// NewRouter builds a Router. logger defaults to slog.Default() if nil.
func NewRouter(logger *slog.Logger, b *brand.Brand, s Store, opts ...Option) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	rt := &Router{
		logger:                logger,
		brand:                 b,
		apps:                  s,
		appGroups:             s,
		deploys:               s,
		deployAttempts:        s,
		databases:             s,
		auth:                  s,
		tokens:                s,
		nodes:                 s,
		projects:              s,
		certs:                 s,
		staticSites:           s,
		ingressSettings:       s,
		domains:               s,
		lookupHost:            defaultLookupHost,
		domainChecks:          newDomainCheckCache(),
		backupTargets:         s,
		backupHistory:         s,
		restoreHistory:        s,
		gitSources:            s,
		githubApp:             s,
		githubAppClient:       githubapp.NewClient(),
		githubAppState:        newGitHubAppRegistrationState(),
		fetch:                 gitCheckout,
		listBranches:          listRemoteBranches,
		gitSourceFetch:        gitCheckoutWithToken,
		logins:                newLoginLimiter(),
		oauthSettings:         s,
		oauthIdentities:       s,
		oauthState:            newOAuthStateStore(),
		oauthClientFactory:    defaultOAuthClientFactory,
		emailSettings:         s,
		passwordResetTokens:   s,
		forgotPasswordByIP:    newLoginLimiter(),
		forgotPasswordByEmail: newLoginLimiter(),
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
	// POST /system/prune deletes real Docker resources (stopped
	// containers, dangling images, anonymous volumes, unused build
	// cache) fleet-wide, not scoped to one app: AbilityRoot, the same
	// gate handleDrainNode uses for its own fleet-wide, no-undo action,
	// not AbilityWrite (which a narrower, single-app token could hold).
	mux.HandleFunc("POST /api/v1/system/prune", rt.requireAbility(AbilityRoot, rt.handleSystemPrune))

	// Auth. Login and first-run registration are necessarily public;
	// everything else requires an existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", rt.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireAuth(rt.handleLogout))
	mux.HandleFunc("PUT /api/v1/auth/password", rt.requireAuth(rt.handleChangePassword))
	mux.HandleFunc("GET /api/v1/auth/session", rt.requireAuth(rt.handleGetSession))
	mux.HandleFunc("POST /api/v1/auth/sessions/revoke-others", rt.requireAuth(rt.handleRevokeOtherSessions))

	// Multi-user: an already-authenticated user creating another
	// local-password user (see handleRegister's own doc comment), and a
	// read-only "who has access" listing.
	mux.HandleFunc("POST /api/v1/auth/users", rt.requireAuth(rt.handleCreateUser))
	mux.HandleFunc("GET /api/v1/users", rt.requireAbility(AbilityRead, rt.handleListUsers))
	mux.HandleFunc("DELETE /api/v1/users/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteUser))

	// OAuth sign-in (Google, GitHub). /providers, /start, /callback are
	// all necessarily public; /link/start is requireAuth-gated (see its
	// own doc comment).
	mux.HandleFunc("GET /api/v1/auth/oauth/providers", rt.handleListPublicOAuthProviders)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", rt.handleOAuthStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", rt.handleOAuthCallback)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/link/start", rt.requireAuth(rt.handleOAuthLinkStart))

	// OAuth settings: GET is AbilityRead, PUT is AbilityRoot, matching
	// /api/v1/settings/ingress's own tiers.
	mux.HandleFunc("GET /api/v1/settings/oauth", rt.requireAbility(AbilityRead, rt.handleListOAuthSettings))
	mux.HandleFunc("PUT /api/v1/settings/oauth/{provider}", rt.requireAbility(AbilityRoot, rt.handleUpdateOAuthProviderSettings))

	// Forgot/reset password: necessarily unauthenticated, gated by
	// possession of the emailed token instead of a session or ability.
	mux.HandleFunc("POST /api/v1/auth/forgot-password", rt.handleForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", rt.handleResetPassword)

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

	// Stage 1 of multi-service apps (migrations/0039_apps.sql,
	// apps_group.go): a service plus its siblings under the same
	// store.App, with a worst-condition-wins rollup status. Additive,
	// read-only; GET /api/v1/apps/{name} above is unchanged.
	mux.HandleFunc("GET /api/v1/apps/{name}/group", rt.requireAbility(AbilityRead, rt.handleGetAppGroup))

	// Clone: duplicates an app's desired state under a new name.
	// AbilityWrite, the same gate POST /api/v1/apps itself uses, since a
	// clone is a creation shaped as "copy {name}" rather than "start
	// from scratch": handleCloneApp's own doc comment covers what does
	// and doesn't carry over.
	mux.HandleFunc("POST /api/v1/apps/{name}/clone", rt.requireAbility(AbilityWrite, rt.handleCloneApp))

	// Placement (TASKS.md 3.3): AbilityRoot, not AbilityWrite, matching
	// the sensitivity of the standalone node routes above: moving a
	// service between physical machines is infrastructure placement,
	// not ordinary app config, even though it's reached through this
	// app-scoped URL.
	mux.HandleFunc("PUT /api/v1/apps/{name}/node", rt.requireAbility(AbilityRoot, rt.handleSetAppNode))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityDeploy, rt.handleTriggerDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityRead, rt.handleDeployHistory))

	// Restart (handleRestartApp's own doc comment): AbilityDeploy, the
	// same boundary as the deploy trigger above, since forcing a
	// container recreation is the same class of action as triggering a
	// deploy, just without a new image.
	mux.HandleFunc("POST /api/v1/apps/{name}/restart", rt.requireAbility(AbilityDeploy, rt.handleRestartApp))

	// Stop/start (handleStopApp/handleStartApp's own doc comments): same
	// AbilityDeploy tier as restart above, the same class of lifecycle
	// action.
	mux.HandleFunc("POST /api/v1/apps/{name}/stop", rt.requireAbility(AbilityDeploy, rt.handleStopApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/start", rt.requireAbility(AbilityDeploy, rt.handleStartApp))

	// One-off exec (handleExecApp's own doc comment): AbilityRoot, not
	// AbilityDeploy. Secrets are injected as plaintext env vars into a
	// container at create time (CLAUDE.md 4.10) and this package
	// deliberately never decrypts one back into a response body anywhere
	// else, see the secrets route above: "never decrypts a value for a
	// response body." Exec is the one route that can read them anyway,
	// by running `env` inside the container, so it must sit behind the
	// same tier that boundary already implies it needs, not the deploy
	// tier. AbilityRoot is this project's existing "breaks an assumption
	// other tiers rely on" boundary (see restore's own reasoning above).
	mux.HandleFunc("POST /api/v1/apps/{name}/exec", rt.requireAbility(AbilityRoot, rt.handleExecApp))

	// Real deploy-attempt history (deploy_attempts.go): a row per
	// trigger call across all three real trigger paths, additional to
	// (not a replacement for) the reconcile-conditions route above. See
	// DeployAttemptStore's own doc comment for why this is a separate
	// endpoint rather than a change to GET .../deploys's existing
	// response shape. AbilityRead, matching every other passive view of
	// an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-attempts", rt.requireAbility(AbilityRead, rt.handleListDeployAttempts))

	// Deploy-attempt build/log stream (deploy_attempts.go): SSE, serving
	// either a live tail (attempt still running) or a full persisted
	// replay (attempt already finished), the exact contract
	// web/src/hooks/useDeployLogStream.ts was built against. AbilityRead:
	// this is a read of one attempt's own output, the same sensitivity
	// as the deploy-attempts list above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/logs", rt.requireAbility(AbilityRead, rt.handleDeployLogStream))

	// Manual build trigger (see Builder/WithBuilder above and
	// handleTriggerBuild's own doc comment): builds an image from a git
	// source through the same internal/deploy.Pipeline the webhook
	// receiver uses, for an operator with no working git webhook
	// configured. AbilityDeploy, the same boundary as the image-tag
	// trigger above: this also ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/builds", rt.requireAbility(AbilityDeploy, rt.handleTriggerBuild))

	// Branch listing for an arbitrary public git remote (handleListGitBranches's
	// own doc comment): not scoped to an existing app, since the create-app-
	// from-git wizard needs this before an app exists to attach a build
	// to. AbilityDeploy, the same tier the build trigger above requires:
	// listing what a repo could be built from is a strict subset of
	// actually triggering that build.
	mux.HandleFunc("POST /api/v1/git/branches", rt.requireAbility(AbilityDeploy, rt.handleListGitBranches))

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

	// List known secret keys (never values) and toggle a key's lock.
	// GET at AbilityRead, matching GET .../git-source's own
	// GET=Read/PUT=WriteSensitive split just below: a key NAME is no
	// more sensitive than a git-source's connection config.
	mux.HandleFunc("GET /api/v1/apps/{name}/secrets", rt.requireAbility(AbilityRead, rt.handleListSecrets))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/lock", rt.requireAbility(AbilityWriteSensitive, rt.handleSetSecretLock))

	// Git source (TASKS.md 1.7's own deferred follow-up, git_sources.go):
	// persist a repo/branch/build config per app so a git push can
	// auto-deploy it, the multi-app evolution of internal/webhook's own
	// single-app, env-var-configured Config. AbilityWriteSensitive for
	// PUT/DELETE, matching PUT .../secrets/{key} above: connecting a repo
	// accepts an optional live deploy token in the same request body.
	mux.HandleFunc("GET /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityRead, rt.handleGetGitSource))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityWriteSensitive, rt.handleSetGitSource))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteGitSource))

	// Git push webhook (git_webhook.go), the per-app-URL evolution of the
	// original static POST /webhook (still mounted separately by
	// cmd/levelrail/main.go for the single-app, env-var-configured path).
	// Deliberately unauthenticated, like that route: GitHub cannot
	// present a session or API token, so this is not wrapped in
	// requireAbility. Its own per-app HMAC signature check (the secret
	// generated at connect time, PUT .../git-source above) is what stands
	// in for auth here, the same trust boundary internal/webhook.Handler's
	// own doc comment establishes for the single-app path.
	mux.HandleFunc("POST /api/v1/webhooks/github/{name}", rt.handleGitPushWebhook)

	// Telemetry query (TASKS.md 2.3): metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryMetrics))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryLogs))
	// Live log tail (additive to the historical search route just above,
	// see handleLiveLogStream's own doc comment): AbilityRead, the same
	// passive-visibility boundary as every other view of telemetry data
	// in this router, including the deploy-log stream at
	// GET .../deploys/{deployId}/logs above.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/stream", rt.requireAbility(AbilityRead, rt.handleLiveLogStream))

	// Alerting (TASKS.md 2.5/2.7): threshold and crashloop rules scoped
	// to one app, fanned through a *alerting.DB when configured (see
	// WithAlertRules).
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityWrite, rt.handleCreateAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityRead, rt.handleListAlertRules))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/alerts/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteAlertRule))

	// Deploy-outcome notifications (wave-2 roadmap item #5): a Slack/
	// Discord/Telegram/generic-webhook/email ping fired once per deploy
	// attempt reaching a terminal state, distinct from the threshold/
	// crashloop alert rules just above (see
	// internal/alerting/deploy_notify.go's own doc comment). Also fanned
	// through *alerting.DB when configured (see WithDeployNotifyTargets).
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbility(AbilityWrite, rt.handleCreateDeployNotifyTarget))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbility(AbilityRead, rt.handleListDeployNotifyTargets))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/deploy-notify-targets/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteDeployNotifyTarget))

	// Notification channels: the global, connect-once destination the
	// deploy-notify-target routes above attach to by channel_id.
	// AbilityWrite, same tier as deploy-notify-targets' own POST.
	mux.HandleFunc("GET /api/v1/notification-channels", rt.requireAbility(AbilityRead, rt.handleListNotificationChannels))
	mux.HandleFunc("POST /api/v1/notification-channels", rt.requireAbility(AbilityWrite, rt.handleCreateNotificationChannel))
	mux.HandleFunc("DELETE /api/v1/notification-channels/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/test", rt.requireAbility(AbilityWrite, rt.handleTestNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/{id}/test", rt.requireAbility(AbilityWrite, rt.handleTestExistingNotificationChannel))

	// Prometheus remote read (TASKS.md 2.6). Gated by requireAbility the
	// same as every other read route, not left open: leaving a metrics
	// endpoint unauthenticated would let any caller pull every service's
	// resource usage. Prometheus's own remote_read config supports
	// bearer-token auth (authorization.credentials), which is exactly an
	// API token scoped to at least "read" (see the token management
	// routes above); requireAbility already accepts one the same way it
	// does for every JSON route, this isn't a special case.
	mux.HandleFunc("POST /api/v1/prometheus/read", rt.requireAbility(AbilityRead, rt.handlePrometheusRead))

	// Projects (projects.go): a lightweight, non-auth organizational
	// grouping, explicitly not the deferred Phase 4 teams/RBAC work (see
	// that file's own package doc comment). AbilityRead/AbilityWrite,
	// the same ordinary boundary apps/databases CRUD already uses, not
	// AbilityRoot: unlike a node (real infrastructure, TASKS.md 3.1),
	// creating or deleting a project has no fleet-level consequence.
	mux.HandleFunc("GET /api/v1/projects", rt.requireAbility(AbilityRead, rt.handleListProjects))
	mux.HandleFunc("POST /api/v1/projects", rt.requireAbility(AbilityWrite, rt.handleCreateProject))
	mux.HandleFunc("GET /api/v1/projects/{id}", rt.requireAbility(AbilityRead, rt.handleGetProject))
	mux.HandleFunc("DELETE /api/v1/projects/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteProject))

	// Move an app/database into (or out of, with project_id: "") a
	// project: the project-kind counterpart to PUT /apps/{name}/node
	// and PUT /databases/{name}/node above, same narrow-dedicated-
	// mutation shape those routes establish (appResource/
	// databaseResource's own ProjectID field is response-only, exactly
	// like NodeID, see handleSetAppProject's own doc comment for why).
	// AbilityWrite, not AbilityRoot: project membership is an ordinary
	// organizational edit, not infrastructure placement, so it sits at
	// the same sensitivity as the rest of apps/databases CRUD rather
	// than the node routes' fleet-level boundary.
	mux.HandleFunc("PUT /api/v1/apps/{name}/project", rt.requireAbility(AbilityWrite, rt.handleSetAppProject))
	mux.HandleFunc("PUT /api/v1/databases/{name}/project", rt.requireAbility(AbilityWrite, rt.handleSetDatabaseProject))

	// Set (or clear, with a nil body field) a database's resource limits:
	// databaseResource's own Resources field doc comment explains why
	// this is a dedicated route rather than folded into handleUpdateApp's
	// general-PUT equivalent, which has no database counterpart.
	// AbilityWrite, the same ordinary-config-edit sensitivity as the
	// project routes just above, not AbilityRoot: a resource cap is not
	// fleet-level placement.
	mux.HandleFunc("PUT /api/v1/databases/{name}/resources", rt.requireAbility(AbilityWrite, rt.handleSetDatabaseResources))

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
	// Node-level metrics (sum of per-container samples for everything
	// placed on this node, see handleQueryNodeMetrics's own doc comment
	// for exactly what that does and doesn't mean): same AbilityRoot
	// boundary and nil-telemetry 501 shape as every route above, and the
	// same query-param contract as GET /apps/{name}/metrics above it.
	mux.HandleFunc("GET /api/v1/nodes/{id}/metrics", rt.requireAbility(AbilityRoot, rt.handleQueryNodeMetrics))

	// Certificates (TLS renewal visibility): this project treats
	// "a cert renewal fails silently at 3am" as its central
	// risk to catch before it bites a real user, and until now nothing
	// in the API surfaced certificate state at all. Read-only, so
	// AbilityRead like handleSystemStatus/handleGetNodeHealth, not
	// AbilityRoot: seeing whether a domain's certificate is about to
	// expire is ordinary operator visibility, not a fleet-admin
	// mutation the way the node routes above are.
	mux.HandleFunc("GET /api/v1/certificates", rt.requireAbility(AbilityRead, rt.handleListCertificates))

	// Ingress settings (ACME toggle, platform primary domain, ADR 005's
	// own "Verified" section names this exact gap: real ACME issuance
	// was explicitly unproven, spot-checked against a real domain, not
	// assumed to follow automatically). GET is AbilityRead, the same
	// passive-visibility tier as GET /api/v1/certificates above: reading
	// today's toggle state is ordinary operator visibility. PUT is
	// AbilityRoot, matching handleSetAppNode/handleDrainNode/POST
	// /system/prune's own precedent for "real infrastructure, high
	// blast radius, not an ordinary per-app write": flipping
	// acme_enabled changes what every currently-routed host's
	// certificate automation does, fleet-wide, on the very next ingress
	// reconcile pass, the same class of change node placement and
	// draining already reserve AbilityRoot for.
	mux.HandleFunc("GET /api/v1/settings/ingress", rt.requireAbility(AbilityRead, rt.handleGetIngressSettings))
	mux.HandleFunc("PUT /api/v1/settings/ingress", rt.requireAbility(AbilityRoot, rt.handleUpdateIngressSettings))

	// Domain DNS check (domain_check.go): the guidance layer on top of
	// domain connection, so DomainEditor can show an operator the exact
	// DNS record to add and watch it flip to "connected" once it actually
	// resolves. AbilityRead, same passive-visibility tier as GET
	// /apps/{name}/git-source: a live DNS lookup, no write.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/check", rt.requireAbility(AbilityRead, rt.handleCheckDomain))

	// Email settings: same precedent as ingress settings just above.
	// GET is AbilityRead; PUT is AbilityRoot, real infrastructure config.
	mux.HandleFunc("GET /api/v1/settings/email", rt.requireAbility(AbilityRead, rt.handleGetEmailSettings))
	mux.HandleFunc("PUT /api/v1/settings/email", rt.requireAbility(AbilityRoot, rt.handleUpdateEmailSettings))

	// Domains (centralized cross-app list, web/src/routes/domains):
	// every service_domains row, AbilityRead like GET /api/v1/apps,
	// no new ability tier: this is the same data DomainEditor already
	// exposes per-app, aggregated across every app in one read-only call.
	mux.HandleFunc("GET /api/v1/domains", rt.requireAbility(AbilityRead, rt.handleListDomains))

	// Static sites (build.type: static): read-only
	// dashboard visibility for sites served directly by embedded Caddy
	// with no container, closing the gap flagged when static-site
	// support (migration 0015) first landed. AbilityRead, the same
	// passive-visibility boundary as handleListCertificates above: no
	// create/update/delete route exists yet because the backend has no
	// mutation path for a static site beyond internal/deploy.Pipeline's
	// own git-push-triggered write.
	mux.HandleFunc("GET /api/v1/static-sites", rt.requireAbility(AbilityRead, rt.handleListStaticSites))

	// Backup targets (connected S3-compatible buckets a database backup
	// can be uploaded to). Create and delete need AbilityWriteSensitive:
	// create because its request body carries live bucket credentials,
	// delete because it gates access to the same. List/get are ordinary
	// AbilityRead, the same boundary handleListCertificates/
	// handleListStaticSites already draw between visibility and mutation.
	mux.HandleFunc("GET /api/v1/backup-targets", rt.requireAbility(AbilityRead, rt.handleListBackupTargets))
	mux.HandleFunc("POST /api/v1/backup-targets", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateBackupTarget))
	mux.HandleFunc("GET /api/v1/backup-targets/{id}", rt.requireAbility(AbilityRead, rt.handleGetBackupTarget))
	mux.HandleFunc("DELETE /api/v1/backup-targets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteBackupTarget))

	// GitHub App connection: the manifest-based registration flow,
	// installation, and repo/branch browsing through it
	// (internal/api/github_app.go, github_app_register.go,
	// github_app_repos.go). Every route that reads or mutates the
	// connection itself (status, register/start, callback, installed,
	// disconnect) is AbilityRoot, matching PUT /api/v1/settings/ingress's
	// own precedent above rather than the plain AbilityRead most other
	// GET routes use: this is platform-wide configuration with a real
	// external-account relationship behind it (an installed GitHub App
	// can read a private repository's contents), the same "real
	// infrastructure, high blast radius" class ingress settings and node
	// placement already reserve this tier for, not an ordinary per-app
	// read. register/start, callback, and installed are all real,
	// full-page browser navigations (GitHub's manifest flow is
	// inherently that, not a fetch call), not XHR/fetch calls: a session
	// cookie is implicitly AbilityRoot (requireAbility's own doc
	// comment), so the operator's own logged-in browser satisfies this
	// gate the same way it satisfies every other AbilityRoot route.
	//
	// Repo and branch listing are the one exception, at
	// AbilityReadSensitive: see handleListGitHubAppRepos's own doc
	// comment for why reading already-connected repo/branch names is a
	// materially different (lower) risk class than changing the
	// connection itself.
	mux.HandleFunc("GET /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleGetGitHubAppStatus))
	mux.HandleFunc("DELETE /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitHubApp))
	mux.HandleFunc("GET /api/v1/github-app/register/start", rt.requireAbility(AbilityRoot, rt.handleStartGitHubAppRegistration))
	mux.HandleFunc("GET /api/v1/github-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitHubAppCallback))
	mux.HandleFunc("GET /api/v1/github-app/installed", rt.requireAbility(AbilityRoot, rt.handleGitHubAppInstalled))
	mux.HandleFunc("GET /api/v1/github-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppRepos))
	mux.HandleFunc("GET /api/v1/github-app/repos/{owner}/{repo}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppBranches))

	// Backup history and manual trigger, per database. Trigger needs
	// AbilityWriteSensitive: it starts real work against a live bucket
	// using a previously-stored credential, the same sensitivity class
	// creating or deleting the backup target itself already carries.
	// History listing is ordinary AbilityRead.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups", rt.requireAbility(AbilityWriteSensitive, rt.handleTriggerBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups", rt.requireAbility(AbilityRead, rt.handleListBackupHistory))

	// Download one succeeded backup's own object, streamed straight to
	// the browser. AbilityReadSensitive, not AbilityRead: this returns
	// the actual dump bytes, a full database's worth of content, not
	// metadata about an attempt the way history listing above does; it's
	// also not a mutation, so AbilityWriteSensitive (the tier the trigger
	// route above uses) would overstate it. Not AbilityRoot either,
	// unlike restore.go's own explicit "single most destructive
	// endpoint" reasoning for that tier: restore is irreversible,
	// in-place destruction of live data, a materially worse risk class
	// than a read-only fetch from a bucket, even though this route is a
	// genuine full-database exfiltration primitive on the confidentiality
	// axis. Today every session is implicitly root (requireAbility only
	// gates scoped bearer/MCP tokens), so this choice mainly matters once
	// Phase 4 mints scoped automation tokens against this ability tier.
	// See handleDownloadBackup's own doc comment for the full reasoning.
	mux.HandleFunc("GET /api/v1/databases/{name}/backups/{historyId}/download", rt.requireAbility(AbilityReadSensitive, rt.handleDownloadBackup))

	// Scheduled backup config, per database (wave-2 roadmap item 6):
	// which backup target, cron schedule, and retention count
	// internal/backup.Scheduler uses for this database, if any.
	// AbilityWriteSensitive for both PUT and DELETE, the same tier the
	// manual trigger route above already uses: this is the config that
	// decides where an unattended, recurring backup ends up, a live
	// bucket with previously-stored credentials, the identical
	// sensitivity class. GET reuses handleGetDatabase's own existing
	// AbilityRead response (databaseResource already carries these three
	// fields, see that handler's own file), so there is no separate GET
	// route here.
	mux.HandleFunc("PUT /api/v1/databases/{name}/backup-schedule", rt.requireAbility(AbilityWriteSensitive, rt.handleSetBackupSchedule))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backup-schedule", rt.requireAbility(AbilityWriteSensitive, rt.handleClearBackupSchedule))

	// Public/host-exposed access, per database
	// (database_public_access.go): whether this database's container
	// port is bound to a host port so an operator's own database GUI
	// tool can connect directly. AbilityWriteSensitive for both PUT and
	// DELETE, the same tier scheduled backups above already use: this
	// changes real network exposure, the identical sensitivity class as
	// where an unattended backup ends up. GET reuses handleGetDatabase's
	// own existing AbilityRead response (databaseResource already
	// carries publicly_accessible/public_port), the same "no separate
	// GET route" reasoning the backup-schedule routes above already
	// apply.
	mux.HandleFunc("PUT /api/v1/databases/{name}/public-access", rt.requireAbility(AbilityWriteSensitive, rt.handleSetDatabasePublicAccess))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/public-access", rt.requireAbility(AbilityWriteSensitive, rt.handleClearDatabasePublicAccess))

	// Restore, per database (restore.go). AbilityRoot, not
	// AbilityWriteSensitive: see handleTriggerRestore's own doc comment
	// for why this, alone among every backup-related route, needs the
	// same top tier as node management and placement. History listing is
	// ordinary AbilityRead, the same boundary the backup history route
	// above already draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore", rt.requireAbility(AbilityRoot, rt.handleTriggerRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/restores", rt.requireAbility(AbilityRead, rt.handleListRestoreHistory))

	// Object-storage attachment, per app (apps_storage.go): which
	// connected backup_targets bucket (the same S3-compatible connection
	// a database's scheduled backups can already point at, reused rather
	// than a second "storage target" concept) this app's own container
	// gets S3_* credentials injected from at container-create time.
	// AbilityWriteSensitive for both PUT and DELETE, the same tier
	// scheduled backups and database public access above already use:
	// this changes which live bucket credentials an app's own container
	// receives, the identical sensitivity class. GET reuses
	// handleGetApp's own existing AbilityRead response (appResource
	// carries storage_target_id), the same "no separate GET route"
	// reasoning the backup-schedule/public-access routes above already
	// apply.
	mux.HandleFunc("PUT /api/v1/apps/{name}/storage", rt.requireAbility(AbilityWriteSensitive, rt.handleSetAppStorage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/storage", rt.requireAbility(AbilityWriteSensitive, rt.handleClearAppStorage))
	// Read-only, not scoped to any one app: the static list of env var
	// names attaching storage can inject, backed by
	// application.StorageEnvKeys rather than a hardcoded list, see
	// handleListStorageEnvKeys' own doc comment.
	mux.HandleFunc("GET /api/v1/storage-env-keys", rt.requireAbility(AbilityRead, rt.handleListStorageEnvKeys))

	return mux
}
