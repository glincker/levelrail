// Package nginxdemo is the Phase 0 exit criterion: one controller that
// keeps a single hardcoded nginx container running. Kill the container
// by hand, watch it come back. Desired state is hardcoded on purpose:
// reading desired state from the SQLite store is Phase 1, not this.
package nginxdemo

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

const (
	// ContainerName is fixed for the demo. A real controller would derive
	// this from a resource's declared name; here there's exactly one
	// resource and it never changes.
	ContainerName = "levelrail-demo-nginx"
	image         = "nginx:alpine"
)

// Controller converges a single hardcoded nginx container to "exists and
// running." Every Reconcile call re-derives what to do from the runtime's
// current observed state: it never assumes a previous call finished
// cleanly, which is what makes it safe to interrupt mid-operation and
// safe to call again immediately after (reconcilers are idempotent,
// level-triggered, and safe to interrupt by design).
type Controller struct {
	runtime     docker.Runtime
	meshDNSAddr string // empty is valid: no mesh DNS server is running, or it hasn't resolved a container-reachable address, see WithMeshDNSAddr
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithMeshDNSAddr points the demo container at addr, a bare nameserver IP
// (never "ip:port": neither Docker's DNS HostConfig field nor a
// container's own resolv.conf supports a non-standard nameserver port,
// confirmed live while building this option, see cmd/levelrail/mesh.go's
// dockerNameserverPort doc comment), as an additional nameserver ahead of
// whatever Docker's own resolver would otherwise configure
// (docker.ContainerSpec.DNS). Without one configured (the default, empty
// string), Reconcile behaves exactly as before this field existed: no DNS
// override at all.
func WithMeshDNSAddr(addr string) Option {
	return func(c *Controller) { c.meshDNSAddr = addr }
}

// New builds a Controller against the given Runtime. Pass a fake in
// tests, the real docker.Client in production.
func New(runtime docker.Runtime, opts ...Option) *Controller {
	c := &Controller{runtime: runtime}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "nginx-demo" }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	state, err := c.runtime.InspectByName(ctx, ContainerName)
	if err != nil {
		return notReady("InspectFailed", err), fmt.Errorf("nginx-demo: inspect %q: %w", ContainerName, err)
	}

	if state == nil {
		spec := docker.ContainerSpec{Name: ContainerName, Image: image}
		if c.meshDNSAddr != "" {
			spec.DNS = []string{c.meshDNSAddr}
		}
		id, err := c.runtime.Create(ctx, spec)
		if err != nil {
			return notReady("CreateFailed", err), fmt.Errorf("nginx-demo: create: %w", err)
		}
		// A container that exists but isn't started yet is a valid
		// observed state the next Reconcile call would find and fix on
		// its own: but starting it now is still the right first move
		// rather than waiting for the next trigger.
		if err := c.runtime.Start(ctx, id); err != nil {
			return notReady("StartFailedAfterCreate", err), fmt.Errorf("nginx-demo: start after create: %w", err)
		}
		return ready("Created"), nil
	}

	if !state.Running {
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err), fmt.Errorf("nginx-demo: restart: %w", err)
		}
		return ready("Restarted"), nil
	}

	return ready("AlreadyRunning"), nil
}

func ready(reason string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:   "Ready",
		Status: reconcile.ConditionTrue,
		Reason: reason,
	}}}
}

func notReady(reason string, err error) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:    "Ready",
		Status:  reconcile.ConditionFalse,
		Reason:  reason,
		Message: err.Error(),
	}}}
}
