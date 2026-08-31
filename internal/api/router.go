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
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Router wires every internal/api handler onto one http.Handler.
type Router struct {
	logger                 *slog.Logger
	brand                  *brand.Brand
	apps                   AppStore
	appGroups              AppGroupLister
	appCompose             AppComposeStore
	deploys                DeployStore
	databases              DatabaseStore
	auth                   AuthStore
	tokens                 TokenStore
	nodes                  NodeStore
	projects               ProjectStore
	organizations          OrganizationStore
	environments           EnvironmentStore
	secrets                SecretSetter       // nil is valid: a control plane with no master key configured serves everything except secret-setting
	composeSecrets         ComposeSecretStore // nil is valid: a compose file needing a generated secret fails loudly instead, see handleDeployCompose
	telemetry              TelemetryQuerier   // nil is valid: metrics/logs query routes return 501, same shape as secrets above
	alertRules             AlertRules         // nil is valid: alert rule routes return 501, same shape as secrets/telemetry above
	sessions               *sessionStore
	logins                 *loginLimiter
	recoveryCodes          RecoveryCodeStore      // always set, same "core Store interface" shape as auth above
	twoFactorSecrets       TwoFactorSecrets       // nil is valid: POST /api/v1/auth/2fa/setup (and confirm/disable/regenerate) return 501, same "not configured" shape as githubAppSecrets above
	mfaPending             *mfaPendingStore       // always set, same "always present, not an Option" shape sessions itself has
	mfaVerify              *loginLimiter          // separate budget from logins above: brute-forcing a 6-digit code after a correct password is a distinct attack this must independently rate limit
	sessionTTL             time.Duration          // 0 means "use defaultSessionTTL", set via WithSessionTTL
	dataDir                string                 // "" means "don't report disk usage", set via WithDataDir
	dockerPinger           DockerPinger           // nil is valid: a control plane started without one reports DockerConnected: false, same shape as secrets/telemetry/alertRules above
	images                 ImageLister            // nil is valid: GET /apps/{name}/images returns an empty list, same shape as dockerPinger above
	dockerDiskUsage        DockerDiskUsager       // nil is valid: GET /system/status omits its docker_disk_usage field, same "optional signal, absence is not an error" shape as dockerPinger above
	dockerPruner           DockerPruner           // nil is valid: POST /system/prune returns 501, same shape as builder/secrets above
	execRuntime            NodeRuntimeResolver    // nil is valid: POST /apps/{name}/exec returns 501, same shape as dockerPruner above
	certs                  CertStore              // always set, part of the core Store interface: unlike dockerPinger/images this isn't an optional plug-in, every *store.DB already has it
	ingressSettings        IngressSettingsStore   // always set, same "core Store interface, not an optional plug-in" shape as certs above: the settings row always exists (migrations/0023's own seeded row)
	domains                DomainStore            // always set, same shape as ingressSettings above: service_domains is always queryable, empty is a valid, non-error result
	domainBasicAuth        DomainBasicAuthStore   // always set, same "core Store interface" shape as domains above
	domainBasicAuthSecrets DomainBasicAuthSecrets // nil is valid: PUT/DELETE .../domains/{domain}/auth return 501, same shape as cloudflareTunnelSecrets above
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
	// fetchLatestRelease is handleGetUpdates' GitHub Releases lookup;
	// always non-nil, defaulted to defaultFetchLatestRelease in
	// NewRouter, overridable in tests the same way lookupHost is above.
	fetchLatestRelease fetchLatestReleaseFunc
	// updatesCache caches fetchLatestRelease's result; always non-nil,
	// constructed in NewRouter.
	updatesCache *updatesCache
	// certExpiryWarningWindow overrides defaultCertExpiryWarningWindow
	// for GET /api/v1/certificates's "expiring_soon" threshold. 0 means
	// "use the default", set via WithCertExpiryWarningWindow.
	certExpiryWarningWindow   time.Duration
	builder                   Builder                         // nil is valid: POST /apps/{name}/builds returns 501, same shape as secrets/telemetry/alertRules above
	fetch                     fetchFunc                       // git source fetcher for handleTriggerBuild; always non-nil, defaulted to gitCheckout in NewRouter, overridable in this package's own tests
	listBranches              listBranchesFunc                // remote branch lister for handleListGitBranches; always non-nil, defaulted to listRemoteBranches in NewRouter, overridable in this package's own tests
	staticSites               StaticSiteStore                 // always set, same "core Store interface, not an optional plug-in" shape as certs above
	backupTargets             BackupTargetStore               // always set, same "core Store interface" shape as certs/staticSites above: listing/getting/deleting a backup target needs no secrets configuration, only creating one does
	backupSecrets             BackupSecretsSetter             // nil is valid: POST /api/v1/backup-targets returns 501, same shape as secrets above
	registryCredentials       RegistryCredentialStore         // always set, same "core Store interface" shape as backupTargets above
	registryCredentialSecrets RegistryCredentialSecretsSetter // nil is valid: POST /api/v1/registry-credentials returns 501, same shape as backupSecrets above
	backupHistory             BackupHistoryStore              // always set, same "core Store interface" shape as backupTargets above: listing backup history needs no runner configuration, only triggering a new one does
	backupRunner              BackupRunner                    // nil is valid: POST /api/v1/databases/{name}/backups returns 501, same shape as backupSecrets above
	backupDownloader          BackupDownloader                // nil is valid: GET .../backups/{historyId}/download returns 501, same shape as backupRunner above
	restoreHistory            RestoreHistoryStore             // always set, same "core Store interface" shape as backupHistory above
	restoreRunner             RestoreRunner                   // nil is valid: POST /api/v1/databases/{name}/restore returns 501, same shape as backupRunner above
	deployAttempts            DeployAttemptStore              // always set, same "core Store interface" shape as certs/staticSites above
	deployLogStore            DeployLogQuerier                // nil is valid: a finished attempt's log route returns 501, same shape as secrets/telemetry/alertRules above
	deployRecorder            *deploylog.Recorder             // nil is valid: an in-progress attempt's live tail returns 501, and handleTriggerBuild falls back to build.SlogProgress with no persisted log, same "not configured" shape as builder/telemetry above
	logBroadcaster            *telemetry.LogBroadcaster       // nil is valid: GET /apps/{name}/logs/stream returns 501, same "not configured" shape as deployRecorder above
	deployNotifyTargets       DeployNotifyTargets             // nil is valid: deploy-notify-target routes return 501, same shape as alertRules above
	deployNotifier            DeployNotifier                  // nil is valid: recordPlainDeployAttempt/beginBuildDeployAttempt simply don't dispatch a deploy-outcome notification, same "optional signal, absence is not an error" shape as dockerPinger above
	notificationChannels      NotificationChannels            // nil is valid: notification-channel routes return 501, same shape as deployNotifyTargets above
	notificationChannelTester NotificationChannelTester       // nil is valid: the test-send routes return 501, same shape as deployNotifier above
	notificationDeliveries    NotificationDeliveryStore       // nil is valid: the deliveries route returns 501, and test-send simply doesn't record history, same shape as notificationChannelTester above
	gitSources                GitSourceStore                  // always set, same "core Store interface" shape as backupTargets above: listing/getting/deleting a git source needs no secrets configuration, only connecting one does
	previewEnvironments       PreviewEnvironmentStore         // always set, same "core Store interface" shape as gitSources above: listing/tearing down a preview needs no extra secrets configuration, deploying a new one reuses gitSourceSecrets/gitSourceFetch/builder already above
	gitSourceSecrets          GitSourceSecrets                // nil is valid: PUT /apps/{name}/git-source and the git-push webhook route both return 501, same shape as backupSecrets above
	gitSourceFetch            gitSourceFetchFunc              // git-source fetcher for handleGitPushWebhook; always non-nil, defaulted to gitCheckoutWithToken in NewRouter, overridable in this package's own tests, the same "seam, not an interface" shape fetch/listBranches above already use
	githubApp                 GitHubAppStore                  // always set, same "core Store interface" shape as backupTargets/certs above: the connection row/its absence is always queryable, no secrets configuration needed just to read status
	githubAppSecrets          GitHubAppSecrets                // nil is valid: every github-app route that needs it (register/start, callback, installed, repos, branches) returns 501, same shape as backupSecrets above
	githubAppClient           GitHubAppClient                 // always set (NewRouter defaults it to a real *githubapp.Client, which needs no configuration to construct), overridable in this package's own tests the same way fetch is
	githubAppState            *pendingState                   // always set (NewRouter constructs one unconditionally); purely in-memory bookkeeping, see pendingState's own doc comment
	githubAppManifestConfig   githubapp.ManifestConfig        // always set, defaulted to githubapp.DefaultManifestConfig() in NewRouter, overridable via WithGitHubAppManifestConfig: the permissions/events a fresh App registration requests
	gitlabApp                 GitLabAppStore                  // always set, same "core Store interface" shape as githubApp above
	gitlabAppSecrets          GitLabAppSecrets                // nil is valid: every gitlab-app route that needs it returns 501, same shape as githubAppSecrets above
	gitlabAppClient           GitLabAppClient                 // always set (NewRouter defaults it to a real *gitlabapp.Client), overridable in this package's own tests
	gitlabAppState            *pendingState                   // always set (NewRouter constructs one unconditionally); purely in-memory OAuth CSRF state, see pendingState's own doc comment
	oauthSettings             OAuthSettingsStore              // always set, same "core Store interface" shape as ingressSettings above: both provider rows always exist (migrations/0035's own seeded rows)
	oauthIdentities           OAuthIdentityStore              // always set, same shape as oauthSettings above
	oauthSecrets              OAuthSecrets                    // nil is valid: every /auth/oauth/... sign-in route and PUT /settings/oauth/{provider} return 501/501, same "not configured" shape as gitSourceSecrets above
	oauthState                *oauthStateStore                // built in NewRouter unconditionally, the same "always present, not an Option" shape sessions itself has
	oauthClientFactory        oauthClientFactory              // defaulted to defaultOAuthClientFactory in NewRouter, overridable in this package's own tests, the same "seam, not an interface" shape fetch/listBranches/gitSourceFetch above already use
	emailSettings             EmailSettingsStore              // always set, same shape as ingressSettings above
	emailSecrets              EmailSecretsStore               // nil is valid: PUT /api/v1/settings/email returns 501
	cloudflareTunnel          CloudflareTunnelStore           // always set, same shape as emailSettings above
	cloudflareTunnelSecrets   CloudflareTunnelSecrets         // nil is valid: PUT/DELETE /api/v1/settings/cloudflare-tunnel return 501, same shape as emailSecrets above
	cloudflareDNS             CloudflareDNSStore              // always set, same shape as cloudflareTunnel above
	cloudflareDNSSecrets      CloudflareDNSSecrets            // nil is valid: PUT/DELETE /api/v1/settings/cloudflare-dns return 501, same shape as cloudflareTunnelSecrets above
	emailSender               email.Sender                    // nil is valid: forgot-password still returns its generic success response
	passwordResetTokens       PasswordResetTokenStore         // always set, same shape as backupTargets above
	forgotPasswordByIP        *loginLimiter                   // per-IP forgot-password budget, distinct from logins above
	forgotPasswordByEmail     *loginLimiter                   // per-(IP,email) forgot-password budget; both this and forgotPasswordByIP must allow a request
	auditLog                  AuditStore                      // always set, same "core Store interface" shape as backupTargets/certs above: requireAbility's audit hook (auth.go) writes through this on every request, GET /api/v1/audit-log (audit.go) reads through it
	scheduledTasks            ScheduledTaskStore              // always set, same "core Store interface" shape as backupTargets above: CRUD on a scheduled task needs no runner configuration, only actually running one does
	scheduledTaskRunner       ScheduledTaskRunner             // nil is valid: POST .../scheduled-tasks/{id}/run returns 501, same shape as backupRunner above
	featureFlags              FeatureFlagStore                // always set, same "core Store interface" shape as scheduledTasks above
	bitbucketApp              BitbucketAppStore               // always set, same "core Store interface" shape as gitlabApp above
	bitbucketAppSecrets       BitbucketAppSecrets             // nil is valid: every bitbucket-app route that needs it returns 501, same shape as gitlabAppSecrets above
	bitbucketAppClient        BitbucketAppClient              // always set (NewRouter defaults it to a real *bitbucketapp.Client), overridable in this package's own tests
	bitbucketAppState         *pendingState                   // always set (NewRouter constructs one unconditionally); purely in-memory OAuth CSRF state, see pendingState's own doc comment
	onboarding                OnboardingStore                 // always set, same "core Store interface, not an optional plug-in" shape as ingressSettings above: the row always exists (migrations/0067's own seeded row)
	dbPinger                  DBPinger                        // nil is valid: GET /system/doctor reports its database check as unknown, same shape as dockerPinger above
	doctorDiskWarningBytes    int64                           // 0 means "use defaultDoctorDiskWarningBytes", set via WithDoctorDiskWarningBytes
}

