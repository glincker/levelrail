package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// registerResourceNotesRoutes gates notes like labels: reads at read, edits at the app edit level.
// Patterns stay literal so the authz matrix and API reference can read them.
func (rt *Router) registerResourceNotesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/apps/{name}/notes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.resourceNotesHandler(store.LabelKindApp, false)))
	mux.HandleFunc("PUT /api/v1/apps/{name}/notes", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.resourceNotesHandler(store.LabelKindApp, true)))
	mux.HandleFunc("GET /api/v1/databases/{name}/notes", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.resourceNotesHandler(store.LabelKindDatabase, false)))
	mux.HandleFunc("PUT /api/v1/databases/{name}/notes", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.resourceNotesHandler(store.LabelKindDatabase, true)))
}
