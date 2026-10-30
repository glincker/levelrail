// Package agent is the transport boundary between reconcile controllers and the node they run on.
// Local is the in-process form; GRPCTransport (grpc_transport.go) is the remote form.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// Transport is what a reconcile controller needs from the node a resource is placed on.
// It embeds docker.Runtime so the two cannot drift; optional agent capabilities go in separate interfaces like RemoteBuilder.
type Transport interface {
	docker.Runtime
}

// Local adapts this process's own docker.Runtime into a Transport for the local node, with no network hop.
type Local struct {
	docker.Runtime
}

// NewLocal wraps rt as this process's own node Transport.
func NewLocal(rt docker.Runtime) Local {
	return Local{Runtime: rt}
}

// ExecTTY implements docker.TTYRuntime by forwarding to the wrapped
// runtime, which the embedded docker.Runtime interface would otherwise
// hide even when the concrete value behind it supports interactive exec.
func (l Local) ExecTTY(ctx context.Context, containerID string, opts docker.ExecTTYOptions) (docker.ExecSession, error) {
	tty, ok := l.Runtime.(docker.TTYRuntime)
	if !ok {
		return nil, ErrTTYUnsupported
	}
	return tty.ExecTTY(ctx, containerID, opts)
}

// InspectExitState forwards to the wrapped runtime, since embedding would hide the optional ExitStateInspector.
func (l Local) InspectExitState(ctx context.Context, name string) (*docker.ExitState, error) {
	inspector, ok := l.Runtime.(docker.ExitStateInspector)
	if !ok {
		return nil, ErrExitStateUnsupported
	}
	return inspector.InspectExitState(ctx, name)
}

// InspectImageID implements docker.ImageInspector by forwarding; an
// unsupporting runtime reports "" so digest verification is skipped.
func (l Local) InspectImageID(ctx context.Context, ref string) (string, error) {
	inspector, ok := l.Runtime.(docker.ImageInspector)
	if !ok {
		return "", nil
	}
	id, err := inspector.InspectImageID(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("agent: local inspect image: %w", err)
	}
	return id, nil
}

// InspectImageLayers implements docker.ImageLayerInspector by forwarding.
func (l Local) InspectImageLayers(ctx context.Context, ref string) ([]string, error) {
	inspector, ok := l.Runtime.(docker.ImageLayerInspector)
	if !ok {
		return nil, ErrImageLoadUnsupported
	}
	layers, err := inspector.InspectImageLayers(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("agent: local inspect image layers: %w", err)
	}
	return layers, nil
}

// LoadImage implements docker.ImageLoader by forwarding to the wrapped runtime.
func (l Local) LoadImage(ctx context.Context, r io.Reader) error {
	loader, ok := l.Runtime.(docker.ImageLoader)
	if !ok {
		return ErrImageLoadUnsupported
	}
	if err := loader.LoadImage(ctx, r); err != nil {
		return fmt.Errorf("agent: local load image: %w", err)
	}
	return nil
}

// Stats implements docker.StatsInspector by forwarding to the wrapped
// runtime, the same forwarding InspectExitState above already does for
// a different optional capability.
func (l Local) Stats(ctx context.Context, containerID string) (docker.ContainerStats, error) {
	inspector, ok := l.Runtime.(docker.StatsInspector)
	if !ok {
		return docker.ContainerStats{}, ErrStatsUnsupported
	}
	return inspector.Stats(ctx, containerID)
}

// Logs forwards to the wrapped runtime, which embedding would otherwise hide.
func (l Local) Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error) {
	if reader, ok := l.Runtime.(docker.LogReader); ok {
		return reader.Logs(ctx, containerID, follow, since)
	}
	out := make(chan docker.LogLine)
	errCh := make(chan error, 1)
	errCh <- ErrLogsUnsupported
	close(out)
	close(errCh)
	return out, errCh
}

// RemoveVolume forwards to the wrapped runtime.
func (l Local) RemoveVolume(ctx context.Context, name string) error {
	remover, ok := l.Runtime.(docker.VolumeRemover)
	if !ok {
		return ErrVolumeRemoveUnsupported
	}
	return remover.RemoveVolume(ctx, name)
}

// ErrNodeNotRegistered is Registry.Get's failure mode for an unknown
// node ID.
var ErrNodeNotRegistered = errors.New("agent: node not registered in this transport registry")

// Registry resolves a node ID to the Transport that reaches it.
type Registry struct {
	mu       sync.RWMutex
	byID     map[string]Transport
	localID  string
	localTpt Transport
}

// NewRegistry builds an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]Transport)}
}

// Register associates nodeID with t, replacing any existing entry for
// that ID. Safe for concurrent use.
func (r *Registry) Register(nodeID string, t Transport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[nodeID] = t
}

// Unregister removes nodeID's entry, if any. Not an error if nodeID was
// never registered, matching this codebase's established idempotent-
// delete convention (store.DeleteDesiredService, store.DeleteNode).
func (r *Registry) Unregister(nodeID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, nodeID)
}

// Get returns the Transport registered for nodeID, or
// ErrNodeNotRegistered.
func (r *Registry) Get(nodeID string) (Transport, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[nodeID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNodeNotRegistered, nodeID)
	}
	return t, nil
}

// SetLocal makes Resolve answer localID with t, this process's own runtime.
// It is deliberately not in byID: Get callers (build dispatch, mesh sink)
// must keep treating the local node as "not a remote agent".
func (r *Registry) SetLocal(localID string, t Transport) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.localID, r.localTpt = localID, t
}

// Resolve is Get plus the local node's own ID, for callers that just need
// a runtime for whichever node a resource is placed on.
func (r *Registry) Resolve(nodeID string) (Transport, error) {
	r.mu.RLock()
	if nodeID != "" && nodeID == r.localID && r.localTpt != nil {
		t := r.localTpt
		r.mu.RUnlock()
		return t, nil
	}
	r.mu.RUnlock()
	return r.Get(nodeID)
}
