package api

import (
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// nodeResourceUsagePlacedMetrics is the subset of nodeSummableMetrics
// (node_metrics.go) this endpoint reads a latest-snapshot of, summed per
// node from each placed service's own newest sample, not a time series.
var nodeResourceUsagePlacedMetrics = []string{
	"cpu_percent",
	"memory_usage_bytes",
}

// nodeResourceUsageHostMetrics are the real host-level readings
// (nodeHostMetrics in node_metrics.go, minus the OS-patch counters,
// which don't belong in a utilization view) this endpoint reads
// directly under nodeResourceID(id) rather than summing.
// HostDiskCollector/HostMemoryCollector only run for this process's own
// host; a remote node's memory capacity comes from its agent's host
// report instead (applyRemoteMemoryAttribution), and its disk fields
// stay nil.
var nodeResourceUsageHostMetrics = []string{
	telemetry.MetricMemoryTotalBytes,
	telemetry.MetricMemoryAvailableBytes,
	telemetry.MetricDiskUsedBytes,
	telemetry.MetricDiskTotalBytes,
}

// nodeResourceUsageResource is one node's latest-known utilization, the
// wire shape behind the fleet-wide "how full are my servers" view (the
// dashboard summary card and the node list columns). CPUPercent/
// MemoryUsageBytes are the sum of every placed service's latest sample,
// the same "sum of containers, not a true host read" contract
// handleQueryNodeMetrics' own doc comment establishes for the time-series
// version of this same number, now collected for a remote node too (see
// nodeResourceUsagePlacedMetrics). MemoryTotalBytes/DiskUsedBytes/
// DiskTotalBytes are real host reads, still only populated for the node
// running the control plane itself (nodeResourceUsageHostMetrics): a
// field stays nil rather than reporting a number that would be silently
// wrong for a node with no host-metrics collector.
type nodeResourceUsageResource struct {
	NodeID           string   `json:"node_id"`
	Name             string   `json:"name"`
	IsLocal          bool     `json:"is_local"`
	CPUPercent       *float64 `json:"cpu_percent,omitempty"`
	MemoryUsageBytes *float64 `json:"memory_usage_bytes,omitempty"`
	MemoryTotalBytes *float64 `json:"memory_total_bytes,omitempty"`
	// MemoryAvailableBytes is the host's available memory, reclaimable
	// caches included.
	MemoryAvailableBytes *float64 `json:"memory_available_bytes,omitempty"`
	DiskUsedBytes        *float64 `json:"disk_used_bytes,omitempty"`
	DiskTotalBytes       *float64 `json:"disk_total_bytes,omitempty"`
	// LevelrailMemoryBytes is the control plane process RSS (embedded
	// ingress included). A remote node reports its agent process RSS.
	LevelrailMemoryBytes *float64 `json:"levelrail_memory_bytes,omitempty"`
	// OtherContainersMemoryBytes is running containers Levelrail does not
	// manage; OtherContainers lists the largest few.
	OtherContainersMemoryBytes *float64                  `json:"other_containers_memory_bytes,omitempty"`
	OtherContainers            []unmanagedContainerUsage `json:"other_containers,omitempty"`
	// SystemMemoryBytes is the used remainder: kernel, cache, other processes.
	SystemMemoryBytes *float64 `json:"system_memory_bytes,omitempty"`
}

// fleetResourceUsageRollup sums nodeResourceUsageResource across every
// node in the fleet. The *Percent fields are only computed from nodes
// that actually reported a capacity figure (see NodesWithMemoryCapacity/
// NodesWithDiskCapacity): summing a missing capacity as zero would
// silently understate the denominator and inflate the percentage, so a
// caller that ignores those counts and shows e.g. disk_used_percent as
// "fleet-wide" when only one of three nodes actually reported disk
// capacity would be showing a real but misleading number. The frontend
// must surface the coverage counts alongside the percentage, not just
// the percentage alone.
type fleetResourceUsageRollup struct {
	NodeCount int `json:"node_count"`
	// TotalCPUPercent is the sum of every node's CPUPercent: a raw total
	// (each node's own containers can already exceed 100% on a
	// multi-core host), not a 0-100 percentage of fleet capacity, since
	// core count per node isn't tracked anywhere in this codebase yet.
	TotalCPUPercent         *float64 `json:"total_cpu_percent,omitempty"`
	TotalMemoryUsageBytes   *float64 `json:"total_memory_usage_bytes,omitempty"`
	TotalMemoryBytes        *float64 `json:"total_memory_bytes,omitempty"`
	MemoryUsedPercent       *float64 `json:"memory_used_percent,omitempty"`
	NodesWithMemoryCapacity int      `json:"nodes_with_memory_capacity"`
	TotalDiskUsedBytes      *float64 `json:"total_disk_used_bytes,omitempty"`
	TotalDiskBytes          *float64 `json:"total_disk_bytes,omitempty"`
	DiskUsedPercent         *float64 `json:"disk_used_percent,omitempty"`
	NodesWithDiskCapacity   int      `json:"nodes_with_disk_capacity"`
}

// fleetResourceUsageResponse is GET /api/v1/nodes/resource-usage's full
// body: one row per node (sorted by name, matching handleListNodes) plus
// the fleet-wide rollup.
type fleetResourceUsageResponse struct {
	Nodes []nodeResourceUsageResource `json:"nodes"`
	Fleet fleetResourceUsageRollup    `json:"fleet"`
}

// handleFleetResourceUsage handles GET /api/v1/nodes/resource-usage: the
// latest CPU/memory/disk reading for every node in one response, so the
// dashboard and node list can show fleet-wide utilization without one
// metrics query per node per page load (the same N+1 shape
// handleAppResourceUsage already avoids for apps, section 4.12's "never
// fetch the full resource graph on page load" rule).
func (rt *Router) handleFleetResourceUsage(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		rt.internalError(w, "api: fleet resource usage: list nodes failed", err)
		return
	}

	byID := make(map[string]*nodeResourceUsageResource, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = &nodeResourceUsageResource{
			NodeID:  n.ID,
			Name:    n.Name,
			IsLocal: n.ID == rt.localNodeID,
		}
	}

	rt.ensureLocalUsageRow(byID)

	services, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: fleet resource usage: list services failed", err)
		return
	}
	// nodeForService resolves the empty-string local-node sentinel
	// (store.DesiredService.NodeID's own doc comment,
	// validatePlacementTarget in nodes.go) to rt.localNodeID, the same
	// normalization toNodeResource's IsLocal comparison relies on, so a
	// service placed without an explicit node still attributes its usage
	// to the local node's row.
	nodeForService := make(map[string]string, len(services))
	for _, svc := range services {
		nodeID := svc.NodeID
		if nodeID == "" {
			nodeID = rt.localNodeID
		}
		nodeForService[svc.Name] = nodeID
	}

	for _, metric := range nodeResourceUsagePlacedMetrics {
		samples, err := rt.latestByMetric(r.Context(), metric)
		if err != nil {
			rt.logger.Warn("api: fleet resource usage: query placed metric failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			name, ok := strings.CutPrefix(s.ResourceID, "service:")
			if !ok {
				continue
			}
			nodeID, tracked := nodeForService[name]
			if !tracked {
				continue // stale telemetry for a service that no longer exists
			}
			usage, onFleet := byID[nodeID]
			if !onFleet {
				continue // placed on a node that no longer exists (or the local sentinel with no mesh-bootstrapped row)
			}
			value := s.Value
			switch metric {
			case "cpu_percent":
				addFloat(&usage.CPUPercent, value)
			case "memory_usage_bytes":
				addFloat(&usage.MemoryUsageBytes, value)
			}
		}
	}

	for _, metric := range nodeResourceUsageHostMetrics {
		samples, err := rt.latestByMetric(r.Context(), metric)
		if err != nil {
			rt.logger.Warn("api: fleet resource usage: query host metric failed", slog.String("metric", metric), slog.String("error", err.Error()))
			continue
		}
		for _, s := range samples {
			nodeID, ok := strings.CutPrefix(s.ResourceID, "node:")
			if !ok {
				continue
			}
			usage, onFleet := byID[nodeID]
			if !onFleet {
				continue
			}
			value := s.Value
			switch metric {
			case telemetry.MetricMemoryTotalBytes:
				usage.MemoryTotalBytes = &value
			case telemetry.MetricMemoryAvailableBytes:
				usage.MemoryAvailableBytes = &value
			case telemetry.MetricDiskUsedBytes:
				usage.DiskUsedBytes = &value
			case telemetry.MetricDiskTotalBytes:
				usage.DiskTotalBytes = &value
			}
		}
	}

	for id, usage := range byID {
		if id == rt.localNodeID {
			rt.applyMemoryAttribution(r.Context(), usage)
			continue
		}
		rt.applyRemoteMemoryAttribution(usage)
	}

	out := make([]nodeResourceUsageResource, 0, len(byID))
	for _, usage := range byID {
		out = append(out, *usage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	writeJSON(w, http.StatusOK, fleetResourceUsageResponse{
		Nodes: out,
		Fleet: rollupFleetResourceUsage(out),
	})
}

// addFloat adds value into the float64 *dst points to, allocating dst on
// first write: nodeResourceUsagePlacedMetrics' loop calls this once per
// service placed on a node, so a node's CPUPercent/MemoryUsageBytes
// field is the running sum across every service it hosts, absent only
// when not one of them has ever reported that metric.
func addFloat(dst **float64, value float64) {
	if *dst == nil {
		v := value
		*dst = &v
		return
	}
	**dst += value
}

// rollupFleetResourceUsage sums nodeResourceUsageResource across the
// fleet. See fleetResourceUsageRollup's own doc comment for why the
// *Percent fields are gated on NodesWithMemoryCapacity/
// NodesWithDiskCapacity rather than computed unconditionally.
func rollupFleetResourceUsage(nodes []nodeResourceUsageResource) fleetResourceUsageRollup {
	roll := fleetResourceUsageRollup{NodeCount: len(nodes)}
	// memUsedForPercent/memTotal only accumulate over capacity-reporting
	// nodes, kept separate from roll.TotalMemoryUsageBytes (the genuine
	// fleet-wide container-memory sum, populated for every node with a
	// placed-service reading regardless of whether that node's host
	// capacity is known) so the percentage's numerator and denominator
	// stay scoped to the same node set instead of the percent silently
	// using a wider numerator than its own denominator covers.
	var memUsedForPercent, memTotal, diskUsed, diskTotal float64

	for _, n := range nodes {
		if n.CPUPercent != nil {
			addFloat(&roll.TotalCPUPercent, *n.CPUPercent)
		}
		if n.MemoryUsageBytes != nil {
			addFloat(&roll.TotalMemoryUsageBytes, *n.MemoryUsageBytes)
		}
		if n.MemoryTotalBytes != nil {
			memTotal += *n.MemoryTotalBytes
			roll.NodesWithMemoryCapacity++
			if n.MemoryUsageBytes != nil {
				memUsedForPercent += *n.MemoryUsageBytes
			}
		}
		if n.DiskTotalBytes != nil {
			diskTotal += *n.DiskTotalBytes
			roll.NodesWithDiskCapacity++
			if n.DiskUsedBytes != nil {
				diskUsed += *n.DiskUsedBytes
			}
		}
	}

	if roll.NodesWithMemoryCapacity > 0 {
		roll.TotalMemoryBytes = &memTotal
		if memTotal > 0 {
			pct := memUsedForPercent / memTotal * 100
			roll.MemoryUsedPercent = &pct
		}
	}
	if roll.NodesWithDiskCapacity > 0 {
		roll.TotalDiskBytes = &diskTotal
		diskUsedCopy := diskUsed
		roll.TotalDiskUsedBytes = &diskUsedCopy
		if diskTotal > 0 {
			pct := diskUsed / diskTotal * 100
			roll.DiskUsedPercent = &pct
		}
	}

	return roll
}

// ensureLocalUsageRow adds the control plane's own host when no node row
// exists for it, which is the normal single-node state, so its capacity and
// the services placed on it are not dropped from the fleet view.
func (rt *Router) ensureLocalUsageRow(byID map[string]*nodeResourceUsageResource) {
	if rt.localNodeID == "" {
		return
	}
	if _, ok := byID[rt.localNodeID]; ok {
		return
	}
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = rt.localNodeID
	}
	byID[rt.localNodeID] = &nodeResourceUsageResource{NodeID: rt.localNodeID, Name: name, IsLocal: true}
}
