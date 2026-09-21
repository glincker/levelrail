package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ServiceResources caps a service's memory and CPU, in the same units
// internal/docker.Resources already uses (bytes, nano-CPUs), not
// app.yaml's human-friendly "512Mi"/0.5-cores strings. Translating those
// is the deploy pipeline's job, not this package's;
// by the time a DesiredService exists, its units are already resolved.
type ServiceResources struct {
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
	// SwapMemoryBytes is Docker's own MemorySwap: total memory plus swap
	// combined, only meaningful alongside MemoryBytes.
	SwapMemoryBytes int64 `json:"swap_memory_bytes,omitempty"`
	// CPUSetCPUs pins the container to specific host CPUs, Docker's own
	// cpuset-cpus format (e.g. "0-3" or "0,2").
	CPUSetCPUs string `json:"cpuset_cpus,omitempty"`
}

// ServiceProbe is one readiness or liveness check.
type ServiceProbe struct {
	Path     string        `json:"path"`
	Interval time.Duration `json:"interval,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
	Failures int           `json:"failures,omitempty"`
}

// ServiceHealth holds a service's probe configuration.
type ServiceHealth struct {
	Readiness *ServiceProbe `json:"readiness,omitempty"`
	Liveness  *ServiceProbe `json:"liveness,omitempty"`
	// ReadyTimeout overrides internal/reconcile/application's own
	// defaultReadyBudget (60s) for this service alone, zero meaning
	// "use the default". See internal/spec.Health.ReadyTimeout, its
	// app.yaml source.
	ReadyTimeout time.Duration `json:"ready_timeout,omitempty"`
}

// ServiceHooks holds a service's pre/post-deploy hook commands
// (internal/spec.Hooks' storage home, migrations/0082_service_hooks.sql).
// See internal/reconcile/application.Controller's own doc comment for
// when each one runs and what a failure does.
type ServiceHooks struct {
	PreDeploy  string `json:"pre_deploy,omitempty"`
	PostDeploy string `json:"post_deploy,omitempty"`
}

// ServiceEgressAllow is one host+port pair a service's outbound traffic
// may reach when its ServiceEgressPolicy.Mode is EgressModeAllowlist.
type ServiceEgressAllow struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// EgressModeAllowlist is the only meaningful ServiceEgressPolicy.Mode
// value today, matching internal/spec.EgressModeAllowlist exactly (this
// package deliberately doesn't import internal/spec, the same "define
// locally, don't import a higher-level package's vocabulary" convention
// Strategy's own field doc comment already establishes).
const EgressModeAllowlist = "allowlist"

// ServiceEgressPolicy holds a service's outbound network allowlist
// (internal/spec.Egress's storage home,
// migrations/0112_service_egress_policy.sql). nil means egress is
// unrestricted, today's behavior for every service before this field
// existed and the permanent default for any service that never opts in:
// see internal/spec.Egress's own doc comment for why this is opt-in
// rather than deny-by-default.
type ServiceEgressPolicy struct {
	Mode  string               `json:"mode"`
	Allow []ServiceEgressAllow `json:"allow,omitempty"`
}

// DatabaseEnvRef is one env var's parsed { from: "<database>.<field>" }
// reference (internal/spec.EnvVar.From), stored on DesiredService.
// DatabaseEnv. Database is a desired_databases.name; Field is one of
// "url", "host", "port", "username", "password", "database", resolved by
// the application controller (resolveDatabaseEnv) immediately before
// container creation, the same "declaration only, resolved later" split
// SecretEnv already makes for { secret: true }.
type DatabaseEnvRef struct {
	Database string `json:"database"`
	Field    string `json:"field"`
}

// VaultEnvRef is one env var's parsed { vault: { path, key } } reference
// (internal/spec.EnvVar.Vault), stored on DesiredService.VaultEnv. Path
// is a KV v2 secret path in the operator's own Vault instance (see
// VaultSettings); Key is the field name inside that secret's data.
// Resolved fresh from a live Vault read by the application controller
// immediately before container creation, the same "declaration only,
// resolved later" split DatabaseEnvRef already makes.
type VaultEnvRef struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

// SecretEnvRef is one env var's { secret: true } declaration
// (internal/spec.EnvVar), stored on DesiredService.SecretEnv. Name is
// the secret storage key (internal/secrets), resolved and decrypted by
// the application controller immediately before container creation, the
// same "declaration only, resolved later" split DatabaseEnvRef/VaultEnvRef
// already make. Required mirrors app.yaml's own { required: true } flag:
// carrying it through here is what lets a build-triggered deploy
// (internal/api/builds.go's specServiceFromDesired, feeding
// internal/deploy.Pipeline's validateEnv) reject a still-unset required
// secret the same way a fresh app.yaml parse already does on the
// webhook path, instead of silently deploying without it.
type SecretEnvRef struct {
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
}

// DatabaseAttachment is which managed database (desired_databases.name)
// an app resolves one connection env var from, set through PUT/DELETE
// /api/v1/apps/{name}/database rather than app.yaml: the UI/CLI-facing
// equivalent of DatabaseEnv above, for an app that was created directly
// (build.type: image via the API) rather than deployed from a spec file.
// Exactly one per app, mirroring StorageTargetID's single-attachment
// shape rather than DatabaseEnv's open-ended map.
type DatabaseAttachment struct {
	DatabaseName string
	EnvVar       string
	Field        string
}

// ServiceVolume is one named Docker volume an application service's
// container mounts, the same shape internal/docker.VolumeMount already
// has for the database controller's own (single, fixed-path) volume.
// Name is this platform's own volume name (already scoped/prefixed, see
// internal/reconcile/application's own volume-naming helper), not
// whatever a compose file's own top-level volumes: key called it.
type ServiceVolume struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
}

// ServiceBindMount is one host directory an application service's
// container mounts directly, distinct from ServiceVolume (a Docker-
// managed named volume): HostPath is a real path on whichever node the
// service runs on, not a Docker volume name. internal/compose is the
// only producer today (compose.go's own doc comment on volumes:'s
// bind-mount short form); internal/api gates persisting any non-empty
// BindMounts to AbilityRoot callers, the same tier POST /apps/{name}/exec
// sits behind, since a bind mount is a real host-filesystem access
// capability. Kept as its own type rather than an optional HostPath
// field on ServiceVolume so a bind mount and a named volume are never
// ambiguous in code or storage.
type ServiceBindMount struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
}

// DesiredService is what the application controller reconciles running
// containers against: an already-resolved image plus what it needs to
// run. See the migration comment in migrations/0002_desired_services.sql
// for why this is a distinct type from internal/spec.Service rather than
// reusing it directly.
type DesiredService struct {
	Name  string
	Image string
	Port  int
	// HostPort pins the host-side port Docker binds Port to
	// (internal/docker.PortBinding.HostPort), migrations/0056's own
	// operator-facing counterpart to Port itself. nil means "let Docker
	// assign one" (today's only behavior, and the ordinary case), the
	// same "an ordinary desired-state field, written on every
	// SaveDesiredService call" treatment Port itself already gets, not
	// a dedicated setter: an app.yaml redeploy or an API update is meant
	// to be able to change or clear a pin the same way it already
	// changes Port.
	HostPort *int
	// BindAddress picks which network interface Port (and HostPort, if
	// pinned) binds to on the host (migrations/0103_service_bind_address.sql):
	// "private", "public", or a literal IP, see internal/bindaddr.Resolve.
	// Unlike HostPort, this is never empty after SaveDesiredService: an
	// unset value resolves to DefaultBindAddress there, the same
	// resolve-and-always-persist-concrete treatment Strategy/Replicas
	// already get.
	BindAddress string
	Domains     []string
	Env         map[string]string
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

	// Egress is this service's outbound network allowlist
	// (internal/spec.Service.Egress), nil meaning unrestricted egress,
	// today's behavior. A normal deploy-time field written on every
	// SaveDesiredService call, same shape Resources/Health/Hooks already
	// follow: an app.yaml redeploy that drops the egress: block clears it
	// here too, the same declarative "this call replaces the whole
	// record" contract every other field in this group already has.
	// UpdateServiceEgressPolicy additionally lets PUT/DELETE
	// /api/v1/apps/{name}/egress-policy set this outside a redeploy,
	// mirroring VaultEnv's own dual write path (SetServiceVaultEnvVar).
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

	// Labels are arbitrary operator-supplied Docker labels applied to the
	// service's container at create time (internal/spec.Service.Labels'
	// storage home, migrations/0027_service_labels.sql). Already
	// validated by the time a DesiredService exists: internal/spec.
	// ValidateLabels runs at every input boundary (app.yaml parsing,
	// the HTTP API's validateAppResource), so this package and
	// internal/docker both trust it's collision-free with this
	// platform's own reserved label namespace by the time it gets here,
	// the same "resolve/validate once, store the resolved form" shape
	// Resources/Health already follow.
	Labels map[string]string

	// NodeID is which node (internal/store's own nodes table)
	// this service should run on. Empty string is the explicit
	// "this control plane's own local node" value (the placement
	// migration comment explains why that's not NULL or a foreign key),
	// the only value that existed before this field did, so an existing
	// single-node deployment's services keep running exactly where they
	// already were on upgrade.
	NodeID string

	// Strategy and Replicas are always the *resolved* (never empty/zero)
	// values: internal/spec.Service.EffectiveStrategy()/
	// EffectiveReplicas() already define what "unset in app.yaml" means,
	// and this package stores that already-resolved decision rather than
	// re-deriving it on every read, the same "resolve once, store the
	// resolved form" shape Resources/Health already follow (app.yaml's
	// human units are converted before a DesiredService exists at all).
	// SaveDesiredService independently defends against an empty
	// Strategy/zero Replicas from any caller, so this field is never
	// ambiguous regardless of who constructs the struct.
	Strategy string
	Replicas int

	// RestartNonce is opaque and only ever compared for equality, never
	// interpreted: internal/reconcile/application.ContainerName folds it
	// into the container name hash (only when non-empty, so a service
	// that has never been restarted keeps the exact container name it
	// always has). Changing it is therefore indistinguishable, from the
	// reconciler's level-triggered point of view, from an image change:
	// the existing blue-green/recreate cutover logic already handles it
	// correctly with no new branch. Like NodeID, this is deliberately
	// excluded from SaveDesiredService's full-record-replace semantics;
	// only RestartService writes it, see that method's own doc comment.
	RestartNonce string

	// ProjectID is which store.Project (migrations/0022_projects.sql)
	// this service is organizationally grouped under; empty string is
	// "no project", a real, permanent, equally-valid state, not merely
	// "unset" (see that migration's own comment on why the column is
	// genuinely nullable rather than reusing NodeID's NOT NULL DEFAULT
	// '' convention). Like NodeID and RestartNonce, SaveDesiredService
	// never writes this field, on either an INSERT or an UPDATE: only
	// UpdateServiceProject does, see that method's own doc comment for
	// why, and internal/api/apps.go's handleCreateApp for how a create-
	// time project choice still reaches it without this method's
	// full-replace semantics being allowed to silently move an
	// already-placed service between projects.
	ProjectID string

	// EnvironmentID is which store.Environment (migrations/0054) this
	// service is tagged with; empty is "no environment", set only via
	// SetServiceEnvironment, same non-full-replace shape as ProjectID.
	EnvironmentID string

	// StorageTargetID is which store.BackupTarget (migrations/0018_backup_targets.sql)
	// this app's own object-storage credentials resolve from
	// (migrations/0030_service_storage_target.sql): the same bucket
	// connection an operator may already use for a database's scheduled
	// backups, reused rather than duplicated as a second "storage
	// target" concept. Empty string is "no storage attached", a real,
	// permanent, equally-valid state, not merely "unset" (SQL NULL,
	// mirroring ProjectID's own empty-string-means-NULL convention just
	// above, for the identical reasoning: unlike NodeID, there is no
	// meaningful non-empty default this could fall back to). Like
	// NodeID/ProjectID/RestartNonce, SaveDesiredService never writes this
	// field, on either an INSERT or an UPDATE: only
	// UpdateServiceStorageTarget does, see that method's own doc comment
	// for why. internal/reconcile/application's controller resolves the
	// target's Endpoint/Region/Bucket plus its credentials (internal/secrets,
	// store.BackupTargetSecretsKey) into env vars at container-create
	// time when this is non-empty.
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

	// AppID is which store.App (migrations/0039_apps.sql) this service
	// belongs to; empty string means none. SaveDesiredService only
	// writes it on first INSERT, never on an ON CONFLICT update, the
	// same "an ordinary edit must never silently move it" invariant
	// NodeID/ProjectID/StorageTargetID/Suspended already have (via
	// their own dedicated Update* setters); a service's AppID is fixed
	// at creation.
	AppID string

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

	// ExecEnabled gates POST /apps/{name}/exec and GET
	// /apps/{name}/terminal (migrations/0112_service_exec_enabled.sql,
	// internal/api/exec.go and terminal.go): both check this in addition
	// to the existing AbilityRoot IAM check, so an operator can lock a
	// specific app's shell access even for a token that otherwise has
	// full IAM abilities. Default true, unlike AutoRollbackOnCrashloop:
	// exec is available today with no equivalent gate, so this preserves
	// existing behavior for every app until someone explicitly disables
	// it. Like LogDrain/PreviewEnvOverrides/AutoRollbackOnCrashloop,
	// SaveDesiredService never writes this field: only
	// SetServiceExecEnabled does.
	ExecEnabled bool
}

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

// ErrDomainTaken is returned by SaveDesiredService when one of svc's
// Domains is already claimed by a different service. Enforced here, not
// only by internal/spec's Validate() (which only ever sees one app.yaml
// at a time and so cannot see a domain already claimed by an earlier,
// separate deploy), because internal/reconcile/ingress's controller
// builds one Caddy route per domain from every desired service in a
// single reconcile pass: two services claiming the same
// host would silently produce two routes matching the same Host header,
// with whichever sorted last winning inside Caddy's own matcher
// evaluation and silently shadowing the other. See that controller's
// package doc comment and migrations/0011_service_domains.sql.
type ErrDomainTaken struct {
	Domain string
	Owner  string
}

func (e *ErrDomainTaken) Error() string {
	return fmt.Sprintf("store: domain %q is already in use by service %q", e.Domain, e.Owner)
}

// SaveDesiredService creates or fully replaces the desired state for a
// named service. There's no partial update: a service's desired state is
// always written as a whole record, matching how it'll actually be
// produced (a deploy pipeline resolving a complete DesiredService from
// one app.yaml service block, not assembling one field at a time).
//
// One deliberate exception: NodeID is never written by this method,
// only ever by UpdateServiceNode below. internal/deploy.Pipeline calls
// this on every ordinary redeploy without ever setting NodeID (it has
// no opinion on placement), and svc.NodeID passed in here is always
// silently ignored, not just on an update but even on the very first
// INSERT: a new service starts on the local node ("") until an operator
// explicitly places it elsewhere via UpdateServiceNode, and a redeploy
// of an already-placed service must never un-assign it from wherever
// that placement decision put it.
//
// The whole write, including reconciling svc.Domains against the
// service_domains table (0011), happens in one transaction: either the
// service's desired state and its domain claims both land, or neither
// does. Returns *ErrDomainTaken (via errors.As) if any domain in
// svc.Domains is already claimed by a different service; the caller's
// entire desired-state write is rejected in that case, not partially
// applied with some domains silently dropped.
func (db *DB) SaveDesiredService(ctx context.Context, svc DesiredService) error {
	domainsJSON, err := json.Marshal(nonNilSlice(svc.Domains))
	if err != nil {
		return fmt.Errorf("store: marshal domains for service %q: %w", svc.Name, err)
	}
	envJSON, err := json.Marshal(nonNilMap(svc.Env))
	if err != nil {
		return fmt.Errorf("store: marshal env for service %q: %w", svc.Name, err)
	}
	commandJSON, err := json.Marshal(nonNilSlice(svc.Command))
	if err != nil {
		return fmt.Errorf("store: marshal command for service %q: %w", svc.Name, err)
	}
	entrypointJSON, err := json.Marshal(nonNilSlice(svc.Entrypoint))
	if err != nil {
		return fmt.Errorf("store: marshal entrypoint for service %q: %w", svc.Name, err)
	}
	secretEnvJSON, err := json.Marshal(nonNilSecretEnv(svc.SecretEnv))
	if err != nil {
		return fmt.Errorf("store: marshal secret_env for service %q: %w", svc.Name, err)
	}
	databaseEnv := svc.DatabaseEnv
	if databaseEnv == nil {
		databaseEnv = map[string]DatabaseEnvRef{}
	}
	databaseEnvJSON, err := json.Marshal(databaseEnv)
	if err != nil {
		return fmt.Errorf("store: marshal database_env for service %q: %w", svc.Name, err)
	}
	vaultEnv := svc.VaultEnv
	if vaultEnv == nil {
		vaultEnv = map[string]VaultEnvRef{}
	}
	vaultEnvJSON, err := json.Marshal(vaultEnv)
	if err != nil {
		return fmt.Errorf("store: marshal vault_env for service %q: %w", svc.Name, err)
	}
	resourcesJSON, err := json.Marshal(svc.Resources)
	if err != nil {
		return fmt.Errorf("store: marshal resources for service %q: %w", svc.Name, err)
	}
	healthJSON, err := json.Marshal(svc.Health)
	if err != nil {
		return fmt.Errorf("store: marshal health for service %q: %w", svc.Name, err)
	}
	hooksJSON, err := json.Marshal(svc.Hooks)
	if err != nil {
		return fmt.Errorf("store: marshal hooks for service %q: %w", svc.Name, err)
	}
	egressJSON, err := json.Marshal(svc.Egress)
	if err != nil {
		return fmt.Errorf("store: marshal egress_policy for service %q: %w", svc.Name, err)
	}
	labelsJSON, err := json.Marshal(nonNilMap(svc.Labels))
	if err != nil {
		return fmt.Errorf("store: marshal labels for service %q: %w", svc.Name, err)
	}
	volumes := svc.Volumes
	if volumes == nil {
		volumes = []ServiceVolume{}
	}
	volumesJSON, err := json.Marshal(volumes)
	if err != nil {
		return fmt.Errorf("store: marshal volumes for service %q: %w", svc.Name, err)
	}
	bindMounts := svc.BindMounts
	if bindMounts == nil {
		bindMounts = []ServiceBindMount{}
	}
	bindMountsJSON, err := json.Marshal(bindMounts)
	if err != nil {
		return fmt.Errorf("store: marshal bind_mounts for service %q: %w", svc.Name, err)
	}

	strategy := svc.Strategy
	if strategy == "" {
		strategy = DefaultDeployStrategy
	}
	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = DefaultReplicas
	}
	bindAddress := svc.BindAddress
	if bindAddress == "" {
		bindAddress = DefaultBindAddress
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save desired service %q: begin transaction: %w", svc.Name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO desired_services (name, image, port, host_port, bind_address, domains, env, command, entrypoint, secret_env, env_dirty, database_env, vault_env, resources, health, hooks, egress_policy, node_id, strategy, replicas, labels, volumes, bind_mounts, registry_credential_id, project_id, environment_id, app_id, pull_policy, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			image = excluded.image,
			port = excluded.port,
			host_port = excluded.host_port,
			bind_address = excluded.bind_address,
			domains = excluded.domains,
			env = excluded.env,
			command = excluded.command,
			entrypoint = excluded.entrypoint,
			secret_env = excluded.secret_env,
			env_dirty = excluded.env_dirty,
			database_env = excluded.database_env,
			vault_env = excluded.vault_env,
			resources = excluded.resources,
			health = excluded.health,
			hooks = excluded.hooks,
			egress_policy = excluded.egress_policy,
			strategy = excluded.strategy,
			replicas = excluded.replicas,
			labels = excluded.labels,
			volumes = excluded.volumes,
			bind_mounts = excluded.bind_mounts,
			registry_credential_id = excluded.registry_credential_id,
			pull_policy = excluded.pull_policy,
			updated_at = excluded.updated_at
	`, svc.Name, svc.Image, svc.Port, hostPortToNull(svc.HostPort), bindAddress, string(domainsJSON), string(envJSON), string(commandJSON), string(entrypointJSON), string(secretEnvJSON), svc.EnvDirty, string(databaseEnvJSON), string(vaultEnvJSON), string(resourcesJSON), string(healthJSON), string(hooksJSON), string(egressJSON), strategy, replicas, string(labelsJSON), string(volumesJSON), string(bindMountsJSON), svc.RegistryCredentialID, sql.NullString{String: svc.AppID, Valid: svc.AppID != ""}, svc.PullPolicy)
	if err != nil {
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}

	if err := claimServiceDomains(ctx, tx, svc.Name, svc.Domains); err != nil {
		// Wrapped with %w, not returned bare: still findable via
		// errors.As for a *ErrDomainTaken caller that wants the
		// specific domain/owner, per this method's error-wrapping
		// convention and the house rule that errors are wrapped with
		// context at every layer boundary.
		return fmt.Errorf("store: save desired service %q: %w", svc.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save desired service %q: commit: %w", svc.Name, err)
	}
	return nil
}

