package api

import "net/http"

// registerMailPortRoutes wires the experimental mail port helper. Listing is
// read-gated; open needs the sensitive write ability, like other exposure changes.
func (rt *Router) registerMailPortRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/mail/ports", rt.requireAbility(AbilityRead, rt.handleMailPorts))
	mux.HandleFunc("POST /api/v1/mail/ports/open", rt.requireAbility(AbilityWriteSensitive, rt.handleMailPortsOpen))
}
