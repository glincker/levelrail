package store

import (
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/kit/probe"
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
	// GPU requests NVIDIA GPUs. nil means none.
	GPU *ServiceGPU `json:"gpu,omitempty"`
	// ShmSizeBytes sizes /dev/shm; 0 keeps Docker's default (64 MB).
	ShmSizeBytes int64 `json:"shm_size_bytes,omitempty"`
	// ReadOnlyRootfs is the per-app read-only root choice; nil inherits the
	// fleet default. Kept here so it needs no new column.
	ReadOnlyRootfs *bool `json:"read_only_rootfs,omitempty"`
}

// ServiceGPU is a service's GPU request: Count -1 means every GPU,
// DeviceIDs (indexes or UUIDs) take precedence over Count.
type ServiceGPU struct {
	Count     int      `json:"count,omitempty"`
	DeviceIDs []string `json:"device_ids,omitempty"`
}

// ServiceProbe is one readiness or liveness check.
type ServiceProbe struct {
	Path            string        `json:"path"`
	Scheme          string        `json:"scheme,omitempty"`
	Host            string        `json:"host,omitempty"`
	TLSSkipVerify   bool          `json:"tls_skip_verify,omitempty"`
	FollowRedirects *bool         `json:"follow_redirects,omitempty"`
	ExpectedStatus  string        `json:"expected_status,omitempty"`
	Exec            []string      `json:"exec,omitempty"`
	TCP             bool          `json:"tcp,omitempty"`
	Interval        time.Duration `json:"interval,omitempty"`
	Timeout         time.Duration `json:"timeout,omitempty"`
	Failures        int           `json:"failures,omitempty"`
	// Platform marks a probe declared to run from outside the container
	// (compose x-levelrail-healthcheck), never as an in-container exec.
	Platform bool `json:"platform,omitempty"`
}

// Probe kinds ServiceProbe.Kind reports.
const (
	ProbeKindHTTP = "http"
	ProbeKindTCP  = "tcp"
	ProbeKindExec = "exec"
)

// Kind reports whether p is an http, tcp or exec probe.
func (p ServiceProbe) Kind() string {
	switch {
	case p.TCP:
		return ProbeKindTCP
	case len(p.Exec) > 0:
		return ProbeKindExec
	default:
		return ProbeKindHTTP
	}
}

// ProbeConfig converts p into the shape internal/probe runs.
func (p ServiceProbe) ProbeConfig() probe.Config {
	return probe.Config{
		Path:            p.Path,
		Scheme:          p.Scheme,
		Host:            p.Host,
		TLSSkipVerify:   p.TLSSkipVerify,
		FollowRedirects: p.FollowRedirects,
		ExpectedStatus:  p.ExpectedStatus,
		Exec:            p.Exec,
		TCP:             p.TCP,
		Interval:        p.Interval,
		Timeout:         p.Timeout,
	}
}

// NeedsPort reports whether p can only run against a published port.
func (p ServiceProbe) NeedsPort() bool { return len(p.Exec) == 0 }

// ServiceHealth holds a service's probe configuration.
type ServiceHealth struct {
	Readiness *ServiceProbe `json:"readiness,omitempty"`
	Liveness  *ServiceProbe `json:"liveness,omitempty"`
	// ReadyTimeout overrides internal/reconcile/application's own
	// defaultReadyBudget (60s) for this service alone, zero meaning
	// "use the default". See internal/spec.Health.ReadyTimeout, its
	// app.yaml source.
	ReadyTimeout time.Duration `json:"ready_timeout,omitempty"`
	// Restart is a compose file's explicit restart: "no" or "on-failure[:N]",
	// which makes the service run to completion. Empty keeps it running.
	Restart string `json:"restart,omitempty"`
	// DependsOnCompleted is the subset of DependsOn declared with compose
	// condition: service_completed_successfully: the dependency must exit 0 first.
	DependsOnCompleted []string `json:"depends_on_completed,omitempty"`
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
//
// Conn and Version are set for a secret:// reference to a named connection;
// an empty Conn is the legacy single-Vault form. Never a value.
type VaultEnvRef struct {
	Path    string `json:"path"`
	Key     string `json:"key"`
	Conn    string `json:"conn,omitempty"`
	Version int    `json:"version,omitempty"`
	// Unvetted marks a reference written by the deploy pipeline, which has no actor to check
	// against a connection's allow lists. Only gated API writes leave it false.
	Unvetted bool `json:"unvetted,omitempty"`
}

// SecretRef returns the connection reference text, or "" for a legacy ref.
func (r VaultEnvRef) SecretRef() string {
	if r.Conn == "" {
		return ""
	}
	s := "secret://" + r.Conn + "/" + r.Path + "#" + r.Key
	if r.Version > 0 {
		s += "?version=" + strconv.Itoa(r.Version)
	}
	return s
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

// Port protocols a PublishedPort may carry.
const (
	PortProtocolTCP = spec.PortProtocolTCP
	PortProtocolUDP = spec.PortProtocolUDP
)

// PublishedPort is one host-published port of a service, see
// spec.PublishedPort.
type PublishedPort = spec.PublishedPort
