package api

import "net/http"

// registerAppSourceRoutes registers the app secrets, git source, webhooks and previews routes.
func (rt *Router) registerAppSourceRoutes(mux *http.ServeMux) {
	// Secrets. Set-only: there is deliberately no GET,
	// returning a value (even to its own owner over an authenticated
	// session) is exactly the kind of exposure envelope encryption
	// exists to avoid.
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetSecret))

	// List known secret keys (never values) and toggle a key's lock.
	// GET at AbilityRead, matching GET .../git-source's own
	// GET=Read/PUT=WriteSensitive split just below: a key NAME is no
	// more sensitive than a git-source's connection config.
	mux.HandleFunc("GET /api/v1/apps/{name}/secrets", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListSecrets))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/secrets/{key}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteSecret))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/lock", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetSecretLock))

	// Rotation policies hold schedule and progress only, never a value.
	mux.HandleFunc("GET /api/v1/apps/{name}/secret-rotation", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListSecretRotation))
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}/rotation", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetSecretRotation))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/secrets/{key}/rotation", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteSecretRotation))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/rotation/rotate", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRotateSecretNow))
	mux.HandleFunc("GET /api/v1/secret-rotation", rt.requireAbility(AbilityRoot, rt.handleListAllSecretRotation))

	// Git source (a deferred follow-up, git_sources.go):
	// persist a repo/branch/build config per app so a git push can
	// auto-deploy it, the multi-app evolution of internal/webhook's own
	// single-app, env-var-configured Config. AbilityWriteSensitive for
	// PUT/DELETE, matching PUT .../secrets/{key} above: connecting a repo
	// accepts an optional live deploy token in the same request body.
	mux.HandleFunc("GET /api/v1/apps/{name}/git-source", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetGitSource))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetGitSource))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source/deploy-settings", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetGitDeploySettings))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source/build", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetGitSourceBuild))
	mux.HandleFunc("GET /api/v1/apps/{name}/git-source/commits", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleListGitSourceCommits))
	mux.HandleFunc("POST /api/v1/apps/{name}/git-source/detect", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDetectGitSourceBuild))
	mux.HandleFunc("POST /api/v1/apps/{name}/git-source/affected", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGitSourceAffected))
	mux.HandleFunc("POST /api/v1/apps/{name}/git-source/webhook/register", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRegisterGitSourceWebhook))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/git-source", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteGitSource))
	// Rotate webhook secret (git_sources.go): mints a fresh secret without
	// touching repo_url/branch/build config, the narrow alternative to a
	// full DELETE-then-PUT reconnect. Same AbilityWriteSensitive tier as
	// PUT .../git-source, since it's the identical class of credential
	// write.
	mux.HandleFunc("POST /api/v1/apps/{name}/git-source/validate", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleValidateGitSourceAccess))
	mux.HandleFunc("POST /api/v1/apps/{name}/git-source/rotate-webhook-secret", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRotateGitSourceWebhookSecret))

	// Git push deploy remote (git_push_remote.go): token management for the
	// beta push remote, GET=Read, everything else WriteSensitive like git-source.
	mux.HandleFunc("GET /api/v1/apps/{name}/git-remote", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetGitRemote))
	mux.HandleFunc("POST /api/v1/apps/{name}/git-remote", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleIssueGitRemote))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-remote", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetGitRemoteBranch))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/git-remote", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteGitRemote))
	// The push endpoints (git_push_http.go) take HTTP basic auth with the per-app
	// push token (hash compared in constant time, uniform 401, rate limited), not
	// a session. An anonymous request gets the 401 challenge git clients expect.
	mux.HandleFunc("GET /api/v1/git/{repo}/info/refs", rt.handleGitInfoRefs)
	mux.HandleFunc("POST /api/v1/git/{repo}/git-receive-pack", rt.handleGitReceivePack)

	// Scheduled deploys (app_schedule.go): a per-app cron schedule that
	// redeploys the latest commit on a branch, checked by
	// internal/scheduledeploy.Scheduler. Same GET=Read/PUT=WriteSensitive
	// split as git-source just above.
	mux.HandleFunc("GET /api/v1/apps/{name}/schedule", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppSchedule))
	mux.HandleFunc("PUT /api/v1/apps/{name}/schedule", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppSchedule))
	// GitHub Actions deploy action report settings (app_github_actions.go).
	mux.HandleFunc("GET /api/v1/apps/{name}/github-actions", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppGitHubActions))
	mux.HandleFunc("PUT /api/v1/apps/{name}/github-actions", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppGitHubActions))
	mux.HandleFunc("GET /api/v1/apps/{name}/ci/workflow", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetCIWorkflow))
	mux.HandleFunc("GET /api/v1/apps/{name}/ci/trusts", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListCITrusts))
	mux.HandleFunc("POST /api/v1/apps/{name}/ci/trusts", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleCreateCITrust))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/ci/trusts/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteCITrust))
	mux.HandleFunc("GET /api/v1/apps/{name}/schedule/history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppScheduleHistory))

	// Git push webhook: deliberately unauthenticated because GitHub cannot present a session.
	// The per-app HMAC signature check (secret set at git-source connect time) stands in for auth.
	mux.HandleFunc("POST /api/v1/webhooks/github/{name}", rt.handleGitPushWebhook)

	// Recent webhook deliveries (webhook_deliveries.go): real visibility
	// into what a git provider actually sent, for the exact route above,
	// plus a manual replay of a stored delivery. AbilityRead for the
	// list, matching GET .../git-source; AbilityDeploy for replay,
	// matching POST .../deploys, since a replay can trigger the same
	// real build/deploy side effect.
	mux.HandleFunc("GET /api/v1/apps/{name}/webhook-deliveries", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListWebhookDeliveries))
	mux.HandleFunc("POST /api/v1/apps/{name}/webhook-deliveries/{id}/replay", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleReplayWebhookDelivery))

	// Preview environments per pull request (preview_environments.go/
	// preview_environments_handlers.go): opt-in per app, off by default.
	// AbilityWriteSensitive for the toggle, matching PUT .../git-source's
	// own tier since it's the same connect-time-adjacent configuration
	// surface; AbilityRead for the list, matching GET .../git-source;
	// AbilityDeploy for the manual teardown, the same lifecycle-action
	// tier POST .../restart and POST .../stop already use.
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-settings", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetPreviewEnabled))
	mux.HandleFunc("GET /api/v1/apps/{name}/previews", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPreviewEnvironments))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/teardown", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleTeardownPreviewEnvironment))

	// Branch deploys: a stable per-branch deployment (branch_deploys.go), separate from per pull request previews.
	mux.HandleFunc("GET /api/v1/apps/{name}/branch-deploys", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListBranchDeploys))
	mux.HandleFunc("PUT /api/v1/apps/{name}/branch-deploys", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetBranchDeploySettings))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/branch-deploys/{slug}", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteBranchDeploy))

	// Preview environment TTL sweep (preview_environments_sweep.go): the
	// fallback for a pull-request-closed webhook delivery that never
	// arrived, cross-app since staleness is a control-plane-wide sweep,
	// not a per-app one. Same manual-trigger-alongside-the-automatic-path
	// shape as POST .../previews/{number}/teardown above, same
	// AbilityDeploy tier.
	mux.HandleFunc("POST /api/v1/previews/sweep", rt.requireAbility(AbilityDeploy, rt.handleSweepPreviewEnvironments))
	mux.HandleFunc("GET /api/v1/previews", rt.requireAbility(AbilityRead, rt.handleListAllPreviews))
	mux.HandleFunc("GET /api/v1/apps/{name}/preview-policy", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPreviewPolicy))
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetPreviewPolicy))
	mux.HandleFunc("GET /api/v1/apps/{name}/previews/{number}/env", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListPRPreviewEnv))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/env", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetPRPreviewEnv))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/previews/{number}/env/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeletePRPreviewEnv))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/extend", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleExtendPreviewEnvironment))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/approve", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleApprovePreviewEnvironment))

	// Telemetry query: metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleQueryMetrics))
}
