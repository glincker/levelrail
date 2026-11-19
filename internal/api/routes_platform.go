package api

import "net/http"

// registerPlatformRoutes is Handler()'s second half: nodes, certs,
// ingress, email, domains, backups, restore, storage, git provider
// integrations, and audit. See routes.go's own doc comment for why the
// split.
func (rt *Router) registerPlatformRoutes(mux *http.ServeMux) {
	rt.registerAppSourceRoutes(mux)
	rt.registerAppRuntimeRoutes(mux)
	rt.registerProjectRoutes(mux)
	rt.registerNodeRoutes(mux)
	rt.registerIngressRoutes(mux)
	rt.registerAutoHostRoutes(mux)
	rt.registerServiceRuntimeRoutes(mux)
	rt.registerSettingsRoutes(mux)
	rt.registerMailRoutes(mux)
	// Registry token realm for build image transfer, authenticated only by a job-scoped push token. Answers a bare 401 on any failure.
	mux.HandleFunc("GET /api/v1/registry/token", rt.handleRegistryToken)
	rt.registerGitProviderRoutes(mux)
	rt.registerBackupRoutes(mux)
	rt.registerAuditRoutes(mux)
}
