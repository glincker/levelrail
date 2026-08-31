package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

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

// WithRegistryCredentialSecrets enables POST /api/v1/registry-credentials.
// Without one configured (the default), that route returns 501; GET and
// DELETE work regardless, the same shape WithBackupSecrets establishes.
func WithRegistryCredentialSecrets(s RegistryCredentialSecretsSetter) Option {
	return func(rt *Router) { rt.registryCredentialSecrets = s }
}

// WithEmailSecrets enables PUT /api/v1/settings/email. Without one
// configured (the default), that route returns 501; GET works regardless.
func WithEmailSecrets(s EmailSecretsStore) Option {
	return func(rt *Router) { rt.emailSecrets = s }
}

// WithCloudflareTunnelSecrets enables PUT/DELETE
// /api/v1/settings/cloudflare-tunnel. Without one configured (the
// default), both return 501; GET works regardless, the same shape
// WithEmailSecrets establishes.
func WithCloudflareTunnelSecrets(s CloudflareTunnelSecrets) Option {
	return func(rt *Router) { rt.cloudflareTunnelSecrets = s }
}

// WithCloudflareDNSSecrets enables PUT/DELETE
// /api/v1/settings/cloudflare-dns. Without one configured (the default),
// both return 501; GET works regardless, the same shape
// WithCloudflareTunnelSecrets establishes.
func WithCloudflareDNSSecrets(s CloudflareDNSSecrets) Option {
	return func(rt *Router) { rt.cloudflareDNSSecrets = s }
}

// WithDomainBasicAuthSecrets enables PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/auth. Without one configured
// (the default), both return 501; GET works regardless, the same shape
// WithCloudflareTunnelSecrets establishes.
func WithDomainBasicAuthSecrets(s DomainBasicAuthSecrets) Option {
	return func(rt *Router) { rt.domainBasicAuthSecrets = s }
}

