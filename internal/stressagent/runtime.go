package stressagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

var errUnsupported = errors.New("stressagent: not supported by the fake runtime")

// Runtime is an in-memory container runtime: created containers start instantly and stay running.
type Runtime struct {
	mu         sync.Mutex
	next       int
	byName     map[string]*docker.ContainerState
	networks   map[string]docker.NetworkInfo
	closeEvent sync.Once
}

// NewRuntime returns an empty runtime.
func NewRuntime() *Runtime {
	return &Runtime{byName: map[string]*docker.ContainerState{}, networks: map[string]docker.NetworkInfo{}}
}

// Count reports how many containers exist.
func (r *Runtime) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byName)
}

func (r *Runtime) find(id string) *docker.ContainerState {
	for _, c := range r.byName {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// InspectByName returns a copy of the container, or nil when absent.
func (r *Runtime) InspectByName(_ context.Context, name string) (*docker.ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byName[name]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

// Create records a stopped container.
func (r *Runtime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	id := fmt.Sprintf("fake%012d", r.next)
	r.byName[spec.Name] = &docker.ContainerState{ID: id, Name: spec.Name, Image: spec.Image, Created: time.Now()}
	return id, nil
}

// Start marks the container running.
func (r *Runtime) Start(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.find(id)
	if c == nil {
		return fmt.Errorf("stressagent: no container %s", id)
	}
	c.Running = true
	return nil
}

// Events never emits; it only closes when ctx ends.
func (r *Runtime) Events(ctx context.Context) (<-chan docker.Event, <-chan error) {
	ev, errs := make(chan docker.Event), make(chan error, 1)
	go func() {
		<-ctx.Done()
		close(ev)
		close(errs)
	}()
	return ev, errs
}

// ListImages reports no local images.
func (r *Runtime) ListImages(context.Context, string) ([]docker.ImageInfo, error) { return nil, nil }

// ListByPrefix returns containers whose name starts with prefix, sorted by name.
func (r *Runtime) ListByPrefix(_ context.Context, prefix string) ([]docker.ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []docker.ContainerState
	for name, c := range r.byName {
		if strings.HasPrefix(name, prefix) {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Stop marks the container not running.
func (r *Runtime) Stop(_ context.Context, id string, _ time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.find(id); c != nil {
		c.Running = false
	}
	return nil
}

// Remove deletes the container.
func (r *Runtime) Remove(_ context.Context, id string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.find(id); c != nil {
		delete(r.byName, c.Name)
	}
	return nil
}

// UpdateResources is accepted and ignored.
func (r *Runtime) UpdateResources(context.Context, string, docker.Resources) error { return nil }

// EnsureVolume is accepted and ignored.
func (r *Runtime) EnsureVolume(context.Context, string) error { return nil }

// EnsureNetwork records the network and returns a stable id.
func (r *Runtime) EnsureNetwork(_ context.Context, name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.networks[name]
	if !ok {
		n = docker.NetworkInfo{ID: "net-" + name, Name: name}
		r.networks[name] = n
	}
	return n.ID, nil
}

// RemoveNetwork forgets the network.
func (r *Runtime) RemoveNetwork(_ context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.networks, name)
	return nil
}

// ListNetworksByPrefix returns networks whose name starts with prefix.
func (r *Runtime) ListNetworksByPrefix(_ context.Context, prefix string) ([]docker.NetworkInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []docker.NetworkInfo
	for name, n := range r.networks {
		if strings.HasPrefix(name, prefix) {
			out = append(out, n)
		}
	}
	return out, nil
}

// NetworkConnect is accepted and ignored.
func (r *Runtime) NetworkConnect(context.Context, string, string) error { return nil }

// NetworkDisconnect is accepted and ignored.
func (r *Runtime) NetworkDisconnect(context.Context, string, string, bool) error { return nil }

// Exec is not supported: stress nodes run no processes.
func (r *Runtime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	return nil, errUnsupported
}

// ExecWithInput is not supported: stress nodes run no processes.
func (r *Runtime) ExecWithInput(context.Context, string, []string, io.Reader) (io.ReadCloser, error) {
	return nil, errUnsupported
}
