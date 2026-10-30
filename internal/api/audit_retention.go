package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// defaultAuditLogRetention mirrors the registered default of retention.audit_log_days
// (env APP_AUDIT_LOG_RETENTION_DAYS).
const defaultAuditLogRetention = 90 * 24 * time.Hour

// auditWindows resolves the general and security windows from the runtime settings (saved, then env,
// then default). Zero means keep forever. The explicit router option wins for the general window.
func (rt *Router) auditWindows(ctx context.Context) (general, security time.Duration) {
	gDays, sDays := cleanup.AuditWindows(
		rt.runtimeSettings.Int(ctx, runtimesettings.KeyAuditLogDays),
		rt.runtimeSettings.Int(ctx, runtimesettings.KeyAuditSecurityDays))
	general, security = time.Duration(gDays)*24*time.Hour, time.Duration(sDays)*24*time.Hour
	if rt.auditLogRetention > 0 {
		general = rt.auditLogRetention
		if security > 0 {
			security = max(security, general)
		}
	}
	return general, security
}

func (rt *Router) effectiveAuditLogRetention(ctx context.Context) time.Duration {
	general, _ := rt.auditWindows(ctx)
	return general
}

// PurgeOldAuditEntries deletes audit_log rows past their windows, returning how many rows were removed.
// A general window of zero keeps everything.
func (rt *Router) PurgeOldAuditEntries(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	general, security := rt.auditWindows(ctx)
	rt.pruneIdempotency(ctx)
	if general == 0 {
		return 0, nil
	}
	cutoff := now.Add(-general)
	if tiered, ok := rt.auditLog.(tieredAuditPurger); ok {
		var securityCutoff time.Time
		if security > 0 {
			securityCutoff = now.Add(-security)
		}
		return tiered.DeleteAuditEntriesTiered(ctx, cutoff, securityCutoff)
	}
	return rt.auditLog.DeleteAuditEntriesOlderThan(ctx, cutoff)
}

type tieredAuditPurger interface {
	DeleteAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error)
}

// RunAuditLogSweeper calls PurgeOldAuditEntries on interval until ctx is
// done, the same ticker shape RunPreviewSweeper already establishes.
func (rt *Router) RunAuditLogSweeper(ctx context.Context, interval time.Duration) error {
	return footprint.Every(ctx, "audit log sweep", interval, rt.logger, func(ctx context.Context) error {
		n, err := rt.PurgeOldAuditEntries(ctx)
		if err == nil && n > 0 {
			rt.logger.Info("api: swept old audit log entries", slog.Int64("deleted", n))
		}
		return err
	})
}

// handlePurgeAuditLog handles POST /api/v1/audit-log/purge: the manual
// trigger alongside RunAuditLogSweeper's own periodic loop, for an
// operator who wants old entries cleared right now rather than waiting
// for the next tick. AbilityRoot-gated, the same tier as reading the
// audit log itself (clearing audit history is at least as sensitive as
// reading it).
func (rt *Router) handlePurgeAuditLog(w http.ResponseWriter, r *http.Request) {
	deleted, err := rt.PurgeOldAuditEntries(r.Context())
	if err != nil {
		rt.internalError(w, "api: purge audit log failed", err)
		return
	}
	writeJSON(w, http.StatusOK, purgeAuditLogResponse{Deleted: deleted})
}

type purgeAuditLogResponse struct {
	Deleted int64 `json:"deleted"`
}
