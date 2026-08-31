// Package docker wraps the Docker Engine API. Every call goes through the
// SDK's HTTP-over-socket client, never a shelled-out `docker` CLI command,
// per the project's rules for node communication and orchestration. This
// file defines the narrow interface reconcile controllers depend on, so
// tests can fake it without a daemon.
package docker

import (
	"context"
	"io"
	"time"
)

// PortBinding maps one container port to a host port. In a ContainerSpec,
// HostPort of 0 means "let Docker assign one"; in an observed
// ContainerState it's always the concrete port actually bound, never 0.
//
// Host ports, not direct container-IP routing, on purpose: this process
// runs as a plain host process today (the agent's single-node mode,
// where control plane and agent share a process), and container IPs on
// Docker's bridge network are only reachable from
// the host on Linux, not from Docker Desktop's macOS VM boundary. Every
// target this matters for (a Linux managed node, and this Mac during
// development) can always reach localhost:hostPort, so that's the one
// mechanism used everywhere rather than branching on platform.
type PortBinding struct {
	ContainerPort int
	HostPort      int
	// Protocol is "tcp" or "udp". Empty is treated as "tcp" by Create.
	Protocol string
}

// Resources caps a container's memory and CPU, in the Engine API's own
// units directly (bytes, nano-CPUs) rather than an invented "cores as
// float64" indirection. Translating from app.yaml's human-friendly units
// (spec.Resources: "512Mi", 0.5 cores) into these is the caller's job,
// keeping this package spec-agnostic, same as ContainerSpec.Image being
// a plain string rather than something spec-aware.
type Resources struct {
	MemoryBytes int64
	NanoCPUs    int64
	// SwapMemoryBytes is Docker's own MemorySwap: total memory plus swap
	// combined, only meaningful alongside MemoryBytes.
	SwapMemoryBytes int64
	// CPUSetCPUs pins the container to specific host CPUs, Docker's own
	// cpuset-cpus format (e.g. "0-3" or "0,2").
	CPUSetCPUs string
}

// ContainerState is the observed state of a single container, trimmed to
// the fields a controller actually needs to decide what to do next.
type ContainerState struct {
	ID      string
	Name    string
	Image   string
	Running bool
	// Ports reflects live port bindings. Docker only actually binds a
	// container's published ports once it's running, so this is empty
	// for a created-but-not-yet-started container, not an error.
	Ports []PortBinding
}

// VolumeMount attaches one named Docker volume to a path inside a
// container, e.g. Postgres's /var/lib/postgresql/data or Redis's /data
// (TASKS.md 1.8). Name is a Docker volume name, not a host path: bind
// mounts aren't exposed here, keeping this package's surface to what a
// single-node managed database actually needs today.
type VolumeMount struct {
	Name          string
	ContainerPath string
	// ReadOnly mounts the volume read-only inside the container. False
	// (read-write) for every caller before internal/backup's volume
	// archiver started setting it explicitly true, so this is
	// byte-identical to every mount created before this field existed.
	ReadOnly bool
}

