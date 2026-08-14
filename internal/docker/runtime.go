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

	// EnsureVolume creates a named Docker volume if it doesn't already
	// exist. Idempotent: calling it for a volume that's already present
	// is not an error, matching the Engine API's own VolumeCreate
	// semantics (creating by an existing name returns that volume
	// unchanged rather than failing).
	EnsureVolume(ctx context.Context, name string) error

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
}
