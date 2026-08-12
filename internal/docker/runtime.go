// Package docker wraps the Docker Engine API. Every call goes through the
// SDK's HTTP-over-socket client, never a shelled-out `docker` CLI command
// (CLAUDE.md 4.3, 4.2 point 3). This file defines the narrow interface
// reconcile controllers depend on, so tests can fake it without a daemon.
package docker

import (
	"context"
	"time"
)

// ContainerState is the observed state of a single container, trimmed to
// the fields a controller actually needs to decide what to do next.
type ContainerState struct {
	ID      string
	Name    string
	Image   string
	Running bool
}

// ContainerSpec is desired state for a container a controller wants to
// exist. Deliberately minimal for the Phase 0 skeleton — CLAUDE.md 4.9's
// full app spec (ports, health checks, resources, env) lands in Phase 1.
type ContainerSpec struct {
	Name  string
	Image string
}

// EventAction mirrors the subset of Docker container lifecycle events a
// reconciler cares about.
type EventAction string

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
// Engine API. Tests use a hand-written fake — see nginxdemo's test file
// for the pattern every future controller's tests should follow.
type Runtime interface {
	// InspectByName returns the current state of the container with this
	// name, or (nil, nil) if no such container exists. It never returns
	// an error for "not found" — that's a valid observed state, not a
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
}
