package api

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/meshpath"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// remoteIngressBlock reports why svc's hosts cannot be routed by this
// control plane's ingress: svc is placed on another node and no usable
// WireGuard path reaches it. blocked is false for a local app, an app with
// no host to route, or a remote app whose mesh path is up.
func (rt *Router) remoteIngressBlock(ctx context.Context, svc store.DesiredService) (hosts []string, why string, blocked bool) {
	hosts, why, blocked = rt.meshIngressBlock(ctx, svc)
	if blocked && rt.viaAgentTunnel(svc.NodeID) {
		return nil, "", false
	}
	return hosts, why, blocked
}

// viaAgentTunnel is true when the flag is on and the node's agent can carry ingress tunnels.
func (rt *Router) viaAgentTunnel(nodeID string) bool {
	return rt.agentTunnels != nil && experimental.Enabled(experimental.AgentIngressTunnel) && rt.agentTunnels(nodeID)
}

// meshIngressBlock is the mesh-only verdict, before any tunnel fallback.
func (rt *Router) meshIngressBlock(ctx context.Context, svc store.DesiredService) (hosts []string, why string, blocked bool) {
	if rt.isLocalNode(svc.NodeID) {
		return nil, "", false
	}
	hosts = svc.Domains
	if len(hosts) == 0 {
		if u := rt.fallbackURLFor(ctx, svc); u != "" {
			hosts = []string{strings.TrimPrefix(u, "https://")}
		}
	}
	if len(hosts) == 0 {
		return nil, "", false
	}
	if rt.meshPaths == nil {
		return hosts, "mesh networking is off on this control plane (set APP_MESH_ENABLED=1 and restart it)", true
	}
	path, err := meshPathMemoFrom(ctx).resolve(ctx, rt.meshPaths, svc.NodeID)
	if err != nil {
		return hosts, fmt.Sprintf("the mesh path to the node could not be read: %s", err), true
	}
	if path.Local || path.Usable {
		return nil, "", false
	}
	return hosts, path.Reason, true
}

// doctorCheckCrossNodeIngress flags every app on a remote node whose hosts
// this control plane's ingress cannot reach over the WireGuard mesh, a
// failure that is otherwise silent (a visitor just sees a TLS error).
func (rt *Router) doctorCheckCrossNodeIngress(ctx context.Context) []doctorCheckResource {
	if rt.apps == nil {
		return nil
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return []doctorCheckResource{{
			Code:    "cross_node_ingress",
			Name:    "Cross-node ingress reachability",
			Status:  doctorStatusUnknown,
			Message: fmt.Sprintf("could not list apps: %s", err),
		}}
	}

	var nodeNames map[string]string
	var out []doctorCheckResource
	for _, svc := range services {
		hosts, why, blocked := rt.remoteIngressBlock(ctx, svc)
		if !blocked {
			continue
		}
		if nodeNames == nil {
			nodeNames = rt.doctorNodeNames(ctx)
		}
		nodeLabel := svc.NodeID
		if name := nodeNames[svc.NodeID]; name != "" {
			nodeLabel = fmt.Sprintf("%s (%s)", name, svc.NodeID)
		}
		out = append(out, doctorCheckResource{
			Code:   "cross_node_ingress:" + svc.Name,
			Name:   fmt.Sprintf("Ingress reachability for %s", svc.Name),
			Status: doctorStatusWarn,
			Message: fmt.Sprintf(
				"app %q is placed on node %s and its host(s) %s cannot be routed: this control plane reaches that node over the WireGuard mesh and the path is not usable (%s). Visitors get a TLS handshake error until it is",
				svc.Name, nodeLabel, strings.Join(hosts, ", "), why,
			),
			Fix:      fmt.Sprintf("levelrail-cli nodes mesh   # fix the mesh path; or: levelrail-cli apps set-node %s <this control plane's own node id>", svc.Name),
			DocsPath: "/multi-node#routing-to-apps-on-remote-nodes",
		})
	}
	return out
}

// crossNodeIngressAppCondition surfaces the same finding on the app's own
// status: the ingress controller reports CrossNodeIngress under its own
// singleton name, so summarizeAppConditions would otherwise show "Healthy"
// for an app whose host is unreachable. Returns nil (clearing it) when the
// app is local, has no host, or its mesh path is usable.
func (rt *Router) crossNodeIngressAppCondition(ctx context.Context, svc store.DesiredService) *reconcile.Condition {
	hosts, why, blocked := rt.meshIngressBlock(ctx, svc)
	if !blocked {
		return nil
	}
	if rt.viaAgentTunnel(svc.NodeID) {
		return &reconcile.Condition{
			Type:    "CrossNodeIngress",
			Status:  reconcile.ConditionTrue,
			Reason:  "AgentTunnel",
			Message: fmt.Sprintf("host(s) %s reach %q through the agent tunnel because the node has no usable mesh path", strings.Join(hosts, ", "), svc.Name),
		}
	}
	return &reconcile.Condition{
		Type:   "CrossNodeIngress",
		Status: reconcile.ConditionFalse,
		Reason: "NoMeshIngressPath",
		Message: fmt.Sprintf(
			"host(s) %s cannot receive traffic: %q runs on another node and the WireGuard mesh path to it is not usable (%s). Run `levelrail-cli nodes mesh`. See docs/multi-node.md#routing-to-apps-on-remote-nodes.",
			strings.Join(hosts, ", "), svc.Name, why,
		),
	}
}

// doctorNodeNames maps node ID to display name; a lookup failure degrades
// to an empty map rather than blocking the report.
func (rt *Router) doctorNodeNames(ctx context.Context) map[string]string {
	names := map[string]string{}
	if rt.nodes == nil {
		return names
	}
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return names
	}
	for _, n := range nodes {
		names[n.ID] = n.Name
	}
	return names
}

type meshPathMemoKey struct{}

// meshPathMemo remembers one mesh path answer per node for the life of a request,
// so a list of many apps on one node resolves that node once.
type meshPathMemo struct {
	mu    sync.Mutex
	paths map[string]meshPathAnswer
}

type meshPathAnswer struct {
	path meshpath.Path
	err  error
}

func withMeshPathMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, meshPathMemoKey{}, &meshPathMemo{paths: map[string]meshPathAnswer{}})
}

func meshPathMemoFrom(ctx context.Context) *meshPathMemo {
	m, _ := ctx.Value(meshPathMemoKey{}).(*meshPathMemo)
	return m
}

// resolve falls through to the resolver when the request carries no memo.
func (m *meshPathMemo) resolve(ctx context.Context, r meshpath.Resolver, nodeID string) (meshpath.Path, error) {
	if m == nil {
		return r.Path(ctx, nodeID)
	}
	m.mu.Lock()
	a, ok := m.paths[nodeID]
	m.mu.Unlock()
	if ok {
		return a.path, a.err
	}
	p, err := r.Path(ctx, nodeID)
	m.mu.Lock()
	m.paths[nodeID] = meshPathAnswer{path: p, err: err}
	m.mu.Unlock()
	return p, err
}
