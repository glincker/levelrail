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
	rt.registerSettingsRoutes(mux)
	rt.registerMailRoutes(mux)
	rt.registerGitProviderRoutes(mux)
	rt.registerBackupRoutes(mux)
	rt.registerAuditRoutes(mux)
}
