package store

import (
	"fmt"
)

// DesiredService is what the application controller reconciles running
// containers against: an already-resolved image plus what it needs to
// run. See the migration comment in migrations/0002_desired_services.sql
// for why this is a distinct type from internal/spec.Service rather than
// reusing it directly.
type DesiredService struct {
	Name  string
	Image string
	// ImageID is the local content ID a build produced for ImageIDRef.
	// Only trusted while ImageIDRef still equals Image (LocalImageID).
	ImageID    string
	ImageIDRef string
	Port       int
	// HostPort pins the host-side port Docker binds Port to; nil lets Docker
	// choose. An ordinary field written on every SaveDesiredService call.
	HostPort *int
	// BindAddress picks which network interface Port (and HostPort, if
	// pinned) binds to on the host (migrations/0103_service_bind_address.sql):
	// "private", "public", or a literal IP, see internal/bindaddr.Resolve.
	// Unlike HostPort, this is never empty after SaveDesiredService: an
	// unset value resolves to DefaultBindAddress there, the same
	// resolve-and-always-persist-concrete treatment Strategy/Replicas
	// already get.
	BindAddress string
	// PublishedPorts are extra ports bound straight on the host, beside the
	// routed Port; today only UDP, which the HTTP proxy cannot carry.
	PublishedPorts []PublishedPort
	Domains        []string
	Env            map[string]string
	// Command overrides the image's own default CMD
	// (internal/docker.ContainerSpec.Command), nil/empty meaning the
	// image's own default. Populated from a compose service's command:
	// (internal/compose.Service.Command); app.yaml has no equivalent
	// field yet.
	Command []string
	// Entrypoint overrides the image's own default ENTRYPOINT
	// (internal/docker.ContainerSpec.Entrypoint), same nil/empty and
	// compose-only sourcing as Command above, from
	// internal/compose.Service.Entrypoint.
	Entrypoint []string
	// DependsOn names sibling services (bare key, e.g. "db") this service waits
	// on before its containers start, like compose depends_on service_started.
	DependsOn []string
	// DependsOnHealthy is the subset of DependsOn that waits for the
	// dependency's Ready condition (compose condition: service_healthy).
	DependsOnHealthy []string
	// PullPolicy is PullPolicyAlways to force a fresh image pull at
	// deploy time even when the tag already exists locally
	// (internal/docker.ContainerSpec.ForcePull), or empty for today's
	// existing pull-if-absent behavior. Compose-only sourcing, same as
	// Command/Entrypoint above, from internal/compose.Service.PullPolicy
	// (migrations/0098_service_pull_policy.sql).
	PullPolicy string
	// EnvDirty is true when Env was saved through the update endpoint
	// since the container was last recreated (migrations/0066): env is
	// baked in at create time, so the change isn't live yet. Written by
	// SaveDesiredService like Env itself, unlike RestartNonce/Suspended,
	// so an ordinary fresh-state redeploy clears it for free; callers
	// that bypass SaveDesiredService (RestartService,
	// UpdateServiceSuspended(false)) clear it explicitly instead.
	EnvDirty bool
	// RegistryCredentialID is which store.RegistryCredential
	// (migrations/0046_registry_credentials.sql) to authenticate with
	// when pulling Image, resolved by internal/docker at container-
	// create time; empty string means an unauthenticated (public) pull.
	// A normal deploy-time field, unlike NodeID/ProjectID/
	// StorageTargetID below: SaveDesiredService writes it on every
	// call, the same as Image itself, since it comes from the same
	// app.yaml build.type: image block.
	RegistryCredentialID string
	// SecretEnv lists env vars whose values live in secret storage
	// (internal/secrets), resolved and decrypted by the
	// application controller immediately before container creation.
	// Never holds a value itself, only the key name plus whether it was
	// declared { required: true } (see SecretEnvRef's own doc comment):
	// a name is not a secret, only the value is.
	SecretEnv []SecretEnvRef

	// DatabaseEnv names env vars whose values resolve from a managed
	// database's own connection details (internal/spec's { from:
	// "<database>.<field>" } env var syntax), resolved by the application
	// controller immediately before container creation, the same
	// "ordinary desired state, written on every save" treatment SecretEnv
	// gets: derived fresh from app.yaml on every deploy.
	DatabaseEnv map[string]DatabaseEnvRef

	// VaultEnv names env vars whose values resolve from an external
	// HashiCorp Vault instance (internal/spec's { vault: { path, key } }
	// env var syntax), resolved by the application controller
	// immediately before container creation the same way DatabaseEnv is:
	// derived fresh from app.yaml on every deploy, never persisted with
	// a value.
	VaultEnv map[string]VaultEnvRef

	Resources *ServiceResources
	Health    *ServiceHealth
	// Hooks are this service's pre/post-deploy commands
	// (internal/spec.Service.Hooks), nil meaning neither is configured.
	// A normal deploy-time field, resolved and stored as a whole on every
	// SaveDesiredService call, the same shape Resources/Health already
	// follow.
	Hooks *ServiceHooks

	// Egress is the outbound allowlist; nil means unrestricted. Written on every
	// SaveDesiredService call and also by UpdateServiceEgressPolicy.
	Egress *ServiceEgressPolicy

	// Volumes are named Docker volumes this service's container mounts
	// (migrations/0041_service_volumes.sql), previously a
	// database-controller-only capability. Empty for the ordinary case
	// (a stateless app), same "declarative, resolved before storing"
	// shape as Resources/Health above.
	Volumes []ServiceVolume

	// BindMounts are real host directories this service's container
	// mounts directly (migrations/0088_service_bind_mounts.sql), see
	// ServiceBindMount's own doc comment for how this differs from
	// Volumes and how it's gated. Empty for the ordinary case, same
	// "declarative, resolved before storing" shape Volumes itself
	// follows.
	BindMounts []ServiceBindMount

	// Files are inline config files written into the container before it
	// starts (migrations/3042). nil leaves the stored list alone on
	// SaveDesiredService, so an update path that never touched files cannot
	// wipe them; a non-nil empty slice clears it.
	Files []ServiceFile

	// Labels are operator Docker labels applied at container create. Validated
	// at every input boundary, so stored values are trusted here.
	Labels map[string]string

	// NodeID is which node (internal/store's own nodes table)
	// this service should run on. Empty string is the explicit
	// "this control plane's own local node" value (the placement
	// migration comment explains why that's not NULL or a foreign key),
	// the only value that existed before this field did, so an existing
	// single-node deployment's services keep running exactly where they
	// already were on upgrade.
	NodeID string

	// Strategy and Replicas are always the resolved (never empty or zero) values;
	// SaveDesiredService also defends against empty input from any caller.
	Strategy string
	Replicas int

	// RestartNonce is only compared for equality: the reconciler folds it into
	// the container name when non-empty, so changing it forces a recreate. Only
	// RestartService writes it.
	RestartNonce string

	// ProjectID is the owning project; empty means none (stored as NULL). Only
	// UpdateServiceProject writes it, never SaveDesiredService.
	ProjectID string

	// EnvironmentID is which store.Environment (migrations/0054) this
	// service is tagged with; empty is "no environment", set only via
	// SetServiceEnvironment, same non-full-replace shape as ProjectID.
	EnvironmentID string

	// StorageTargetID is the BackupTarget whose bucket credentials this app
	// uses; empty means none (stored as NULL). Only UpdateServiceStorageTarget
	// writes it, never SaveDesiredService.
	StorageTargetID string

	// DatabaseAttachment is which managed database this app resolves one
	// connection env var from (migrations/0050_service_database_env.sql),
	// set via PUT/DELETE /api/v1/apps/{name}/database rather than
	// app.yaml: see DatabaseAttachment's own doc comment for how it
	// relates to DatabaseEnv above. nil means "no attachment", a real,
	// permanent state, not merely "unset". Like NodeID/ProjectID/
	// StorageTargetID, SaveDesiredService never writes this field: only
	// UpdateServiceDatabaseAttachment does.
	DatabaseAttachment *DatabaseAttachment

	// Suspended is an operator-requested stop, distinct from delete: the
	// desired service row, image, env, and domains are untouched, only
	// the reconciler's converge target changes to zero running
	// containers (internal/reconcile/application.Controller.Reconcile).
	// Like NodeID/ProjectID/RestartNonce/StorageTargetID,
	// SaveDesiredService never writes this field: only
	// UpdateServiceSuspended does.
	Suspended bool

	// MaintenanceHold, when non-empty, names the operation that holds the
	// service still: the reconciler leaves its containers exactly as they are.
	// Only UpdateServiceMaintenanceHold writes it.
	MaintenanceHold string

	// AppID is which store.App (migrations/0039_apps.sql) this service
	// belongs to; empty string means none. SaveDesiredService only
	// writes it on first INSERT, never on an ON CONFLICT update, the
	// same "an ordinary edit must never silently move it" invariant
	// NodeID/ProjectID/StorageTargetID/Suspended already have (via
	// their own dedicated Update* setters); a service's AppID is fixed
	// at creation.
	AppID string

	// IsTrial marks a service deployed via the one-click template
	// "Deploy now" path (migrations/0264_service_is_trial.sql) as an
	// obviously-temporary trial instance. Like AppID, SaveDesiredService
	// writes it only on first INSERT, never on an ON CONFLICT update: a
	// redeploy must never flip an existing service's trial status either
	// way.
	IsTrial bool
	// TemplateID is the catalog template this service was deployed from; insert-only like IsTrial.
	TemplateID string
	// Incarnation tells two lives of the same app name apart. It is set once on first INSERT, never on update, and never copied by clone; '' marks a legacy row.
	Incarnation string

	// LogDrain is this service's external log-forwarding config
	// (migrations/0047_service_log_drain.sql), nil meaning none
	// configured. Like NodeID/ProjectID/StorageTargetID/Suspended,
	// SaveDesiredService never writes this field: only
	// UpdateServiceLogDrain does.
	LogDrain *LogDrain

	// PreviewEnvOverrides names, for a subset of this service's own env
	// vars, a preview-specific value that replaces the parent's own value
	// only when a preview environment is created from it
	// (migrations/0098_service_preview_env_overrides.sql,
	// internal/api/preview_environments.go's deployPreviewSingle); never
	// applied to this service's own deploy. Like LogDrain,
	// SaveDesiredService never writes this field: only
	// SetServicePreviewEnvOverride does.
	PreviewEnvOverrides map[string]string

	// AutoRollbackOnCrashloop opts this app into automatic rollback
	// (migrations/0106_service_auto_rollback_on_crashloop.sql): once a
	// KindCrashloop alert rule fires for this app, internal/alerting
	// points Image back at the most recent successful deploy's image
	// instead of only notifying. Off by default, like PreviewEnabled.
	// Like LogDrain/PreviewEnvOverrides, SaveDesiredService never writes
	// this field: only SetServiceAutoRollbackOnCrashloop does.
	AutoRollbackOnCrashloop bool

	// AutoRollbackOnSLOBurn is one of the AutoRollbackSLOBurn* modes; "" means
	// off. Only SetServiceAutoRollbackOnSLOBurn writes it.
	AutoRollbackOnSLOBurn string

	// ExecEnabled gates exec and terminal access for this app on top of the IAM
	// check. Default true to keep existing behavior; only SetServiceExecEnabled
	// writes it, never SaveDesiredService.
	ExecEnabled bool

	// BadgeEnabled opts this app into the public, unauthenticated
	// GET /api/v1/apps/{name}/badge.svg deploy-status badge
	// (migrations/0278_service_badge_enabled.sql). Default false, unlike
	// ExecEnabled: a badge exposes deploy status to anyone with the URL,
	// so it must be an explicit per-app choice. Like ExecEnabled,
	// SaveDesiredService never writes this field: only
	// SetServiceBadgeEnabled does.
	BadgeEnabled bool
}

