package api

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/glincker/levelrail/internal/bindaddr"
	"github.com/glincker/levelrail/internal/placement"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// KeyPlacementReplicaSpread turns on spreading one app's replicas over nodes.
const KeyPlacementReplicaSpread = "placement.replica_spread"

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyPlacementReplicaSpread, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "false", Env: "APP_PLACEMENT_REPLICA_SPREAD", Apply: runtimesettings.ApplyLive,
		Title:       "Spread replicas over nodes",
		Description: "Place each replica of a stateless app with more than one replica on a different eligible node, honoring taints, affinity and platform. Apps with volumes, local-only grants or a loopback bind address stay on one node. Off by default; read on every reconcile pass.",
	})
}

// ReplicaSpreadPlan is the outcome of planning one app's replicas.
// Nodes is nil when the app stays on one node; Refusal then says why, or is
// empty when spreading is simply off or not needed.
type ReplicaSpreadPlan struct {
	Nodes   []string
	Refusal string
}

// PlanReplicaSpread picks the node of every replica of svc. Node ids use the
// service convention, so the control plane node is "".
func (rt *Router) PlanReplicaSpread(ctx context.Context, svc store.DesiredService) ReplicaSpreadPlan {
	if rt.runtimeSettings == nil || !rt.runtimeSettings.Bool(ctx, KeyPlacementReplicaSpread) || svc.Replicas <= 1 {
		return ReplicaSpreadPlan{}
	}
	if why := spreadRefusal(svc); why != "" {
		return ReplicaSpreadPlan{Refusal: why}
	}
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return ReplicaSpreadPlan{Refusal: "could not read the node list: " + err.Error()}
	}
	sel, err := rt.selectorFor(ctx, store.NodeMoveKindApp, svc.Name)
	if err != nil {
		return ReplicaSpreadPlan{Refusal: "could not read the node selector: " + err.Error()}
	}
	req := placement.Request{
		Selector: sel,
		Needs:    serviceRequests(svc.Resources, 1),
		Policy:   rt.currentPlacementPolicy(ctx),
	}
	cands, req, err = rt.ruledRequest(ctx, store.NodeMoveKindApp, svc.Name, cands, req)
	if err != nil {
		return ReplicaSpreadPlan{Refusal: "could not read the placement rules: " + err.Error()}
	}
	current := make([]string, svc.Replicas)
	for i := range current {
		switch {
		case i < len(svc.ReplicaNodes):
			current[i] = rt.placementID(svc.ReplicaNodes[i])
		case i == 0:
			current[i] = rt.placementID(svc.NodeID)
		}
	}
	plan, err := placement.PlanReplicas(cands, req, svc.Replicas, current)
	if err != nil {
		if errors.Is(err, placement.ErrNoEligibleNode) {
			return ReplicaSpreadPlan{Refusal: fmt.Sprintf("no node can take a replica: %v", err)}
		}
		return ReplicaSpreadPlan{Refusal: err.Error()}
	}
	nodes := make([]string, len(plan))
	for i, id := range plan {
		if id == rt.localNodeID {
			id = ""
		}
		nodes[i] = id
	}
	if placement.SpreadsNodes(nodes) < 2 {
		return ReplicaSpreadPlan{Refusal: "only one node is eligible, so the replicas cannot spread"}
	}
	return ReplicaSpreadPlan{Nodes: nodes}
}

// placementID maps the service convention ("" is the control plane node) to a candidate id.
func (rt *Router) placementID(nodeID string) string {
	if nodeID == "" {
		return rt.localNodeID
	}
	return nodeID
}

// spreadRefusal names why svc must stay on one node, or returns "".
func spreadRefusal(svc store.DesiredService) string {
	if why := placement.SpreadBlock(len(svc.Volumes), len(svc.BindMounts)); why != "" {
		return why
	}
	if msg := remoteGrantRefusal(svc, "another node"); msg != "" {
		return msg
	}
	// A replica bound to loopback on another node cannot be dialed, so it would get no traffic.
	ip, err := bindaddr.Resolve(svc.BindAddress)
	if err != nil || net.ParseIP(ip).IsLoopback() {
		return "the app binds to a loopback address, so replicas on other nodes would get no traffic. Set bind_address to public or a mesh address"
	}
	return ""
}
