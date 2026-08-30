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
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get node patch status: look up node failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	now := time.Now()
	from := now.Add(-osPatchLookback)

	total, err := rt.telemetry.QueryMetrics(r.Context(), nodeResourceID(id), telemetry.MetricOSPatchesAvailable, from, now)
	if err != nil {
		rt.logger.Error("api: get node patch status: query total failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}
	if len(total) == 0 {
		writeJSON(w, http.StatusOK, nodePatchStatusResponse{Checked: false})
		return
	}

	security, err := rt.telemetry.QueryMetrics(r.Context(), nodeResourceID(id), telemetry.MetricOSSecurityPatchesAvailable, from, now)
	if err != nil {
		rt.logger.Error("api: get node patch status: query security failed", slog.String("error", err.Error()), slog.String("node_id", id))
		writeError(w, http.StatusInternalServerError, errInternal)
		return
	}

	securityCount := 0
	if len(security) > 0 {
		securityCount = int(security[len(security)-1].Value)
	}

	latest := total[len(total)-1]
	checkedAt := latest.Timestamp
	writeJSON(w, http.StatusOK, nodePatchStatusResponse{
		Checked:   true,
		Total:     int(latest.Value),
		Security:  securityCount,
		CheckedAt: &checkedAt,
	})
}
