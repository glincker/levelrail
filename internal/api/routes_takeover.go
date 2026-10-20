package api

import "net/http"

// registerTakeoverRoutes registers the same-server port takeover. Every route
// is root only: it can stop another platform's proxy and restart this process.
func (rt *Router) registerTakeoverRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/migration/takeover/plan", rt.requireAbility(AbilityRoot, rt.handleTakeoverPlan))
	mux.HandleFunc("GET /api/v1/migration/takeover/runs", rt.requireAbility(AbilityRoot, rt.handleListTakeoverRuns))
	mux.HandleFunc("POST /api/v1/migration/takeover/runs", rt.requireAbility(AbilityRoot, rt.handleStartTakeover))
	mux.HandleFunc("GET /api/v1/migration/takeover/runs/{run}", rt.requireAbility(AbilityRoot, rt.handleGetTakeoverRun))
	mux.HandleFunc("POST /api/v1/migration/takeover/runs/{run}/rollback", rt.requireAbility(AbilityRoot, rt.handleRollbackTakeover))
}
