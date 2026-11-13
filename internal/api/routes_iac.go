package api

import "net/http"

// registerIaCRoutes: declarative plan, apply and export. Plan and export
// only read, apply writes; every change inside an apply goes back through
// the ordinary routes with the caller's own credentials.
func (rt *Router) registerIaCRoutes(mux *http.ServeMux) {
	// Plan only: diff declared documents against live state without a stack. Persists nothing. Error codes: docs/iac/errors.md.
	mux.HandleFunc("POST /api/v1/apply/plan", rt.requireAbility(AbilityRead, rt.handleIaCPlan))
	// Apply declared documents to live state without a stack record. 503 apply_disabled while the kill switch is off. Error codes: docs/iac/errors.md.
	mux.HandleFunc("POST /api/v1/apply", rt.requireAbility(AbilityWrite, rt.handleIaCApply))
	mux.HandleFunc("POST /api/v1/import/k8s", rt.requireAbility(AbilityWrite, rt.handleImportK8s))
	// Export live state as declarative documents. Secret values are never included.
	mux.HandleFunc("GET /api/v1/export", rt.requireAbility(AbilityRead, rt.handleIaCExport))
	// JSON schema of the declarative document kinds.
	mux.HandleFunc("GET /api/v1/apply/schema", rt.requireAbility(AbilityRead, rt.handleIaCSchema))
}
