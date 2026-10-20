package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/janitor"
)

const errCodeLowDisk = "low_disk"

// SetDiskGuard enables the low-disk refusals and GET /api/v1/resilience/disk.
func (rt *Router) SetDiskGuard(g *diskguard.Guard) { rt.diskGuard = g }

// refuseLowDisk writes a 507 with the low_disk code and reports true when op must not start.
func (rt *Router) refuseLowDisk(w http.ResponseWriter, op diskguard.Op) bool {
	err := rt.diskGuard.Refuse(op)
	if err == nil {
		return false
	}
	var low *diskguard.LowDiskError
	if errors.As(err, &low) {
		writeCodedError(w, http.StatusInsufficientStorage, errCodeLowDisk, low.Error())
		return true
	}
	return false
}

type diskResilienceResponse struct {
	diskguard.Status
	Refusing  bool `json:"refusing"`
	FixAction bool `json:"free_space_available"`
}

func (rt *Router) diskResponse() diskResilienceResponse {
	st := rt.diskGuard.Status()
	return diskResilienceResponse{Status: st, Refusing: st.Worst == diskguard.LevelCritical, FixAction: rt.janitor != nil && rt.janitorStore != nil}
}

// handleGetDiskResilience handles GET /api/v1/resilience/disk.
func (rt *Router) handleGetDiskResilience(w http.ResponseWriter, _ *http.Request) {
	if rt.diskGuard == nil {
		writeError(w, http.StatusNotImplemented, "disk monitoring is not configured on this control plane")
		return
	}
	writeJSON(w, http.StatusOK, rt.diskResponse())
}

type freeSpaceResponse struct {
	ReclaimedBytes int64                  `json:"reclaimed_bytes"`
	Disk           diskResilienceResponse `json:"disk"`
}

// handleFreeDiskSpace handles POST /api/v1/resilience/disk/free-space by running a forced janitor pass, the Fix for a low-disk refusal.
func (rt *Router) handleFreeDiskSpace(w http.ResponseWriter, r *http.Request) {
	if rt.diskGuard == nil {
		writeError(w, http.StatusNotImplemented, "disk monitoring is not configured on this control plane")
		return
	}
	if !rt.janitorReady(w) {
		return
	}
	rep, err := rt.janitor.Run(r.Context(), janitor.Options{Trigger: janitor.TriggerManual, Force: true})
	if errors.Is(err, janitor.ErrBusy) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: free disk space failed", err, slog.String("trigger", janitor.TriggerManual))
		return
	}
	writeJSON(w, http.StatusOK, freeSpaceResponse{ReclaimedBytes: rep.ReclaimedBytes, Disk: rt.diskResponse()})
}
