package api

import "net/http"

// registerDataExportRoutes wires the metrics scrape endpoint, the audit log
// export and the export destination CRUD. Management is root only.
func (rt *Router) registerDataExportRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /metrics", rt.requireAbility(AbilityRead, rt.handleScrapeMetrics))
	mux.HandleFunc("GET /api/v1/audit-log/export", rt.requireAbility(AbilityRoot, rt.handleExportAuditLog))
	mux.HandleFunc("GET /api/v1/export/destinations", rt.requireAbility(AbilityRoot, rt.handleListExportDestinations))
	mux.HandleFunc("POST /api/v1/export/destinations", rt.requireAbility(AbilityRoot, rt.handleCreateExportDestination))
	mux.HandleFunc("GET /api/v1/export/destinations/{id}", rt.requireAbility(AbilityRoot, rt.handleGetExportDestination))
	mux.HandleFunc("PATCH /api/v1/export/destinations/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateExportDestination))
	mux.HandleFunc("DELETE /api/v1/export/destinations/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteExportDestination))
	mux.HandleFunc("POST /api/v1/export/destinations/{id}/test", rt.requireAbility(AbilityRoot, rt.handleTestExportDestination))
}
