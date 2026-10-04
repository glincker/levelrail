package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// doctorCheckCrossNodeIngress flags every app whose domain(s) can never be
// reached: this control plane's own ingress never routes a different
// node's container (no mesh path yet), and that failure mode is otherwise
// silent (see internal/reconcile/ingress's CrossNodeIngress condition).
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
		if len(svc.Domains) == 0 || rt.isLocalNode(svc.NodeID) {
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
				"app %q is placed on node %s, but its domain(s) %s are only ever routed by this control plane's own ingress on its local node; a visitor's TLS handshake fails there since no automation policy is created for an unreachable backend, even though the app itself is healthy",
				svc.Name, nodeLabel, strings.Join(svc.Domains, ", "),
			),
			Fix:      fmt.Sprintf("levelrail-cli apps set-node %s <this control plane's own node id>   # or: levelrail-cli apps clear-node %s", svc.Name, svc.Name),
			DocsPath: "/multi-node#wireguard-mesh-and-internal-dns",
		})
	}
	return out
}

// crossNodeIngressAppCondition is doctorCheckCrossNodeIngress's gap,
// surfaced on the app's own status instead of only the system-wide
// doctor report: the ingress controller reports CrossNodeIngress under
// its own singleton controller name (internal/reconcile/ingress/
// cross_node.go), never under this app's own controller, so
// summarizeAppConditions never sees it and shows "Healthy" for an app
// whose domain is provably unreachable. Returns nil when the app has no
// domain or is on the local node.
func (rt *Router) crossNodeIngressAppCondition(svc store.DesiredService) *reconcile.Condition {
	if len(svc.Domains) == 0 || rt.isLocalNode(svc.NodeID) {
		return nil
	}
	return &reconcile.Condition{
		Type:   "CrossNodeIngress",
		Status: reconcile.ConditionFalse,
		Reason: "NoMeshIngressPath",
		Message: fmt.Sprintf(
			"domain(s) %s cannot receive traffic: this control plane's ingress only routes to its own node, and %q is placed on a different node with no mesh path to it yet. See docs/multi-node.md#wireguard-mesh-and-internal-dns.",
			strings.Join(svc.Domains, ", "), svc.Name,
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
