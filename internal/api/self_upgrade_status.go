package api

import (
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

type selfUpgradeStatusResource struct {
	CurrentVersion string                  `json:"current_version"`
	Running        *selfUpgradeAttemptItem `json:"running"`
	LastFinished   *selfUpgradeAttemptItem `json:"last_finished"`
	// Verified is true when the last finished attempt succeeded and its
	// health confirmation step passed.
	Verified bool `json:"verified"`
}

// handleSelfUpgradeStatus handles GET /api/v1/updates/self-upgrade/status:
// the in-progress attempt, if any, and the last finished one. The server is
// the source of truth, so a reloaded page or a new process reads the same.
func (rt *Router) handleSelfUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	out := selfUpgradeStatusResource{CurrentVersion: version.Version}
	if rt.selfUpgrade == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if rt.dataDir != "" {
		if _, err := selfupgrade.ImportJournal(r.Context(), selfupgrade.NewJournal(rt.dataDir), rt.selfUpgrade.store); err != nil {
			rt.logger.Warn("api: import self-upgrade journal failed", slog.String("error", err.Error()))
		}
	}
	rows, err := rt.selfUpgrade.store.ListSelfUpgradeAttempts(r.Context(), selfUpgradeListLimit)
	if err != nil {
		rt.logger.Error("api: read self-upgrade status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out.Running, out.LastFinished = pickStatusAttempts(rows)
	if out.LastFinished != nil && out.LastFinished.Outcome == selfupgrade.OutcomeSucceeded {
		out.Verified = stepPassed(out.LastFinished.Steps, selfupgrade.StepHealth)
	}
	writeJSON(w, http.StatusOK, out)
}

// pickStatusAttempts takes rows newest first and returns the running attempt
// and the newest finished one.
func pickStatusAttempts(rows []store.SelfUpgradeAttempt) (running, last *selfUpgradeAttemptItem) {
	for _, row := range rows {
		item := toSelfUpgradeItem(row)
		switch {
		case row.Outcome == selfupgrade.OutcomeRunning && running == nil:
			running = &item
		case row.Outcome != selfupgrade.OutcomeRunning && last == nil:
			last = &item
		}
	}
	return running, last
}

func stepPassed(steps []selfUpgradeStepItem, name string) bool {
	for _, s := range steps {
		if s.Name == name {
			return s.Status == "ok"
		}
	}
	return false
}

// runningSelfUpgradeID returns the id of an attempt the host journal says is
// still running: the single-upgrade lock.
func (rt *Router) runningSelfUpgradeID() string {
	running, _ := selfupgrade.NewJournal(rt.dataDir).Running()
	if len(running) == 0 {
		return ""
	}
	return running[len(running)-1].ID
}
