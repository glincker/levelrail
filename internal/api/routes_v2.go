package api

import "net/http"

// registerV2Routes registers the additive v2 resource view. Each kind keeps
// the exact v1 gate: apps and databases read plus per-object checks, nodes root.
func (rt *Router) registerV2Routes(mux *http.ServeMux) {
	v2 := newV2Server(rt)
	mux.HandleFunc("GET /api/v2/"+v2PathApps, rt.requireAbility(AbilityRead, v2.handleList(v2PathApps)))
	mux.HandleFunc("GET /api/v2/"+v2PathApps+"/{name}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, v2.handleGet(v2PathApps)))
	mux.HandleFunc("GET /api/v2/"+v2PathDatabases, rt.requireAbility(AbilityRead, v2.handleList(v2PathDatabases)))
	mux.HandleFunc("GET /api/v2/"+v2PathDatabases+"/{name}", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, v2.handleGet(v2PathDatabases)))
	mux.HandleFunc("GET /api/v2/"+v2PathNodes, rt.requireAbility(AbilityRoot, v2.handleList(v2PathNodes)))
	mux.HandleFunc("GET /api/v2/"+v2PathNodes+"/{name}", rt.requireAbility(AbilityRoot, v2.handleGet(v2PathNodes)))
}