// claimServiceDomains replaces serviceName's rows in service_domains
// with exactly domains: every domain serviceName previously claimed but
// no longer declares is released, and every domain it now declares is
// claimed, unless a different service or static site already claims it
// (domainOwner, migrations/0015, checks both service_domains and
// static_site_domains), in which case this returns *ErrDomainTaken and
// the caller's whole transaction rolls back (claimServiceDomains never
// partially claims a service's domain list). Duplicate entries within
// domains are claimed once, not reported as a conflict against
// themselves.
func claimServiceDomains(ctx context.Context, tx *sql.Tx, serviceName string, domains []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_domains WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("clear existing domain claims: %w", err)
	}

	claimed := make(map[string]bool, len(domains))
	for _, domain := range domains {
		if domain == "" || claimed[domain] {
			continue
		}
		claimed[domain] = true

		owner, found, err := domainOwner(ctx, tx, domain)
		if err != nil {
			return fmt.Errorf("check domain %q availability: %w", domain, err)
		}
		if found {
			return &ErrDomainTaken{Domain: domain, Owner: owner}
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO service_domains (domain, service_name) VALUES (?, ?)
		`, domain, serviceName); err != nil {
			return fmt.Errorf("claim domain %q: %w", domain, err)
		}
	}
	return nil
}

// UpdateServiceNode reassigns svc to run on nodeID ("" for this control
// plane's own local node, per the placement migration's comment), the
// only way node_id ever changes: SaveDesiredService's own doc comment
// explains why it's deliberately excluded from that method's
// full-record-replace semantics.
func (db *DB) UpdateServiceNode(ctx context.Context, name, nodeID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET node_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nodeID, name)
	if err != nil {
		return fmt.Errorf("store: update node for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update node for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceProject reassigns svc to project projectID ("" for "no
// project", the same real-and-permanent sentinel DesiredService.
// ProjectID's own doc comment describes), the only way project_id ever
// changes: SaveDesiredService's own doc comment explains why it's
// deliberately excluded from that method's full-record-replace
// semantics, the same reasoning UpdateServiceNode already establishes
// for NodeID. An empty projectID is written as SQL NULL, not the empty
// string node_id uses, because unlike node_id (NOT NULL DEFAULT ”),
// this column is genuinely nullable (migrations/0022_projects.sql's own
// comment on why).
func (db *DB) UpdateServiceProject(ctx context.Context, name, projectID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET project_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: projectID, Valid: projectID != ""}, name)
	if err != nil {
		return fmt.Errorf("store: update project for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update project for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceStorageTarget reassigns svc's object-storage credential
// source to storageTargetID ("" for "no storage attached"), the only way
// storage_target_id ever changes: SaveDesiredService's own doc comment
// explains why it's deliberately excluded from that method's
// full-record-replace semantics, the same reasoning UpdateServiceNode/
// UpdateServiceProject already establish for their own single-purpose
// updates. An empty storageTargetID is written as SQL NULL, not the
// empty string node_id uses, the same reasoning UpdateServiceProject's
// own doc comment gives: this column is genuinely nullable, mirroring
// backup_target_id's own convention on desired_databases
// (migrations/0023_scheduled_backups.sql) rather than node_id's.
//
// Deliberately does not validate storageTargetID against backup_targets
// itself: that check belongs to the caller (internal/api's
// handleSetAppStorage), the same "own endpoint validates, this method
// just writes" boundary UpdateServiceProject leaves to
// handleSetAppProject's own validateProjectID call.
func (db *DB) UpdateServiceStorageTarget(ctx context.Context, name, storageTargetID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET storage_target_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: storageTargetID, Valid: storageTargetID != ""}, name)
	if err != nil {
		return fmt.Errorf("store: update storage target for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update storage target for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceEgressPolicy replaces svc's egress allowlist as a whole
// (policy nil clears it, back to unrestricted egress), the narrow,
// full-column write PUT/DELETE /api/v1/apps/{name}/egress-policy needs
// without going through SaveDesiredService's full-record-replace, the
// same "own endpoint, own narrow update" shape UpdateServiceStorageTarget
// already establishes. Unlike UpdateServiceStorageTarget's single scalar,
// egress_policy is a JSON blob (like health/hooks), so this marshals the
// whole policy rather than passing a bare column value.
func (db *DB) UpdateServiceEgressPolicy(ctx context.Context, name string, policy *ServiceEgressPolicy) error {
	egressJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("store: update egress policy for service %q: marshal: %w", name, err)
	}
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET egress_policy = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(egressJSON), name)
	if err != nil {
		return fmt.Errorf("store: update egress policy for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update egress policy for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceDatabaseAttachment is DesiredService.DatabaseAttachment's
// only writer, the same separation-from-ordinary-update reasoning
// UpdateServiceStorageTarget already establishes for StorageTargetID.
// att nil clears the attachment (all three columns back to ”).
func (db *DB) UpdateServiceDatabaseAttachment(ctx context.Context, name string, att *DatabaseAttachment) error {
	var dbName, envVar, field string
	if att != nil {
		dbName, envVar, field = att.DatabaseName, att.EnvVar, att.Field
	}
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services
		SET database_attachment_name = ?, database_attachment_env_var = ?, database_attachment_field = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE name = ?
	`, dbName, envVar, field, name)
	if err != nil {
		return fmt.Errorf("store: update database attachment for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update database attachment for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// SetServiceVaultEnvVar adds or replaces (ref non-nil) or removes (ref
// nil) exactly one entry in name's VaultEnv map, the narrow single-key
// mutation PUT/DELETE /api/v1/apps/{name}/vault-env/{key} needs. Unlike
// SaveDesiredService's own full-record-replace semantics, this never
// touches any other field: VaultEnv is a JSON blob (like SecretEnv,
// DatabaseEnv), not one column per key, so adding or removing a single
// entry needs a read-modify-write, done here inside one transaction so a
// concurrent call for a different key on the same service can never
// silently lose the other's write.
func (db *DB) SetServiceVaultEnvVar(ctx context.Context, name, envVar string, ref *VaultEnvRef) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var vaultEnvJSON string
	err = tx.QueryRowContext(ctx, `SELECT vault_env FROM desired_services WHERE name = ?`, name).Scan(&vaultEnvJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: read existing: %w", name, err)
	}

	vaultEnv := map[string]VaultEnvRef{}
	if vaultEnvJSON != "" {
		if err := json.Unmarshal([]byte(vaultEnvJSON), &vaultEnv); err != nil {
			return fmt.Errorf("store: set vault env var for service %q: decode existing: %w", name, err)
		}
	}
	if ref == nil {
		delete(vaultEnv, envVar)
	} else {
		vaultEnv[envVar] = *ref
	}

	updated, err := json.Marshal(vaultEnv)
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: encode: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE desired_services SET vault_env = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(updated), name); err != nil {
		return fmt.Errorf("store: set vault env var for service %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set vault env var for service %q: commit: %w", name, err)
	}
	return nil
}

// UpdateServiceApp assigns svc to app appID, the only way
// desired_services.app_id ever changes outside migrations/0039_apps.sql's
// own backfill: SaveDesiredService's own doc comment explains why this
// is deliberately excluded from that method's full-record-replace
// semantics, the same reasoning UpdateServiceNode/UpdateServiceProject
// already establish for NodeID/ProjectID.
func (db *DB) UpdateServiceApp(ctx context.Context, name, appID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET app_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, appID, name)
	if err != nil {
		return fmt.Errorf("store: update app for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update app for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceSuspended is the only way suspended ever changes, the
// same "own single-purpose setter, excluded from SaveDesiredService"
// reasoning UpdateServiceNode/UpdateServiceProject/
// UpdateServiceStorageTarget already establish. Setting it true does
// not by itself stop any container: internal/reconcile/application's
// controller is what converges to zero containers once it observes
// Suspended on its next reconcile, the same level-triggered separation
// DeleteDesiredService's own doc comment describes for delete.
func (db *DB) UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error {
	query := `UPDATE desired_services SET suspended = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`
	if !suspended {
		// Resuming clears env_dirty too: removeStale tore down every
		// replica while suspended, so the next reconcile creates brand
		// new containers from current desired state, the same "fresh
		// container creation" moment RestartService already clears it on.
		query = `UPDATE desired_services SET suspended = ?, env_dirty = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`
	}
	res, err := db.ExecContext(ctx, query, suspended, name)
	if err != nil {
		return fmt.Errorf("store: update suspended for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update suspended for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// SetServiceAutoRollbackOnCrashloop is the only way
// auto_rollback_on_crashloop ever changes, the same "own single-purpose
// setter, excluded from SaveDesiredService" reasoning UpdateServiceNode/
// UpdateServiceProject/UpdateServiceStorageTarget/UpdateServiceSuspended
// already establish.
func (db *DB) SetServiceAutoRollbackOnCrashloop(ctx context.Context, name string, enabled bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET auto_rollback_on_crashloop = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
	if err != nil {
		return fmt.Errorf("store: update auto rollback on crashloop for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update auto rollback on crashloop for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// SetServiceExecEnabled is the only way exec_enabled ever changes, the
// same "own single-purpose setter, excluded from SaveDesiredService"
// reasoning SetServiceAutoRollbackOnCrashloop and
// UpdateServiceNode/UpdateServiceProject/UpdateServiceStorageTarget/
// UpdateServiceSuspended already establish.
func (db *DB) SetServiceExecEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET exec_enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
	if err != nil {
		return fmt.Errorf("store: update exec enabled for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update exec enabled for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// RestartService is the only way restart_nonce ever changes: SaveDesiredService's
// own doc comment (and this field's own doc comment on DesiredService)
// explains why it's deliberately excluded from that method's
// full-record-replace semantics, the same reasoning UpdateServiceNode
// already establishes for NodeID.
//
// The generated value is opaque and only ever compared for equality by
// internal/reconcile/application.ContainerName, never interpreted, so a
// short random hex string is enough; it does not need to be
// cryptographically unpredictable the way a session token or API key
// does, only different from whatever was there before.
func (db *DB) RestartService(ctx context.Context, name string) error {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("store: restart service %q: generate nonce: %w", name, err)
	}
	nonce := hex.EncodeToString(buf)

	// Also clears env_dirty: a restart recreates the container from
	// current desired state, the same "fresh container creation" moment
	// DesiredService.EnvDirty's own doc comment says clears it.
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET restart_nonce = ?, env_dirty = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nonce, name)
	if err != nil {
		return fmt.Errorf("store: restart service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: restart service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// ErrServiceNotFound is returned by GetDesiredService when no service
// has that name.
var ErrServiceNotFound = errors.New("store: service not found")

// ErrServiceVolumeNotFound is returned by ResolveServiceVolumeDockerName
// when serviceName has no volume by that logical name.
var ErrServiceVolumeNotFound = errors.New("store: service volume not found")

// ServiceVolumeDockerName finds svc's real Docker volume whose logical
// name (spec.Volume.Name, what an operator wrote in app.yaml) is
// volumeName, deriving it back from ServiceVolume.Name by stripping the
// fixed prefix internal/deploy's own volumeName() always applies
// ("app-"+svc.Name+"-"): ServiceVolume itself only stores the already-
// resolved Docker volume name (this struct's own doc comment), not the
// logical one it came from, so this is the one place that direction gets
// reversed. Exact-prefix stripping, not a generic split, so a logical
// name that itself contains "-" is never misparsed.
func ServiceVolumeDockerName(svc DesiredService, volumeName string) (string, bool) {
	prefix := "app-" + svc.Name + "-"
	for _, v := range svc.Volumes {
		if strings.TrimPrefix(v.Name, prefix) == volumeName {
			return v.Name, true
		}
	}
	return "", false
}

// ResolveServiceVolumeDockerName looks up serviceName and resolves
// volumeName to its real Docker volume name, the single DB-round-trip
// convenience ServiceVolumeDockerName's own callers that don't already
// have a DesiredService in hand need (internal/backup.Scheduler's own
// per-tick volume evaluation).
func (db *DB) ResolveServiceVolumeDockerName(ctx context.Context, serviceName, volumeName string) (string, error) {
	svc, err := db.GetDesiredService(ctx, serviceName)
	if err != nil {
		return "", err
	}
	dockerName, ok := ServiceVolumeDockerName(*svc, volumeName)
	if !ok {
		return "", ErrServiceVolumeNotFound
	}
	return dockerName, nil
}

// GetDesiredService returns the desired state for name, or
// ErrServiceNotFound if no such service has been saved.
func (db *DB) GetDesiredService(ctx context.Context, name string) (*DesiredService, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE name = ?
	`, name)

	svc, err := scanDesiredService(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get desired service %q: %w", name, err)
	}
	return svc, nil
}

// ListDesiredServices returns every saved service, ordered by name.
func (db *DB) ListDesiredServices(ctx context.Context) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredService
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired service rows: %w", err)
	}
	return out, nil
}

// ListDesiredServicesByNode returns every saved service currently
// placed on nodeID, ordered by name. Node drain
// (internal/api's handleDrainNode) uses this to find what to move off a
// node before it's removed, and handleDeleteNode uses it as the guard
// that makes node deletion refuse to run while placements remain.
// nodeID="" (the local-node sentinel, see DesiredService.NodeID's own
// doc comment) is a valid argument, matching every other node_id
// comparison in this package.
func (db *DB) ListDesiredServicesByNode(ctx context.Context, nodeID string) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE node_id = ?
		ORDER BY name
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for node %q: %w", nodeID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredService
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired service rows: %w", err)
	}
	return out, nil
}

// ListDesiredServicesByProject returns every saved service filed under
// projectID, ordered by name, the project-kind counterpart to
// ListDesiredServicesByNode. Used by handleStopProject/handleStartProject
// (internal/api/project_stop_start.go) and internal/api's bulk-restart
// endpoint to find every app in a project without listing every service.
func (db *DB) ListDesiredServicesByProject(ctx context.Context, projectID string) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE project_id = ?
		ORDER BY name
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired services for project %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredService
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired service rows: %w", err)
	}
	return out, nil
}

// DeleteDesiredService removes a service's desired state, e.g. because
// the app was deleted through the HTTP API. It returns
// ErrServiceNotFound if no such service exists, the same sentinel
// GetDesiredService uses, so callers handle "not found" one way
// regardless of which method produced it.
//
// Deleting desired state does not, by itself, stop or remove any
// container currently running for this service: as of this writing the
// application controller (internal/reconcile/application) treats a
// missing desired service as "nothing to do yet" (NoDesiredState), not
// "tear down what's running". Making delete actually converge to zero
// containers is a reconciler change, not a store one, and is a known gap
// left for that package.
func (db *DB) DeleteDesiredService(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM desired_services WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete desired service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete desired service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// desiredServiceColumns is the column list every desired_services SELECT
// in this package shares, kept in one place so scanDesiredService's
// destination order and each query's column order can never drift apart.
const desiredServiceColumns = "name, image, port, host_port, bind_address, domains, env, secret_env, env_dirty, database_env, vault_env, resources, health, hooks, egress_policy, node_id, strategy, replicas, restart_nonce, project_id, labels, storage_target_id, suspended, app_id, volumes, registry_credential_id, database_attachment_name, database_attachment_env_var, database_attachment_field, log_drain, environment_id, command, bind_mounts, entrypoint, pull_policy, preview_env_overrides, auto_rollback_on_crashloop, exec_enabled"

// scanDesiredService reads the column shape both GetDesiredService
// and ListDesiredServices query, via either row.Scan or rows.Scan (same
// signature), so the decode-JSON-columns logic exists exactly once.
func scanDesiredService(scan func(dest ...any) error) (*DesiredService, error) {
	var (
		svc                                                                                                                                                                                 DesiredService
		domainsJSON, envJSON, secretEnvJSON, databaseEnvJSON, vaultEnvJSON, resourcesJSON, health, hooks, egress, labels, volumes, command, bindMounts, entrypoint, previewEnvOverridesJSON string
		projectID, storageTargetID, appID, logDrainJSON, environmentID                                                                                                                      sql.NullString
		hostPort                                                                                                                                                                            sql.NullInt64
		dbAttachmentName, dbAttachmentEnvVar, dbAttachmentField                                                                                                                             string
	)
	if err := scan(&svc.Name, &svc.Image, &svc.Port, &hostPort, &svc.BindAddress, &domainsJSON, &envJSON, &secretEnvJSON, &svc.EnvDirty, &databaseEnvJSON, &vaultEnvJSON, &resourcesJSON, &health, &hooks, &egress, &svc.NodeID, &svc.Strategy, &svc.Replicas, &svc.RestartNonce, &projectID, &labels, &storageTargetID, &svc.Suspended, &appID, &volumes, &svc.RegistryCredentialID, &dbAttachmentName, &dbAttachmentEnvVar, &dbAttachmentField, &logDrainJSON, &environmentID, &command, &bindMounts, &entrypoint, &svc.PullPolicy, &previewEnvOverridesJSON, &svc.AutoRollbackOnCrashloop, &svc.ExecEnabled); err != nil {
		return nil, err
	}
	svc.ProjectID = projectID.String
	svc.StorageTargetID = storageTargetID.String
	svc.AppID = appID.String
	svc.EnvironmentID = environmentID.String
	if hostPort.Valid {
		v := int(hostPort.Int64)
		svc.HostPort = &v
	}
	if dbAttachmentName != "" {
		svc.DatabaseAttachment = &DatabaseAttachment{DatabaseName: dbAttachmentName, EnvVar: dbAttachmentEnvVar, Field: dbAttachmentField}
	}

	if err := json.Unmarshal([]byte(domainsJSON), &svc.Domains); err != nil {
		return nil, fmt.Errorf("unmarshal domains: %w", err)
	}
	if err := json.Unmarshal([]byte(envJSON), &svc.Env); err != nil {
		return nil, fmt.Errorf("unmarshal env: %w", err)
	}
	secretEnv, err := unmarshalSecretEnv(secretEnvJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal secret_env: %w", err)
	}
	svc.SecretEnv = secretEnv
	if err := json.Unmarshal([]byte(databaseEnvJSON), &svc.DatabaseEnv); err != nil {
		return nil, fmt.Errorf("unmarshal database_env: %w", err)
	}
	if err := json.Unmarshal([]byte(vaultEnvJSON), &svc.VaultEnv); err != nil {
		return nil, fmt.Errorf("unmarshal vault_env: %w", err)
	}
	if resourcesJSON != "null" {
		if err := json.Unmarshal([]byte(resourcesJSON), &svc.Resources); err != nil {
			return nil, fmt.Errorf("unmarshal resources: %w", err)
		}
	}
	if health != "null" {
		if err := json.Unmarshal([]byte(health), &svc.Health); err != nil {
			return nil, fmt.Errorf("unmarshal health: %w", err)
		}
	}
	if hooks != "null" {
		if err := json.Unmarshal([]byte(hooks), &svc.Hooks); err != nil {
			return nil, fmt.Errorf("unmarshal hooks: %w", err)
		}
	}
	if egress != "null" {
		if err := json.Unmarshal([]byte(egress), &svc.Egress); err != nil {
			return nil, fmt.Errorf("unmarshal egress_policy: %w", err)
		}
	}
	if err := json.Unmarshal([]byte(labels), &svc.Labels); err != nil {
		return nil, fmt.Errorf("unmarshal labels: %w", err)
	}
	if err := json.Unmarshal([]byte(volumes), &svc.Volumes); err != nil {
		return nil, fmt.Errorf("unmarshal volumes: %w", err)
	}
	if err := json.Unmarshal([]byte(command), &svc.Command); err != nil {
		return nil, fmt.Errorf("unmarshal command: %w", err)
	}
	if err := json.Unmarshal([]byte(bindMounts), &svc.BindMounts); err != nil {
		return nil, fmt.Errorf("unmarshal bind_mounts: %w", err)
	}
	if err := json.Unmarshal([]byte(entrypoint), &svc.Entrypoint); err != nil {
		return nil, fmt.Errorf("unmarshal entrypoint: %w", err)
	}
	if err := json.Unmarshal([]byte(previewEnvOverridesJSON), &svc.PreviewEnvOverrides); err != nil {
		return nil, fmt.Errorf("unmarshal preview_env_overrides: %w", err)
	}
	if logDrainJSON.Valid {
		if err := json.Unmarshal([]byte(logDrainJSON.String), &svc.LogDrain); err != nil {
			return nil, fmt.Errorf("unmarshal log_drain: %w", err)
		}
	}

	return &svc, nil
}

// nonNilMap makes sure an unset Env marshals to "{}", not the JSON null
// a nil Go map produces, so GetDesiredService always round-trips into a
// non-nil (if possibly empty) map, one less nil-check for every caller.
func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// nonNilSlice is nonNilMap's counterpart for Domains: an unset slice
// marshals to "[]", not JSON null.
func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// nonNilSecretEnv is nonNilSlice's counterpart for SecretEnv, which
// can't share that generic-less helper since it holds SecretEnvRef, not
// string.
func nonNilSecretEnv(s []SecretEnvRef) []SecretEnvRef {
	if s == nil {
		return []SecretEnvRef{}
	}
	return s
}

// unmarshalSecretEnv decodes secret_env's JSON, accepting both the
// current { name, required } object shape and the plain string-name
// array shape every row written before Required existed still has on
// disk (secret_env is a JSON blob column, migrations/0006: reshaping it
// is a decode-time concern, not a SQL migration). An old row decodes
// with Required defaulting to false for each name, the same permissive
// direction a declared-but-unresolvable secret already falls back to
// elsewhere in this codebase.
func unmarshalSecretEnv(raw string) ([]SecretEnvRef, error) {
	var refs []SecretEnvRef
	if err := json.Unmarshal([]byte(raw), &refs); err == nil {
		return refs, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	refs = make([]SecretEnvRef, len(names))
	for i, n := range names {
		refs[i] = SecretEnvRef{Name: n}
	}
	return refs, nil
}

// SecretEnvRefsFromNames builds []SecretEnvRef for names with Required
// false for each: the shape a caller that only ever tracks secret-backed
// env var names by name needs (e.g. the direct app API and Docker
// Compose imports, neither of which has an app.yaml { required: true }
// flag to carry over).
func SecretEnvRefsFromNames(names []string) []SecretEnvRef {
	if len(names) == 0 {
		return nil
	}
	refs := make([]SecretEnvRef, len(names))
	for i, n := range names {
		refs[i] = SecretEnvRef{Name: n}
	}
	return refs
}

// SecretEnvNames extracts just the names from refs, the shape a wire
// type like appResource.SecretEnv ([]string) still uses.
func SecretEnvNames(refs []SecretEnvRef) []string {
	if len(refs) == 0 {
		return nil
	}
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	return names
}

// hostPortToNull converts DesiredService.HostPort to the database/sql
// type ExecContext needs to write either a real value or a genuine SQL
// NULL, the same *int-to-sql.NullInt64 idiom github_app.go's
// nullableInt64 already establishes for *int64.
func hostPortToNull(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}
