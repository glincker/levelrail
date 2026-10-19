package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// rollbackOneClick reports whether the dashboard may apply plan itself. Only
// a binary-only rollback to a retained release qualifies: anything that
// restores a database loses data and stays a typed host command.
func (rt *Router) rollbackOneClick(plan rollbackPlanResource) (bool, string) {
	switch {
	case rt.selfUpgrade == nil:
		return false, "rollback from the dashboard is not configured"
	case plan.Blocked:
		return false, "a preflight check failed"
	case plan.RestoreRequired:
		return false, "this release is older than the database and needs a backup restore, which only the host command does"
	case plan.Verdict != upgrade.VerdictBinaryOnly:
		return false, "the release does not publish a schema version that matches this database"
	case !plan.Target.Retained:
		return false, "this release is not kept on this host yet"
	}
	return rt.selfUpgrade.canLaunch()
}

type rollbackStartRequest struct {
	Version string `json:"version"`
	Confirm string `json:"confirm"`
}

// handleRollbackStart handles POST /api/v1/updates/rollback. Root only. It
// re-runs the preflight, then hands the swap to a transient host unit, since
// the control plane cannot replace the binary it is running from.
func (rt *Router) handleRollbackStart(w http.ResponseWriter, r *http.Request) {
	if rt.selfUpgrade == nil {
		writeError(w, http.StatusNotImplemented, "rollback is not configured")
		return
	}
	var req rollbackStartRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Version = strings.TrimSpace(req.Version)
	plan, status, msg := rt.buildRollbackPlan(r.Context(), req.Version)
	if status != http.StatusOK {
		writeError(w, status, msg)
		return
	}
	if strings.TrimSpace(req.Confirm) != req.Version {
		writeError(w, http.StatusBadRequest, "type the version to confirm the rollback")
		return
	}
	if ok, why := rt.rollbackOneClick(plan); !ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "this rollback cannot be applied from the dashboard: " + why, "command": plan.Command})
		return
	}
	if running, _ := selfupgrade.NewJournal(rt.dataDir).Running(); len(running) > 0 {
		writeError(w, http.StatusConflict, "an earlier upgrade did not finish; run self-upgrade --recover on the host")
		return
	}
	_, actorID, actorName, ok := rt.currentActor(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "cannot resolve the requesting user")
		return
	}
	initiator := "dashboard:" + sanitizeInitiator(firstNonEmpty(actorName, actorID))
	if err := rt.selfUpgrade.rollback(r.Context(), []string{"--to", req.Version, "--confirm", req.Version, "--json"}); err != nil {
		rt.logger.Error("api: start rollback failed", slog.String("target", req.Version), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not start the rollback on the host")
		return
	}
	rt.logger.Info("api: rollback started", slog.String("from", version.Version), slog.String("to", req.Version), slog.String("initiator", initiator))
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "started", "target_version": req.Version})
}
