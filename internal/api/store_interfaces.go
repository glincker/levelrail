package api

import (
	"context"

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
	// ListAppStreamsForService backs internal/reconcile/application's
	// ServiceStore (the same interface this type satisfies for the
	// application controller's own New call): one extra published
	// container port per raw TCP stream (migrations/0279_app_streams.sql).
	ListAppStreamsForService(ctx context.Context, serviceName string) ([]store.AppStream, error)
	// UpdateServiceNode is the placement mutation, separate
	// from SaveDesiredService on purpose: see store.DB.SaveDesiredService's
	// own doc comment for why an ordinary app update must never be able
	// to silently move a service between nodes.
	UpdateServiceNode(ctx context.Context, name, nodeID string) error
	// ListDesiredServicesByNode is the drain and
	// delete-guard primitive (handleDrainNode, handleDeleteNode): find
	// what's placed on a node without listing every service.
	ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]store.DesiredService, error)
	// ListDesiredServicesByProject is ListDesiredServicesByNode's
	// project-kind counterpart: handleStopProject/handleStartProject
	// (project_stop_start.go) and handleRestartProject
	// (project_restart.go) use it to find every app in a project without
	// listing every service.
	ListDesiredServicesByProject(ctx context.Context, projectID string) ([]store.DesiredService, error)
	// ListDesiredServicesByEnvironment is ListDesiredServicesByProject's
	// environment-kind counterpart: environment_clone.go's own clone
	// preview/execute handlers use it to find every app tagged with the
	// environment being cloned.
	ListDesiredServicesByEnvironment(ctx context.Context, environmentID string) ([]store.DesiredService, error)
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
	// SetServiceAutoRollbackOnCrashloop backs PUT
	// /api/v1/apps/{name}/auto-rollback (deploys.go): whether
	// internal/alerting.MaybeAutoRollback rolls this app back
	// automatically the next time a KindCrashloop rule fires for it. Same
	// separation-from-ordinary-update reasoning as UpdateServiceStorageTarget,
	// see store.DB.SetServiceAutoRollbackOnCrashloop's own doc comment.
	SetServiceAutoRollbackOnCrashloop(ctx context.Context, name string, enabled bool) error
	// SetServiceAutoRollbackOnSLOBurn backs PUT
	// /api/v1/apps/{name}/auto-rollback-slo-burn (deploys.go): which mode
	// internal/alerting.MaybeAutoRollbackOnSLOBurn acts in the next time a
	// KindSLOBurn rule fires for this app. Same separation-from-ordinary-
	// update reasoning as SetServiceAutoRollbackOnCrashloop, see
	// store.DB.SetServiceAutoRollbackOnSLOBurn's own doc comment.
	SetServiceAutoRollbackOnSLOBurn(ctx context.Context, name, mode string) error
	// SetServiceExecEnabled backs PUT /api/v1/apps/{name}/exec-access
	// (exec.go): whether POST .../exec and GET .../terminal are even
	// attempted for this app, independent of the caller's own IAM
	// abilities. Same separation-from-ordinary-update reasoning as
	// SetServiceAutoRollbackOnCrashloop, see
	// store.DB.SetServiceExecEnabled's own doc comment.
	SetServiceExecEnabled(ctx context.Context, name string, enabled bool) error
	// SetServiceBadgeEnabled backs PUT /api/v1/apps/{name}/badge
	// (app_badge.go): whether GET .../badge.svg serves anything for
	// this app, or 404s. Same separation-from-ordinary-update reasoning
	// as SetServiceAutoRollbackOnCrashloop, see
	// store.DB.SetServiceBadgeEnabled's own doc comment.
	SetServiceBadgeEnabled(ctx context.Context, name string, enabled bool) error
	// SetServiceVaultEnvVar backs PUT/DELETE
	// /api/v1/apps/{name}/vault-env/{key} (apps_vault_env.go): the
	// UI/CLI-facing way to declare (or remove) one Vault-sourced env var
	// on an app that already exists, without going through the general
	// PUT /api/v1/apps/{name}, which would silently drop every other
	// declarative field (VaultEnv included) it doesn't itself carry
	// forward. Same separation-from-ordinary-update reasoning as
	// UpdateServiceDatabaseAttachment, see store.DB.SetServiceVaultEnvVar's
	// own doc comment.
	SetServiceVaultEnvVar(ctx context.Context, name, envVar string, ref *store.VaultEnvRef) error
	// SetServiceDatabaseEnvVar backs POST/DELETE
	// /api/v1/apps/{name}/connections[/{env_var}] (apps_connections.go):
	// the UI/CLI-facing way to declare (or remove) one entry in an app's
	// DatabaseEnv map, letting an app connect to more than one managed
	// database without app.yaml. Same separation-from-ordinary-update
	// reasoning as SetServiceVaultEnvVar, see
	// store.DB.SetServiceDatabaseEnvVar's own doc comment.
	SetServiceDatabaseEnvVar(ctx context.Context, name, envVar string, ref *store.DatabaseEnvRef) error
	// SetServicePreviewEnvOverride backs PUT/DELETE
	// /api/v1/apps/{name}/preview-env/{key} (apps_preview_env.go): the
	// UI/CLI-facing way to declare (or remove), on an app that already
	// exists, a preview-specific value for one env var that replaces
	// whatever this app's own Env/SecretEnv/VaultEnv holds for that key,
	// applied only when a preview environment is next created from this
	// app (deployPreviewSingle, preview_environments.go), never to this
	// app's own deploy. Same separation-from-ordinary-update reasoning as
	// SetServiceVaultEnvVar, see store.DB.SetServicePreviewEnvOverride's
	// own doc comment.
	SetServicePreviewEnvOverride(ctx context.Context, name, envVar string, value *string) error
	// UpdateServiceEgressPolicy backs GET/PUT/DELETE
	// /api/v1/apps/{name}/egress-policy (apps_egress.go): the UI/CLI-
	// facing way to set (or clear) an app's outbound network allowlist
	// outside a redeploy, mirroring SetServiceVaultEnvVar's own dual
	// write path (app.yaml's egress: block also sets this, via the
	// ordinary SaveDesiredService full-record-replace). Same
	// separation-from-ordinary-update reasoning as
	// UpdateServiceStorageTarget, see
	// store.DB.UpdateServiceEgressPolicy's own doc comment.
	UpdateServiceEgressPolicy(ctx context.Context, name string, policy *store.ServiceEgressPolicy) error
	// UpdateServiceHealth backs PUT /api/v1/apps/{name}/health (apps_health.go).
	UpdateServiceHealth(ctx context.Context, name string, health *store.ServiceHealth) error
	// UpdateServiceVolumes backs PUT /api/v1/apps/{name}/volumes
	// (apps_volumes_attach.go): the UI/CLI-facing way to attach or
	// remove a named Docker volume outside a redeploy, mirroring
	// UpdateServiceEgressPolicy's own dual write path. See
	// store.DB.UpdateServiceVolumes's own doc comment.
	UpdateServiceVolumes(ctx context.Context, name string, volumes []store.ServiceVolume) error
	// SetServiceBranchEnvOverride, DeleteServiceBranchEnvOverride, and
	// ListServiceBranchEnvOverrides back POST/DELETE/GET
	// /api/v1/apps/{name}/branch-env (apps_branch_env.go): a narrower,
	// branch-pattern-matched sibling of SetServicePreviewEnvOverride
	// above, applied only when a preview's own branch matches, see
	// store.SetServiceBranchEnvOverride's own doc comment.
	SetServiceBranchEnvOverride(ctx context.Context, serviceName, branchPattern, key, value string, secret bool) (string, error)
	DeleteServiceBranchEnvOverride(ctx context.Context, serviceName, id string) error
	ListServiceBranchEnvOverrides(ctx context.Context, serviceName string) ([]store.ServiceBranchEnvOverride, error)
}

