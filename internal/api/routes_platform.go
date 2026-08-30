package api

import "net/http"

// registerPlatformRoutes is Handler()'s second half: nodes, certs,
// ingress, email, domains, backups, restore, storage, git provider
// integrations, and audit. See routes.go's own doc comment for why the
// split.
func (rt *Router) registerPlatformRoutes(mux *http.ServeMux) {
	// Secrets (TASKS.md 1.7). Set-only: there is deliberately no GET,
	// returning a value (even to its own owner over an authenticated
	// session) is exactly the kind of exposure envelope encryption
	// exists to avoid.
	mux.HandleFunc("PUT /api/v1/apps/{name}/secrets/{key}", rt.requireAbility(AbilityWriteSensitive, rt.handleSetSecret))

	// List known secret keys (never values) and toggle a key's lock.
	// GET at AbilityRead, matching GET .../git-source's own
	// GET=Read/PUT=WriteSensitive split just below: a key NAME is no
	// more sensitive than a git-source's connection config.
	mux.HandleFunc("GET /api/v1/apps/{name}/secrets", rt.requireAbility(AbilityRead, rt.handleListSecrets))
	mux.HandleFunc("POST /api/v1/apps/{name}/secrets/{key}/lock", rt.requireAbility(AbilityWriteSensitive, rt.handleSetSecretLock))

	// Git source (TASKS.md 1.7's own deferred follow-up, git_sources.go):
	// persist a repo/branch/build config per app so a git push can
	// auto-deploy it, the multi-app evolution of internal/webhook's own
	// single-app, env-var-configured Config. AbilityWriteSensitive for
	// PUT/DELETE, matching PUT .../secrets/{key} above: connecting a repo
	// accepts an optional live deploy token in the same request body.
	mux.HandleFunc("GET /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityRead, rt.handleGetGitSource))
	mux.HandleFunc("PUT /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityWriteSensitive, rt.handleSetGitSource))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/git-source", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteGitSource))

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

	// Preview environments per pull request (preview_environments.go/
	// preview_environments_handlers.go): opt-in per app, off by default.
	// AbilityWriteSensitive for the toggle, matching PUT .../git-source's
	// own tier since it's the same connect-time-adjacent configuration
	// surface; AbilityRead for the list, matching GET .../git-source;
	// AbilityDeploy for the manual teardown, the same lifecycle-action
	// tier POST .../restart and POST .../stop already use.
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-settings", rt.requireAbility(AbilityWriteSensitive, rt.handleSetPreviewEnabled))
	mux.HandleFunc("GET /api/v1/apps/{name}/previews", rt.requireAbility(AbilityRead, rt.handleListPreviewEnvironments))
	mux.HandleFunc("POST /api/v1/apps/{name}/previews/{number}/teardown", rt.requireAbility(AbilityDeploy, rt.handleTeardownPreviewEnvironment))

	// Telemetry query (TASKS.md 2.3): metrics and logs for one app,
	// fanned out through a Federator (today, exactly one local source).
	mux.HandleFunc("GET /api/v1/apps/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryMetrics))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryLogs))
	// Live log tail (additive to the historical search route just above,
	// see handleLiveLogStream's own doc comment): AbilityRead, the same
	// passive-visibility boundary as every other view of telemetry data
	// in this router, including the deploy-log stream at
	// GET .../deploys/{deployId}/logs above.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/stream", rt.requireAbility(AbilityRead, rt.handleLiveLogStream))

	// Alerting (TASKS.md 2.5/2.7): threshold and crashloop rules scoped
	// to one app, fanned through a *alerting.DB when configured (see
	// WithAlertRules).
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityWrite, rt.handleCreateAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alerts", rt.requireAbility(AbilityRead, rt.handleListAlertRules))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/alerts/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteAlertRule))

	// Scheduled tasks: run an arbitrary command inside this app's
	// container on a cron schedule (internal/scheduledtask). CRUD sits at
	// AbilityWrite, the same config-mutation tier alert rules above and
	// env var updates already use; "run now" sits one tier up at
	// AbilityDeploy, matching restart/stop/start above, since it has the
	// identical immediate side effect (a real command actually runs
	// inside a running container right now), not just a config change.
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks", rt.requireAbility(AbilityWrite, rt.handleCreateScheduledTask))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks", rt.requireAbility(AbilityRead, rt.handleListScheduledTasks))
	mux.HandleFunc("GET /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbility(AbilityRead, rt.handleGetScheduledTask))
	mux.HandleFunc("PUT /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbility(AbilityWrite, rt.handleUpdateScheduledTask))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/scheduled-tasks/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteScheduledTask))
	mux.HandleFunc("POST /api/v1/apps/{name}/scheduled-tasks/{id}/run", rt.requireAbility(AbilityDeploy, rt.handleRunScheduledTaskNow))

	// Deploy-outcome notifications (wave-2 roadmap item #5): a Slack/
	// Discord/Telegram/generic-webhook/email ping fired once per deploy
	// attempt reaching a terminal state, distinct from the threshold/
	// crashloop alert rules just above (see
	// internal/alerting/deploy_notify.go's own doc comment). Also fanned
	// through *alerting.DB when configured (see WithDeployNotifyTargets).
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbility(AbilityWrite, rt.handleCreateDeployNotifyTarget))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbility(AbilityRead, rt.handleListDeployNotifyTargets))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/deploy-notify-targets/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteDeployNotifyTarget))

	// Notification channels: the global, connect-once destination the
	// deploy-notify-target routes above attach to by channel_id.
	// AbilityWrite, same tier as deploy-notify-targets' own POST.
	mux.HandleFunc("GET /api/v1/notification-channels", rt.requireAbility(AbilityRead, rt.handleListNotificationChannels))
	mux.HandleFunc("POST /api/v1/notification-channels", rt.requireAbility(AbilityWrite, rt.handleCreateNotificationChannel))
	mux.HandleFunc("DELETE /api/v1/notification-channels/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/test", rt.requireAbility(AbilityWrite, rt.handleTestNotificationChannel))
	mux.HandleFunc("POST /api/v1/notification-channels/{id}/test", rt.requireAbility(AbilityWrite, rt.handleTestExistingNotificationChannel))

	// Prometheus remote read (TASKS.md 2.6). Gated by requireAbility the
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
	// AbilityRoot: unlike a node (real infrastructure, TASKS.md 3.1),
	// creating or deleting a project has no fleet-level consequence.
	mux.HandleFunc("GET /api/v1/projects", rt.requireAbility(AbilityRead, rt.handleListProjects))
	mux.HandleFunc("POST /api/v1/projects", rt.requireAbility(AbilityWrite, rt.handleCreateProject))
	mux.HandleFunc("GET /api/v1/projects/{id}", rt.requireAbility(AbilityRead, rt.handleGetProject))
	mux.HandleFunc("DELETE /api/v1/projects/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteProject))

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

	// Environments (environments.go): staging/production-style labels
	// scoped to a project, tagged onto a service via its own app route.
	mux.HandleFunc("GET /api/v1/projects/{id}/environments", rt.requireAbility(AbilityRead, rt.handleListEnvironments))
	mux.HandleFunc("POST /api/v1/projects/{id}/environments", rt.requireAbility(AbilityWrite, rt.handleCreateEnvironment))
	mux.HandleFunc("DELETE /api/v1/environments/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteEnvironment))
	mux.HandleFunc("PUT /api/v1/apps/{name}/environment", rt.requireAbility(AbilityWrite, rt.handleSetAppEnvironment))

	// Shared env vars every service tagged with this environment inherits
	// (environment_env.go): the tier between organizations/{id}/env and
	// projects/{id}/env above and a service's own env below.
	mux.HandleFunc("GET /api/v1/environments/{id}/env", rt.requireAbility(AbilityRead, rt.handleGetEnvironmentEnv))
	mux.HandleFunc("PUT /api/v1/environments/{id}/env", rt.requireAbility(AbilityWrite, rt.handleSetEnvironmentEnv))

	// Shared env vars every app filed under this project inherits
	// (project_env.go): same AbilityRead/AbilityWrite boundary as the
	// project CRUD routes just above.
	mux.HandleFunc("GET /api/v1/projects/{id}/env", rt.requireAbility(AbilityRead, rt.handleGetProjectEnv))
	mux.HandleFunc("PUT /api/v1/projects/{id}/env", rt.requireAbility(AbilityWrite, rt.handleSetProjectEnv))

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
	mux.HandleFunc("PUT /api/v1/apps/{name}/project", rt.requireAbility(AbilityWrite, rt.handleSetAppProject))
	mux.HandleFunc("PUT /api/v1/databases/{name}/project", rt.requireAbility(AbilityWrite, rt.handleSetDatabaseProject))

	// Set (or clear, with a nil body field) a database's resource limits:
	// databaseResource's own Resources field doc comment explains why
	// this is a dedicated route rather than folded into handleUpdateApp's
	// general-PUT equivalent, which has no database counterpart.
	// AbilityWrite, the same ordinary-config-edit sensitivity as the
	// project routes just above, not AbilityRoot: a resource cap is not
	// fleet-level placement.
	mux.HandleFunc("PUT /api/v1/databases/{name}/resources", rt.requireAbility(AbilityWrite, rt.handleSetDatabaseResources))

	// Nodes (TASKS.md 3.1): fleet-level infrastructure, not scoped to any
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
	// Health, cordon, drain (TASKS.md 3.7), same AbilityRoot boundary as
	// every other node route above.
	mux.HandleFunc("GET /api/v1/nodes/{id}/health", rt.requireAbility(AbilityRoot, rt.handleGetNodeHealth))
	mux.HandleFunc("POST /api/v1/nodes/{id}/cordon", rt.requireAbility(AbilityRoot, rt.handleCordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/uncordon", rt.requireAbility(AbilityRoot, rt.handleUncordonNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", rt.requireAbility(AbilityRoot, rt.handleDrainNode))
	// Node-level metrics (sum of per-container samples for everything
	// placed on this node, see handleQueryNodeMetrics's own doc comment
	// for exactly what that does and doesn't mean): same AbilityRoot
	// boundary and nil-telemetry 501 shape as every route above, and the
	// same query-param contract as GET /apps/{name}/metrics above it.
	mux.HandleFunc("GET /api/v1/nodes/{id}/metrics", rt.requireAbility(AbilityRoot, rt.handleQueryNodeMetrics))
	// Latest OS-patch reading (internal/telemetry/hostpatch.go's
	// HostPatchCollector), a single current fact rather than a time
	// series, same AbilityRoot boundary as every other node route.
	mux.HandleFunc("GET /api/v1/nodes/{id}/patch-status", rt.requireAbility(AbilityRoot, rt.handleGetNodePatchStatus))

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
	mux.HandleFunc("PUT /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbility(AbilityRoot, rt.handleSetDomainBasicAuth))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/domains/{domain}/auth", rt.requireAbility(AbilityRoot, rt.handleClearDomainBasicAuth))

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

	// Registry credentials (registry_credentials.go): same ability tiers
	// as backup targets just above, same reasoning (POST/PUT/DELETE
	// handle live pull credentials).
	mux.HandleFunc("GET /api/v1/registry-credentials", rt.requireAbility(AbilityRead, rt.handleListRegistryCredentials))
	mux.HandleFunc("POST /api/v1/registry-credentials", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateRegistryCredential))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityRead, rt.handleGetRegistryCredential))
	mux.HandleFunc("PUT /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateRegistryCredential))
	mux.HandleFunc("DELETE /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteRegistryCredential))

	// Aggregated git provider capability summary (git_providers.go): one
	// AbilityReadSensitive call the git-source picker uses instead of the
	// three AbilityRoot status endpoints below, so a non-root deploy-scoped
	// user can open the app creation wizard without hitting a 403. See
	// handleListGitProviders's own doc comment.
	mux.HandleFunc("GET /api/v1/git-providers", rt.requireAbility(AbilityReadSensitive, rt.handleListGitProviders))

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

	// Backup history and manual trigger, per database. Trigger needs
	// AbilityWriteSensitive: it starts real work against a live bucket
	// using a previously-stored credential, the same sensitivity class
	// creating or deleting the backup target itself already carries.
	// History listing is ordinary AbilityRead.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups", rt.requireAbility(AbilityWriteSensitive, rt.handleTriggerBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups", rt.requireAbility(AbilityRead, rt.handleListBackupHistory))

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
	mux.HandleFunc("PUT /api/v1/databases/{name}/backup-schedule", rt.requireAbility(AbilityWriteSensitive, rt.handleSetBackupSchedule))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backup-schedule", rt.requireAbility(AbilityWriteSensitive, rt.handleClearBackupSchedule))

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
	mux.HandleFunc("PUT /api/v1/databases/{name}/public-access", rt.requireAbility(AbilityWriteSensitive, rt.handleSetDatabasePublicAccess))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/public-access", rt.requireAbility(AbilityWriteSensitive, rt.handleClearDatabasePublicAccess))

	// Restore, per database (restore.go). AbilityRoot, not
	// AbilityWriteSensitive: see handleTriggerRestore's own doc comment
	// for why this, alone among every backup-related route, needs the
	// same top tier as node management and placement. History listing is
	// ordinary AbilityRead, the same boundary the backup history route
	// above already draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore", rt.requireAbility(AbilityRoot, rt.handleTriggerRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/restores", rt.requireAbility(AbilityRead, rt.handleListRestoreHistory))

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
	mux.HandleFunc("PUT /api/v1/apps/{name}/storage", rt.requireAbility(AbilityWriteSensitive, rt.handleSetAppStorage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/storage", rt.requireAbility(AbilityWriteSensitive, rt.handleClearAppStorage))

	// Real app-to-database attachment (apps_database.go): AbilityWrite,
	// not AbilityDeploy, since this is a config write, not itself a
	// deploy trigger (the next reconcile picks up the change on its own
	// schedule, the same "desired state changes, containers converge
	// later" shape PUT .../storage/.../node/.../project already have).
	mux.HandleFunc("PUT /api/v1/apps/{name}/database", rt.requireAbility(AbilityWrite, rt.handleSetAppDatabase))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/database", rt.requireAbility(AbilityWrite, rt.handleClearAppDatabase))
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

	// Log drain (apps_log_drain.go): forwards an app's container logs to
	// an external HTTP or syslog sink, additive to the existing
	// node-local store. AbilityWriteSensitive for write/clear, the same
	// tier as storage above (both configure where data leaves this
	// control plane to); AbilityRead for the GET.
	mux.HandleFunc("GET /api/v1/apps/{name}/log-drain", rt.requireAbility(AbilityRead, rt.handleGetAppLogDrain))
	mux.HandleFunc("PUT /api/v1/apps/{name}/log-drain", rt.requireAbility(AbilityWriteSensitive, rt.handleSetAppLogDrain))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/log-drain", rt.requireAbility(AbilityWriteSensitive, rt.handleClearAppLogDrain))
}
