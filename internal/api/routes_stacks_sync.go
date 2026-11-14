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
	// Rotate the signing secret of the stack sync webhook and return it once with Cache-Control no-store; no GET ever returns it. Root or stack admin, audited. 501 without a master key.
	mux.HandleFunc("POST /api/v1/stacks/{name}/sync/webhook-secret/rotate", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleRotateStackSyncWebhookSecret))
	// Git push webhook for a stack sync: no session, authenticated only by X-Hub-Signature-256 (HMAC SHA-256 of the raw body; SHA-1 alone is refused) or X-Gitlab-Token against the stack's webhook secret. JSON body capped by stacks.sync_webhook_max_body_kb (413). Only ref and after are read: a push to the configured branch queues one sync that plans and applies per the stack's apply flags; other branches, tags and deletes answer 202 with ignored. Unknown stack, sync off and a bad signature all answer the same 404. Returns 202 without waiting; bursts coalesce into one pending run.
	mux.HandleFunc("POST /api/v1/hooks/stacks/{name}/sync", rt.handleStackSyncWebhook)
	// Same as the sync webhook, but the queued run only plans, whatever the stack's apply flag says.
	mux.HandleFunc("POST /api/v1/hooks/stacks/{name}/refresh", rt.handleStackSyncRefreshWebhook)
}
