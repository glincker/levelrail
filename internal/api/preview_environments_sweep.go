package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// defaultPreviewTTL is how long a preview environment can go with no
// webhook update (a synchronize push, or the pull-request-closed event
// itself) before SweepStalePreviewEnvironments tears it down. This is
// the fallback for a missed webhook delivery: webhook_deliveries.go's
// own history feature exists specifically because deliveries can fail,
// and a preview whose close event never arrived would otherwise leak
// forever. Overridable via APP_PREVIEW_TTL (cmd/levelrail/main.go's
// previewTTL, WithPreviewTTL), the project's "no hardcoded thresholds"
// rule.
const defaultPreviewTTL = 7 * 24 * time.Hour

func (rt *Router) effectivePreviewTTL() time.Duration {
	if rt.previewTTL > 0 {
		return rt.previewTTL
	}
	return defaultPreviewTTL
}

// SweepStalePreviewEnvironments tears down every preview environment
// last updated before now minus effectivePreviewTTL, reusing
// teardownPreviewRecord, the exact same deletion path the pull-request-
// closed webhook and the manual teardown route already use. One
// preview's teardown failure never blocks the rest, the same "one broken
// resource must not block others" principle every other periodic loop
// in this codebase follows (alerting.Engine.Tick, backup.Scheduler.Tick,
// scheduledtask.Scheduler.Tick).
//
// A partial teardown (http.StatusMultiStatus) counts as handled, not an
// error: teardownPreviewRecord already marks the row Failed with a
// reason and bumps its UpdatedAt, so the next sweep won't immediately
// retry the same broken preview every tick.
func (rt *Router) SweepStalePreviewEnvironments(ctx context.Context) (swept int, err error) {
	cutoff := time.Now().UTC().Add(-rt.effectivePreviewTTL())
	stale, listErr := rt.previewEnvironments.ListStalePreviewEnvironments(ctx, cutoff)
	if listErr != nil {
		return 0, fmt.Errorf("api: sweep stale preview environments: list: %w", listErr)
	}

	var errs []error
	for _, p := range stale {
		status, message := rt.teardownPreviewRecord(ctx, p)
		if status >= http.StatusInternalServerError {
			errs = append(errs, fmt.Errorf("preview %q (app %q pr #%d): %s", p.ID, p.AppName, p.PRNumber, message))
			continue
		}
		swept++
		rt.logger.Info("api: swept stale preview environment",
			slog.String("app_name", p.AppName), slog.Int("pr_number", p.PRNumber),
			slog.String("preview_app_id", p.PreviewAppID), slog.String("updated_at", p.UpdatedAt))
	}
	return swept, errors.Join(errs...)
}

// RunPreviewSweeper calls SweepStalePreviewEnvironments on interval
// until ctx is done, matching the shape of every other periodic loop in
// this codebase (backup.Scheduler.Run, scheduledtask.Scheduler.Run,
// alerting.Engine.Run).
func (rt *Router) RunPreviewSweeper(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := rt.SweepStalePreviewEnvironments(ctx); err != nil {
				rt.logger.Warn("api: preview sweep tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// handleSweepPreviewEnvironments handles POST /api/v1/previews/sweep:
// the manual trigger alongside RunPreviewSweeper's own periodic loop,
// for an operator who wants the TTL fallback to run right now instead
// of waiting for its next tick.
func (rt *Router) handleSweepPreviewEnvironments(w http.ResponseWriter, r *http.Request) {
	swept, err := rt.SweepStalePreviewEnvironments(r.Context())
	if err != nil {
		rt.logger.Error("api: sweep preview environments failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "sweep failed")
		return
	}
	writeJSON(w, http.StatusOK, sweepPreviewEnvironmentsResponse{Swept: swept})
}

type sweepPreviewEnvironmentsResponse struct {
	Swept int `json:"swept"`
}
