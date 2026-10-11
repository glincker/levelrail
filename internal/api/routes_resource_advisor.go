package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Advisor routes filter by team and check each resource themselves, so
// tenants may call them (see tenantRoutes in team_gate.go).
func init() {
	for _, k := range []string{
		"GET /api/v1/resource-advisor/findings",
		"GET /api/v1/resource-advisor/summary",
		"POST /api/v1/resource-advisor/findings/{id}/apply",
		"POST /api/v1/resource-advisor/findings/{id}/dismiss",
		"POST /api/v1/resource-advisor/findings/{id}/snooze",
		"POST /api/v1/resource-advisor/findings/{id}/restore",
	} {
		tenantRoutes[k] = true
	}
}

// registerResourceAdvisorRoutes: reads need AbilityRead and are filtered to
// the caller's resources; changes need AbilityWrite here and the matching
// per-resource ability in the handler.
func (rt *Router) registerResourceAdvisorRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/resource-advisor/findings", rt.requireAbility(AbilityRead, rt.handleResourceAdvisorFindings))
	mux.HandleFunc("GET /api/v1/resource-advisor/summary", rt.requireAbility(AbilityRead, rt.handleResourceAdvisorSummary))
	mux.HandleFunc("POST /api/v1/resource-advisor/findings/{id}/apply", rt.requireAbility(AbilityWrite, rt.handleResourceAdvisorApply))
	mux.HandleFunc("POST /api/v1/resource-advisor/findings/{id}/dismiss", rt.requireAbility(AbilityWrite, rt.handleResourceAdvisorState(store.AdvisorFindingDismissed)))
	mux.HandleFunc("POST /api/v1/resource-advisor/findings/{id}/snooze", rt.requireAbility(AbilityWrite, rt.handleResourceAdvisorState(store.AdvisorFindingSnoozed)))
	mux.HandleFunc("POST /api/v1/resource-advisor/findings/{id}/restore", rt.requireAbility(AbilityWrite, rt.handleResourceAdvisorState("")))
	mux.HandleFunc("GET /api/v1/settings/beta-features", rt.requireAbility(AbilityRead, rt.handleListBetaFeatures))
	mux.HandleFunc("PUT /api/v1/settings/beta-features/{feature}", rt.requireAbility(AbilityRoot, rt.handleSetBetaFeature))
}
