package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// osPatchLookback is how far back handleGetNodePatchStatus searches for
// HostPatchCollector's most recent sample: wide enough to survive a slow
// or just-restarted collector (defaultOSPatchCheckInterval is an hour)
// without losing the last real reading.
const osPatchLookback = 48 * time.Hour

// nodePatchStatusResponse is GET /api/v1/nodes/{id}/patch-status's wire
// shape. Checked distinguishes "never checked" (no sample yet: unknown
// package manager, or the collector hasn't run) from Total == 0 (checked,
// genuinely up to date); the two must never collapse into the same JSON,
// per HostPatchCollector's own degrade-to-unknown contract.
type nodePatchStatusResponse struct {
	Checked   bool       `json:"checked"`
	Total     int        `json:"total"`
	Security  int        `json:"security"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// handleGetNodePatchStatus handles GET /api/v1/nodes/{id}/patch-status:
// the latest OS-patch reading HostPatchCollector wrote for this node,
// read directly rather than through the generic /metrics query (this is
// a single current fact for a status indicator, not a chart).
func (rt *Router) handleGetNodePatchStatus(w http.ResponseWriter, r *http.Request) {
	if rt.telemetry == nil {
		writeError(w, http.StatusNotImplemented, "telemetry is not configured on this control plane")
		return
	}

	id := r.PathValue("id")
	guardID := rt.guardKey(id)
	if rt.isLocalGuardNode(id) {
		// The collector keys the control plane host by its mesh node id, so the
		// "local" alias must map to it like the patch preview and apply routes do.
		if rt.localNodeID != "" {
			id = rt.localNodeID
		}
	} else if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get node patch status: look up node failed", err, slog.String("node_id", id))
		return
	}

	now := time.Now()
	from := now.Add(-osPatchLookback)

	total, err := rt.telemetry.QueryMetrics(r.Context(), nodeResourceID(id), telemetry.MetricOSPatchesAvailable, from, now)
	if err != nil {
		rt.internalError(w, "api: get node patch status: query total failed", err, slog.String("node_id", id))
		return
	}
	if len(total) == 0 {
		if resp, ok := rt.patchStatusFromRun(guardID, time.Time{}); ok {
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeJSON(w, http.StatusOK, nodePatchStatusResponse{Checked: false})
		return
	}

	security, err := rt.telemetry.QueryMetrics(r.Context(), nodeResourceID(id), telemetry.MetricOSSecurityPatchesAvailable, from, now)
	if err != nil {
		rt.internalError(w, "api: get node patch status: query security failed", err, slog.String("node_id", id))
		return
	}

	securityCount := 0
	if len(security) > 0 {
		securityCount = int(security[len(security)-1].Value)
	}

	latest := total[len(total)-1]
	checkedAt := latest.Timestamp
	// The collector only samples hourly, so a finished run's own after-count is fresher.
	if resp, ok := rt.patchStatusFromRun(guardID, checkedAt); ok {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	writeJSON(w, http.StatusOK, nodePatchStatusResponse{
		Checked:   true,
		Total:     int(latest.Value),
		Security:  securityCount,
		CheckedAt: &checkedAt,
	})
}

// patchStatusFromRun returns the newest successful run's after-count when it
// finished later than the stored sample, so the status does not stay stale.
func (rt *Router) patchStatusFromRun(nodeKey string, newerThan time.Time) (nodePatchStatusResponse, bool) {
	p := rt.patcher()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, run := range p.runs[nodeKey] {
		if run.Status != patchRunSucceeded || run.PatchStatusAfter == nil || run.FinishedAt == nil {
			continue
		}
		if !run.FinishedAt.After(newerThan) {
			return nodePatchStatusResponse{}, false
		}
		at := *run.FinishedAt
		return nodePatchStatusResponse{Checked: true, Total: run.PatchStatusAfter.Total, Security: run.PatchStatusAfter.Security, CheckedAt: &at}, true
	}
	return nodePatchStatusResponse{}, false
}
