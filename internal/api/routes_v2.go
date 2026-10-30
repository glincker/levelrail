package api

import "net/http"

// registerV2Routes registers the additive v2 resource view. Each kind keeps
// the exact v1 gate: apps and databases read plus per-object checks, nodes root.
// Server-side apply (PATCH) takes the same per-object write gate as v1 writes.
func (rt *Router) registerV2Routes(mux *http.ServeMux) {
	v2 := newV2Server(rt)
	mux.HandleFunc("GET /api/v2/apps", rt.requireAbility(AbilityRead, v2.handleList(v2PathApps)))
	mux.HandleFunc("GET /api/v2/apps/{name}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, v2.handleGet(v2PathApps)))
	mux.HandleFunc("PATCH /api/v2/apps/{name}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, v2.handleApply(v2PathApps)))
	mux.HandleFunc("GET /api/v2/databases", rt.requireAbility(AbilityRead, v2.handleList(v2PathDatabases)))
	mux.HandleFunc("GET /api/v2/databases/{name}", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, v2.handleGet(v2PathDatabases)))
	mux.HandleFunc("PATCH /api/v2/databases/{name}", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, v2.handleApply(v2PathDatabases)))
	mux.HandleFunc("GET /api/v2/nodes", rt.requireAbility(AbilityRoot, v2.handleList(v2PathNodes)))
	mux.HandleFunc("GET /api/v2/nodes/{name}", rt.requireAbility(AbilityRoot, v2.handleGet(v2PathNodes)))
	mux.HandleFunc("PATCH /api/v2/{kind}/{name}", rt.requireAbility(AbilityWrite, v2.handleApplyUnsupported))
}
