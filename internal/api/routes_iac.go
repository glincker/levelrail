package api

import "net/http"

// registerIaCRoutes: declarative plan, apply and export. Plan and export
// only read, apply writes; every change inside an apply goes back through
// the ordinary routes with the caller's own credentials.
func (rt *Router) registerIaCRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/apply/plan", rt.requireAbility(AbilityRead, rt.handleIaCPlan))
	mux.HandleFunc("POST /api/v1/apply", rt.requireAbility(AbilityWrite, rt.handleIaCApply))
	mux.HandleFunc("GET /api/v1/export", rt.requireAbility(AbilityRead, rt.handleIaCExport))
	mux.HandleFunc("GET /api/v1/apply/schema", rt.requireAbility(AbilityRead, rt.handleIaCSchema))
}
