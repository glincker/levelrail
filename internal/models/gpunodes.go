package models

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

// LocalNodeName labels the control plane's own host in GPU listings.
const LocalNodeName = "local"

// GPUNode is one node's GPU snapshot plus the models placed on it.
type GPUNode struct {
	NodeID     string
	Name       string
	IsLocal    bool
	Info       gpu.Info
	UpdatedAt  time.Time
	ModelCount int
	// PlacementID is the node ID as services and models record it: "" for
	// the control plane's own host.
	PlacementID string
	Schedulable bool
	Online      bool
	Claims      []gpu.Claim
}

// Ledger returns the node's reservations, leaving out the claim named
// exclude so a workload can be re-checked against its own node.
func (n GPUNode) Ledger(exclude string) gpu.Ledger {
	claims := make([]gpu.Claim, 0, len(n.Claims))
	for _, c := range n.Claims {
		if c.Name != exclude {
			claims = append(claims, c)
		}
	}
	return gpu.NewLedger(n.Info, claims)
}

// Eligible reports whether new workloads may be placed on the node.
func (n GPUNode) Eligible() bool { return n.Schedulable && n.Online }

// ServiceClaimName is the reservation label of an app.
func ServiceClaimName(name string) string { return "app:" + name }

// ModelClaimName is the reservation label of a model.
func ModelClaimName(name string) string { return "model:" + name }

// ModelClaim is the GPU reservation of a model.
func ModelClaim(m store.Model) gpu.Claim {
	return gpu.Claim{Name: ModelClaimName(m.Name), Count: m.GPUCount, DeviceIDs: m.GPUDeviceIDs}
}

// ServiceClaim is the GPU reservation of an app, ok false when it asks
// for none.
func ServiceClaim(svc store.DesiredService) (gpu.Claim, bool) {
	if svc.Resources == nil || svc.Resources.GPU == nil {
		return gpu.Claim{}, false
	}
	g := svc.Resources.GPU
	return gpu.Claim{Name: ServiceClaimName(svc.Name), Count: g.Count, DeviceIDs: g.DeviceIDs}, true
}

// GPUNodes lists every node that has reported a GPU snapshot, the local
// host first.
func (s *Service) GPUNodes(ctx context.Context) ([]GPUNode, error) {
	snaps, err := s.store.ListNodeGPUs(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list node gpus: %w", err)
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list nodes: %w", err)
	}
	list, err := s.store.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list models: %w", err)
	}
	services, err := s.store.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list services: %w", err)
	}
	counts := map[string]int{}
	claims := map[string][]gpu.Claim{}
	for _, m := range list {
		if m.Deleting {
			continue
		}
		id := s.placementID(m.NodeID)
		counts[id]++
		claims[id] = append(claims[id], ModelClaim(m))
	}
	for _, svc := range services {
		if c, ok := ServiceClaim(svc); ok {
			id := s.placementID(svc.NodeID)
			claims[id] = append(claims[id], c)
		}
	}

	var out []GPUNode
	if snap, ok := snaps[store.LocalNodeGPUKey]; ok {
		out = append(out, GPUNode{NodeID: s.localID, Name: LocalNodeName, IsLocal: true, Info: snap.Info, UpdatedAt: snap.UpdatedAt,
			ModelCount: counts[""], Schedulable: true, Online: true, Claims: claims[""]})
	}
	remote := make([]GPUNode, 0, len(nodes))
	for _, n := range nodes {
		snap, ok := snaps[n.ID]
		if !ok || n.ID == s.localID {
			continue
		}
		remote = append(remote, GPUNode{NodeID: n.ID, PlacementID: n.ID, Name: n.Name, Info: snap.Info, UpdatedAt: snap.UpdatedAt,
			ModelCount: counts[n.ID], Schedulable: n.Schedulable, Online: n.Status == store.NodeStatusOnline, Claims: claims[n.ID]})
	}
	sort.Slice(remote, func(i, j int) bool { return remote[i].Name < remote[j].Name })
	return append(out, remote...), nil
}

func (s *Service) placementID(nodeID string) string {
	if nodeID == s.localID {
		return ""
	}
	return nodeID
}

// Unplaced is a GPU workload no reported node can host right now.
type Unplaced struct {
	Kind   string
	Name   string
	NodeID string
	Reason string
}

// UnplacedWorkloads lists GPU apps and models whose own node cannot serve
// them and for which no other eligible GPU node has enough free GPUs.
func (s *Service) UnplacedWorkloads(ctx context.Context) ([]Unplaced, error) {
	nodes, err := s.GPUNodes(ctx)
	if err != nil {
		return nil, err
	}
	byPlacement := make(map[string]GPUNode, len(nodes))
	for _, n := range nodes {
		byPlacement[n.PlacementID] = n
	}
	type workload struct {
		kind, name, nodeID string
		claim              gpu.Claim
	}
	var work []workload
	list, err := s.store.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list models: %w", err)
	}
	for _, m := range list {
		if !m.Deleting {
			work = append(work, workload{"model", m.Name, s.placementID(m.NodeID), ModelClaim(m)})
		}
	}
	services, err := s.store.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list services: %w", err)
	}
	for _, svc := range services {
		if c, ok := ServiceClaim(svc); ok {
			work = append(work, workload{"app", svc.Name, s.placementID(svc.NodeID), c})
		}
	}

	var out []Unplaced
	for _, w := range work {
		own, known := byPlacement[w.nodeID]
		if !known {
			own = GPUNode{}
		}
		ownErr := own.Ledger(w.claim.Name).Fit(w.claim)
		if ownErr == nil {
			continue
		}
		if s.anyNodeFits(nodes, w.claim) {
			continue
		}
		out = append(out, Unplaced{Kind: w.kind, Name: w.name, NodeID: w.nodeID, Reason: ownErr.Error()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+out[i].Name < out[j].Kind+out[j].Name })
	return out, nil
}

func (s *Service) anyNodeFits(nodes []GPUNode, c gpu.Claim) bool {
	for _, n := range nodes {
		if n.Eligible() && n.Ledger(c.Name).Fit(c) == nil {
			return true
		}
	}
	return false
}
