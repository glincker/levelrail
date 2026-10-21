package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

type restorePointItem struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Schema    int    `json:"schema"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
	HasBinary bool   `json:"has_binary"`
	HasUnit   bool   `json:"has_unit"`
	// DatabaseRestoreNeeded is true when restoring would also put the
	// database copy back, discarding changes made since CreatedAt.
	DatabaseRestoreNeeded bool   `json:"database_restore_needed"`
	CanRestore            bool   `json:"can_restore"`
	CannotRestoreWhy      string `json:"cannot_restore_reason,omitempty"`
}

// handleRestorePointList handles GET /api/v1/updates/restore-points. Root only.
func (rt *Router) handleRestorePointList(w http.ResponseWriter, r *http.Request) {
	out := []restorePointItem{}
	if rt.selfUpgrade == nil || rt.dataDir == "" {
		writeJSON(w, http.StatusOK, map[string]any{"restore_points": out})
		return
	}
	rp := selfupgrade.NewRestorePoints(rt.dataDir)
	if db, ok := rt.selfUpgrade.store.(selfupgrade.RestorePointStore); ok {
		if err := selfupgrade.SyncRestorePoints(r.Context(), rp, db); err != nil {
			rt.logger.Warn("api: sync restore points failed", slog.String("error", err.Error()))
		}
	}
	pts, err := rp.List()
	if err != nil {
		rt.logger.Error("api: list restore points failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	head, _ := store.MaxSchemaVersion()
	canLaunch, why := rt.selfUpgrade.canLaunch()
	for _, p := range pts {
		item := restorePointItem{
			ID: p.ID, Version: p.Version, Schema: p.Schema, CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
			SizeBytes: p.SizeBytes, HasBinary: p.HasBinary, HasUnit: p.HasUnit,
			DatabaseRestoreNeeded: head > 0 && p.Schema != head, CanRestore: canLaunch && p.HasBinary,
		}
		switch {
		case !p.HasBinary:
			item.CannotRestoreWhy = "this restore point holds no binary"
		case !canLaunch:
			item.CannotRestoreWhy = why
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"restore_points": out})
}

type restorePointRestoreRequest struct {
	Confirm string `json:"confirm"`
}

// handleRestorePointRestore handles POST /api/v1/updates/restore-points/{id}/restore.
// Root only. The host command does the swap, since the control plane cannot
// replace the binary it runs from.
func (rt *Router) handleRestorePointRestore(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil || rt.dataDir == "" {
		writeError(w, http.StatusNotImplemented, "restore points are not configured")
		return
	}
	id := r.PathValue("id")
	var req restorePointRestoreRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	pt, err := selfupgrade.NewRestorePoints(rt.dataDir).Get(id)
	if err != nil {
		if errors.Is(err, selfupgrade.ErrNoRestorePoint) {
			writeError(w, http.StatusNotFound, "restore point not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if strings.TrimSpace(req.Confirm) != pt.ID {
		writeError(w, http.StatusBadRequest, "type the restore point id to confirm")
		return
	}
	if !pt.HasBinary {
		writeError(w, http.StatusConflict, "this restore point holds no binary, so it cannot be restored")
		return
	}
	if ok, why := rt.selfUpgrade.canLaunch(); !ok {
		writeError(w, http.StatusConflict, "this host cannot restore from the dashboard: "+why)
		return
	}
	if running, _ := selfupgrade.NewJournal(rt.dataDir).Running(); len(running) > 0 {
		writeError(w, http.StatusConflict, "an upgrade is running or did not finish; wait for it or run self-upgrade --recover on the host")
		return
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	initiator := "dashboard:" + sanitizeInitiator(firstNonEmpty(actorName, actorID))
	if err := rt.selfUpgrade.restore(r.Context(), []string{"--id", pt.ID, "--initiator", initiator, "--json"}); err != nil {
		rt.logger.Error("api: start restore failed", slog.String("restore_point", pt.ID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not start the restore on the host")
		return
	}
	rt.logger.Info("api: restore point restore started", slog.String("from", version.Version), slog.String("restore_point", pt.ID), slog.String("initiator", initiator))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "restore_point": pt.ID, "version": pt.Version})
}
