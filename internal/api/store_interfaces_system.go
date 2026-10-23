package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SecretSetter is the surface the secrets handlers need from
// internal/secrets.Manager: set a value (with a reversible per-key lock
// guard), list which keys exist, toggle a key's lock, and check whether
// one exists without decrypting it (Exists, used by
// databases.go's databaseTLSEnabled to show a database's TLS status,
// never to read the certificate back). Every other secret-backed
// feature in this file keeps using Manager's plain SetValue/Resolve
// directly, unaffected by this narrower interface.
type SecretSetter interface {
	SetValueGuarded(ctx context.Context, serviceName, envKey, plaintext string, overwriteLocked bool) error
	ListKeys(ctx context.Context, serviceName string) ([]store.SecretKeyInfo, error)
	SetLocked(ctx context.Context, serviceName, envKey string, locked bool) error
	// SetBuildEnabled flips whether build.secrets may mount the secret.
	SetBuildEnabled(ctx context.Context, serviceName, envKey string, enabled bool) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
	// ExistsForServices is Exists batched over many service names in one
	// call, used by databases.go's handleListDatabases to show every
	// row's TLS status without an Exists call per database.
	ExistsForServices(ctx context.Context, serviceNames []string, envKey string) (map[string]bool, error)
	// Resolve decrypts and returns one secret's plaintext value, the same
	// read internal/reconcile/application already does immediately before
	// container creation. environment_clone.go's own explicit,
	// opt-in-only CopySecretValues path is the one HTTP-reachable caller
	// that ever moves a decrypted value between two secret namespaces
	// (source app/environment -> cloned app/environment) rather than
	// only ever writing a caller-supplied plaintext the way
	// SetValueGuarded's other callers do.
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	// DeleteAll permanently removes every value stored under a
	// namespace, used by handleDetachAppIntegration to clear one
	// attached integration's field values in the same request that
	// removes its app_integrations row.
	DeleteAll(ctx context.Context, serviceName string) error
}

// MasterKeyRotator rotates every stored DEK to a new master key and reports the last success.
// Separate from SecretSetter because it is gated at AbilityRoot.
type MasterKeyRotator interface {
	RotateMasterKey(ctx context.Context, newMasterKey string) (rotatedAt time.Time, err error)
	GetMasterKeyRotatedAt(ctx context.Context) (rotatedAt time.Time, ok bool, err error)
}

// DockerPinger is the liveness check behind GET /api/v1/system/status. Narrower than
// docker.Runtime on purpose, to avoid rippling into its fakes. *docker.Client satisfies it.
type DockerPinger interface {
	Ping(ctx context.Context) error
}

// ReconcileNudger is *reconcile.Engine's own Nudge method, narrowed the
// same consumer-defined way DockerPinger is above: a handler that just
// changed desired state calls it to request an immediate reconcile pass
// instead of leaving an operator watching a create/stop/start/restart
// take up to a full resyncInterval (default 30s) to visibly do anything.
// Reconcile's own idempotent, level-triggered contract is what makes an
// extra, unscheduled pass always safe.
type ReconcileNudger interface {
	Nudge()
}

// nudgeReconciler calls rt.reconcileNudger.Nudge() if one is configured,
// the one-line nil-check every mutating handler below shares instead of
// repeating "if rt.reconcileNudger != nil" at each call site.
func (rt *Router) nudgeReconciler() {
	if rt.reconcileNudger != nil {
		rt.reconcileNudger.Nudge()
	}
}

// ImageLister discovers previously built tags under a repo for the deploy form. It mirrors
// docker.Runtime.ListImages. *docker.Client satisfies it.
type ImageLister interface {
	ListImages(ctx context.Context, repo string) ([]docker.ImageInfo, error)
}

// ContainerLister lists every container on the node, managed or not (empty prefix matches all).
// *docker.Client satisfies it.
type ContainerLister interface {
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
}

// DockerDiskUsager reports Docker's own storage accounting for GET /api/v1/system/status,
// distinct from the data dir totals. *docker.Client satisfies it via DiskUsage.
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

// StaleSecretCounter is the surface GET /api/v1/system/doctor's
// stale_secrets check needs: count every secret-backed value (per-app
// and every secret-marked shared-env row alike) last set or rotated
// before cutoff. *store.DB satisfies this via CountStaleSecrets
// (internal/store/secret_rotation.go).
type StaleSecretCounter interface {
	CountStaleSecrets(ctx context.Context, cutoff time.Time) (int, error)
}

// DockerPruner backs POST /api/v1/system/prune (AbilityRoot). Split from DockerDiskUsager so a
// read and a destructive action stay independently swappable. *docker.Client satisfies it.
type DockerPruner interface {
	Prune(ctx context.Context, keep []string) docker.PruneResult
}

// OrphanedVolumeManager lists this instance's named volumes and removes one by name. This package
// decides which are orphaned; internal/docker stays store-agnostic. *docker.Client satisfies it.
type OrphanedVolumeManager interface {
	ListNamedVolumes(ctx context.Context) ([]docker.NamedVolume, error)
	RemoveVolume(ctx context.Context, name string) error
}

// OrphanedContainerManager stops or removes one container by ID. Every calling route first
// re-confirms the container is orphaned (isManagedContainer), never trusting a client flag.
// *docker.Client satisfies it.
type OrphanedContainerManager interface {
	Stop(ctx context.Context, id string, timeout time.Duration) error
	Remove(ctx context.Context, id string, force bool) error
}

// NodeRuntimeResolver picks the docker.Runtime owning a nodeID: the local daemon for "", a connected
// agent's Transport otherwise. A func type so this package avoids importing internal/agent.
// handleExecApp needs per-request node routing, unlike the fixed local-daemon interfaces above.
type NodeRuntimeResolver func(nodeID string) (docker.Runtime, error)
