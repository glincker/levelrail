package api

import "net/http"

// registerMigrationAgentRoutes registers the read-only migration agent routes. They all
// stay AbilityWriteSensitive except the session list, since creating a session mints a credential and the detail exposes host paths and container names.
func (rt *Router) registerMigrationAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/migration/agent/sessions", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateMigrationAgentSession))
	mux.HandleFunc("GET /api/v1/migration/agent/sessions", rt.requireAbility(AbilityRead, rt.handleListMigrationAgentSessions))
	mux.HandleFunc("GET /api/v1/migration/agent/sessions/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleGetMigrationAgentSession))
	mux.HandleFunc("POST /api/v1/migration/agent/sessions/{id}/discover", rt.requireAbility(AbilityWriteSensitive, rt.handleDiscoverMigrationAgent))
	mux.HandleFunc("POST /api/v1/migration/agent/sessions/{id}/verify", rt.requireAbility(AbilityWriteSensitive, rt.handleVerifyMigrationAgent))
	mux.HandleFunc("DELETE /api/v1/migration/agent/sessions/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleRevokeMigrationAgentSession))
}