// ContainerSpec is desired state for a container a controller wants to
// exist.
//
// No restart policy field, deliberately: the reconciler, not Docker, is
// meant to be the sole authority on "should this container
// be running." A Docker-native restart policy running alongside a
// reconciler that also restarts dead containers is two independent
// systems racing to make the same decision, exactly the kind of drift
// the reconciler's level-triggered design exists to avoid. Every
// container Levelrail creates gets Docker's "no" restart policy; staying
// running is the reconciler's job, proven live in nginxdemo (Phase 0).
//
// No health check field either: the app spec's readiness/liveness probes
// are modeled on Kubernetes' prober pattern (the controller calls out to
// the container, not the container reporting its own health via Docker's
// HEALTHCHECK state machine), the same design choice ADR 002's
// Consequences section already commits to over Coolify's confirmed
// weaker alternative (health check disabled by default, gated entirely
// on Docker's own HEALTHCHECK). The prober itself belongs in the
// application controller (Phase 1, TASKS.md 1.3), not here; this package
// only needs to make a container reachable, via Ports above.
type ContainerSpec struct {
	Name      string
	Image     string
	Ports     []PortBinding
	Env       map[string]string
	Resources *Resources
	// Volumes are named Docker volumes to mount at create time. A
	// database controller (TASKS.md 1.8) is the first caller; ordinary
	// application containers leave this nil.
	Volumes []VolumeMount
	// DNS lists nameserver IPs Docker writes into the container's
	// /etc/resolv.conf, ahead of whatever the daemon would otherwise
	// configure. Empty/nil is byte-identical to today: only a caller
	// that resolved a real, container-reachable mesh DNS address
	// (cmd/levelrail/mesh.go's containerDNSAddr) ever sets this, and
	// only when the mesh is actually enabled and running.
	DNS []string
	// Labels are applied to the container verbatim via the Engine API's
	// own label mechanism (container.Config.Labels), operator-supplied
	// custom Docker labels (internal/spec.Service.Labels, an escape
	// hatch for external tooling that keys off container labels). This
	// package stays spec-agnostic (same reasoning as Image being a
	// plain string, not something spec-aware, this struct's own doc
	// comment above): it trusts Labels arrived here already validated,
	// the same way it already trusts Env and every other field. See
	// internal/spec.ValidateLabels for what's rejected before a caller
	// ever builds a ContainerSpec with this set.
	Labels map[string]string
	// Network attaches the container to a non-default Docker network at
	// create time, with Alias as the name sibling containers on that
	// network can reach it by (Docker's embedded per-network DNS
	// resolves it). Nil means Docker's default "bridge" network,
	// unchanged from every container this codebase created before this
	// field existed: that network has no embedded DNS, so this is what
	// makes multi-service apps able to reach each other by name at all.
	Network *NetworkAttachment
	// RegistryAuth authenticates the image pull (Create's own ensureImage
	// step) against a private registry. Nil means an unauthenticated
	// pull, unchanged from every container this codebase created before
	// this field existed.
	RegistryAuth *RegistryAuth
	// Command overrides the image's own default CMD. Nil means the
	// image's own default, unchanged from every container this codebase
	// created before this field existed. First caller:
	// internal/reconcile/cloudflaretunnel, whose cloudflared image needs
	// an explicit "tunnel run" argument rather than relying on the
	// image's bare entrypoint.
	Command []string
}

// RegistryAuth is a plaintext username/password pair for pulling a
// private image, resolved by the caller (internal/reconcile/application)
// from internal/secrets immediately before Create; never persisted by
// this package.
type RegistryAuth struct {
	Username string
	Password string
}

// NetworkAttachment is a container's non-default network attachment:
// which Docker network to join, and what DNS-resolvable alias sibling
// containers on that network can reach it by.
type NetworkAttachment struct {
	Name  string
	Alias string
}

// ImageInfo is one tagged image available locally, used to discover
// rollback candidates, per the rule that the previous N images stay
// pinned so garbage collection cannot orphan a rollback target.
type ImageInfo struct {
	Tag       string
	CreatedAt time.Time
}

// EventAction mirrors the subset of Docker container lifecycle events a
// reconciler cares about.
type EventAction string

// The lifecycle actions a reconciler can receive from Events.
const (
	EventStart EventAction = "start"
	EventDie   EventAction = "die"
	EventStop  EventAction = "stop"
)

// Event is a trimmed container lifecycle event from the Docker event
// stream. Reconcilers key off ContainerName, not ID, since a container's
// ID changes across recreates but the name a controller manages does not.
type Event struct {
	Action        EventAction
	ContainerName string
	Time          time.Time
}

// NetworkInfo is one Docker network, trimmed to what
// ListNetworksByPrefix's caller needs to decide whether it's still
// wanted.
type NetworkInfo struct {
	ID   string
	Name string
}

