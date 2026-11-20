package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/store"
)

// BackupTargetDrill writes a sample object to a target, reads it back and removes it.
type BackupTargetDrill interface {
	DrillTarget(ctx context.Context, targetID string) (backup.DrillReport, error)
}

// handleDrillBackupTarget handles POST /api/v1/backup-targets/{id}/drill: a write, read-back and
// delete round trip, answering 200 with ok=false and the failing stage when storage refuses.
func (rt *Router) handleDrillBackupTarget(w http.ResponseWriter, r *http.Request) {
	drill, ok := rt.backupTargetTester.(BackupTargetDrill)
	if !ok || rt.backupTargetTester == nil {
		writeError(w, http.StatusNotImplemented, "backup target drill is not configured on this control plane (no master key set)")
		return
	}
	id := r.PathValue("id")
	if _, err := rt.backupTargets.GetBackupTarget(r.Context(), id); errors.Is(err, store.ErrBackupTargetNotFound) {
		writeError(w, http.StatusNotFound, "backup target not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: drill backup target: load failed", err, slog.String("id", id))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupTargetTestTimeout)
	defer cancel()
	rep, err := drill.DrillTarget(ctx, id)
	if errors.Is(err, backup.ErrDrillUnsupported) {
		writeError(w, http.StatusNotImplemented, "this backup target cannot run the restore drill")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, backupTargetTestErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
