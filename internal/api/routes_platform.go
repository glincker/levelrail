package api

import "net/http"

// registerPlatformRoutes is Handler()'s second half: nodes, certs,
// ingress, email, domains, backups, restore, storage, git provider
// integrations, and audit. See routes.go's own doc comment for why the
// split.
func (rt *Router) registerPlatformRoutes(mux *http.ServeMux) {
	rt.registerPlatformAppRoutes(mux)
	rt.registerPlatformProjectRoutes(mux)
	rt.registerPlatformProjectRoutesPart2(mux)
	rt.registerPlatformSettingsRoutes(mux)
	rt.registerPlatformIntegrationRoutes(mux)
	rt.registerPlatformMiscRoutes(mux)
	rt.registerPlatformVolumeRoutes(mux)
	rt.registerPlatformAuditRoutes(mux)
}

func (rt *Router) registerPlatformAppRoutes(mux *http.ServeMux) {
	// Secrets. Set-only: there is deliberately no GET,
	// returning a value (even to its own owner over an authenticated
	// session) is exactly the kind of exposure envelope encryption
	// exists to avoid.
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetSecret))

	// List known secret keys (never values) and toggle a key's lock.
	// GET at AbilityRead, matching GET .../git-source's own
	// GET=Read/PUT=WriteSensitive split just below: a key NAME is no
	// more sensitive than a git-source's connection config.
	mux.HandleFunc("GET /api/v1/apps/{name}/secrets", rt.requireAbility(AbilityRead, rt.handleListSecrets))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/lock", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetSecretLock))

	// Git source (a deferred follow-up, git_sources.go):
	// persist a repo/branch/build config per app so a git push can
	// auto-deploy it, the multi-app evolution of internal/webhook's own
	// single-app, env-var-configured Config. AbilityWriteSensitive for
	// PUT/DELETE, matching PUT .../secrets/{key} above: connecting a repo
	// accepts an optional live deploy token in the same request body.
	mux.HandleFunc("GET /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityRead, rt.handleGetGitSource))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetGitSource))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/git-source", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteGitSource))

	// Git push webhook (git_webhook.go), the per-app-URL evolution of the
	// original static POST /webhook (still mounted separately by
	// cmd/levelrail/main.go for the single-app, env-var-configured path).
	// Deliberately unauthenticated, like that route: GitHub cannot
	// present a session or API token, so this is not wrapped in
	// requireAbility. Its own per-app HMAC signature check (the secret
	// generated at connect time, PUT .../git-source above) is what stands
	// in for auth here, the same trust boundary internal/webhook.Handler's
	// own doc comment establishes for the single-app path.
	mux.HandleFunc("POST /api/v1/webhooks/github/{name}", rt.handleGitPushWebhook)

	// Recent webhook deliveries (webhook_deliveries.go): real visibility
	// into what a git provider actually sent, for the exact route above,
	// plus a manual replay of a stored delivery. AbilityRead for the
	// list, matching GET .../git-source; AbilityDeploy for replay,
	// matching POST .../deploys, since a replay can trigger the same
	// real build/deploy side effect.
	mux.HandleFunc("GET /api/v1/apps/{name}/webhook-deliveries", rt.requireAbility(AbilityRead, rt.handleListWebhookDeliveries))
	mux.HandleFunc("POST /api/v1/apps/{name}/webhook-deliveries/{id}/replay", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleReplayWebhookDelivery))

	// Preview environments per pull request (preview_environments.go/
	// preview_environments_handlers.go): opt-in per app, off by default.
	// AbilityWriteSensitive for the toggle, matching PUT .../git-source's
	// own tier since it's the same connect-time-adjacent configuration
	// surface; AbilityRead for the list, matching GET .../git-source;
	// AbilityDeploy for the manual teardown, the same lifecycle-action
	// tier POST .../restart and POST .../stop already use.
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-settings", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetPreviewEnabled))
	mux.HandleFunc("GET /api/v1/apps/{name}/previews", rt.requireAbility(AbilityRead, rt.handleListPreviewEnvironments))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/teardown", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleTeardownPreviewEnvironment))

	// Preview environment TTL sweep (preview_environments_sweep.go): the
	// fallback for a pull-request-closed webhook delivery that never
	// arrived, cross-app since staleness is a control-plane-wide sweep,
	// not a per-app one. Same manual-trigger-alongside-the-automatic-path
	// shape as POST .../previews/{number}/teardown above, same
	// AbilityDeploy tier.
	mux.HandleFunc("POST /api/v1/previews/sweep", rt.requireAbility(AbilityDeploy, rt.handleSweepPreviewEnvironments))

	// Telemetry query: metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryMetrics))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryLogs))
	// Cross-app resource usage ranking (app_resource_usage.go): a
	// literal segment, so Go's ServeMux resolves it ahead of the
	// {name} wildcard on GET /api/v1/apps/{name} in routes.go.
	mux.HandleFunc("GET /api/v1/apps/resource-usage", rt.requireAbility(AbilityRead, rt.handleAppResourceUsage))
	// Live log tail (additive to the historical search route just above,
	// see handleLiveLogStream's own doc comment): AbilityRead, the same
	// passive-visibility boundary as every other view of telemetry data
	// in this router, including the deploy-log stream at
	// GET .../deploys/{deployId}/logs above.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/stream", rt.requireAbility(AbilityRead, rt.handleLiveLogStream))
	// Log export (a plain-text attachment of a bounded window, see
	// logs_download.go): same AbilityRead boundary as the query/stream
	// routes above, since it reads the same store through the same
	// telemetry.QueryLogs call, nothing more sensitive than either.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/download", rt.requireAbility(AbilityRead, rt.handleDownloadLogs))

	// Alerting: threshold and crashloop rules scoped
	// to one app, fanned through a *alerting.DB when configured (see
	// WithAlertRules).
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityRead, rt.handleListAlertRules))
	mux.HandleFunc("PUT /api/v1/apps/{name}/alerts/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateAlertRule))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/alerts/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAlertRule))

	// Scheduled tasks: run an arbitrary command inside this app's
	// container on a cron schedule (internal/scheduledtask). CRUD sits at
	// AbilityWrite, the same config-mutation tier alert rules above and
	// env var updates already use; "run now" sits one tier up at
	// AbilityDeploy, matching restart/stop/start above, since it has the
	// identical immediate side effect (a real command actually runs
	// inside a running container right now), not just a config change.
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateScheduledTask))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks", rt.requireAbility(AbilityRead, rt.handleListScheduledTasks))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbility(AbilityRead, rt.handleGetScheduledTask))
	mux.HandleFunc("PUT /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateScheduledTask))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteScheduledTask))
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks/{id}/run", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRunScheduledTaskNow))

}

