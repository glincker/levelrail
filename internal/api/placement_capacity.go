package api

import (
	"context"
	"runtime"

	"github.com/glincker/levelrail/internal/placement"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

// placementInventory is the registered nodes plus what is already placed on them.
type placementInventory struct {
	nodes     []store.Node
	counts    map[string]int
	requested map[string]placement.Resources
}

func (rt *Router) loadPlacementInventory(ctx context.Context) (placementInventory, error) {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return placementInventory{}, err
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return placementInventory{}, err
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return placementInventory{}, err
	}
	databases = rt.withoutPendingDatabases(ctx, databases)
	inv := placementInventory{nodes: nodes, counts: make(map[string]int, len(nodes)), requested: make(map[string]placement.Resources, len(nodes))}
	for _, s := range services {
		inv.counts[s.NodeID]++
		inv.requested[s.NodeID] = inv.requested[s.NodeID].Add(serviceRequests(s.Resources, s.Replicas))
	}
	for _, d := range databases {
		inv.counts[d.NodeID]++
	}
	return inv, nil
}

// serviceRequests is what a service asks for across all its replicas.
func serviceRequests(res *store.ServiceResources, replicas int) placement.Resources {
	if res == nil {
		return placement.Resources{}
	}
	return placement.Resources{CPUMilli: res.NanoCPUs / 1_000_000, MemoryBytes: res.MemoryBytes}.Times(int64(max(replicas, 1)))
}

// nodeCapacity is what node reports about itself; anything it did not
// report stays unknown rather than becoming zero.
func (rt *Router) nodeCapacity(nodeID string) placement.Capacity {
	var c placement.Capacity
	if rt.localNodeID != "" && nodeID == rt.localNodeID {
		cores := rt.doctorNumCPU
		if cores == nil {
			cores = runtime.NumCPU
		}
		if n := cores(); n > 0 {
			c.CPUMilli = placement.Measured(int64(n) * 1000)
		}
		if total, _, err := telemetry.HostMemoryBytes(); err == nil && total > 0 {
			c.MemoryBytes = placement.Measured(total)
		}
		if rt.dataDir != "" {
			if total, used, err := telemetry.HostDiskBytes(rt.dataDir); err == nil && total > 0 {
				c.DiskTotalBytes, c.DiskFreeBytes = placement.Measured(total), placement.Measured(max(total-used, 0))
			}
		}
		return c
	}
	if rt.nodeHostReports == nil {
		return c
	}
	rep, ok := rt.nodeHostReports.Get(nodeID)
	if !ok {
		return c
	}
	if rep.CPUCores > 0 {
		c.CPUMilli = placement.Measured(int64(rep.CPUCores) * 1000)
	}
	if rep.MemoryTotalBytes > 0 {
		c.MemoryBytes = placement.Measured(int64(min(rep.MemoryTotalBytes, 1<<62))) //nolint:gosec // clamped to int64 range
	}
	if rep.DiskTotalBytes > 0 {
		c.DiskTotalBytes = placement.Measured(int64(min(rep.DiskTotalBytes, 1<<62))) //nolint:gosec // clamped to int64 range
		c.DiskFreeBytes = placement.Measured(int64(min(rep.DiskFreeBytes, 1<<62)))   //nolint:gosec // clamped to int64 range
	}
	return c
}

func (rt *Router) candidatesFrom(ctx context.Context, inv placementInventory) ([]placement.Candidate, error) {
	labels := map[string]map[string]string{}
	if rt.multiNode != nil {
		var err error
		if labels, err = rt.multiNode.store.ListNodeLabels(ctx); err != nil {
			return nil, err
		}
	}
	var taints map[string][]store.NodeTaint
	if rt.multiNode != nil {
		var err error
		if taints, err = rt.multiNode.store.ListNodeTaints(ctx); err != nil {
			return nil, err
		}
	}
	out := make([]placement.Candidate, 0, len(inv.nodes))
	for _, n := range inv.nodes {
		load, requested := inv.counts[n.ID], inv.requested[n.ID]
		if n.ID == rt.localNodeID {
			load += inv.counts[""]
			requested = requested.Add(inv.requested[""])
		}
		nodeLabels := withDomainLabels(labels[n.ID], n)
		out = append(out, placement.Candidate{
			ID: n.ID, Name: n.Name, Labels: nodeLabels, Schedulable: n.Schedulable && rt.nodeBlockReason(ctx, n.ID) == "",
			Online: n.Status == store.NodeStatusOnline, Load: load,
			Capacity: rt.nodeCapacity(n.ID), Requested: requested, Taints: toPlacementTaints(taints[n.ID]),
		})
	}
	return out, nil
}

// withDomainLabels fills region, provider and zone from the node row unless
// an explicit node label of the same key overrides them.
func withDomainLabels(labels map[string]string, n store.Node) map[string]string {
	fill := map[string]string{placement.LabelRegion: n.Region, placement.LabelProvider: n.Provider, placement.LabelZone: n.Zone}
	var out map[string]string
	for k, v := range fill {
		if _, ok := labels[k]; ok || v == "" {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(labels)+len(fill))
			for lk, lv := range labels {
				out[lk] = lv
			}
		}
		out[k] = v
	}
	if out == nil {
		return labels
	}
	return out
}

// nodeReservation compares what apps reserved on a node with what the host
// actually uses, so stranded memory is visible before any repack.
type nodeReservation struct {
	NodeID              string `json:"node_id"`
	Name                string `json:"name"`
	CPUMilliReserved    int64  `json:"cpu_milli_reserved"`
	CPUMilliCapacity    int64  `json:"cpu_milli_capacity,omitempty"`
	MemoryReservedBytes int64  `json:"memory_reserved_bytes"`
	MemoryCapacityBytes int64  `json:"memory_capacity_bytes,omitempty"`
	MemoryUsedBytes     int64  `json:"memory_used_bytes,omitempty"`
	// MemoryStrandedBytes is reserved memory the host is not using; zero when usage is unknown.
	MemoryStrandedBytes int64 `json:"memory_stranded_bytes"`
}

func (rt *Router) nodeReservations(cands []placement.Candidate) []nodeReservation {
	out := make([]nodeReservation, 0, len(cands))
	for _, c := range cands {
		r := nodeReservation{NodeID: c.ID, Name: c.Name, CPUMilliReserved: c.Requested.CPUMilli, MemoryReservedBytes: c.Requested.MemoryBytes}
		if c.Capacity.CPUMilli.Known {
			r.CPUMilliCapacity = c.Capacity.CPUMilli.Value
		}
		if c.Capacity.MemoryBytes.Known {
			r.MemoryCapacityBytes = c.Capacity.MemoryBytes.Value
		}
		if s, ok := rt.nodeSample(c.ID); ok && s.MemTotal > 0 {
			r.MemoryUsedBytes = int64(min(s.MemTotal-min(s.MemAvail, s.MemTotal), 1<<62)) //nolint:gosec // clamped to int64 range
			r.MemoryStrandedBytes = max(r.MemoryReservedBytes-r.MemoryUsedBytes, 0)
		}
		out = append(out, r)
	}
	return out
}