// AutoRollbackSLOBurn* are DesiredService.AutoRollbackOnSLOBurn's valid
// values.
const (
	AutoRollbackSLOBurnOff           = "off"
	AutoRollbackSLOBurnAuto          = "auto"
	AutoRollbackSLOBurnDryRun        = "dry_run"
	AutoRollbackSLOBurnPauseForHuman = "pause_for_human"
)

// DefaultDeployStrategy and DefaultReplicas mirror internal/spec's
// StrategyBlueGreen/DefaultReplicas exactly (same values), kept as this
// package's own constants rather than importing internal/spec: this
// package's own doc comment already establishes that DesiredService is
// deliberately independent of spec.Service, and every other cross-
// package enum in this file (NodeStatus, and so on) follows the same
// "define locally, don't import a higher-level package's vocabulary"
// convention.
const (
	DefaultDeployStrategy = "blue-green"
	DefaultReplicas       = 1
	// DefaultBindAddress mirrors internal/bindaddr.Default (same value,
	// same "define locally" reasoning above): loopback-only, the safe
	// default for a service whose caller never set BindAddress.
	DefaultBindAddress = "private"
)

// PullPolicyAlways mirrors internal/compose's own PullPolicyAlways
// exactly (same value), kept as this package's own constant for the
// same "define locally, don't import a higher-level package's
// vocabulary" reasoning DefaultDeployStrategy's own doc comment gives.
const PullPolicyAlways = "always"

// ErrDomainTaken is returned by SaveDesiredService when a domain is already
// claimed by another service. Two services on one host would yield two
// ingress routes matching the same Host header, one silently shadowing the other.
type ErrDomainTaken struct {
	Domain string
	Owner  string
}

// Error names the domain and the service that already owns it.
func (e *ErrDomainTaken) Error() string {
	return fmt.Sprintf("store: domain %q is already in use by service %q", e.Domain, e.Owner)
}
