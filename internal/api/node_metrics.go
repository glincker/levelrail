package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// nodeResourceID is the identifier this handler stamps onto the summed,
// computed-on-the-fly series SumAcrossResources returns for
// nodeSummableMetrics, and the real metric_samples resource ID
// cmd/levelrail's HostDiskCollector writes nodeHostMetrics samples
// under directly: unlike the summed metrics, a host disk reading is
// already one real per-node measurement, not an aggregate computed at
// query time.
func nodeResourceID(nodeID string) string {
	return "node:" + nodeID
}

// nodeSummableMetrics is exactly the subset of MetricName
// (web/src/types/metrics.ts) it is honest to sum across every container
// placed on one node. memory_limit_bytes is deliberately excluded: an
// unconstrained container's reported limit is (on typical cgroup
// configurations) approximately the host's total memory, not "0" or
// "unset", so summing it across N containers on the same node would
// multiply that number by N rather than approach host capacity. See
// handleQueryNodeMetrics' own doc comment for the fuller "sum of
// containers, not true host utilization" framing this endpoint commits
// to instead.
var nodeSummableMetrics = map[string]bool{
	"cpu_percent":        true,
	"memory_usage_bytes": true,
	"network_rx_bytes":   true,
	"network_tx_bytes":   true,
	"disk_read_bytes":    true,
	"disk_write_bytes":   true,
}

// nodeHostMetrics are real host-level readings (disk capacity, available
// OS package updates: cmd/levelrail.HostDiskCollector/HostPatchCollector,
// internal/telemetry/hostdisk.go/hostpatch.go), queried directly by this
// node's own resource ID rather than summed across placed services like
// nodeSummableMetrics above, because there is exactly one real reading
// per node, not many containers to add together.
var nodeHostMetrics = map[string]bool{
	telemetry.MetricDiskUsedBytes:              true,
	telemetry.MetricDiskTotalBytes:             true,
	telemetry.MetricOSPatchesAvailable:         true,
	telemetry.MetricOSSecurityPatchesAvailable: true,
}

// nodeMetricsResponse mirrors metricsResponse (metrics.go) plus
// ResourceCount, surfaced so the dashboard can honestly label what
// "total" means here rather than implying a host-level reading: how
// many of the node's placed services actually contributed a sample to
// this response, not how many are placed there in total (a service
// with no running container in range contributes nothing to either
// number).
type nodeMetricsResponse struct {
	Metric        string        `json:"metric"`
	Points        []metricPoint `json:"points"`
	ResourceCount int           `json:"resource_count"`
}

