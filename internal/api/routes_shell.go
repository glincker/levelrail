package api

import "net/http"

// registerShellRoutes returns the aggregate so Handler can hand it the full
// middleware chain once that chain is built.
func (rt *Router) registerShellRoutes(mux *http.ServeMux) *shellOverview {
	s := &shellOverview{timeout: rt.shellSectionTimeout}
	// Shell bootstrap: each section is replayed as the caller, so it only holds what they can already read.
	mux.HandleFunc("GET /api/v1/shell", rt.requireAbility(AbilityRead, s.handleShellOverview))
	return s
}