// NewRouter builds a Router. logger defaults to slog.Default() if nil.
func NewRouter(logger *slog.Logger, b *brand.Brand, s Store, opts ...Option) *Router {
	if logger == nil {
		logger = slog.Default()
	}
	rt := &Router{
		logger:                  logger,
		brand:                   b,
		apps:                    s,
		appGroups:               s,
		appCompose:              s,
		deploys:                 s,
		deployAttempts:          s,
		databases:               s,
		auth:                    s,
		tokens:                  s,
		nodes:                   s,
		projects:                s,
		organizations:           s,
		environments:            s,
		certs:                   s,
		staticSites:             s,
		ingressSettings:         s,
		domains:                 s,
		domainBasicAuth:         s,
		lookupHost:              defaultLookupHost,
		domainChecks:            newDomainCheckCache(),
		backupTargets:           s,
		registryCredentials:     s,
		backupHistory:           s,
		restoreHistory:          s,
		gitSources:              s,
		previewEnvironments:     s,
		githubApp:               s,
		githubAppClient:         githubapp.NewClient(),
		githubAppState:          newPendingState(),
		githubAppManifestConfig: githubapp.DefaultManifestConfig(),
		gitlabApp:               s,
		gitlabAppClient:         gitlabapp.NewClient(),
		gitlabAppState:          newPendingState(),
		fetch:                   gitCheckout,
		listBranches:            listRemoteBranches,
		gitSourceFetch:          gitCheckoutWithToken,
		logins:                  newLoginLimiter(),
		recoveryCodes:           s,
		mfaPending:              newMFAPendingStore(),
		mfaVerify:               newLoginLimiter(),
		oauthSettings:           s,
		oauthIdentities:         s,
		oauthState:              newOAuthStateStore(),
		oauthClientFactory:      defaultOAuthClientFactory,
		emailSettings:           s,
		cloudflareTunnel:        s,
		cloudflareDNS:           s,
		passwordResetTokens:     s,
		forgotPasswordByIP:      newLoginLimiter(),
		forgotPasswordByEmail:   newLoginLimiter(),
		fetchLatestRelease:      defaultFetchLatestRelease,
		updatesCache:            newUpdatesCache(),
		auditLog:                s,
		scheduledTasks:          s,
		featureFlags:            s,
		bitbucketApp:            s,
		bitbucketAppClient:      bitbucketapp.NewClient(),
		bitbucketAppState:       newPendingState(),
		onboarding:              s,
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
