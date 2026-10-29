package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	backupCopyListDefault = 100
	backupCopyListMax     = 500
)

type backupCopyStore interface {
	GetBackupCopySettings(ctx context.Context) (store.BackupCopySettings, error)
	SetBackupCopySettings(ctx context.Context, enabled bool, targetID, now string) error
	ListBackupCopies(ctx context.Context, limit int) ([]store.BackupCopy, error)
}

type backupCopySettingsResource struct {
	Enabled    bool   `json:"enabled"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name,omitempty"`
}

type setBackupCopySettingsRequest struct {
	Enabled  bool   `json:"enabled"`
	TargetID string `json:"target_id"`
}

type backupCopyResource struct {
	BackupID       string `json:"backup_id"`
	Resource       string `json:"resource"`
	ResourceKind   string `json:"resource_kind"`
	SourceTargetID string `json:"source_target_id"`
	TargetID       string `json:"target_id"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	Error          string `json:"error,omitempty"`
	SizeBytes      int64  `json:"size_bytes"`
	BackupFinished string `json:"backup_finished_at"`
	CopiedAt       string `json:"copied_at,omitempty"`
	// LagSeconds is the copy delay: finished to copied, or finished to now while still waiting.
	LagSeconds int64 `json:"lag_seconds"`
}

func (rt *Router) backupCopyStore(w http.ResponseWriter) (backupCopyStore, bool) {
	s, ok := rt.backupTargets.(backupCopyStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "backup copies are not available on this control plane")
	}
	return s, ok
}

func (rt *Router) copySettingsResource(ctx context.Context, s store.BackupCopySettings) backupCopySettingsResource {
	res := backupCopySettingsResource{Enabled: s.Enabled, TargetID: s.TargetID}
	if s.TargetID != "" {
		if t, err := rt.backupTargets.GetBackupTarget(ctx, s.TargetID); err == nil {
			res.TargetName = t.Name
		}
	}
	return res
}

// handleGetBackupCopySettings handles GET /api/v1/backups/copy-settings.
func (rt *Router) handleGetBackupCopySettings(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.backupCopyStore(w)
	if !ok {
		return
	}
	s, err := st.GetBackupCopySettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get backup copy settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.copySettingsResource(r.Context(), s))
}

// handleSetBackupCopySettings handles PUT /api/v1/backups/copy-settings.
func (rt *Router) handleSetBackupCopySettings(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.backupCopyStore(w)
	if !ok {
		return
	}
	var req setBackupCopySettingsRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Enabled {
		if req.TargetID == "" {
			writeError(w, http.StatusBadRequest, "target_id is required to enable backup copies")
			return
		}
		if _, err := rt.backupTargets.GetBackupTarget(r.Context(), req.TargetID); errors.Is(err, store.ErrBackupTargetNotFound) {
			writeError(w, http.StatusNotFound, "backup target not found")
			return
		} else if err != nil {
			rt.internalError(w, "api: get backup target failed", err, slog.String("id", req.TargetID))
			return
		}
	}
	if err := st.SetBackupCopySettings(r.Context(), req.Enabled, req.TargetID, time.Now().UTC().Format(time.RFC3339)); err != nil {
		rt.internalError(w, "api: set backup copy settings failed", err)
		return
	}
	s, err := st.GetBackupCopySettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get backup copy settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.copySettingsResource(r.Context(), s))
}

// handleListBackupCopies handles GET /api/v1/backups/copies?limit=N.
func (rt *Router) handleListBackupCopies(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.backupCopyStore(w)
	if !ok {
		return
	}
	limit := backupCopyListDefault
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > backupCopyListMax {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(backupCopyListMax))
			return
		}
		limit = n
	}
	copies, err := st.ListBackupCopies(r.Context(), limit)
	if err != nil {
		rt.internalError(w, "api: list backup copies failed", err)
		return
	}
	now := time.Now().UTC()
	out := make([]backupCopyResource, 0, len(copies))
	for _, c := range copies {
		out = append(out, toBackupCopyResource(c, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func toBackupCopyResource(c store.BackupCopy, now time.Time) backupCopyResource {
	res := backupCopyResource{
		BackupID: c.BackupHistoryID, Resource: c.ResourceName, ResourceKind: c.ResourceKind,
		SourceTargetID: c.SourceTargetID, TargetID: c.TargetID, Status: c.Status, Attempts: c.Attempts,
		Error: c.Error, SizeBytes: c.SizeBytes, BackupFinished: c.BackupFinished, CopiedAt: c.CopiedAt,
	}
	finished, err := time.Parse(time.RFC3339, c.BackupFinished)
	if err != nil {
		return res
	}
	end := now
	if copied, err := time.Parse(time.RFC3339, c.CopiedAt); err == nil && c.Status == store.BackupCopySucceeded {
		end = copied
	}
	if lag := end.Sub(finished); lag > 0 {
		res.LagSeconds = int64(lag.Seconds())
	}
	return res
}
