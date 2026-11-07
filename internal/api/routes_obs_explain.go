package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploytimeline"
)

const (
	envDeployExplainLines     = "APP_DEPLOY_EXPLAIN_LOG_LINES"
	defaultDeployExplainLines = 8
)

func (rt *Router) registerObsExplainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/deployments/{id}/explain", rt.requireAbility(AbilityRead, rt.handleExplainDeploy))
}

// handleExplainDeploy handles GET /api/v1/deployments/{id}/explain: a
// deterministic, read-only account of why a deploy failed, built from its
// timeline. No model is involved and nothing is changed.
func (rt *Router) handleExplainDeploy(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	n := defaultDeployExplainLines
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envDeployExplainLines))); err == nil && v > 0 {
		n = v
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, deploytimeline.Explain(rt.buildTimeline(r.Context(), a), n))
}