func (rt *Router) registerPlatformProjectRoutes(mux *http.ServeMux) {
	// Feature flags (feature_flags.go): a boolean plus optional gradual
	// rollout percentage a running app's own code reads live via the
	// evaluate route below, never baked into a container at create time.
	// CRUD sits at AbilityWrite/AbilityRead, the same ordinary
	// config-mutation tier scheduled tasks above use. Evaluate is a
	// separate, flat, AbilityRead-only route (no app name in its URL:
	// see handleEvaluateFeatureFlag's own doc comment for why), the
	// hot-path endpoint an app calls at runtime with a minimally-scoped
	// read-only token, reusing the existing token/ability system with no
	// new auth surface.
	mux.HandleFunc("POST /api/v1/apps/{name}/flags", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateFeatureFlag))
	mux.HandleFunc("GET /api/v1/apps/{name}/flags", rt.requireAbility(AbilityRead, rt.handleListFeatureFlags))
	mux.HandleFunc("GET /api/v1/apps/{name}/flags/{id}", rt.requireAbility(AbilityRead, rt.handleGetFeatureFlag))
	mux.HandleFunc("PUT /api/v1/apps/{name}/flags/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateFeatureFlag))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/flags/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteFeatureFlag))
	mux.HandleFunc("GET /api/v1/flags/evaluate/{key}", rt.requireAbility(AbilityRead, rt.handleEvaluateFeatureFlag))

	// Tags (tags.go): arbitrary operator-defined labels for organizing
	// and filtering apps, independent of the project/environment
	// hierarchy. The global collection lives flat under /api/v1/tags
	// (create/list/delete, plus filter-by-tag); attach/detach is scoped
	// under the app it applies to, the same nesting scheduled-tasks/flags
	// above use for their own per-app child resources.
	mux.HandleFunc("POST /api/v1/tags", rt.requireAbility(AbilityWrite, rt.handleCreateTag))
	mux.HandleFunc("GET /api/v1/tags", rt.requireAbility(AbilityRead, rt.handleListTags))
	mux.HandleFunc("DELETE /api/v1/tags/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteTag))
	mux.HandleFunc("GET /api/v1/tags/{id}/apps", rt.requireAbility(AbilityRead, rt.handleListAppsByTag))
	mux.HandleFunc("GET /api/v1/apps/{name}/tags", rt.requireAbility(AbilityRead, rt.handleListAppTags))
	mux.HandleFunc("POST /api/v1/apps/{name}/tags", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleAttachAppTag))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/tags/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDetachAppTag))

	// Deploy-outcome notifications (wave-2 roadmap item #5): a Slack/
	// Discord/Telegram/generic-webhook/email ping fired once per deploy
	// attempt reaching a terminal state, distinct from the threshold/
	// crashloop alert rules just above (see
	// internal/alerting/deploy_notify.go's own doc comment). Also fanned
	// through *alerting.DB when configured (see WithDeployNotifyTargets).
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateDeployNotifyTarget))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbility(AbilityRead, rt.handleListDeployNotifyTargets))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/deploy-notify-targets/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteDeployNotifyTarget))

	// Notification channels: the global, connect-once destination the
	// deploy-notify-target routes above attach to by channel_id.
	// AbilityWrite, same tier as deploy-notify-targets' own POST.
	mux.HandleFunc("GET /api/v1/notification-channels", rt.requireAbility(AbilityRead, rt.handleListNotificationChannels))
	mux.HandleFunc("POST /api/v1/notification-channels", rt.requireAbility(AbilityWrite, rt.handleCreateNotificationChannel))
	mux.HandleFunc("PUT /api/v1/notification-channels/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateNotificationChannel))
	mux.HandleFunc("DELETE /api/v1/notification-channels/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/test", rt.requireAbility(AbilityWrite, rt.handleTestNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/{id}/test", rt.requireAbility(AbilityWrite, rt.handleTestExistingNotificationChannel))
	mux.HandleFunc("GET /api/v1/notification-channels/{id}/deliveries", rt.requireAbility(AbilityRead, rt.handleListNotificationDeliveries))

	// Prometheus remote read. Gated by requireAbility the
	// same as every other read route, not left open: leaving a metrics
	// endpoint unauthenticated would let any caller pull every service's
	// resource usage. Prometheus's own remote_read config supports
	// bearer-token auth (authorization.credentials), which is exactly an
	// API token scoped to at least "read" (see the token management
	// routes above); requireAbility already accepts one the same way it
	// does for every JSON route, this isn't a special case.
	mux.HandleFunc("POST /api/v1/prometheus/read", rt.requireAbility(AbilityRead, rt.handlePrometheusRead))

	// Projects (projects.go): a lightweight, non-auth organizational
	// grouping, explicitly not the deferred Phase 4 teams/RBAC work (see
	// that file's own package doc comment). AbilityRead/AbilityWrite,
	// the same ordinary boundary apps/databases CRUD already uses, not
	// AbilityRoot: unlike a node (real infrastructure),
	// creating or deleting a project has no fleet-level consequence.
	mux.HandleFunc("GET /api/v1/projects", rt.requireAbility(AbilityRead, rt.handleListProjects))
	mux.HandleFunc("POST /api/v1/projects", rt.requireAbility(AbilityWrite, rt.handleCreateProject))
	mux.HandleFunc("GET /api/v1/projects/{id}", rt.requireAbility(AbilityRead, rt.handleGetProject))
	mux.HandleFunc("DELETE /api/v1/projects/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteProject))
	// Bulk restart every app filed under this project (project_restart.go):
	// AbilityDeploy, the same ability POST /api/v1/apps/{name}/restart
	// itself uses, not AbilityWrite, since this triggers a real container
	// recreation rather than an ordinary label edit.
	mux.HandleFunc("POST /api/v1/projects/{id}/restart", rt.requireAbility(AbilityDeploy, rt.handleRestartProject))

	// Stop/start (handleStopProject/handleStartProject's own doc
	// comments): same AbilityDeploy tier as the per-app
	// POST /api/v1/apps/{name}/stop and .../start routes, the same class
	// of lifecycle action, applied to every app and database in the
	// project at once.
	mux.HandleFunc("POST /api/v1/projects/{id}/stop", rt.requireAbility(AbilityDeploy, rt.handleStopProject))
	mux.HandleFunc("POST /api/v1/projects/{id}/start", rt.requireAbility(AbilityDeploy, rt.handleStartProject))

	// Organizations (organizations.go): groups projects, same ordinary
	// AbilityRead/AbilityWrite boundary as projects above.
	mux.HandleFunc("GET /api/v1/organizations", rt.requireAbility(AbilityRead, rt.handleListOrganizations))
	mux.HandleFunc("POST /api/v1/organizations", rt.requireAbility(AbilityWrite, rt.handleCreateOrganization))
	mux.HandleFunc("GET /api/v1/organizations/{id}", rt.requireAbility(AbilityRead, rt.handleGetOrganization))
	mux.HandleFunc("DELETE /api/v1/organizations/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteOrganization))
	mux.HandleFunc("PUT /api/v1/projects/{id}/organization", rt.requireAbility(AbilityWrite, rt.handleSetProjectOrganization))

	// Shared env vars every project filed under this organization
	// inherits (organization_env.go), the base layer beneath
	// projects/{id}/env below.
	mux.HandleFunc("GET /api/v1/organizations/{id}/env", rt.requireAbility(AbilityRead, rt.handleGetOrganizationEnv))
	mux.HandleFunc("PUT /api/v1/organizations/{id}/env", rt.requireAbility(AbilityWrite, rt.handleSetOrganizationEnv))

	// Secret-backed shared env vars at the organization tier
	// (shared_env_secrets.go/organization_env.go): the encrypted
	// counterpart to the plain-value GET/PUT above, reusing
	// internal/secrets.Manager (Router.secrets) for the actual
	// encryption.
	mux.HandleFunc("GET /api/v1/organizations/{id}/env/all", rt.requireAbility(AbilityRead, rt.handleListOrganizationEnvAll))
	mux.HandleFunc("GET /api/v1/organizations/{id}/env/secrets", rt.requireAbility(AbilityRead, rt.handleListOrganizationEnvSecretKeys))
	mux.HandleFunc("PUT /api/v1/organizations/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleSetOrganizationEnvSecret))
	mux.HandleFunc("DELETE /api/v1/organizations/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleDeleteOrganizationEnvSecret))

}

func (rt *Router) registerPlatformProjectRoutesPart2(mux *http.ServeMux) {
	// Environments (environments.go): staging/production-style labels
	// scoped to a project, tagged onto a service via its own app route.
	mux.HandleFunc("GET /api/v1/projects/{id}/environments", rt.requireAbility(AbilityRead, rt.handleListEnvironments))
	mux.HandleFunc("POST /api/v1/projects/{id}/environments", rt.requireAbility(AbilityWrite, rt.handleCreateEnvironment))
	mux.HandleFunc("PATCH /api/v1/environments/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateEnvironment))
	mux.HandleFunc("DELETE /api/v1/environments/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteEnvironment))
	mux.HandleFunc("PUT /api/v1/apps/{name}/environment", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppEnvironment))

	// Clone environment (environment_clone.go): copy a whole
	// environment's app set plus config into a brand-new environment,
	// AbilityDeploy-gated like promote.go's own trigger route since it
	// actually deploys real containers, not just database rows.
	mux.HandleFunc("GET /api/v1/environments/{id}/clone/preview", rt.requireAbility(AbilityRead, rt.handleEnvironmentClonePreview))
	mux.HandleFunc("POST /api/v1/environments/{id}/clone", rt.requireAbility(AbilityDeploy, rt.handleEnvironmentClone))

	// Deploy approvals (deploy_approvals.go): the real two-person gate
	// behind a protected environment's confirm: true, replacing a
	// same-actor confirm flag. List/get are AbilityRead; approve/reject
	// are AbilityDeploy, the same tier the gated deploy/promote itself
	// needs, plus loadDecidableApproval's own same-actor rejection.
	mux.HandleFunc("GET /api/v1/deploy-approvals", rt.requireAbility(AbilityRead, rt.handleListDeployApprovals))
	mux.HandleFunc("GET /api/v1/deploy-approvals/{id}", rt.requireAbility(AbilityRead, rt.handleGetDeployApproval))
	mux.HandleFunc("POST /api/v1/deploy-approvals/{id}/approve", rt.requireAbility(AbilityDeploy, rt.handleApproveDeployApproval))
	mux.HandleFunc("POST /api/v1/deploy-approvals/{id}/reject", rt.requireAbility(AbilityDeploy, rt.handleRejectDeployApproval))

	// Shared env vars every service tagged with this environment inherits
	// (environment_env.go): the tier between organizations/{id}/env and
	// projects/{id}/env above and a service's own env below.
	mux.HandleFunc("GET /api/v1/environments/{id}/env", rt.requireAbility(AbilityRead, rt.handleGetEnvironmentEnv))
	mux.HandleFunc("PUT /api/v1/environments/{id}/env", rt.requireAbility(AbilityWrite, rt.handleSetEnvironmentEnv))

	// Secret-backed shared env vars at the environment tier, mirroring
	// the organization tier's own four routes above.
	mux.HandleFunc("GET /api/v1/environments/{id}/env/all", rt.requireAbility(AbilityRead, rt.handleListEnvironmentEnvAll))
	mux.HandleFunc("GET /api/v1/environments/{id}/env/secrets", rt.requireAbility(AbilityRead, rt.handleListEnvironmentEnvSecretKeys))
	mux.HandleFunc("PUT /api/v1/environments/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleSetEnvironmentEnvSecret))
	mux.HandleFunc("DELETE /api/v1/environments/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleDeleteEnvironmentEnvSecret))

	// Shared env vars every app filed under this project inherits
	// (project_env.go): same AbilityRead/AbilityWrite boundary as the
	// project CRUD routes just above.
	mux.HandleFunc("GET /api/v1/projects/{id}/env", rt.requireAbility(AbilityRead, rt.handleGetProjectEnv))
	mux.HandleFunc("PUT /api/v1/projects/{id}/env", rt.requireAbility(AbilityWrite, rt.handleSetProjectEnv))

	// Secret-backed shared env vars at the project tier, mirroring the
	// organization/environment tiers' own four routes above.
	mux.HandleFunc("GET /api/v1/projects/{id}/env/all", rt.requireAbility(AbilityRead, rt.handleListProjectEnvAll))
	mux.HandleFunc("GET /api/v1/projects/{id}/env/secrets", rt.requireAbility(AbilityRead, rt.handleListProjectEnvSecretKeys))
	mux.HandleFunc("PUT /api/v1/projects/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleSetProjectEnvSecret))
	mux.HandleFunc("DELETE /api/v1/projects/{id}/env/secrets/{key}", rt.requireAbility(AbilityWrite, rt.handleDeleteProjectEnvSecret))

	// Move an app/database into (or out of, with project_id: "") a
	// project: the project-kind counterpart to PUT /apps/{name}/node
	// and PUT /databases/{name}/node above, same narrow-dedicated-
	// mutation shape those routes establish (appResource/
	// databaseResource's own ProjectID field is response-only, exactly
	// like NodeID, see handleSetAppProject's own doc comment for why).
	// AbilityWrite, not AbilityRoot: project membership is an ordinary
	// organizational edit, not infrastructure placement, so it sits at
	// the same sensitivity as the rest of apps/databases CRUD rather
	// than the node routes' fleet-level boundary.
	mux.HandleFunc("PUT /api/v1/apps/{name}/project", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppProject))
	mux.HandleFunc("PUT /api/v1/databases/{name}/project", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleSetDatabaseProject))

	// Set (or clear, with a nil body field) a database's resource limits:
	// databaseResource's own Resources field doc comment explains why
	// this is a dedicated route rather than folded into handleUpdateApp's
	// general-PUT equivalent, which has no database counterpart.
	// AbilityWrite, the same ordinary-config-edit sensitivity as the
	// project routes just above, not AbilityRoot: a resource cap is not
	// fleet-level placement.
	mux.HandleFunc("PUT /api/v1/databases/{name}/resources", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleSetDatabaseResources))

	// Nodes: fleet-level infrastructure, not scoped to any
	// one app, so every route here requires AbilityRoot specifically
	// rather than AbilityRead/AbilityWrite: minting a join token or
	// removing a node is a materially more sensitive operation than
	// editing one app's config, the same reasoning secrets.go's
	// AbilityWriteSensitive already applies one level down at the
	// per-app layer.
	mux.HandleFunc("GET /api/v1/nodes", rt.requireAbility(AbilityRoot, rt.handleListNodes))
	mux.HandleFunc("GET /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleGetNode))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteNode))
	mux.HandleFunc("PUT /api/v1/nodes/{id}/workloads", rt.requireAbility(AbilityRoot, rt.handleSetNodeWorkloads))
	mux.HandleFunc("POST /api/v1/nodes/join-tokens", rt.requireAbility(AbilityRoot, rt.handleCreateNodeJoinToken))
	// Health, cordon, drain, same AbilityRoot boundary as
	// every other node route above.
	mux.HandleFunc("GET /api/v1/nodes/{id}/health", rt.requireAbility(AbilityRoot, rt.handleGetNodeHealth))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cordon", rt.requireAbility(AbilityRoot, rt.handleCordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/uncordon", rt.requireAbility(AbilityRoot, rt.handleUncordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", rt.requireAbility(AbilityRoot, rt.handleDrainNode))
	// Mesh status and key rotation, same AbilityRoot boundary: WireGuard
	// peer/handshake data and a node's own key material are fleet
	// infrastructure, not app-scoped, matching every other node route
	// above.
	mux.HandleFunc("GET /api/v1/mesh", rt.requireAbility(AbilityRoot, rt.handleGetMeshStatus))
	mux.HandleFunc("POST /api/v1/nodes/{id}/mesh/rotate-key", rt.requireAbility(AbilityRoot, rt.handleRotateNodeMeshKey))
	// Node-level metrics (sum of per-container samples for everything
	// placed on this node, see handleQueryNodeMetrics's own doc comment
	// for exactly what that does and doesn't mean): same AbilityRoot
	// boundary and nil-telemetry 501 shape as every route above, and the
	// same query-param contract as GET /apps/{name}/metrics above it.
	mux.HandleFunc("GET /api/v1/nodes/{id}/metrics", rt.requireAbility(AbilityRoot, rt.handleQueryNodeMetrics))
	// Fleet-wide latest CPU/memory/disk snapshot plus rollup
	// (node_resource_usage.go), the node-scoped counterpart to
	// GET /apps/resource-usage above: registered as a literal path
	// segment under /nodes/, which Go's net/http mux matches ahead of
	// the /nodes/{id} wildcard above regardless of registration order,
	// the same precedent apps/resource-usage already relies on.
	mux.HandleFunc("GET /api/v1/nodes/resource-usage", rt.requireAbility(AbilityRoot, rt.handleFleetResourceUsage))
	// Latest OS-patch reading (internal/telemetry/hostpatch.go's
	// HostPatchCollector), a single current fact rather than a time
	// series, same AbilityRoot boundary as every other node route.
	mux.HandleFunc("GET /api/v1/nodes/{id}/patch-status", rt.requireAbility(AbilityRoot, rt.handleGetNodePatchStatus))

}

func (rt *Router) registerPlatformSettingsRoutes(mux *http.ServeMux) {
	// Certificates (TLS renewal visibility): this project treats
	// "a cert renewal fails silently at 3am" as its central
	// risk to catch before it bites a real user, and until now nothing
	// in the API surfaced certificate state at all. Read-only, so
	// AbilityRead like handleSystemStatus/handleGetNodeHealth, not
	// AbilityRoot: seeing whether a domain's certificate is about to
	// expire is ordinary operator visibility, not a fleet-admin
	// mutation the way the node routes above are.
	mux.HandleFunc("GET /api/v1/certificates", rt.requireAbility(AbilityRead, rt.handleListCertificates))

	// Ingress settings (ACME toggle, platform primary domain, ADR 005's
	// own "Verified" section names this exact gap: real ACME issuance
	// was explicitly unproven, spot-checked against a real domain, not
	// assumed to follow automatically). GET is AbilityRead, the same
	// passive-visibility tier as GET /api/v1/certificates above: reading
	// today's toggle state is ordinary operator visibility. PUT is
	// AbilityRoot, matching handleSetAppNode/handleDrainNode/POST
	// /system/prune's own precedent for "real infrastructure, high
	// blast radius, not an ordinary per-app write": flipping
	// acme_enabled changes what every currently-routed host's
	// certificate automation does, fleet-wide, on the very next ingress
	// reconcile pass, the same class of change node placement and
	// draining already reserve AbilityRoot for.
	mux.HandleFunc("GET /api/v1/settings/ingress", rt.requireAbility(AbilityRead, rt.handleGetIngressSettings))
	mux.HandleFunc("PUT /api/v1/settings/ingress", rt.requireAbility(AbilityRoot, rt.handleUpdateIngressSettings))
	mux.HandleFunc("GET /api/v1/settings/dashboard-url", rt.requireAbility(AbilityRead, rt.handleGetDashboardURL))
	mux.HandleFunc("PUT /api/v1/settings/dashboard-url", rt.requireAbility(AbilityRoot, rt.handleUpdateDashboardURL))

	// Platform-level DNS check (ingress_settings.go): runDomainCheck
	// against the platform's own PrimaryDomain instead of a per-app one.
	// AbilityRoot, matching PUT /api/v1/settings/ingress just above: the
	// result speaks directly to whether that endpoint's ACMEEnabled
	// toggle can actually succeed.
	mux.HandleFunc("GET /api/v1/settings/ingress/check", rt.requireAbility(AbilityRoot, rt.handleCheckIngressDomain))

	// Domain DNS check (domain_check.go): the guidance layer on top of
	// domain connection, so DomainEditor can show an operator the exact
	// DNS record to add and watch it flip to "connected" once it actually
	// resolves. AbilityRead, same passive-visibility tier as GET
	// /apps/{name}/git-source: a live DNS lookup, no write.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/check", rt.requireAbility(AbilityRead, rt.handleCheckDomain))

	// Domain basic auth (domain_basic_auth.go): HTTP Basic Auth
	// protection on one app-owned domain, enforced by Caddy's
	// authentication handler (internal/reconcile/ingress) on the next
	// reconcile pass. GET is AbilityRead, the same passive-visibility
	// tier GET /api/v1/settings/cloudflare-tunnel uses for its own
	// has_token-shaped read. PUT/DELETE are AbilityRoot, matching PUT/
	// DELETE /api/v1/settings/cloudflare-tunnel: this changes how a
	// live, currently-routed host is secured, the same "real
	// infrastructure, high blast radius" class of change Cloudflare
	// Tunnel/DNS and the ACME toggle already reserve AbilityRoot for.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbility(AbilityRead, rt.handleGetDomainBasicAuth))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetDomainBasicAuth))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleClearDomainBasicAuth))

	// Maintenance mode: GET is AbilityRead, matching the auth routes'
	// own passive-visibility tier. PUT/DELETE are AbilityDeploy, not
	// AbilityRoot: this changes an app's runtime routing behavior, the
	// same "app lifecycle" tier POST .../stop and .../start already
	// use, not a credential-bearing change like basic auth.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbility(AbilityRead, rt.handleGetDomainMaintenance))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainMaintenance))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/maintenance", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainMaintenance))

	// BYO TLS certificate upload (domain_tls_cert.go): an operator-
	// supplied certificate/key pair used in place of Caddy's automatic
	// ACME/internal issuance for one app-owned domain, enforced on the
	// next ingress reconcile pass. GET is AbilityRead, matching the auth
	// routes' own passive-visibility tier. PUT/DELETE are AbilityRoot,
	// the same "real infrastructure, high blast radius" tier PUT/DELETE
	// .../domains/{domain}/auth already reserves for a credential-bearing
	// change.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbility(AbilityRead, rt.handleGetDomainTLSCert))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetDomainTLSCert))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/tls-cert", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleClearDomainTLSCert))

	// Opt-in WAF and rate limiting (domain_waf.go): OWASP Coraza and
	// Caddy's rate_limit handler on one app-owned domain, enforced on
	// the next ingress reconcile pass. GET is AbilityRead, matching the
	// auth/maintenance routes' own passive-visibility tier. PUT/DELETE
	// are AbilityDeploy, the same "app lifecycle, runtime routing
	// behavior, not a credential" tier PUT/DELETE .../maintenance
	// already uses: unlike basic auth or a BYO cert, nothing here is
	// secret material.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbility(AbilityRead, rt.handleGetDomainWAF))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainWAF))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/waf", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainWAF))

	// Domain redirect (domain_redirect.go): points one app-owned domain
	// at an arbitrary target URL, enforced by Caddy's static_response
	// handler on the next ingress reconcile pass. GET is AbilityRead,
	// matching the auth/maintenance/waf routes' own passive-visibility
	// tier. PUT/DELETE are AbilityDeploy, the same "app lifecycle,
	// runtime routing behavior, not a credential" tier PUT/DELETE
	// .../maintenance and .../waf already use.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbility(AbilityRead, rt.handleGetDomainRedirect))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainRedirect))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/redirect", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainRedirect))

	// Custom error pages (domain_error_pages.go): replaces Caddy's bare
	// default error text, or whatever the backend itself returned, with
	// the operator's own HTML for a fixed set of status codes (404, 500,
	// 502, 503), enforced on the next ingress reconcile pass. GET is
	// AbilityRead; PUT/DELETE are AbilityDeploy, the same tier
	// PUT/DELETE .../waf already uses.
	mux.HandleFunc("GET /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbility(AbilityRead, rt.handleGetDomainErrorPages))
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetDomainErrorPage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/error-pages", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleClearDomainErrorPages))

}

func (rt *Router) registerPlatformIntegrationRoutes(mux *http.ServeMux) {
	// Email settings: same precedent as ingress settings just above.
	// GET is AbilityRead; PUT is AbilityRoot, real infrastructure config.
	mux.HandleFunc("GET /api/v1/settings/email", rt.requireAbility(AbilityRead, rt.handleGetEmailSettings))
	mux.HandleFunc("PUT /api/v1/settings/email", rt.requireAbility(AbilityRoot, rt.handleUpdateEmailSettings))

	// Cloudflare Tunnel (instance-level, one connection per control
	// plane): GET is AbilityRead; PUT/DELETE are AbilityRoot, matching
	// PUT /api/v1/settings/email's own tier for infrastructure config
	// that changes how this control plane is reachable.
	mux.HandleFunc("GET /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRead, rt.handleGetCloudflareTunnelSettings))
	mux.HandleFunc("PUT /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudflareTunnelSettings))
	mux.HandleFunc("DELETE /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRoot, rt.handleDisconnectCloudflareTunnel))
	mux.HandleFunc("GET /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRead, rt.handleGetCloudflareDNSSettings))
	mux.HandleFunc("PUT /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudflareDNSSettings))
	mux.HandleFunc("DELETE /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRoot, rt.handleDisconnectCloudflareDNS))
	// Route53 DNS-01: a second, independent ACME DNS-01 provider, same
	// tier as cloudflare-dns above.
	mux.HandleFunc("GET /api/v1/settings/route53-dns", rt.requireAbility(AbilityRead, rt.handleGetRoute53DNSSettings))
	mux.HandleFunc("PUT /api/v1/settings/route53-dns", rt.requireAbility(AbilityRoot, rt.handleUpdateRoute53DNSSettings))
	mux.HandleFunc("DELETE /api/v1/settings/route53-dns", rt.requireAbility(AbilityRoot, rt.handleDisconnectRoute53DNS))

	// External HashiCorp Vault (instance-level, one connection per
	// control plane): same GET AbilityRead / PUT+DELETE AbilityRoot tier
	// as Cloudflare Tunnel above.
	mux.HandleFunc("GET /api/v1/settings/vault", rt.requireAbility(AbilityRead, rt.handleGetVaultSettings))
	mux.HandleFunc("PUT /api/v1/settings/vault", rt.requireAbility(AbilityRoot, rt.handleUpdateVaultSettings))
	mux.HandleFunc("DELETE /api/v1/settings/vault", rt.requireAbility(AbilityRoot, rt.handleDisconnectVault))

	// Built-in container registry (instance-level, one registry per
	// control plane): GET is AbilityRead; PUT/DELETE are AbilityRoot,
	// matching PUT /api/v1/settings/cloudflare-tunnel's own tier for
	// infrastructure config that runs a system container and generates
	// credentials.
	mux.HandleFunc("GET /api/v1/settings/registry", rt.requireAbility(AbilityRead, rt.handleGetRegistrySettings))
	mux.HandleFunc("PUT /api/v1/settings/registry", rt.requireAbility(AbilityRoot, rt.handleUpdateRegistrySettings))
	mux.HandleFunc("DELETE /api/v1/settings/registry", rt.requireAbility(AbilityRoot, rt.handleDisableRegistry))

	// Built-in registry catalog (registry_catalog.go): read-only browsing
	// for the app-creation "existing image" step's repository/tag picker.
	// AbilityRead, same tier as the settings GET just above: no secret is
	// ever returned, the resolved password is only used server-side to
	// authenticate the upstream catalog query.
	mux.HandleFunc("GET /api/v1/registry/repositories", rt.requireAbility(AbilityRead, rt.handleListRegistryRepositories))
	mux.HandleFunc("GET /api/v1/registry/tags", rt.requireAbility(AbilityRead, rt.handleListRegistryTags))

	// Domains (centralized cross-app list, web/src/routes/domains):
	// every service_domains row, AbilityRead like GET /api/v1/apps,
	// no new ability tier: this is the same data DomainEditor already
	// exposes per-app, aggregated across every app in one read-only call.
	mux.HandleFunc("GET /api/v1/domains", rt.requireAbility(AbilityRead, rt.handleListDomains))

	// Static sites (build.type: static): read-only
	// dashboard visibility for sites served directly by embedded Caddy
	// with no container, closing the gap flagged when static-site
	// support (migration 0015) first landed. AbilityRead, the same
	// passive-visibility boundary as handleListCertificates above: no
	// create/update/delete route exists yet because the backend has no
	// mutation path for a static site beyond internal/deploy.Pipeline's
	// own git-push-triggered write.
	mux.HandleFunc("GET /api/v1/static-sites", rt.requireAbility(AbilityRead, rt.handleListStaticSites))

	// Backup targets (connected S3-compatible buckets a database backup
	// can be uploaded to). Create and delete need AbilityWriteSensitive:
	// create because its request body carries live bucket credentials,
	// delete because it gates access to the same. List/get are ordinary
	// AbilityRead, the same boundary handleListCertificates/
	// handleListStaticSites already draw between visibility and mutation.
	mux.HandleFunc("GET /api/v1/backup-targets", rt.requireAbility(AbilityRead, rt.handleListBackupTargets))
	mux.HandleFunc("POST /api/v1/backup-targets", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateBackupTarget))
	mux.HandleFunc("GET /api/v1/backup-targets/{id}", rt.requireAbility(AbilityRead, rt.handleGetBackupTarget))
	mux.HandleFunc("PUT /api/v1/backup-targets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateBackupTarget))
	mux.HandleFunc("DELETE /api/v1/backup-targets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteBackupTarget))
	mux.HandleFunc("POST /api/v1/backup-targets/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestBackupTarget))

	// Registry credentials (registry_credentials.go): same ability tiers
	// as backup targets just above, same reasoning (POST/PUT/DELETE
	// handle live pull credentials).
	mux.HandleFunc("GET /api/v1/registry-credentials", rt.requireAbility(AbilityRead, rt.handleListRegistryCredentials))
	mux.HandleFunc("POST /api/v1/registry-credentials", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateRegistryCredential))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityRead, rt.handleGetRegistryCredential))
	mux.HandleFunc("PUT /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateRegistryCredential))
	mux.HandleFunc("DELETE /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteRegistryCredential))
	mux.HandleFunc("POST /api/v1/registry-credentials/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestRegistryCredential))

	// Registry credential browsing (registry_catalog.go): repository/tag
	// lookup for a stored external credential, the same generic catalog
	// client GET /api/v1/registry/repositories and /api/v1/registry/tags
	// use above for the built-in registry. AbilityReadSensitive, the same
	// tier GET /api/v1/git-providers and the github-app/gitlab-app/
	// bitbucket-app repo-browsing routes below use: read-only, but only
	// works because a stored credential's secret is resolved server-side.
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}/repositories", rt.requireAbility(AbilityReadSensitive, rt.handleListRegistryCredentialRepositories))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}/tags", rt.requireAbility(AbilityReadSensitive, rt.handleListRegistryCredentialTags))

	// Aggregated git provider capability summary (git_providers.go): one
	// AbilityReadSensitive call the git-source picker uses instead of the
	// three AbilityRoot status endpoints below, so a non-root deploy-scoped
	// user can open the app creation wizard without hitting a 403. See
	// handleListGitProviders's own doc comment.
	mux.HandleFunc("GET /api/v1/git-providers", rt.requireAbility(AbilityReadSensitive, rt.handleListGitProviders))

}

func (rt *Router) registerPlatformMiscRoutes(mux *http.ServeMux) {
	// GitHub App connection: the manifest-based registration flow,
	// installation, and repo/branch browsing through it
	// (internal/api/github_app.go, github_app_register.go,
	// github_app_repos.go). Every route that reads or mutates the
	// connection itself (status, register/start, callback, installed,
	// disconnect) is AbilityRoot, matching PUT /api/v1/settings/ingress's
	// own precedent above rather than the plain AbilityRead most other
	// GET routes use: this is platform-wide configuration with a real
	// external-account relationship behind it (an installed GitHub App
	// can read a private repository's contents), the same "real
	// infrastructure, high blast radius" class ingress settings and node
	// placement already reserve this tier for, not an ordinary per-app
	// read. register/start, callback, and installed are all real,
	// full-page browser navigations (GitHub's manifest flow is
	// inherently that, not a fetch call), not XHR/fetch calls: a session
	// cookie is implicitly AbilityRoot (requireAbility's own doc
	// comment), so the operator's own logged-in browser satisfies this
	// gate the same way it satisfies every other AbilityRoot route.
	//
	// Repo and branch listing are the one exception, at
	// AbilityReadSensitive: see handleListGitHubAppRepos's own doc
	// comment for why reading already-connected repo/branch names is a
	// materially different (lower) risk class than changing the
	// connection itself. use-as-source is AbilityWriteSensitive, matching
	// PUT .../git-source's own tier and GitLab/Bitbucket's own
	// use-as-source routes below: it performs that exact action.
	mux.HandleFunc("GET /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleGetGitHubAppStatus))
	mux.HandleFunc("DELETE /api/v1/github-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitHubApp))
	mux.HandleFunc("PUT /api/v1/github-app/manual", rt.requireAbility(AbilityRoot, rt.handleConnectGitHubAppManually))
	mux.HandleFunc("GET /api/v1/github-app/register/preview", rt.requireAbility(AbilityRoot, rt.handleGetGitHubAppManifestPreview))
	mux.HandleFunc("GET /api/v1/github-app/register/start", rt.requireAbility(AbilityRoot, rt.handleStartGitHubAppRegistration))
	mux.HandleFunc("GET /api/v1/github-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitHubAppCallback))
	mux.HandleFunc("GET /api/v1/github-app/installed", rt.requireAbility(AbilityRoot, rt.handleGitHubAppInstalled))
	mux.HandleFunc("GET /api/v1/github-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppRepos))
	mux.HandleFunc("GET /api/v1/github-app/repos/{owner}/{repo}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitHubAppBranches))
	mux.HandleFunc("POST /api/v1/github-app/repos/{owner}/{repo}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGitHubRepoAsSource))

	// GitLab App: the OAuth-Application counterpart of the GitHub App
	// routes above, same ability tiers for the same reasons. connect and
	// callback are real, full-page browser navigations (GitLab's OAuth2
	// authorization endpoint requires that), not fetch calls.
	// use-as-source is AbilityWriteSensitive, matching PUT
	// .../git-source's own tier: it performs that exact action.
	mux.HandleFunc("GET /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleGetGitLabAppStatus))
	mux.HandleFunc("PUT /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleConnectGitLabApp))
	mux.HandleFunc("DELETE /api/v1/gitlab-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGitLabApp))
	mux.HandleFunc("GET /api/v1/gitlab-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGitLabAppConnect))
	mux.HandleFunc("GET /api/v1/gitlab-app/callback", rt.requireAbility(AbilityRoot, rt.handleGitLabAppCallback))
	mux.HandleFunc("GET /api/v1/gitlab-app/projects", rt.requireAbility(AbilityReadSensitive, rt.handleListGitLabAppProjects))
	mux.HandleFunc("GET /api/v1/gitlab-app/projects/{id}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGitLabAppBranches))
	mux.HandleFunc("POST /api/v1/gitlab-app/projects/{id}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGitLabProjectAsSource))

	// Bitbucket App: the OAuth-consumer counterpart of the GitLab App
	// routes above, same ability tiers for the same reasons, same
	// two-step "configure, then authorize" shape. Cloud only, no
	// instance_url (docs/design/git-provider-integrations.md section 3).
	mux.HandleFunc("GET /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleGetBitbucketAppStatus))
	mux.HandleFunc("PUT /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleConnectBitbucketApp))
	mux.HandleFunc("DELETE /api/v1/bitbucket-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectBitbucketApp))
	mux.HandleFunc("GET /api/v1/bitbucket-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartBitbucketAppConnect))
	mux.HandleFunc("GET /api/v1/bitbucket-app/callback", rt.requireAbility(AbilityRoot, rt.handleBitbucketAppCallback))
	mux.HandleFunc("GET /api/v1/bitbucket-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListBitbucketAppRepos))
	mux.HandleFunc("GET /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListBitbucketAppBranches))
	mux.HandleFunc("POST /api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseBitbucketRepoAsSource))

	// Gitea App: the OAuth-Application counterpart of the GitLab App
	// routes above, same ability tiers and self-hosted instance_url shape
	// (Gitea is almost always self-hosted). Repos are addressed by an
	// "owner/repo" path pair like Bitbucket's workspace/repoSlug, not a
	// numeric id like GitLab's.
	mux.HandleFunc("GET /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleGetGiteaAppStatus))
	mux.HandleFunc("PUT /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleConnectGiteaApp))
	mux.HandleFunc("DELETE /api/v1/gitea-app", rt.requireAbility(AbilityRoot, rt.handleDisconnectGiteaApp))
	mux.HandleFunc("GET /api/v1/gitea-app/connect", rt.requireAbility(AbilityRoot, rt.handleStartGiteaAppConnect))
	mux.HandleFunc("GET /api/v1/gitea-app/callback", rt.requireAbility(AbilityRoot, rt.handleGiteaAppCallback))
	mux.HandleFunc("GET /api/v1/gitea-app/repos", rt.requireAbility(AbilityReadSensitive, rt.handleListGiteaAppRepos))
	mux.HandleFunc("GET /api/v1/gitea-app/repos/{owner}/{repo}/branches", rt.requireAbility(AbilityReadSensitive, rt.handleListGiteaAppBranches))
	mux.HandleFunc("POST /api/v1/gitea-app/repos/{owner}/{repo}/use-as-source", rt.requireAbility(AbilityWriteSensitive, rt.handleUseGiteaRepoAsSource))

	// Backup history and manual trigger, per database. Trigger needs
	// AbilityWriteSensitive: it starts real work against a live bucket
	// using a previously-stored credential, the same sensitivity class
	// creating or deleting the backup target itself already carries.
	// History listing is ordinary AbilityRead.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleTriggerBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups", rt.requireAbility(AbilityRead, rt.handleListBackupHistory))

	// Instance-wide backup history across every database and app volume,
	// the aggregated counterpart of the per-resource routes above and
	// below: ordinary AbilityRead, same tier as those, since this is only
	// ever a merged read of history metadata already visible per-resource.
	mux.HandleFunc("GET /api/v1/backups", rt.requireAbility(AbilityRead, rt.handleListAllBackups))

	// Download one succeeded backup's own object, streamed straight to
	// the browser. AbilityReadSensitive, not AbilityRead: this returns
	// the actual dump bytes, a full database's worth of content, not
	// metadata about an attempt the way history listing above does; it's
	// also not a mutation, so AbilityWriteSensitive (the tier the trigger
	// route above uses) would overstate it. Not AbilityRoot either,
	// unlike restore.go's own explicit "single most destructive
	// endpoint" reasoning for that tier: restore is irreversible,
	// in-place destruction of live data, a materially worse risk class
	// than a read-only fetch from a bucket, even though this route is a
	// genuine full-database exfiltration primitive on the confidentiality
	// axis. Today every session is implicitly root (requireAbility only
	// gates scoped bearer/MCP tokens), so this choice mainly matters once
	// Phase 4 mints scoped automation tokens against this ability tier.
	// See handleDownloadBackup's own doc comment for the full reasoning.
	mux.HandleFunc("GET /api/v1/databases/{name}/backups/{historyId}/download", rt.requireAbility(AbilityReadSensitive, rt.handleDownloadBackup))

}

func (rt *Router) registerPlatformVolumeRoutes(mux *http.ServeMux) {
	// Delete one specific archived backup on demand, rather than waiting
	// for retention (BackupRetain/BackupRetainDays above) to age it out.
	// AbilityWriteSensitive, matching the manual trigger route above: this
	// destroys a real stored artifact, the same sensitivity class as
	// triggering a backup or deleting the backup target itself, not the
	// AbilityRoot tier restore below uses, since nothing here touches a
	// live database's own data. See handleDeleteBackup's own doc comment.
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backups/{historyId}", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleDeleteBackup))

	// Backup verification: re-download a succeeded backup and confirm it
	// is still intact (checksum, size, and a lightweight structural
	// check), without ever attempting a live restore. AbilityWriteSensitive
	// for the trigger, matching the manual backup trigger route above
	// exactly (see handleVerifyBackup's own doc comment for why); listing
	// past attempts is ordinary AbilityRead, matching history listing
	// above.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups/{historyId}/verify", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleVerifyBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups/{historyId}/verifications", rt.requireAbility(AbilityRead, rt.handleListBackupVerifications))

	// Scheduled backup config, per database (wave-2 roadmap item 6):
	// which backup target, cron schedule, and retention count
	// internal/backup.Scheduler uses for this database, if any.
	// AbilityWriteSensitive for both PUT and DELETE, the same tier the
	// manual trigger route above already uses: this is the config that
	// decides where an unattended, recurring backup ends up, a live
	// bucket with previously-stored credentials, the identical
	// sensitivity class. GET reuses handleGetDatabase's own existing
	// AbilityRead response (databaseResource already carries these three
	// fields, see that handler's own file), so there is no separate GET
	// route here.
	mux.HandleFunc("PUT /api/v1/databases/{name}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleSetBackupSchedule))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleClearBackupSchedule))

	// Public/host-exposed access, per database
	// (database_public_access.go): whether this database's container
	// port is bound to a host port so an operator's own database GUI
	// tool can connect directly. AbilityWriteSensitive for both PUT and
	// DELETE, the same tier scheduled backups above already use: this
	// changes real network exposure, the identical sensitivity class as
	// where an unattended backup ends up. GET reuses handleGetDatabase's
	// own existing AbilityRead response (databaseResource already
	// carries publicly_accessible/public_port), the same "no separate
	// GET route" reasoning the backup-schedule routes above already
	// apply.
	mux.HandleFunc("PUT /api/v1/databases/{name}/public-access", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleSetDatabasePublicAccess))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/public-access", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleClearDatabasePublicAccess))

	// Stop/start, per database (database_stop_start.go). AbilityWriteSensitive,
	// same tier public-access above uses: taking a database offline (and,
	// on start, back online) is real operational impact, not an ordinary
	// desired-state edit.
	mux.HandleFunc("POST /api/v1/databases/{name}/stop", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleStopDatabase))
	mux.HandleFunc("POST /api/v1/databases/{name}/start", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleStartDatabase))

	// Restore, per database (restore.go). AbilityRoot, not
	// AbilityWriteSensitive: see handleTriggerRestore's own doc comment
	// for why this, alone among every backup-related route, needs the
	// same top tier as node management and placement. History listing is
	// ordinary AbilityRead, the same boundary the backup history route
	// above already draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleTriggerRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/restores", rt.requireAbility(AbilityRead, rt.handleListRestoreHistory))

	// Point-in-time restore (pitr.go): enabling/disabling PITR is
	// AbilityWriteSensitive, the same tier creating a backup target or
	// triggering an ordinary backup already uses; base backups are the
	// physical counterpart of an ordinary backup, same tier again.
	// Triggering an actual PITR restore is AbilityRoot, matching the
	// ordinary restore route above for the identical reason: it
	// overwrites a live database's actual data with no way back.
	mux.HandleFunc("POST /api/v1/databases/{name}/pitr", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleEnablePITR))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/pitr", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleDisablePITR))
	mux.HandleFunc("GET /api/v1/databases/{name}/pitr", rt.requireAbility(AbilityRead, rt.handleGetPITRStatus))
	mux.HandleFunc("POST /api/v1/databases/{name}/base-backups", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleTriggerBaseBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/base-backups", rt.requireAbility(AbilityRead, rt.handleListBaseBackupHistory))
	mux.HandleFunc("POST /api/v1/databases/{name}/pitr-restore", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleTriggerPITRRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/pitr-restores", rt.requireAbility(AbilityRead, rt.handleListPITRRestoreHistory))

	// App service volume backups (app_volume_backups.go/
	// app_volume_backup_download.go/app_volume_backup_verify.go): the
	// exact same ability tiers as the database routes just above, applied
	// to a service's named volume instead of a managed database. See
	// those handlers' own doc comments for the per-route reasoning this
	// mirrors.
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/backups", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleTriggerVolumeBackup))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups", rt.requireAbility(AbilityRead, rt.handleListVolumeBackupHistory))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download", rt.requireAbility(AbilityReadSensitive, rt.handleDownloadVolumeBackup))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteVolumeBackup))
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleVerifyVolumeBackup))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications", rt.requireAbility(AbilityRead, rt.handleListVolumeBackupVerifications))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbility(AbilityRead, rt.handleGetVolumeBackupSchedule))
	mux.HandleFunc("PUT /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetVolumeBackupSchedule))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearVolumeBackupSchedule))
	// AbilityRoot, matching the database restore route above exactly: see
	// handleTriggerVolumeRestore's own doc comment for why this is not a
	// lesser risk tier just because the target is a filesystem.
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/restore", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleTriggerVolumeRestore))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/restores", rt.requireAbility(AbilityRead, rt.handleListVolumeRestoreHistory))
	// Restore as a new volume (app_volume_clone_restore.go): the
	// non-destructive counterpart just above, the same AbilityWriteSensitive
	// tier the database clone-restore route below uses, see
	// handleVolumeCloneRestore's own doc comment for why.
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleVolumeCloneRestore))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/clone-restores", rt.requireAbility(AbilityRead, rt.handleListVolumeCloneRestores))
	// Restore as a new database (database_clone_restore.go): the
	// non-destructive counterpart just above, AbilityWriteSensitive
	// rather than AbilityRoot, see handleCloneRestore's own doc comment
	// for why this endpoint doesn't need the top tier the in-place
	// restore route does. History listing is ordinary AbilityRead, the
	// same boundary the in-place restore history route above draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore-as-new", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleCloneRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/clone-restores", rt.requireAbility(AbilityRead, rt.handleListCloneRestores))

}

func (rt *Router) registerPlatformAuditRoutes(mux *http.ServeMux) {
	// Object-storage attachment, per app (apps_storage.go): which
	// connected backup_targets bucket (the same S3-compatible connection
	// a database's scheduled backups can already point at, reused rather
	// than a second "storage target" concept) this app's own container
	// gets S3_* credentials injected from at container-create time.
	// AbilityWriteSensitive for both PUT and DELETE, the same tier
	// scheduled backups and database public access above already use:
	// this changes which live bucket credentials an app's own container
	// receives, the identical sensitivity class. GET reuses
	// handleGetApp's own existing AbilityRead response (appResource
	// carries storage_target_id), the same "no separate GET route"
	// reasoning the backup-schedule/public-access routes above already
	// apply.
	mux.HandleFunc("PUT /api/v1/apps/{name}/storage", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppStorage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/storage", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppStorage))

	// Real app-to-database attachment (apps_database.go): AbilityWrite,
	// not AbilityDeploy, since this is a config write, not itself a
	// deploy trigger (the next reconcile picks up the change on its own
	// schedule, the same "desired state changes, containers converge
	// later" shape PUT .../storage/.../node/.../project already have).
	mux.HandleFunc("PUT /api/v1/apps/{name}/database", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppDatabase))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/database", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppDatabase))
	// One Vault-sourced env var declaration at a time (apps_vault_env.go):
	// same AbilityWrite tier and "config write, not a deploy trigger"
	// reasoning as PUT/DELETE .../database just above.
	mux.HandleFunc("PUT /api/v1/apps/{name}/vault-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppVaultEnv))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/vault-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppVaultEnv))
	// One preview-specific env var override at a time (apps_preview_env.go):
	// same AbilityWrite tier and "config write, not a deploy trigger"
	// reasoning as PUT/DELETE .../vault-env just above; only takes effect
	// the next time a preview environment is created from this app.
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppPreviewEnvOverride))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/preview-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppPreviewEnvOverride))
	// Outbound network allowlist, per app (apps_egress.go): AbilityWriteSensitive
	// for PUT/DELETE, the same tier PUT/DELETE .../storage above uses,
	// since this changes an app's own network-exfiltration surface, the
	// identical sensitivity class as which live bucket credentials it
	// receives. GET is a real route here (unlike storage's reuse of
	// handleGetApp), since an unconfigured policy is common enough (every
	// app before this feature existed) to be worth its own explicit,
	// ordinary AbilityRead response rather than folding it into the
	// general app resource.
	mux.HandleFunc("GET /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppEgressPolicy))
	mux.HandleFunc("PUT /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppEgressPolicy))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppEgressPolicy))
	mux.HandleFunc("GET /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppHealth))
	mux.HandleFunc("PUT /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppHealth))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppHealth))
	// Read-only, not scoped to any one app: the static list of env var
	// names attaching storage can inject, backed by
	// application.StorageEnvKeys rather than a hardcoded list, see
	// handleListStorageEnvKeys' own doc comment.
	mux.HandleFunc("GET /api/v1/storage-env-keys", rt.requireAbility(AbilityRead, rt.handleListStorageEnvKeys))

	// Audit log (audit.go): who did what, across every session and API
	// token, docs/comparison.md's own "no audit log exists anywhere in
	// the codebase" gap. AbilityRoot, the same tier as the node and
	// GitHub App routes above: this is operational data about every
	// other identity on this control plane, not an ordinary per-app
	// read.
	mux.HandleFunc("GET /api/v1/audit-log", rt.requireAbility(AbilityRoot, rt.handleListAuditLog))
	// Manual retention purge (audit_retention.go): runs the same
	// operator-configured retention window PurgeOldAuditEntries' own
	// periodic sweep uses, on demand instead of waiting for its next
	// tick. AbilityRoot, matching the read route above: clearing audit
	// history is at least as sensitive as reading it.
	mux.HandleFunc("POST /api/v1/audit-log/purge", rt.requireAbility(AbilityRoot, rt.handlePurgeAuditLog))

	// Log drain (apps_log_drain.go): forwards an app's container logs to
	// an external HTTP or syslog sink, additive to the existing
	// node-local store. AbilityWriteSensitive for write/clear, the same
	// tier as storage above (both configure where data leaves this
	// control plane to); AbilityRead for the GET.
	mux.HandleFunc("GET /api/v1/apps/{name}/log-drain", rt.requireAbility(AbilityRead, rt.handleGetAppLogDrain))
	mux.HandleFunc("PUT /api/v1/apps/{name}/log-drain", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppLogDrain))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/log-drain", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppLogDrain))

	// BYOK AI assistant settings (ai_settings.go): GET is AbilityRead;
	// PUT/DELETE are AbilityRoot, the same tier PUT /api/v1/settings/
	// email uses for any other platform-wide credential-bearing config.
	mux.HandleFunc("GET /api/v1/settings/ai-assistant", rt.requireAbility(AbilityRead, rt.handleGetAIAssistantSettings))
	mux.HandleFunc("PUT /api/v1/settings/ai-assistant", rt.requireAbility(AbilityRoot, rt.handleUpdateAIAssistantSettings))
	mux.HandleFunc("DELETE /api/v1/settings/ai-assistant", rt.requireAbility(AbilityRoot, rt.handleDeleteAIAssistantSettings))

	// AI assistant chat sessions (ai_chat.go): AbilityRoot throughout,
	// not a lower tier, because a confirmed message can execute any
	// mutating tool the platform exposes (deploy, rollback, restart, and
	// so on) once a human approves it, the same blast radius as the
	// scoped token itself would need to reach those actions directly.
	mux.HandleFunc("POST /api/v1/ai/sessions", rt.requireAbility(AbilityRoot, rt.handleCreateAIChatSession))
	mux.HandleFunc("GET /api/v1/ai/sessions/{id}", rt.requireAbility(AbilityRoot, rt.handleGetAIChatSession))
	mux.HandleFunc("POST /api/v1/ai/sessions/{id}/messages", rt.requireAbility(AbilityRoot, rt.handleCreateAIChatMessage))
	mux.HandleFunc("POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}", rt.requireAbility(AbilityRoot, rt.handleResolveAIChatConfirmation))

}