// WithComposeSecrets enables POST /api/v1/apps/{name}/compose to
// resolve a compose file's generatable SERVICE_ magic vars. Without one
// configured (the default), that endpoint still works for compose files
// with no such vars, and fails with a clear error for ones that need
// one.
func WithComposeSecrets(s ComposeSecretStore) Option {
	return func(rt *Router) { rt.composeSecrets = s }
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

// WithTwoFactorSecrets enables the 2FA setup/confirm/disable/regenerate
// routes (twofactor.go), which need to write and read back a user's
// TOTP secret. Without one configured, those return 501; GET
// /api/v1/auth/2fa (status) works regardless, since it only reads
// store.User.TOTPEnabled.
func WithTwoFactorSecrets(s TwoFactorSecrets) Option {
	return func(rt *Router) { rt.twoFactorSecrets = s }
}

// WithGitLabAppSecrets enables the GitLab App routes that read or write
// a credential (connect, connect-start, callback, project listing,
// use-as-source). Without one configured, those return 501; GET/DELETE
// /api/v1/gitlab-app work regardless.
func WithGitLabAppSecrets(s GitLabAppSecrets) Option {
	return func(rt *Router) { rt.gitlabAppSecrets = s }
}

// WithBitbucketAppSecrets enables the Bitbucket App routes that read or
// write a credential (connect, connect-start, callback, repo/branch
// listing, use-as-source). Without one configured, those return 501;
// GET/DELETE /api/v1/bitbucket-app work regardless.
func WithBitbucketAppSecrets(s BitbucketAppSecrets) Option {
	return func(rt *Router) { rt.bitbucketAppSecrets = s }
}

// WithGitHubAppManifestConfig overrides the permissions/events a fresh
// App registration requests (githubapp.BuildManifest's own doc
// comment). Without one configured, NewRouter defaults to
// githubapp.DefaultManifestConfig(), the same request this codebase has
// always sent.
func WithGitHubAppManifestConfig(cfg githubapp.ManifestConfig) Option {
	return func(rt *Router) { rt.githubAppManifestConfig = cfg }
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

// WithBackupVerifier enables
// POST /api/v1/databases/{name}/backups/{historyId}/verify. Without one
// configured (the default), that route returns 501, the same
// "not configured" shape WithBackupRunner's absence produces: verifying
// needs the identical live secretsManager WithBackupRunner and
// WithBackupDownloader already depend on, to resolve a target's
// credentials and re-download the object. Listing past verification
// attempts (GET .../verifications) works regardless, the same "listing
// needs no live runner" reasoning WithBackupRunner's own doc comment
// gives for backup history.
func WithBackupVerifier(v BackupVerifier) Option {
	return func(rt *Router) { rt.backupVerifier = v }
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

// WithNotificationDeliveries enables the delivery-history route and
// records a delivery row for every existing-channel test-send. Without
// one, the deliveries route returns 501 and test-send simply skips
// recording.
func WithNotificationDeliveries(d NotificationDeliveryStore) Option {
	return func(rt *Router) { rt.notificationDeliveries = d }
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

// WithRegistryAuthTester enables POST
// /api/v1/registry-credentials/{id}/test. Without one configured (the
// default), that route returns 501, the same shape WithDockerPinger's
// own absence produces for GET /api/v1/system/status.
func WithRegistryAuthTester(t RegistryAuthTester) Option {
	return func(rt *Router) { rt.registryAuthTester = t }
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

// WithDBPinger enables the database check on GET /api/v1/system/doctor.
// Without one configured (the default), that check reports unknown, the
// same "optional signal, absence is not an error" shape WithDockerPinger's
// own absence already has.
func WithDBPinger(p DBPinger) Option {
	return func(rt *Router) { rt.dbPinger = p }
}

// WithDoctorDiskWarningBytes overrides the free-space floor GET
// /api/v1/system/doctor's disk_space check warns below. Without one
// configured (or passed as 0), defaultDoctorDiskWarningBytes (1GiB)
// applies. Same "no hardcoded thresholds, use env vars" shape as
// WithCertExpiryWarningWindow: this package never reads the environment
// directly, cmd/levelrail/main.go reads APP_DOCTOR_DISK_WARNING_BYTES
// and passes the parsed value here.
func WithDoctorDiskWarningBytes(n int64) Option {
	return func(rt *Router) { rt.doctorDiskWarningBytes = n }
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

// WithScheduledTaskRunner enables POST
// /api/v1/apps/{name}/scheduled-tasks/{id}/run. Without one configured
// (the default), that route returns 501: scheduled task CRUD still
// works either way (rt.scheduledTasks is always set), only actually
// running one on demand needs this. r should be the exact same
// *scheduledtask.Runner instance cmd/levelrail/main.go wires into
// scheduledtask.NewScheduler, the same "construct once, share by
// reference" reasoning WithBackupRunner's own doc comment gives for
// backupRunner.
func WithScheduledTaskRunner(r ScheduledTaskRunner) Option {
	return func(rt *Router) { rt.scheduledTaskRunner = r }
}

// WithCertExpiryWarningWindow overrides how far ahead of a certificate's
// expiry GET /api/v1/certificates starts reporting "expiring_soon"
// instead of "healthy" (see alerting.CertExpiryStatus). Without one
// configured (or passed as 0), alerting.DefaultCertExpiryWarningWindow
// (14 days) applies. Same "no hardcoded thresholds, use env vars" shape
// as WithSessionTTL: this package never reads the environment directly,
// cmd/levelrail/main.go reads APP_CERT_EXPIRY_WARNING_WINDOW and passes
// the parsed duration here, and to alerting.NewEngine for a
// kind=cert_expiry rule's own evaluation.
func WithCertExpiryWarningWindow(d time.Duration) Option {
	return func(rt *Router) { rt.certExpiryWarningWindow = d }
}

// WithPreviewTTL overrides how long a preview environment can go without
// a webhook update before SweepStalePreviewEnvironments tears it down.
// Without one configured (or passed as 0), defaultPreviewTTL (7 days)
// applies. Same "no hardcoded thresholds, use env vars" shape as
// WithCertExpiryWarningWindow: this package never reads the environment
// directly, cmd/levelrail/main.go reads APP_PREVIEW_TTL and passes the
// parsed duration here.
func WithPreviewTTL(d time.Duration) Option {
	return func(rt *Router) { rt.previewTTL = d }
}

// WithResourceRecommendationLookback overrides how far back GET
// /api/v1/apps/{name}/resource-recommendation looks for usage history
// (handleAppResourceRecommendation). Without one configured (or passed as
// 0), defaultResourceRecommendationLookback (7 days) applies. Same "no
// hardcoded thresholds, use env vars" shape as WithSessionTTL/
// WithCertExpiryWarningWindow: this package never reads the environment
// directly, cmd/levelrail/main.go reads APP_RESOURCE_RECOMMENDATION_LOOKBACK
// and passes the parsed duration here.
func WithResourceRecommendationLookback(d time.Duration) Option {
	return func(rt *Router) { rt.resourceRecommendationLookback = d }
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

// WithDeployLogQuerier enables a finished deploy attempt's full log replay
// on GET /api/v1/apps/{name}/deploys/{deployId}/logs. Without one
// configured (the default), that route returns 501 for an attempt that
// has already finished; an in-progress attempt's live tail is unaffected
// (see WithDeployRecorder). cmd/levelrail/main.go passes the same
// *telemetry.DB internal/deploylog.Recorder writes to.
func WithDeployLogQuerier(s DeployLogQuerier) Option {
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

// WithAPIRateLimit enables a general per-actor rate limit across every
// requireAbility-gated route (api_rate_limit.go): read-tier (AbilityRead)
// requests get readPerMinute budget, everything else (write,
// write:sensitive, deploy, root) gets the stricter writePerMinute
// budget, keyed per bearer token, per session user, or per client IP for
// an unauthenticated caller. Either argument <= 0 disables that tier.
// Without this option (the default), the whole check is skipped:
// existing tests and any embedder that never opts in see unthrottled
// behavior, the same "nil is valid" shape WithSecretSetter's own absence
// already establishes. This package never reads the environment
// directly: cmd/levelrail/main.go reads APP_API_RATE_LIMIT_READ_RPM /
// APP_API_RATE_LIMIT_WRITE_RPM and calls this unconditionally with their
// resolved (env-or-default) values, so the real control plane is always
// protected even when an operator never sets either env var.
func WithAPIRateLimit(readPerMinute, writePerMinute int) Option {
	return func(rt *Router) { rt.apiRateLimit = newAPIRateLimit(readPerMinute, writePerMinute) }
}
