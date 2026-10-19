package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// defaultAuditLogRetention is how long an audit_log row survives before
// PurgeOldAuditEntries removes it. Overridable via
// APP_AUDIT_LOG_RETENTION_DAYS (cmd/levelrail/main.go's
// auditLogRetention, WithAuditLogRetention), the project's "no hardcoded
// thresholds" rule.
const defaultAuditLogRetention = 90 * 24 * time.Hour

func (rt *Router) effectiveAuditLogRetention() time.Duration {
	if rt.auditLogRetention > 0 {
		return rt.auditLogRetention
	}
	return defaultAuditLogRetention
}

// PurgeOldAuditEntries deletes every audit_log row older than
// effectiveAuditLogRetention, returning how many rows were removed.
func (rt *Router) PurgeOldAuditEntries(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	cutoff := now.Add(-rt.effectiveAuditLogRetention())
	rt.pruneIdempotency(ctx)
	if tiered, ok := rt.auditLog.(tieredAuditPurger); ok {
		return tiered.DeleteAuditEntriesTiered(ctx, cutoff, now.Add(-rt.effectiveSecurityAuditRetention()))
	}
	return rt.auditLog.DeleteAuditEntriesOlderThan(ctx, cutoff)
}

const (
	envAuditSecurityRetentionDays     = "APP_AUDIT_LOG_SECURITY_RETENTION_DAYS"
	defaultAuditSecurityRetentionDays = 365
)

type tieredAuditPurger interface {
	DeleteAuditEntriesTiered(ctx context.Context, cutoff, securityCutoff time.Time) (int64, error)
}

// effectiveSecurityAuditRetention is how long sign-in, step-up, token and
// policy rows survive; it is never shorter than the general retention.
func (rt *Router) effectiveSecurityAuditRetention() time.Duration {
	days := envInt(envAuditSecurityRetentionDays, defaultAuditSecurityRetentionDays)
	return max(time.Duration(days)*24*time.Hour, rt.effectiveAuditLogRetention())
}

// RunAuditLogSweeper calls PurgeOldAuditEntries on interval until ctx is
// done, the same ticker shape RunPreviewSweeper already establishes.
func (rt *Router) RunAuditLogSweeper(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if n, err := rt.PurgeOldAuditEntries(ctx); err != nil {
				rt.logger.Warn("api: audit log sweep tick failed", slog.String("error", err.Error()))
			} else if n > 0 {
				rt.logger.Info("api: swept old audit log entries", slog.Int64("deleted", n))
			}
		}
	}
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
