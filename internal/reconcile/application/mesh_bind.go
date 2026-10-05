package application

import (
	"context"
	"net"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithMeshPaths lets a service placed on a remote node publish its main
// port on that node's WireGuard mesh address, so this control plane's
// ingress can reach it. Without it, remote ports stay on loopback.
func WithMeshPaths(r meshpath.Resolver) Option {
	return func(c *Controller) { c.meshPaths = r }
}

// meshBindIP returns the mesh address desired's main port should bind to,
// or false to keep the normal bind_address behavior. Only the default
// private (loopback) bind is rewritten: "public" and an explicit IP are
// the operator's own choice, and only ever the main port moves, never an
// app stream or a database port.
func (c *Controller) meshBindIP(ctx context.Context, desired *store.DesiredService) (string, bool) {
	if c.meshPaths == nil || desired.NodeID == "" || desired.Port == 0 {
		return "", false
	}
	hostIP, err := bindaddr.Resolve(desired.BindAddress)
	if err != nil || !isLoopbackIP(hostIP) {
		return "", false
	}
	path, err := c.meshPaths.Path(ctx, desired.NodeID)
	if err != nil || path.Local || !path.Usable {
		return "", false
	}
	if meshpath.ValidateBindIP(path.Address) != nil {
		return "", false
	}
	return path.Address, true
}

// applyMeshBind rewrites the main port binding in spec to the mesh address.
func (c *Controller) applyMeshBind(ctx context.Context, desired *store.DesiredService, spec *docker.ContainerSpec) {
	ip, ok := c.meshBindIP(ctx, desired)
	if !ok || len(spec.Ports) == 0 {
		return
	}
	spec.Ports[0].HostIP = ip
}

// meshBindStale reports a container whose main port is not published on the
// mesh address it should be, typically because it was created before the
// mesh path came up. It is never true when the path is down, so a mesh
// flap does not churn containers.
func (c *Controller) meshBindStale(ctx context.Context, state *docker.ContainerState, desired *store.DesiredService) bool {
	if state == nil {
		return false
	}
	ip, ok := c.meshBindIP(ctx, desired)
	if !ok {
		return false
	}
	seen := false
	for _, p := range state.Ports {
		if p.ContainerPort != desired.Port {
			continue
		}
		seen = true
		if p.HostIP == ip {
			return false
		}
	}
	return seen
}

func isLoopbackIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