// Store is the full surface NewRouter needs. *store.DB satisfies it
// structurally, same pattern internal/reconcile/application.ServiceStore
// already established for this codebase.
type Store interface {
	AppStore
	AppGroupLister
	AppComposeStore
	CustomTemplateStore
	DeployStore
	DeployAttemptStore
	ProbeAttemptStore
	DatabaseStore
	AuthStore
	TokenStore
	InstanceStore
	NodeStore
	CertStore
	StaticSiteStore
	BackupTargetStore
	RegistryCredentialStore
	NetworkShareStore
	FirewallRuleStore
	AppStreamStore
	BackupHistoryStore
	BackupVerificationStore
	RestoreHistoryStore
	BaseBackupHistoryStore
	PITRRestoreHistoryStore
	MajorUpgradeStore
	ServiceVolumeBackupHistoryStore
	ServiceVolumeBackupScheduleStore
	ServiceVolumeRestoreHistoryStore
	CloneRestoreHistoryStore
	VolumeCloneRestoreHistoryStore
	AppVolumeMoveStore
	ProjectStore
	OrganizationStore
	EnvironmentStore
	IngressSettingsStore
	UpdateSettingsStore
	DomainStore
	DomainBasicAuthStore
	DomainMaintenanceStore
	DomainTLSCertStore
	DomainWAFStore
	DomainAuthGateStore
	DomainRedirectStore
	DomainErrorPagesStore
	DomainSearchVisibilityStore
	DomainTrafficPolicyStore
	CachePurgeStore
	EdgeFirewallStore
	GitSourceStore
	AppScheduleStore
	AppGitHubActionsStore
	CIOIDCTrustStore
	ImageAutoUpdateStore
	BuildHookStore
	AppSleepStore
	DataImportStore
	MigrationHubStore
	MigrationChecklistStore
	AppImportStore
	LoginCodeStore
	SecurityStore
	ExternalDatabaseStore
	CanaryStore
	DatabaseQueryStore
	PreviewEnvironmentStore
	BranchDeployStore
	GitHubAppStore
	GitConnectionsStore
	OAuthSettingsStore
	OAuthIdentityStore
	EmailSettingsStore
	ObservabilityStore
	CloudflareTunnelStore
	RegistryStore
	CloudflareDNSStore
	Route53DNSStore
	NodeProvisionStore
	NodeProviderConnectionStore
	PackageRegistryStore
	SSHNodeProvisionStore
	VaultSettingsStore
	SessionLinkTokenStore
	InviteStore
	PushSubscriptions
	AuditStore
	ScheduledTaskStore
	FeatureFlagStore
	TagStore
	AppIntegrationStore
	OnboardingStore
	WebhookDeliveryStore
	PolicyStore
	HookRunStore
	DeployApprovalStore
	AIChatStore
	RoleStore
	AIControlStore
}
