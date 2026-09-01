package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
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
	// UpdateServiceApp is stage 2 of multi-service apps
	// (migrations/0039_apps.sql)'s own service-side setter: which
	// store.App a service belongs to. Same separation-from-ordinary-
	// update reasoning as UpdateServiceNode/UpdateServiceProject/
	// UpdateServiceStorageTarget/UpdateServiceSuspended, see
	// store.DB.UpdateServiceApp's own doc comment. Called from
	// ensureAppLinked (apps_multi.go), the shared create-or-reuse-and-link
	// path both handleCreateApp (an ordinary single-service app) and
	// handleDeploySpec (a multi-service fan-out) go through.
	UpdateServiceApp(ctx context.Context, name, appID string) error
	// UpdateServiceLogDrain backs PUT/DELETE
	// /api/v1/apps/{name}/log-drain (apps_log_drain.go): which external
	// sink (internal/telemetry.DrainForwarder) this app's container logs
	// additionally forward to. Same separation-from-ordinary-update
	// reasoning as UpdateServiceStorageTarget, see
	// store.DB.UpdateServiceLogDrain's own doc comment.
	UpdateServiceLogDrain(ctx context.Context, name string, drain *store.LogDrain) error
	// UpdateServiceDatabaseAttachment backs PUT/DELETE
	// /api/v1/apps/{name}/database (apps_database.go): which managed
	// database (DatabaseStore below) this app resolves one connection env
	// var from. Same separation-from-ordinary-update reasoning as
	// UpdateServiceStorageTarget, see store.DB.UpdateServiceDatabaseAttachment's
	// own doc comment.
	UpdateServiceDatabaseAttachment(ctx context.Context, name string, att *store.DatabaseAttachment) error
}

// AppGroupLister is the store surface GET /api/v1/apps/{name}/group
// (apps_group.go) and the app-linking write path (apps_multi.go's
// ensureAppLinked, apps.go's deleteAppIfOrphaned) need: stage 1 of
// multi-service apps (migrations/0039_apps.sql) started this as a
// read-only interface; stage 2 (per-app Docker networking) added the
// store.App CRUD every create/delete path needs to keep that App row's
// lifecycle in sync with its member services, since
// internal/reconcile/application.NetworkCleanupController diffs
// exactly the App rows this interface reports against Docker's own
// observed networks. *store.DB satisfies this structurally.
type AppGroupLister interface {
	ListServicesByApp(ctx context.Context, appID string) ([]store.DesiredService, error)
	DeleteApp(ctx context.Context, id string) error
	SaveApp(ctx context.Context, a store.App) error
	GetAppByName(ctx context.Context, name string) (store.App, error)
}

// AppComposeStore is the store surface POST /api/v1/apps/{name}/compose
// needs (apps_compose.go): create-or-reuse the owning store.App, then
// save each of its member services.
type AppComposeStore interface {
	SaveApp(ctx context.Context, a store.App) error
	GetAppByName(ctx context.Context, name string) (store.App, error)
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
}

// ComposeSecretStore is the surface POST /api/v1/apps/{name}/compose
// needs to resolve a compose file's SERVICE_ magic vars
// (compose.ResolveMagicVars): a canonical generate-once-per-app value
// (Resolve/SetValue keyed by the app's own name, a synthetic bucket
// distinct from any real service), plus one SetValue per real service
// that references it, since internal/secrets.Manager itself is keyed
// per real service, not per app.
type ComposeSecretStore interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
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

// WebhookDeliveryStore is the store surface real inbound webhook
// delivery history needs: row-per-delivery CRUD backing
// GET /api/v1/apps/{name}/webhook-deliveries and its replay endpoint.
// *store.DB satisfies this structurally.
type WebhookDeliveryStore interface {
	SaveWebhookDelivery(ctx context.Context, d store.WebhookDelivery) error
	GetWebhookDelivery(ctx context.Context, id string) (*store.WebhookDelivery, error)
	ListWebhookDeliveries(ctx context.Context, serviceName string, limit int, before *time.Time) ([]store.WebhookDelivery, error)
}

// DeployLogQuerier is the telemetry-side read surface
// handleDeployLogStream needs to serve a full replay once an attempt has
// already finished (see that handler's own doc comment for the
// in-progress-vs-finished split). *telemetry.DB satisfies this
// structurally. Optional (nil is valid, the same "not configured" shape
// WithTelemetryQuerier's own absence already has): without one, a
// finished attempt's log route returns 501; an in-progress attempt's
// live tail (backed by deployRecorder below, a separate optional field)
// is unaffected by this one being unset.
type DeployLogQuerier interface {
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
	SetDatabaseBackupSchedule(ctx context.Context, name, targetID, schedule string, retain, retainDays int) error
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
	// SetProjectEnvVars/ListProjectEnvVars back GET/PUT
	// /api/v1/projects/{id}/env (project_env.go): shared env vars every
	// app filed under this project inherits as its resolveEnv base
	// layer (internal/reconcile/application), full-replace on write, the
	// same shape PUT /api/v1/apps/{name}'s own env field already has.
	SetProjectEnvVars(ctx context.Context, projectID string, vars map[string]string) error
	ListProjectEnvVars(ctx context.Context, projectID string) (map[string]string, error)
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
	UpdateUserAbilities(ctx context.Context, id string, abilities []string) error
	EnableUserTOTP(ctx context.Context, id string, confirmedAt time.Time) error
	DisableUserTOTP(ctx context.Context, id string) error
}

