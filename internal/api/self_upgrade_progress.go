package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

// selfUpgradeProgress is the live view of the newest attempt: what the
// progress bar shows and which buttons are valid. TotalBytes is null when the
// server did not announce a size.
type selfUpgradeProgress struct {
	AttemptID       string `json:"attempt_id"`
	Step            string `json:"step"`
	BytesDownloaded int64  `json:"bytes_downloaded"`
	TotalBytes      *int64 `json:"total_bytes"`
	LastProgressAt  string `json:"last_progress_at"`
	StartedAt       string `json:"started_at"`
	Stalled         bool   `json:"stalled"`
	StallSeconds    int    `json:"stall_seconds"`
	CanCancel       bool   `json:"can_cancel"`
	CanRetry        bool   `json:"can_retry"`
	// CancelRequested is true while a cancel waits for the worker to stop.
	CancelRequested bool   `json:"cancel_requested"`
	Message         string `json:"message,omitempty"`
}

func (rt *Router) upgradeLimits() selfupgrade.Limits { return selfupgrade.LimitsFromEnv(nil) }

// reapUpgrades closes upgrade attempts whose worker is gone and syncs the
// journal to the database, so a status read never reports a dead run as live.
func (rt *Router) reapUpgrades(ctx context.Context) {
	if rt.selfUpgrade == nil || rt.dataDir == "" {
		return
	}
	closed, err := selfupgrade.ReapAndImport(ctx, rt.dataDir, rt.selfUpgrade.store, time.Now(), rt.upgradeLimits())
	if err != nil {
		rt.logger.Warn("api: reap abandoned upgrades failed", slog.String("error", err.Error()))
	}
	for _, a := range closed {
		rt.logger.Warn("api: abandoned upgrade attempt closed", slog.String("attempt", a.ID), slog.String("step", a.FailedStep))
	}
}

// buildUpgradeProgress derives the progress view from the running attempt's
// journal, or from the last finished attempt when nothing runs.
func (rt *Router) buildUpgradeProgress(running, last *selfUpgradeAttemptItem) *selfUpgradeProgress {
	l := rt.upgradeLimits()
	now := time.Now()
	if running == nil {
		if last == nil {
			return nil
		}
		p := &selfUpgradeProgress{AttemptID: last.ID, Step: last.FailedStep, StartedAt: last.StartedAt, LastProgressAt: last.FinishedAt, StallSeconds: int(l.Stall / time.Second)}
		if f := rt.attemptFailure(*last); f != nil {
			p.CanRetry = f.Retryable
			p.Message = last.Error
		}
		return p
	}
	p := &selfUpgradeProgress{AttemptID: running.ID, StartedAt: running.StartedAt, LastProgressAt: running.StartedAt, StallSeconds: int(l.Stall / time.Second)}
	j := selfupgrade.NewJournal(rt.dataDir)
	live, ok := journalAttempt(j, running.ID)
	if !ok {
		return p
	}
	p.Step = live.Current
	p.CanCancel = selfupgrade.CanCancel(live)
	p.CancelRequested = j.CancelRequested(live.ID)
	signal := selfupgrade.LastSignal(j, live)
	progressAt := signal
	if hb, ok := j.ReadHeartbeat(live.ID); ok {
		p.BytesDownloaded = hb.Bytes
		if hb.Total > 0 {
			t := hb.Total
			p.TotalBytes = &t
		}
		progressAt = hb.At
		if !hb.ProgressAt.IsZero() {
			progressAt = hb.ProgressAt
		}
		if p.Step == "" {
			p.Step = hb.Step
		}
	}
	p.LastProgressAt = progressAt.UTC().Format(time.RFC3339)
	silent := now.Sub(signal) > 3*l.Heartbeat
	idle := p.Step == selfupgrade.StepDownload && now.Sub(progressAt) > l.Stall
	p.Stalled = silent || idle
	switch {
	case silent:
		p.Message = "the upgrade process has not reported for a while. It is closed automatically; then Retry becomes available"
	case idle:
		p.Message = "no data has arrived for the stall limit. The download is stopped automatically; then Retry becomes available"
	}
	return p
}

func journalAttempt(j *selfupgrade.Journal, id string) (selfupgrade.Attempt, bool) {
	all, err := j.List()
	if err != nil {
		return selfupgrade.Attempt{}, false
	}
	for _, a := range all {
		if a.ID == id {
			return a, true
		}
	}
	return selfupgrade.Attempt{}, false
}

// handleSelfUpgradeAttemptCancel handles POST .../attempts/{id}/cancel. It is
// safe only before the service is stopped; after that the upgrade rolls back
// by itself if the new release is unhealthy.
func (rt *Router) handleSelfUpgradeAttemptCancel(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil || rt.dataDir == "" {
		writeError(w, http.StatusNotImplemented, "self-upgrade is not configured")
		return
	}
	rt.reapUpgrades(r.Context())
	id := r.PathValue("id")
	j := selfupgrade.NewJournal(rt.dataDir)
	live, ok := journalAttempt(j, id)
	if !ok {
		if _, known := rt.findSelfUpgradeAttempt(r.Context(), id); known {
			writeError(w, http.StatusConflict, "this upgrade attempt already finished, there is nothing to cancel")
			return
		}
		writeError(w, http.StatusNotFound, "upgrade attempt not found")
		return
	}
	if live.Outcome != selfupgrade.OutcomeRunning {
		writeError(w, http.StatusConflict, "this upgrade attempt already finished, there is nothing to cancel")
		return
	}
	if !selfupgrade.CanCancel(live) {
		writeError(w, http.StatusConflict, "the release is already being applied (stop, swap, start or verify), so it cannot be cancelled. If the new release is not healthy in time the previous one is restored automatically")
		return
	}
	if err := j.RequestCancel(id); err != nil {
		rt.internalError(w, "api: request upgrade cancel failed", err)
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Warn("api: self-upgrade cancel requested", slog.String("attempt", id), slog.String("step", live.Current), slog.String("actor", actor))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "cancelling", "attempt_id": id})
}
