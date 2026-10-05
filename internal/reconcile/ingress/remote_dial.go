package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithMeshPaths lets the controller route to services placed on remote
// nodes through the WireGuard mesh. Needs WithNodeUpstreams for the remote
// node's runtime. Without it, remote services stay unrouted.
func WithMeshPaths(r meshpath.Resolver) Option {
	return func(c *Controller) { c.meshPaths = r }
}

// dialForRemoteService resolves the mesh dial address for a service placed
// on another node. A non-empty why means an operator must act and is
// reported in the CrossNodeIngress condition; empty why is a transient
// "no ready backend yet", the same silent skip as a local mid-deploy.
func (c *Controller) dialForRemoteService(ctx context.Context, svc store.DesiredService, targetReady bool) (dial, why string, ok bool) {
	if c.meshPaths == nil {
		return "", "mesh networking is off on this control plane: set APP_MESH_ENABLED=1 and restart it", false
	}
	path, err := c.meshPaths.Path(ctx, svc.NodeID)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: resolving mesh path failed, skipping for this pass",
			slog.String("service", svc.Name), slog.String("node", svc.NodeID), slog.String("error", err.Error()))
		return "", "", false
	}
	if !path.Usable {
		return "", path.Reason, false
	}
	if c.nodeUpstreams == nil {
		return "", "", false
	}
	rt, _, err := c.nodeUpstreams.UpstreamHost(ctx, svc.NodeID)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: resolving remote node runtime failed, skipping for this pass",
			slog.String("service", svc.Name), slog.String("node", svc.NodeID), slog.String("error", err.Error()))
		return "", "", false
	}

	target := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	state, err := rt.InspectByName(ctx, target)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: inspecting remote service container failed, skipping for this pass",
			slog.String("service", svc.Name), slog.String("container", target), slog.String("error", err.Error()))
	} else if state != nil && state.Running && len(state.Ports) > 0 {
		d, reason := meshDial(state.Ports[0], path.Address)
		if reason != "" {
			return "", reason, false
		}
		if targetReady {
			return d, "", true
		}
	}

	containers, err := rt.ListByPrefix(ctx, svc.Name+"-")
	if err != nil {
		return "", "", false
	}
	if best := newestRunningPorted(containers, target); best != nil {
		if d, reason := meshDial(best.Ports[0], path.Address); reason == "" {
			return d, "", true
		}
	}
	return "", "", false
}

// meshDial returns host:port on the node's mesh address, or a reason when
// the port is not published where the mesh can reach it. Loopback binds
// are recreated by the application controller once the path is up.
func meshDial(p docker.PortBinding, meshAddr string) (dial, why string) {
	ip := net.ParseIP(p.HostIP)
	switch {
	case ip != nil && ip.IsLoopback():
		return "", "the container publishes its port on loopback only, the app controller republishes it on the mesh address on its next pass"
	case ip != nil && !ip.IsUnspecified() && p.HostIP != meshAddr:
		return "", fmt.Sprintf("the container publishes its port on %s, which is not the node's mesh address %s: set bind_address to private or public", p.HostIP, meshAddr)
	}
	return net.JoinHostPort(meshAddr, strconv.Itoa(p.HostPort)), ""
}

func newestRunningPorted(containers []docker.ContainerState, exclude string) *docker.ContainerState {
	var best *docker.ContainerState
	for i := range containers {
		cs := &containers[i]
		if cs.Name == exclude || !cs.Running || len(cs.Ports) == 0 {
			continue
		}
		if best == nil || cs.Created.After(best.Created) {
			best = cs
		}
	}
	return best
}
