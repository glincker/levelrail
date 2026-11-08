package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const bulkBackupDeleteMax = 200

type bulkBackupDeleteRequest struct {
	IDs    []string `json:"ids"`
	DryRun bool     `json:"dry_run"`
}

type bulkBackupDeleteItem struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // deleted, would_delete, refused, not_found, failed
	Reason  string `json:"reason,omitempty"`
}

// handleBulkDeleteBackups handles POST /api/v1/backups/bulk-delete. Running or
// restore-referenced backups are refused per item; the rest go ahead, and
// dry_run previews the same verdicts without deleting.
func (rt *Router) handleBulkDeleteBackups(w http.ResponseWriter, r *http.Request) {
	if rt.backupDeleter == nil {
		writeError(w, http.StatusNotImplemented, "backup deletion is not configured on this control plane (no master key set)")
		return
	}
	var req bulkBackupDeleteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.IDs) == 0 || len(req.IDs) > bulkBackupDeleteMax {
		writeError(w, http.StatusBadRequest, "ids must hold between 1 and 200 backup ids")
		return
	}
	seen := make(map[string]bool, len(req.IDs))
	items := make([]bulkBackupDeleteItem, 0, len(req.IDs))
	for _, id := range req.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		item := bulkBackupDeleteItem{ID: id}
		h, err := rt.backupHistory.GetBackupHistory(r.Context(), id)
		switch {
		case errors.Is(err, store.ErrBackupHistoryNotFound):
			item.Outcome = "not_found"
		case err != nil:
			rt.logger.Warn("api: bulk delete backups: load history failed", slog.String("error", err.Error()), slog.String("backup_id", id))
			item.Outcome, item.Reason = "failed", "could not load the backup"
		case h.Status == store.BackupStatusRunning:
			item.Outcome, item.Reason = "refused", "backup is still running"
		case req.DryRun:
			item.Outcome = "would_delete"
		default:
			derr := rt.backupDeleter.DeleteBackup(r.Context(), id)
			switch {
			case errors.Is(derr, store.ErrBackupInUse):
				item.Outcome, item.Reason = "refused", "backup was used by a restore or clone"
			case derr != nil:
				rt.logger.Warn("api: bulk delete backups: delete failed", slog.String("error", derr.Error()), slog.String("backup_id", id))
				item.Outcome, item.Reason = "failed", "could not delete the backup"
			default:
				item.Outcome = "deleted"
				rt.auditBackupBulkDelete(r.Context(), r, id)
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"dry_run": req.DryRun, "items": items})
}

func (rt *Router) auditBackupBulkDelete(ctx context.Context, r *http.Request, backupID string) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: backup delete audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWriteSensitive, Method: http.MethodDelete, Path: "/api/v1/backups/" + backupID,
		StatusCode: http.StatusNoContent, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: backup delete audit save failed", slog.String("error", err.Error()), slog.String("backup_id", backupID))
	}
}
