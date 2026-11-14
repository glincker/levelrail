package api

import "net/http"

// registerStackSyncRoutes: the git sync of a stack. Reading status
// needs read on the stack; settings and refresh need root on stack:<name>.
func (rt *Router) registerStackSyncRoutes(mux *http.ServeMux) {
	// Show the git sync settings of a stack and its latest result: commit, plan hash, counts, pending and a code on failure. Never repository content or credentials.
	mux.HandleFunc("GET /api/v1/stacks/{name}/sync", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleGetStackSync))
	// Set the git sync of a stack: https or ssh repo_url, branch, relative path, credential_ref naming a connected app, interval_seconds, webhook_only, enabled (off by default), and apply, prune and deploy, each required and off unless true; prune and deploy need apply. A sync apply runs the manual apply path on a token scoped to this stack, so require_approval leaves it pending. Root or stack admin; credential_ref needs an administrator. Audited.
	mux.HandleFunc("PUT /api/v1/stacks/{name}/sync", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handlePutStackSync))
	// Fetch the configured branch and plan it now; applies only when the stack sync sets apply. 409 when sync is off, 429 inside stacks.sync_refresh_min_seconds. Error codes: docs/iac/errors.md.
	mux.HandleFunc("POST /api/v1/stacks/{name}/sync/refresh", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleRefreshStackSync))
}
