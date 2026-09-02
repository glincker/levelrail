package api

import "net/http"

// Handler builds the *http.ServeMux with every route registered. Called
// once at startup; the returned handler is what gets wrapped in an
// *http.Server. Route registration is split across this file (core:
// brand, auth, users, oauth, tokens, apps, databases) and
// routes_platform.go (nodes, certs, ingress, email, domains, backups,
// restore, storage, integrations, audit) purely to keep each function
// under a readable size; there is no other meaning to the split.
func (rt *Router) Handler() http.Handler {
	mux := http.NewServeMux()
	rt.registerCoreRoutes(mux)
	rt.registerPlatformRoutes(mux)
	return mux
}

func (rt *Router) registerCoreRoutes(mux *http.ServeMux) {
	// Public: the frontend needs branding before a session exists (e.g.
	// on the login screen itself).
	mux.HandleFunc("GET /api/v1/brand", rt.handleBrand)
	mux.HandleFunc("GET /api/v1/dev-mode", rt.handleDevMode)

	// System status (General settings page): configured/not-configured
	// signals plus disk usage, AbilityRead like everything else an
	// authenticated operator can passively view.
	mux.HandleFunc("GET /api/v1/system/status", rt.requireAbility(AbilityRead, rt.handleSystemStatus))
	// Doctor (levelrail-cli doctor): a superset preflight bundle of the
	// same individual checks above, plus a few doctor-only ones (data
	// dir writability, ingress port availability, SQLite reachability),
	// AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/system/doctor", rt.requireAbility(AbilityRead, rt.handleSystemDoctor))
	// Every container on this node, Levelrail-managed or not (containers.go's
	// own doc comment on why this is read-only, no stop/restart action
	// here), AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/system/containers", rt.requireAbility(AbilityRead, rt.handleListContainers))
	// POST /system/prune deletes real Docker resources (stopped
	// containers, dangling images, anonymous volumes, unused build
	// cache) fleet-wide, not scoped to one app: AbilityRoot, the same
	// gate handleDrainNode uses for its own fleet-wide, no-undo action,
	// not AbilityWrite (which a narrower, single-app token could hold).
	mux.HandleFunc("POST /api/v1/system/prune", rt.requireAbility(AbilityRoot, rt.handleSystemPrune))
	// Master key rotation re-wraps every stored DEK live: AbilityRoot,
	// the same fleet-wide-blast-radius tier as prune above, not
	// AbilityWrite (SecretSetter's own gate for a single app's values).
	mux.HandleFunc("POST /api/v1/system/master-key/rotate", rt.requireAbility(AbilityRoot, rt.handleRotateMasterKey))

	// First-run onboarding state: AbilityRead to check it, AbilityWrite to
	// dismiss/complete it, same tier as any other low-blast-radius
	// per-instance flag (not AbilityRoot, unlike ingress settings).
	mux.HandleFunc("GET /api/v1/onboarding", rt.requireAbility(AbilityRead, rt.handleGetOnboardingState))
	mux.HandleFunc("POST /api/v1/onboarding/complete", rt.requireAbility(AbilityWrite, rt.handleCompleteOnboarding))

	// Updates (Settings > Updates page): running version vs. GitHub's
	// latest published release, AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/updates", rt.requireAbility(AbilityRead, rt.handleGetUpdates))

	// Auth. Login and first-run registration are necessarily public;
	// everything else requires an existing session.
	mux.HandleFunc("POST /api/v1/auth/login", rt.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/register", rt.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/logout", rt.requireAuth(rt.handleLogout))
	mux.HandleFunc("PUT /api/v1/auth/password", rt.requireAuth(rt.handleChangePassword))
	mux.HandleFunc("GET /api/v1/auth/session", rt.requireAuth(rt.handleGetSession))
	mux.HandleFunc("POST /api/v1/auth/sessions/revoke-others", rt.requireAuth(rt.handleRevokeOtherSessions))

	// Two-factor auth (twofactor.go). /2fa/verify is necessarily public
	// (it's step two of login, before a session exists), rate-limited by
	// rt.mfaVerify instead of a session/ability check. Every other route
	// here acts on the caller's own account, so requireAuth
	// (session-only, matching handleChangePassword's own reasoning).
	mux.HandleFunc("GET /api/v1/auth/2fa", rt.requireAuth(rt.handleGetTwoFactorStatus))
	mux.HandleFunc("POST /api/v1/auth/2fa/setup", rt.requireAuth(rt.handleSetupTwoFactor))
	mux.HandleFunc("POST /api/v1/auth/2fa/confirm", rt.requireAuth(rt.handleConfirmTwoFactor))
	mux.HandleFunc("POST /api/v1/auth/2fa/disable", rt.requireAuth(rt.handleDisableTwoFactor))
	mux.HandleFunc("POST /api/v1/auth/2fa/recovery-codes/regenerate", rt.requireAuth(rt.handleRegenerateRecoveryCodes))
	mux.HandleFunc("POST /api/v1/auth/2fa/verify", rt.handleVerifyTwoFactor)

	// Multi-user: creating another local-password user (see
	// handleRegister's own doc comment) is AbilityRoot, not merely
	// requireAuth: the caller also picks the new user's Abilities, so
	// anyone able to reach this route can mint access at any tier,
	// themselves included, only a root caller may do that. Listing stays
	// AbilityRead, same tier as every other passive view.
	mux.HandleFunc("POST /api/v1/auth/users", rt.requireAbility(AbilityRoot, rt.handleCreateUser))
	mux.HandleFunc("GET /api/v1/users", rt.requireAbility(AbilityRead, rt.handleListUsers))
	mux.HandleFunc("DELETE /api/v1/users/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteUser))
	// PUT .../abilities is AbilityRoot like the delete route above, and
	// refuses self-edits inside the handler (self-lockout guard): a root
	// caller may change any other user's abilities, never their own.
	mux.HandleFunc("PUT /api/v1/users/{id}/abilities", rt.requireAbility(AbilityRoot, rt.handleUpdateUserAbilities))
	// Curated role presets (roles.go): static, non-sensitive metadata,
	// AbilityRead like the user list itself, so any signed-in caller can
	// populate a role picker even without AbilityRoot.
	mux.HandleFunc("GET /api/v1/roles", rt.requireAbility(AbilityRead, rt.handleListRoles))

	// IAM policies (iam.go/iam_handlers.go): resource-scoped Allow/Deny
	// documents attached to a user or token, additive on top of the flat
	// Abilities list above. Reading the catalog is AbilityRead like roles
	// above; every mutation (create/update/delete/attach/detach) is
	// AbilityRoot, the same tier as handleUpdateUserAbilities, since a
	// policy can grant or deny access at a resource-scoped level a
	// non-root caller could not otherwise touch.
	mux.HandleFunc("POST /api/v1/iam/policies", rt.requireAbility(AbilityRoot, rt.handleCreatePolicy))
	mux.HandleFunc("GET /api/v1/iam/policies", rt.requireAbility(AbilityRead, rt.handleListPolicies))
	mux.HandleFunc("GET /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRead, rt.handleGetPolicy))
	mux.HandleFunc("PUT /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdatePolicy))
	mux.HandleFunc("DELETE /api/v1/iam/policies/{id}", rt.requireAbility(AbilityRoot, rt.handleDeletePolicy))
	mux.HandleFunc("GET /api/v1/iam/policies/{id}/attachments", rt.requireAbility(AbilityRead, rt.handleListPolicyAttachments))
	mux.HandleFunc("POST /api/v1/iam/policies/{id}/attachments", rt.requireAbility(AbilityRoot, rt.handleAttachPolicy))
	mux.HandleFunc("DELETE /api/v1/iam/policies/{id}/attachments/{principal_type}/{principal_id}", rt.requireAbility(AbilityRoot, rt.handleDetachPolicy))

	// CLI device login (device_auth.go): "levelrail-cli auth login
	// --device" prints a code, the operator approves it here. start/token
	// are necessarily public (no credential exists yet); requests/
	// approve/deny are requireAuth session-only, the same tier
	// tokens.go's own session-only routes use, since approving a device
	// only ever mints a token scoped to the approving operator's own
	// abilities.
	mux.HandleFunc("POST /api/v1/auth/device/start", rt.handleDeviceAuthStart)
	mux.HandleFunc("POST /api/v1/auth/device/token", rt.handleDeviceAuthToken)
	mux.HandleFunc("GET /api/v1/auth/device/requests", rt.requireAuth(rt.handleListDeviceAuthRequests))
	mux.HandleFunc("POST /api/v1/auth/device/{user_code}/approve", rt.requireAuth(rt.handleApproveDeviceAuthRequest))
	mux.HandleFunc("POST /api/v1/auth/device/{user_code}/deny", rt.requireAuth(rt.handleDenyDeviceAuthRequest))

	// OAuth sign-in (Google, GitHub). /providers, /start, /callback are
	// all necessarily public; /link/start is requireAuth-gated (see its
	// own doc comment).
	mux.HandleFunc("GET /api/v1/auth/oauth/providers", rt.handleListPublicOAuthProviders)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", rt.handleOAuthStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", rt.handleOAuthCallback)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/link/start", rt.requireAuth(rt.handleOAuthLinkStart))

	// OAuth settings: GET is AbilityRead, PUT is AbilityRoot, matching
	// /api/v1/settings/ingress's own tiers.
	mux.HandleFunc("GET /api/v1/settings/oauth", rt.requireAbility(AbilityRead, rt.handleListOAuthSettings))
	mux.HandleFunc("PUT /api/v1/settings/oauth/{provider}", rt.requireAbility(AbilityRoot, rt.handleUpdateOAuthProviderSettings))

	// Forgot/reset password: necessarily unauthenticated, gated by
	// possession of the emailed token instead of a session or ability.
	mux.HandleFunc("POST /api/v1/auth/forgot-password", rt.handleForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", rt.handleResetPassword)

	// API tokens: session-only, deliberately never bearer-token
	// authenticated. A token cannot mint or revoke another token on its
	// own behalf; only an interactive human session can manage the
	// token set, the same boundary that stops a leaked scoped token from
	// escalating itself by minting a broader one.
	mux.HandleFunc("POST /api/v1/auth/tokens", rt.requireAuth(rt.handleCreateToken))
	mux.HandleFunc("GET /api/v1/auth/tokens", rt.requireAuth(rt.handleListTokens))
	mux.HandleFunc("DELETE /api/v1/auth/tokens/{id}", rt.requireAuth(rt.handleRevokeToken))

	// Apps CRUD. requireAbility accepts either a session (implicitly
	// root, there is exactly one human identity in Phase 1) or a bearer
	// token scoped to at least the named ability, so a read-only
	// MCP-issued token is provably unable to reach a write/deploy route,
	// not just conventionally discouraged from calling it.
	mux.HandleFunc("GET /api/v1/apps", rt.requireAbility(AbilityRead, rt.handleListApps))
	mux.HandleFunc("POST /api/v1/apps", rt.requireAbility(AbilityWrite, rt.handleCreateApp))
	// Resource-scoped (iam.go): a policy can Deny or narrowly Allow
	// write/delete on one specific app by name, e.g. a token whose flat
	// abilities grant write everywhere except an app: prod-web Deny
	// policy attached to it. Every other apps route stays on plain
	// requireAbility for now; iam.go's own doc comment on
	// requireAbilityForResource explains why extending coverage further
	// is mechanical, not a rewrite.
	mux.HandleFunc("GET /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetApp))
	mux.HandleFunc("PUT /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteApp))

	// Stage 1 of multi-service apps (migrations/0039_apps.sql,
	// apps_group.go): a service plus its siblings under the same
	// store.App, with a worst-condition-wins rollup status. Additive,
	// read-only; GET /api/v1/apps/{name} above is unchanged.
	mux.HandleFunc("GET /api/v1/apps/{name}/group", rt.requireAbility(AbilityRead, rt.handleGetAppGroup))

	// Compose ingestion (apps_compose.go): fans a compose.yaml's
	// services: out into one store.App plus its member services.
	// AbilityDeploy, the same tier POST .../deploys uses: this creates
	// and deploys, it's not ordinary config.
	mux.HandleFunc("POST /api/v1/apps/{name}/compose", rt.requireAbility(AbilityDeploy, rt.handleDeployCompose))

	// Service template catalog (service_templates.go, ADR 015):
	// read-only, static, served straight from internal/catalog.Templates,
	// no store involved. AbilityRead like every other passive view.
	mux.HandleFunc("GET /api/v1/service-templates", rt.requireAbility(AbilityRead, rt.handleListServiceTemplates))
	mux.HandleFunc("GET /api/v1/service-templates/{id}", rt.requireAbility(AbilityRead, rt.handleGetServiceTemplate))

	// Clone: duplicates an app's desired state under a new name.
	// AbilityWrite, the same gate POST /api/v1/apps itself uses, since a
	// clone is a creation shaped as "copy {name}" rather than "start
	// from scratch": handleCloneApp's own doc comment covers what does
	// and doesn't carry over.
	mux.HandleFunc("POST /api/v1/apps/{name}/clone", rt.requireAbility(AbilityWrite, rt.handleCloneApp))

	// Placement (TASKS.md 3.3): AbilityRoot, not AbilityWrite, matching
	// the sensitivity of the standalone node routes above: moving a
	// service between physical machines is infrastructure placement,
	// not ordinary app config, even though it's reached through this
	// app-scoped URL.
	mux.HandleFunc("PUT /api/v1/apps/{name}/node", rt.requireAbility(AbilityRoot, rt.handleSetAppNode))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityDeploy, rt.handleTriggerDeploy))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAbility(AbilityRead, rt.handleDeployHistory))

	// Restart (handleRestartApp's own doc comment): AbilityDeploy, the
	// same boundary as the deploy trigger above, since forcing a
	// container recreation is the same class of action as triggering a
	// deploy, just without a new image.
	mux.HandleFunc("POST /api/v1/apps/{name}/restart", rt.requireAbility(AbilityDeploy, rt.handleRestartApp))

	// Stop/start (handleStopApp/handleStartApp's own doc comments): same
	// AbilityDeploy tier as restart above, the same class of lifecycle
	// action.
	mux.HandleFunc("POST /api/v1/apps/{name}/stop", rt.requireAbility(AbilityDeploy, rt.handleStopApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/start", rt.requireAbility(AbilityDeploy, rt.handleStartApp))

	// One-off exec (handleExecApp's own doc comment): AbilityRoot, not
	// AbilityDeploy. Secrets are injected as plaintext env vars into a
	// container at create time (CLAUDE.md 4.10) and this package
	// deliberately never decrypts one back into a response body anywhere
	// else, see the secrets route above: "never decrypts a value for a
	// response body." Exec is the one route that can read them anyway,
	// by running `env` inside the container, so it must sit behind the
	// same tier that boundary already implies it needs, not the deploy
	// tier. AbilityRoot is this project's existing "breaks an assumption
	// other tiers rely on" boundary (see restore's own reasoning below).
	mux.HandleFunc("POST /api/v1/apps/{name}/exec", rt.requireAbility(AbilityRoot, rt.handleExecApp))

	// Real deploy-attempt history (deploy_attempts.go): a row per
	// trigger call across all three real trigger paths, additional to
	// (not a replacement for) the reconcile-conditions route above. See
	// DeployAttemptStore's own doc comment for why this is a separate
	// endpoint rather than a change to GET .../deploys's existing
	// response shape. AbilityRead, matching every other passive view of
	// an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-attempts", rt.requireAbility(AbilityRead, rt.handleListDeployAttempts))

	// Deploy comparison (deploy_compare.go): a before/after diff between
	// two attempts, or one attempt against the app's current live state
	// when ?to is omitted. AbilityRead, same sensitivity as the
	// deploy-attempts list above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/compare", rt.requireAbility(AbilityRead, rt.handleCompareDeploys))

	// Promotion (promote.go): move a known-good image from this app to a
	// sibling app tagged with another environment in the same project,
	// through the exact same deploy path a plain trigger uses. Preview is
	// AbilityRead like the comparison view above; the trigger itself is
	// AbilityDeploy, matching POST .../deploys.
	mux.HandleFunc("GET /api/v1/apps/{name}/promote/preview", rt.requireAbility(AbilityRead, rt.handlePromotePreview))
	mux.HandleFunc("POST /api/v1/apps/{name}/promote", rt.requireAbility(AbilityDeploy, rt.handlePromoteApp))

	// Deploy-attempt build/log stream (deploy_attempts.go): SSE, serving
	// either a live tail (attempt still running) or a full persisted
	// replay (attempt already finished), the exact contract
	// web/src/hooks/useDeployLogStream.ts was built against. AbilityRead:
	// this is a read of one attempt's own output, the same sensitivity
	// as the deploy-attempts list above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/logs", rt.requireAbility(AbilityRead, rt.handleDeployLogStream))

	// Read-only failure diagnosis (diagnose.go): synthesizes the app's
	// newest (or ?deploy_id=-pinned) deploy attempt, current reconcile
	// conditions, and crashloop state into a deterministic explanation.
	// AbilityRead, same sensitivity as the routes above; never writes
	// anything.
	mux.HandleFunc("GET /api/v1/apps/{name}/diagnose", rt.requireAbility(AbilityRead, rt.handleDiagnoseApp))

	// Read-only resource right-sizing suggestion
	// (resource_recommendation.go): synthesizes the app's historical
	// CPU/memory usage and current limits into a deterministic
	// raise/lower/keep suggestion per dimension. AbilityRead, same
	// sensitivity as diagnose above; never writes anything, never applied
	// automatically.
	mux.HandleFunc("GET /api/v1/apps/{name}/resource-recommendation", rt.requireAbility(AbilityRead, rt.handleAppResourceRecommendation))

	// Manual build trigger (see Builder/WithBuilder above and
	// handleTriggerBuild's own doc comment): builds an image from a git
	// source through the same internal/deploy.Pipeline the webhook
	// receiver uses, for an operator with no working git webhook
	// configured. AbilityDeploy, the same boundary as the image-tag
	// trigger above: this also ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/builds", rt.requireAbility(AbilityDeploy, rt.handleTriggerBuild))

	// Multi-service fan-out (handleDeploySpec's own doc comment,
	// apps_multi.go): one app.yaml's services: map, built and deployed as
	// N independent services under one store.App named {name}. Same
	// AbilityDeploy boundary as the manual build trigger above: this also
	// ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-spec", rt.requireAbility(AbilityDeploy, rt.handleDeploySpec))

	// Branch listing for an arbitrary public git remote (handleListGitBranches's
	// own doc comment): not scoped to an existing app, since the create-app-
	// from-git wizard needs this before an app exists to attach a build
	// to. AbilityDeploy, the same tier the build trigger above requires:
	// listing what a repo could be built from is a strict subset of
	// actually triggering that build.
	mux.HandleFunc("POST /api/v1/git/branches", rt.requireAbility(AbilityDeploy, rt.handleListGitBranches))

	// Previously-built image tags for this app's repo, so the deploy
	// trigger form can offer a dropdown instead of a hand-typed tag
	// (see ImageLister above). AbilityRead like every other passive
	// view of an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/images", rt.requireAbility(AbilityRead, rt.handleListImages))

	// Live traffic path (network.go): declared container port plus the
	// current Docker-assigned host port Caddy is actually proxying to, for
	// the dashboard's Network tab. AbilityRead, same tier as images above.
	mux.HandleFunc("GET /api/v1/apps/{name}/network", rt.requireAbility(AbilityRead, rt.handleGetAppNetwork))

	// Databases CRUD, the database-kind counterpart to apps CRUD above.
	// No PUT (full-replace update) yet: unlike a service's image/port/
	// domains, none of engine/version/name are meant to change in place
	// once created (an engine or major-version change is a migration,
	// not a config edit), so there is nothing for an update endpoint to
	// legitimately do yet.
	// Read-only, ahead of the CRUD routes below since it's not scoped to
	// any one database: every engine this control plane can create at
	// all, backed by the embedded database_engines.yaml registry rather
	// than a hardcoded list, see handleListDatabaseEngines' own doc
	// comment.
	mux.HandleFunc("GET /api/v1/database-engines", rt.requireAbility(AbilityRead, rt.handleListDatabaseEngines))

	mux.HandleFunc("GET /api/v1/databases", rt.requireAbility(AbilityRead, rt.handleListDatabases))
	mux.HandleFunc("POST /api/v1/databases", rt.requireAbility(AbilityWrite, rt.handleCreateDatabase))
	// Resource-scoped, same reasoning as the apps routes above.
	mux.HandleFunc("GET /api/v1/databases/{name}", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabase))
	mux.HandleFunc("DELETE /api/v1/databases/{name}", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleDeleteDatabase))
	mux.HandleFunc("GET /api/v1/databases/{name}/status", rt.requireAbility(AbilityRead, rt.handleDatabaseStatus))

	// Telemetry query, the database counterpart to
	// GET /apps/{name}/metrics, /logs, /logs/stream above: same
	// TelemetryQuerier/logBroadcaster gating (501 when unconfigured).
	// database_metrics.go/database_logs.go share the actual query/stream
	// logic with their app equivalents via queryResourceMetrics/
	// queryResourceLogs/streamResourceLogs (metrics.go/logs.go/
	// live_logs.go), parameterized on a resourceLookup, not a
	// hand-copied duplicate.
	mux.HandleFunc("GET /api/v1/databases/{name}/metrics", rt.requireAbility(AbilityRead, rt.handleQueryDatabaseMetrics))
	mux.HandleFunc("GET /api/v1/databases/{name}/logs", rt.requireAbility(AbilityRead, rt.handleQueryDatabaseLogs))
	mux.HandleFunc("GET /api/v1/databases/{name}/logs/stream", rt.requireAbility(AbilityRead, rt.handleLiveDatabaseLogStream))

	// Resource right-sizing, the database counterpart to
	// GET /apps/{name}/resource-recommendation above
	// (database_resource_recommendation.go).
	mux.HandleFunc("GET /api/v1/databases/{name}/resource-recommendation", rt.requireAbility(AbilityRead, rt.handleDatabaseResourceRecommendation))

	// Placement (TASKS.md 3.3), the database counterpart to
	// PUT /apps/{name}/node above: same AbilityRoot gating.
	mux.HandleFunc("PUT /api/v1/databases/{name}/node", rt.requireAbility(AbilityRoot, rt.handleSetDatabaseNode))
}
