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
	counts := map[string]int{}
	for _, m := range list {
		if m.Deleting {
			continue
		}
		id := m.NodeID
		if id == s.localID {
			id = ""
		}
		counts[id]++
	}

	var out []GPUNode
	if snap, ok := snaps[store.LocalNodeGPUKey]; ok {
		out = append(out, GPUNode{NodeID: s.localID, Name: LocalNodeName, IsLocal: true, Info: snap.Info, UpdatedAt: snap.UpdatedAt, ModelCount: counts[""]})
	}
	remote := make([]GPUNode, 0, len(nodes))
	for _, n := range nodes {
		snap, ok := snaps[n.ID]
		if !ok || n.ID == s.localID {
			continue
		}
		remote = append(remote, GPUNode{NodeID: n.ID, Name: n.Name, Info: snap.Info, UpdatedAt: snap.UpdatedAt, ModelCount: counts[n.ID]})
	}
	sort.Slice(remote, func(i, j int) bool { return remote[i].Name < remote[j].Name })
	return append(out, remote...), nil
}
