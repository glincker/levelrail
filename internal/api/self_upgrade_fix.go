package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const attemptFailureListLimit = 200

// attemptFailure types a failed attempt and trims the buttons to what this
// host can actually do right now.
func (rt *Router) attemptFailure(item selfUpgradeAttemptItem) *selfupgrade.StepFailure {
	if item.FailedStep == "" || item.Outcome == selfupgrade.OutcomeSucceeded || item.Outcome == selfupgrade.OutcomeRunning {
		return nil
	}
	f := selfupgrade.ClassifyFailure(item.FailedStep, item.Error)
	if f.Rollbackable && (item.Outcome == selfupgrade.OutcomeRolledBack || rt.restorePointForVersion(item.FromVersion) == "") {
		f.Rollbackable = false
	}
	if f.Primary == selfupgrade.FixActionRollback && !f.Rollbackable {
		f.Primary = selfupgrade.FixActionRetry
		if !f.Retryable {
			f.Primary = selfupgrade.FixActionNone
		}
	}
	if item.Outcome == selfupgrade.OutcomeRefused {
		f.Retryable = false
	}
	return &f
}

// restorePointForVersion returns the newest restore point holding a binary
// for version, or "".
func (rt *Router) restorePointForVersion(version string) string {
	if rt.dataDir == "" || version == "" {
		return ""
	}
	pts, err := selfupgrade.NewRestorePoints(rt.dataDir).List()
	if err != nil {
		return ""
	}
	best := ""
	var bestAt int64
	for _, p := range pts {
		if p.Version == version && p.HasBinary && p.CreatedAt.UnixNano() >= bestAt {
			best, bestAt = p.ID, p.CreatedAt.UnixNano()
		}
	}
	return best
}

func (rt *Router) findSelfUpgradeAttempt(ctx context.Context, id string) (selfUpgradeAttemptItem, bool) {
	rows, err := rt.selfUpgrade.store.ListSelfUpgradeAttempts(ctx, attemptFailureListLimit)
	if err != nil {
		return selfUpgradeAttemptItem{}, false
	}
	for _, row := range rows {
		if row.ID == id {
			item := toSelfUpgradeItem(row)
			item.Failure = rt.attemptFailure(item)
			return item, true
		}
	}
	return selfUpgradeAttemptItem{}, false
}

// failedAttempt loads the attempt and writes the error response itself when
// the attempt cannot be acted on.
func (rt *Router) failedAttempt(w http.ResponseWriter, r *http.Request) (selfUpgradeAttemptItem, bool) {
	if rt.selfUpgrade == nil || rt.dataDir == "" {
		writeError(w, http.StatusNotImplemented, "self-upgrade is not configured")
		return selfUpgradeAttemptItem{}, false
	}
	item, ok := rt.findSelfUpgradeAttempt(r.Context(), r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "upgrade attempt not found")
		return item, false
	}
	if item.Failure == nil {
		writeError(w, http.StatusConflict, "this attempt has no failed step to act on")
		return item, false
	}
	if running, _ := selfupgrade.NewJournal(rt.dataDir).Running(); len(running) > 0 {
		writeError(w, http.StatusConflict, "an upgrade is running or did not finish; wait for it or run self-upgrade --recover on the host")
		return item, false
	}
	return item, true
}

func (rt *Router) attemptInitiator(w http.ResponseWriter, r *http.Request) (string, bool) {
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return "", false
	}
	return "dashboard:" + sanitizeInitiator(firstNonEmpty(actorName, actorID)), true
}

// handleSelfUpgradeAttemptFix handles POST .../attempts/{id}/fix: it runs the
// auto fix the failure points at (disk cleanup) and reports its outcome.
func (rt *Router) handleSelfUpgradeAttemptFix(w http.ResponseWriter, r *http.Request) {
	item, ok := rt.failedAttempt(w, r)
	if !ok {
		return
	}
	if item.Failure.FixID == "" {
		writeError(w, http.StatusConflict, "this failure has no automatic fix; use Retry or Roll back")
		return
	}
	if !rt.fixitReady(w) {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	res, err := rt.fixitEngine().Apply(withRequestHost(r.Context(), r.Host), item.Failure.FixID, actor, nil)
	if err != nil {
		rt.internalError(w, "api: apply upgrade failure fix failed", err)
		return
	}
	rt.logger.Info("api: upgrade failure fix applied", slog.String("attempt", item.ID), slog.String("fix", item.Failure.FixID), slog.String("outcome", string(res.Outcome)))
	writeJSON(w, http.StatusOK, res)
}

// handleSelfUpgradeAttemptRetry handles POST .../attempts/{id}/retry: it runs
// the same upgrade again, which restarts from the first step and skips none.
func (rt *Router) handleSelfUpgradeAttemptRetry(w http.ResponseWriter, r *http.Request) {
	item, ok := rt.failedAttempt(w, r)
	if !ok {
		return
	}
	if !item.Failure.Retryable {
		writeError(w, http.StatusConflict, "this failure cannot be retried from the dashboard")
		return
	}
	if ok, why := rt.selfUpgrade.canLaunch(); !ok {
		writeError(w, http.StatusConflict, "this host cannot apply the upgrade from the dashboard: "+why)
		return
	}
	initiator, ok := rt.attemptInitiator(w, r)
	if !ok {
		return
	}
	args := []string{"--to", item.ToVersion, "--yes", "--initiator", initiator, "--json"}
	if len(item.Acked) > 0 {
		args = append(args, "--ack", strings.Join(item.Acked, ","))
	}
	if err := rt.selfUpgrade.launch(r.Context(), args); err != nil {
		rt.logger.Error("api: retry self-upgrade failed", slog.String("attempt", item.ID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not start the upgrade on the host")
		return
	}
	rt.logger.Info("api: self-upgrade retry started", slog.String("attempt", item.ID), slog.String("to", item.ToVersion), slog.String("initiator", initiator))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "target_version": item.ToVersion})
}

// handleSelfUpgradeAttemptRollback handles POST .../attempts/{id}/rollback: it
// restores the pre-upgrade restore point taken for the attempt's from-version.
func (rt *Router) handleSelfUpgradeAttemptRollback(w http.ResponseWriter, r *http.Request) {
	item, ok := rt.failedAttempt(w, r)
	if !ok {
		return
	}
	if !item.Failure.Rollbackable {
		writeError(w, http.StatusConflict, "no restore point is available to roll this attempt back to")
		return
	}
	if ok, why := rt.selfUpgrade.canLaunch(); !ok {
		writeError(w, http.StatusConflict, "this host cannot restore from the dashboard: "+why)
		return
	}
	pointID := rt.restorePointForVersion(item.FromVersion)
	initiator, ok := rt.attemptInitiator(w, r)
	if !ok {
		return
	}
	if err := rt.selfUpgrade.restore(r.Context(), []string{"--id", pointID, "--initiator", initiator, "--json"}); err != nil {
		rt.logger.Error("api: start attempt rollback failed", slog.String("attempt", item.ID), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not start the restore on the host")
		return
	}
	rt.logger.Info("api: attempt rollback started", slog.String("attempt", item.ID), slog.String("restore_point", pointID), slog.String("initiator", initiator))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "restore_point": pointID, "version": item.FromVersion})
}