// Runtime is the surface reconcile controllers are allowed to depend on.
// The real implementation (Client, in client.go) talks to the Docker
// Engine API. Tests use a hand-written fake: see nginxdemo's test file
// for the pattern every future controller's tests should follow.
type Runtime interface {
	// InspectByName returns the current state of the container with this
	// name, or (nil, nil) if no such container exists. It never returns
	// an error for "not found": that's a valid observed state, not a
	// failure.
	InspectByName(ctx context.Context, name string) (*ContainerState, error)

	// Create makes a container from spec but does not start it. Returns
	// the new container's ID.
	Create(ctx context.Context, spec ContainerSpec) (id string, err error)

	// Start starts an existing container by ID.
	Start(ctx context.Context, id string) error

	// Events streams container lifecycle events until ctx is cancelled.
	// The error channel receives at most one error (a stream failure)
	// and is then closed; the event channel is closed once the stream
	// ends for any reason.
	Events(ctx context.Context) (<-chan Event, <-chan error)

	// ListImages returns every locally-present tag under repo (e.g.
	// "levelrail/myapp"), newest first, for rollback candidate discovery.
	// An empty result and a nil error both mean "no images found", not
	// an error: a service that's never been built has no images yet.
	ListImages(ctx context.Context, repo string) ([]ImageInfo, error)

	// ListByPrefix returns every container (running or not) whose name
	// starts with prefix, for a controller to find every container
	// belonging to a service it manages, not just the one canonical name
	// nginxdemo's simpler world got away with.
	ListByPrefix(ctx context.Context, prefix string) ([]ContainerState, error)

	// Stop stops a running container, sending Signal (empty means
	// Docker's default, SIGTERM) and waiting up to timeout before
	// forcing it. A negative timeout waits indefinitely.
	Stop(ctx context.Context, id string, timeout time.Duration) error

	// Remove deletes a container. force also removes a still-running
	// one (stopping it first); without force, removing a running
	// container is an error, matching Docker's own default.
	Remove(ctx context.Context, id string, force bool) error

	// UpdateResources applies new memory, CPU, swap, and cpuset limits to
	// an already-running container in place, via the Engine API's own
	// ContainerUpdate call. Unlike every other resource-affecting change
	// a controller can want (image, env, ports, health check), this one
	// has a real live-update path: Docker itself supports adjusting a
	// running container's cgroup limits without stopping it, so a
	// reconciler can converge a resource-limit-only diff without the
	// blue-green cutover a full recreate would otherwise force.
	UpdateResources(ctx context.Context, id string, resources Resources) error

	// EnsureVolume creates a named Docker volume if it doesn't already
	// exist. Idempotent: calling it for a volume that's already present
	// is not an error, matching the Engine API's own VolumeCreate
	// semantics (creating by an existing name returns that volume
	// unchanged rather than failing).
	EnsureVolume(ctx context.Context, name string) error

	// EnsureNetwork creates a Docker bridge network named name if it
	// doesn't already exist, returning its ID either way. Idempotent:
	// inspect first, create only on a genuine miss, the same shape the
	// real implementation's ensureImage already uses for images.
	// Reconcile calls happen often (event-driven plus periodic resync),
	// so this must be safe to call on every pass, not just the first.
	EnsureNetwork(ctx context.Context, name string) (id string, err error)

	// RemoveNetwork deletes a Docker network by name. Removing a network
	// that doesn't exist is not an error: cleanup is level-triggered and
	// may run against state that's already converged.
	RemoveNetwork(ctx context.Context, name string) error

	// ListNetworksByPrefix returns every Docker network whose name
	// starts with prefix, for a reconciler to diff observed networks
	// against current desired state (internal/reconcile/application's
	// NetworkCleanupController).
	ListNetworksByPrefix(ctx context.Context, prefix string) ([]NetworkInfo, error)

	// Exec runs cmd inside the already-running container containerID and
	// returns its stdout as a stream, using the Engine API's exec
	// facility (ContainerExecCreate then ContainerExecAttach), the same
	// two calls `docker exec` itself is built on. This is the one Runtime
	// method that reaches into a process running inside a container
	// rather than managing the container's own lifecycle; it exists for
	// internal/backup's Dumper, which has no other way to run a
	// database's own native dump tool (pg_dump, mysqldump, redis-cli)
	// against a live container without shelling out, which is exactly
	// what every other method on this interface already exists to avoid.
	//
	// The returned ReadCloser carries only cmd's stdout. Stderr is
	// captured by the implementation and, if cmd exits non-zero,
	// surfaced as the error a later Read returns once the stream ends:
	// the same "an error can arrive with or after the final bytes"
	// contract every io.Reader already promises, so a caller only ever
	// has one place to check for failure instead of a second out-of-band
	// exit-code field. Close must be called once the caller is done with
	// the stream, success or failure alike, to release the underlying
	// exec connection.
	Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error)

	// ExecWithInput is Exec plus a stdin stream: it runs cmd inside
	// containerID exactly as Exec does (same stdout/stderr handling, same
	// exit-code-becomes-a-trailing-error contract), except cmd's stdin is
	// wired to stdin instead of being left unattached. This exists for
	// internal/backup's Restorer, which needs to pipe a downloaded dump
	// into psql/mysql running inside a live container; Exec itself can't
	// do this; ContainerExecCreate has to be told AttachStdin: true up
	// front for the exec session to have a stdin pipe at all, and every
	// other caller of Exec (ContainerDumper) never needs one, so this is a
	// second method rather than a third parameter Exec's own existing
	// call site would otherwise have to grow to accommodate.
	//
	// Implementations read stdin to completion (EOF) and then signal
	// end-of-input to the exec'd process the way closing stdin on a real
	// terminal would, before returning; a command like psql that reads
	// until EOF and then exits depends on that signal to ever finish, not
	// only on stdin's bytes arriving. Callers are not required to provide
	// a stdin that reaches EOF on its own if the command they're running
	// doesn't need one; passing an empty reader is equivalent to Exec's
	// own "nothing attached to stdin" behavior for that command.
	//
	// The returned ReadCloser carries stdout, identically to Exec's; Close
	// must be called once done, same contract.
	ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error)
}