// handleQueryNodeMetrics handles GET /api/v1/nodes/{id}/metrics. Query
// params mirror handleQueryMetrics (metrics.go) exactly: metric
// (required, one of nodeSummableMetrics), from/to (RFC3339, default
// now-1h to now), step (a Go duration string, raw samples if omitted).
//
// What "node-level" means here, spelled out because it is genuinely
// different from what a host monitoring agent would report: this is the
// sum of internal/telemetry's already-collected per-container samples
// (collector.go) for every service currently placed on this node
// (store.DesiredService.NodeID == id, resolved via
// ListDesiredServicesByNode), not a read of the host's real free/total
// memory or CPU. internal/agent has no host-level stats collection
// today, no /proc reads, no host-info gRPC message in
// internal/agent/agentpb, and internal/docker.ContainerStats (stats.go)
// is entirely per-container by design; adding true host-level
// monitoring is real, separate agent-side work, out of scope here.
// Databases are collected now (cmd/levelrail's telemetryTargets, see
// database_metrics.go for the per-database query endpoint) but are still
// excluded from this sum: the loop below resolves placed resources via
// ListDesiredServicesByNode, services only, so a database placed on this
// node contributes nothing to the node-level total. Folding it in is a
// real, separate change (a second ListDesiredDatabasesByNode loop plus
// resourceIDForDatabase lookups), not attempted here.
func (rt *Router) handleQueryNodeMetrics(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: query node metrics: load node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	metric := r.URL.Query().Get("metric")
	if metric == "" {
		writeError(w, http.StatusBadRequest, "metric query parameter is required")
		return
	}
	if !nodeSummableMetrics[metric] && !nodeHostMetrics[metric] {
		writeError(w, http.StatusBadRequest,
			"metric must be one of cpu_percent, memory_usage_bytes, network_rx_bytes, network_tx_bytes, "+
				"disk_read_bytes, disk_write_bytes, "+telemetry.MetricDiskUsedBytes+", "+telemetry.MetricDiskTotalBytes+", "+telemetry.MetricOSPatchesAvailable+
				", "+telemetry.MetricOSSecurityPatchesAvailable+" (memory_limit_bytes cannot be "+
				"honestly summed across containers on one node, see handleQueryNodeMetrics's doc comment)")
		return
	}

	from, to, err := parseTimeRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	step, err := parseStep(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if nodeHostMetrics[metric] {
		rt.writeNodeHostMetric(w, r, id, metric, from, to, step)
		return
	}

	services, err := rt.apps.ListDesiredServicesByNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: query node metrics: list placed services failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	all, resourceCount := rt.queryPlacedServiceSamples(r.Context(), id, metric, services, from, to)

	summed := telemetry.SumAcrossResources(nodeResourceID(id), metric, all)
	aggregated := telemetry.Aggregate(summed, from, step)
	points := make([]metricPoint, len(aggregated))
	for i, a := range aggregated {
		points[i] = metricPoint{Timestamp: a.Timestamp, Value: a.Value, Count: a.Count}
	}

	writeJSON(w, http.StatusOK, nodeMetricsResponse{Metric: metric, Points: points, ResourceCount: resourceCount})
}

// queryPlacedServiceSamples fans metric out across every service placed on
// this node and concatenates the results. A federated query per service can
// partially fail the same way a single app's own query can (ADR 008); with
// no per-service error detail worth surfacing to the caller (the returned
// resourceCount already tells the frontend how many services contributed),
// a partial failure is logged as a warning here rather than returned as an
// error.
func (rt *Router) queryPlacedServiceSamples(ctx context.Context, nodeID, metric string, services []store.DesiredService, from, to time.Time) ([]telemetry.Sample, int) {
	var all []telemetry.Sample
	resourceCount := 0
	var queryErrCount int
	for _, svc := range services {
		samples, err := rt.telemetry.QueryMetrics(ctx, resourceIDForApp(svc.Name), metric, from, to)
		if err != nil {
			queryErrCount++
		}
		if len(samples) > 0 {
			resourceCount++
			all = append(all, samples...)
		}
	}
	if queryErrCount > 0 {
		rt.logger.Warn("api: query node metrics: partial result",
			slog.String("node_id", nodeID), slog.String("metric", metric), slog.Int("failed_services", queryErrCount))
	}
	return all, resourceCount
}

// writeNodeHostMetric answers a nodeHostMetrics query: unlike the sum-
// across-placed-services path above, this is a direct read of the one
// real sample series HostDiskCollector writes under nodeResourceID(id),
// so there is nothing to fan out or sum. ResourceCount is 1 when the
// range has any data and 0 otherwise, purely so the wire shape matches
// nodeMetricsResponse; the frontend doesn't read it for this metric
// group (NodeMetricsDashboard.tsx never captions a disk chart with a
// container count).
func (rt *Router) writeNodeHostMetric(w http.ResponseWriter, r *http.Request, id, metric string, from, to time.Time, step time.Duration) {
	samples, err := rt.telemetry.QueryMetrics(r.Context(), nodeResourceID(id), metric, from, to)
	if err != nil {
		rt.logger.Error("api: query node metrics: host disk query failed",
			slog.String("error", err.Error()), slog.String("node_id", id), slog.String("metric", metric))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	aggregated := telemetry.Aggregate(samples, from, step)
	points := make([]metricPoint, len(aggregated))
	for i, a := range aggregated {
		points[i] = metricPoint{Timestamp: a.Timestamp, Value: a.Value, Count: a.Count}
	}

	resourceCount := 0
	if len(samples) > 0 {
		resourceCount = 1
	}
	writeJSON(w, http.StatusOK, nodeMetricsResponse{Metric: metric, Points: points, ResourceCount: resourceCount})
}