// RecoveryCodeStore is the store surface the 2FA handlers (twofactor.go)
// need for single-use fallback codes: always set, part of the core
// Store interface, the same "no secrets configuration needed" shape
// AuthStore itself has (a recovery code's hash is an ordinary column,
// unlike a TOTP secret which goes through TwoFactorSecrets below).
type RecoveryCodeStore interface {
	ReplaceUserRecoveryCodes(ctx context.Context, userID string, hashes []string) error
	ConsumeUserRecoveryCode(ctx context.Context, userID, hash string) (bool, error)
	CountUnusedUserRecoveryCodes(ctx context.Context, userID string) (int, error)
	DeleteUserRecoveryCodes(ctx context.Context, userID string) error
}

// EmailSettingsStore is the store surface GET/PUT
// /api/v1/settings/email need: the single platform-wide row, always
// present, the same shape IngressSettingsStore has for its own row.
type EmailSettingsStore interface {
	GetEmailSettings(ctx context.Context) (store.EmailSettings, error)
	UpdateEmailSettings(ctx context.Context, s store.EmailSettings) error
}

// CloudflareTunnelStore is the store surface GET/PUT/DELETE
// /api/v1/settings/cloudflare-tunnel need: the single platform-wide row,
// always present, the same shape EmailSettingsStore has for its own row.
type CloudflareTunnelStore interface {
	GetCloudflareTunnelSettings(ctx context.Context) (store.CloudflareTunnelSettings, error)
	UpdateCloudflareTunnelSettings(ctx context.Context, s store.CloudflareTunnelSettings) error
}

// CloudflareDNSStore is the store surface GET/PUT
// /api/v1/settings/cloudflare-dns need, the same "single platform-wide
// row" shape CloudflareTunnelStore already establishes.
type CloudflareDNSStore interface {
	GetCloudflareDNSSettings(ctx context.Context) (store.CloudflareDNSSettings, error)
	UpdateCloudflareDNSSettings(ctx context.Context, s store.CloudflareDNSSettings) error
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

// OnboardingStore is defined in onboarding.go, next to the handlers that
// use it (the same "single-file feature" shape scheduled_task.go/
// feature_flags.go already use, rather than every store interface living
// in this file).

// AuditStore is the store surface requireAbility's audit hook
// (internal/api/auth.go), GET /api/v1/audit-log (audit.go), and the
// retention sweep/manual purge (audit_retention.go) need: insert-only
// recording, newest-first cursor-paginated reading, and age-based
// deletion. Always set, the same "core Store interface, not an optional
// plug-in" shape certs/staticSites/backupTargets already have.
type AuditStore interface {
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
	ListAuditEntries(ctx context.Context, limit int, before *time.Time, filter store.AuditEntryFilter) ([]store.AuditEntry, error)
	DeleteAuditEntriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// Store is the full surface NewRouter needs. *store.DB satisfies it
// structurally, same pattern internal/reconcile/application.ServiceStore
// already established for this codebase.
type Store interface {
	AppStore
	AppGroupLister
	AppComposeStore
	DeployStore
	DeployAttemptStore
	DatabaseStore
	AuthStore
	TokenStore
	NodeStore
	CertStore
	StaticSiteStore
	BackupTargetStore
	RegistryCredentialStore
	BackupHistoryStore
	BackupVerificationStore
	RestoreHistoryStore
	ServiceVolumeBackupHistoryStore
	ServiceVolumeBackupScheduleStore
	ServiceVolumeRestoreHistoryStore
	CloneRestoreHistoryStore
	VolumeCloneRestoreHistoryStore
	ProjectStore
	OrganizationStore
	EnvironmentStore
	IngressSettingsStore
	DomainStore
	DomainBasicAuthStore
	GitSourceStore
	PreviewEnvironmentStore
	GitHubAppStore
	GitLabAppStore
	BitbucketAppStore
	OAuthSettingsStore
	OAuthIdentityStore
	EmailSettingsStore
	CloudflareTunnelStore
	CloudflareDNSStore
	PasswordResetTokenStore
	RecoveryCodeStore
	AuditStore
	ScheduledTaskStore
	FeatureFlagStore
	OnboardingStore
	WebhookDeliveryStore
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

// MasterKeyRotator is the surface POST /api/v1/system/master-key/rotate
// and the doctor's rotation-age check need from
// internal/secrets.Manager: rotate every stored DEK to a new master key
// (given as its serialized string, so this interface never imports
// internal/secrets' own MasterKey type), and report when that last
// succeeded. A distinct interface from SecretSetter above since a token
// scoped to AbilityWrite (SecretSetter's own gate) has no business
// rotating the one key every other secret in this control plane depends
// on; this is gated at AbilityRoot instead (see routes.go).
type MasterKeyRotator interface {
	RotateMasterKey(ctx context.Context, newMasterKey string) (rotatedAt time.Time, err error)
	GetMasterKeyRotatedAt(ctx context.Context) (rotatedAt time.Time, ok bool, err error)
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

// DBPinger is the surface GET /api/v1/system/doctor needs to report
// the control plane's own SQLite reachability, the same narrow,
// single-purpose shape DockerPinger above already establishes.
// *store.DB satisfies this structurally via its embedded *sql.DB's
// PingContext method.
type DBPinger interface {
	PingContext(ctx context.Context) error
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

// RegistryAuthTester is the surface POST
// /api/v1/registry-credentials/{id}/test needs: ask the control plane's
// own local Docker daemon to authenticate against a registry host with a
// username/password pair, without pulling anything. Fixed to this
// control plane's own local daemon, the same DockerPinger/ImageLister/
// DockerDiskUsager/DockerPruner reasoning above applies: adding this to
// docker.Runtime would ripple into 6+ fakes for one narrowly-scoped
// signal. *docker.Client satisfies this structurally via
// TestRegistryAuth (internal/docker/client.go).
type RegistryAuthTester interface {
	TestRegistryAuth(ctx context.Context, host, username, password string) error
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
