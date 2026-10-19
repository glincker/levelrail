package api

import "net/http"

// registerProjectRoutes registers the projects, organizations, environments and deploy approvals routes.
func (rt *Router) registerProjectRoutes(mux *http.ServeMux) {
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

	// Topology (topology.go): a read-only diagram-ready graph derived
	// from this project's existing desired state, AbilityRead like the
	// project read itself above, not a new server-side list filter (see
	// this file's own package doc comment on why apps/databases stay
	// client-filtered elsewhere): a graph's nodes and edges aren't a
	// list page.
	mux.HandleFunc("GET /api/v1/projects/{id}/topology", rt.requireAbility(AbilityRead, rt.handleGetProjectTopology))

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

	// Environments (environments.go): staging/production-style labels
	// scoped to a project, tagged onto a service via its own app route.
	mux.HandleFunc("GET /api/v1/projects/{id}/environments", rt.requireAbility(AbilityRead, rt.handleListEnvironments))
	mux.HandleFunc("POST /api/v1/projects/{id}/environments", rt.requireAbility(AbilityWrite, rt.handleCreateEnvironment))
	// Cross-environment env var drift check (environment_compare.go):
	// resolved effective env vars for two of this project's environments,
	// secret values always redacted.
	mux.HandleFunc("GET /api/v1/projects/{id}/environments/compare", rt.requireAbility(AbilityRead, rt.handleCompareEnvironmentEnv))
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

	// Chat-interactive deploy approval (chat_interactions.go): a
	// Slack/Discord button click posts back here, unauthenticated like
	// POST .../webhooks/github/{name} above; its own signature check
	// (Slack HMAC, Discord Ed25519) stands in for auth, and the decision
	// itself runs through the exact same functions the gated routes
	// above call.
	mux.HandleFunc("POST /api/v1/webhooks/slack/interactions", rt.handleSlackInteraction)
	mux.HandleFunc("POST /api/v1/webhooks/discord/interactions", rt.handleDiscordInteraction)

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

	// Move an app or database into a project (project_id "" removes it): AbilityWrite, since
	// project membership is an organizational edit, not infrastructure placement.
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

	// Minor and patch image change for a database, same data volume.
	// AbilityWriteSensitive: it restarts the database container.
	mux.HandleFunc("PUT /api/v1/databases/{name}/version", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleSetDatabaseVersion))
}
