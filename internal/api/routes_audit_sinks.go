package api

import "net/http"

// registerAuditSinkRoutes wires audit chain verification and the streaming
// sink management surface. All root only, like the audit log itself.
func (rt *Router) registerAuditSinkRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/audit-log/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyAuditChain))
	mux.HandleFunc("GET /api/v1/audit-sinks", rt.requireAbility(AbilityRoot, rt.handleListAuditSinks))
	mux.HandleFunc("POST /api/v1/audit-sinks", rt.requireAbility(AbilityRoot, rt.handleCreateAuditSink))
	mux.HandleFunc("PATCH /api/v1/audit-sinks/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateAuditSink))
	mux.HandleFunc("DELETE /api/v1/audit-sinks/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteAuditSink))
	mux.HandleFunc("POST /api/v1/audit-sinks/{id}/test", rt.requireAbility(AbilityRoot, rt.handleTestAuditSink))
	mux.HandleFunc("POST /api/v1/audit-sinks/{id}/rotate-secret", rt.requireAbility(AbilityRoot, rt.handleRotateAuditSinkSecret))
	mux.HandleFunc("GET /api/v1/audit-sinks/{id}/deliveries", rt.requireAbility(AbilityRoot, rt.handleListAuditSinkDeliveries))
}
