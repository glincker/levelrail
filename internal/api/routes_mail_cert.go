package api

import "net/http"

// registerMailCertRoutes wires the mail certificate check and upload (ADR 062).
// Upload changes what the public mail ports serve, so it also asks for step-up.
func (rt *Router) registerMailCertRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/mail/cert", rt.requireAbility(AbilityRead, rt.handleMailCertStatus))
	mux.HandleFunc("PUT /api/v1/mail/cert", rt.requireAbility(AbilityWriteSensitive, rt.handleMailCertUpload))
	mux.HandleFunc("POST /api/v1/mail/cert/obtain", rt.requireAbility(AbilityWriteSensitive, rt.handleMailCertObtain))
}
