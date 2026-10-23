package api

import "net/http"

// registerAppRoutes registers the app CRUD, templates, deploys and builds routes.
func (rt *Router) registerAppRoutes(mux *http.ServeMux) {
	// Apps CRUD. requireAbility accepts either a session (implicitly
	// root, there is exactly one human identity in Phase 1) or a bearer
	// token scoped to at least the named ability, so a read-only
	// MCP-issued token is provably unable to reach a write/deploy route,
	// not just conventionally discouraged from calling it.
	mux.HandleFunc("GET /api/v1/apps", rt.requireAbility(AbilityRead, rt.handleListApps))
	mux.HandleFunc("POST /api/v1/apps/bulk", rt.requireAbility(AbilityWrite, rt.handleBulkApps))
	mux.HandleFunc("GET /api/v1/apps-summary", rt.requireAbility(AbilityRead, rt.handleAppsSummary))
	mux.HandleFunc("GET /api/v1/apps/git-sources", rt.requireAbility(AbilityRead, rt.handleListGitSources))
	mux.HandleFunc("POST /api/v1/apps", rt.requireAbility(AbilityWrite, rt.idempotent(rt.handleCreateApp)))
	// Resource-scoped (iam.go): a policy can Deny or narrowly Allow
	// write/delete on one specific app by name, e.g. a token whose flat
	// abilities grant write everywhere except an app: prod-web Deny
	// policy attached to it. Every other per-app and per-database
	// mutating route in this file and routes_platform.go is now on the
	// same requireAbilityForResource gate; only POST /apps (creation, no
	// existing resource to scope to) and GET .../terminal (websocket
	// upgrade, not touched in this pass) remain on plain requireAbility.
	mux.HandleFunc("GET /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/keep", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleKeepTrialApp))
	mux.HandleFunc("PUT /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateApp))
	mux.HandleFunc("DELETE /api/v1/apps/{name}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteApp))

	// Stage 1 of multi-service apps (migrations/0039_apps.sql,
	// apps_group.go): a service plus its siblings under the same
	// store.App, with a worst-condition-wins rollup status. Additive,
	// read-only; GET /api/v1/apps/{name} above is unchanged.
	mux.HandleFunc("GET /api/v1/apps/{name}/group", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppGroup))

	// Latest pre/post-deploy hook outcome (apps_hooks.go,
	// internal/reconcile/application's own HookRunRecorder). Read-only,
	// same ability tier as the group route just above.
	mux.HandleFunc("GET /api/v1/apps/{name}/hook-runs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppHookRuns))

	// Compose ingestion (apps_compose.go): fans a compose.yaml's
	// services: out into one store.App plus its member services.
	// AbilityDeploy, the same tier POST .../deploys uses: this creates
	// and deploys, it's not ordinary config.
	mux.HandleFunc("POST /api/v1/apps/{name}/compose", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeployCompose))

	// Service template catalog (service_templates.go, ADR 015):
	// read-only, static, served straight from internal/catalog.Templates,
	// no store involved. AbilityRead like every other passive view.
	mux.HandleFunc("GET /api/v1/service-templates", rt.requireAbility(AbilityRead, rt.handleListServiceTemplates))
	mux.HandleFunc("GET /api/v1/service-templates/{id}", rt.requireAbility(AbilityRead, rt.handleGetServiceTemplate))

	// One-click template deploy (handleDeployServiceTemplateNow): not
	// scoped to an existing app (the app doesn't exist until this
	// request creates it), same "no resource to scope to yet" shape as
	// POST /api/v1/git/branches and POST /api/v1/build/detect below.
	// AbilityDeploy, matching POST /api/v1/apps/{name}/compose above,
	// since deployComposeBody is the same create-and-deploy core.
	mux.HandleFunc("POST /api/v1/service-templates/{id}/deploy", rt.requireAbility(AbilityDeploy, rt.handleDeployServiceTemplateNow))

	// Custom templates (service_templates_custom.go): operator-defined
	// templates captured from a running app. Deploy reuses the
	// service-templates/{id}/deploy route above, no second deploy path.
	mux.HandleFunc("GET /api/v1/templates/custom", rt.requireAbility(AbilityRead, rt.handleListCustomTemplates))
	mux.HandleFunc("DELETE /api/v1/templates/custom/{id}", rt.requireAbility(AbilityWrite, rt.handleDeleteCustomTemplate))
	mux.HandleFunc("POST /api/v1/apps/{name}/save-as-template", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSaveAppAsTemplate))

	// Clone: duplicates an app's desired state under a new name.
	// AbilityWrite, the same gate POST /api/v1/apps itself uses, since a
	// clone is a creation shaped as "copy {name}" rather than "start
	// from scratch": handleCloneApp's own doc comment covers what does
	// and doesn't carry over.
	mux.HandleFunc("GET /api/v1/apps/{name}/clone/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleClonePreview))
	mux.HandleFunc("POST /api/v1/apps/{name}/clone", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCloneApp))

	// Placement: AbilityRoot, not AbilityWrite, matching
	// the sensitivity of the standalone node routes above: moving a
	// service between physical machines is infrastructure placement,
	// not ordinary app config, even though it's reached through this
	// app-scoped URL.
	mux.HandleFunc("PUT /api/v1/apps/{name}/node", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetAppNode))

	// move-with-volumes: the same AbilityRoot tier as the plain node move
	// above, since this both changes placement and does an in-place,
	// full-overwrite restore of every named volume onto the destination
	// node (the same risk class POST .../volumes/{volume}/restore already
	// sits behind). The two GETs are AbilityRead, matching every other
	// history listing in this file.
	mux.HandleFunc("POST /api/v1/apps/{name}/move-with-volumes", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleMoveAppWithVolumes))
	mux.HandleFunc("GET /api/v1/apps/{name}/moves", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppVolumeMoves))
	mux.HandleFunc("GET /api/v1/apps/{name}/moves/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppVolumeMove))

	// Deploys.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.idempotent(rt.handleTriggerDeploy)))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/from-cache", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeployFromCache))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDeployHistory))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetDeploy))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/{deployId}/cancel", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCancelDeploy))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/{deployId}/rollback", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.idempotent(rt.handleRollbackToDeploy)))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/{deployId}/rollback-lock", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.idempotent(rt.handleRollbackAndLock)))
	mux.HandleFunc("POST /api/v1/apps/{name}/deploys/{deployId}/publish", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePublishHeldBuild))
	mux.HandleFunc("GET /api/v1/apps/{name}/publish-lock", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetPublishLock))
	mux.HandleFunc("PUT /api/v1/apps/{name}/publish-lock", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutPublishLock))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/publish-lock", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeletePublishLock))
	mux.HandleFunc("GET /api/v1/apps/{name}/cancel-superseded", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetCancelSuperseded))
	mux.HandleFunc("PUT /api/v1/apps/{name}/cancel-superseded", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetCancelSuperseded))
	mux.HandleFunc("GET /api/v1/apps/{name}/auto-rollback", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAutoRollback))
	mux.HandleFunc("PUT /api/v1/apps/{name}/auto-rollback", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetAutoRollback))
	mux.HandleFunc("GET /api/v1/apps/{name}/auto-rollback-slo-burn", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAutoRollbackSLOBurn))
	mux.HandleFunc("PUT /api/v1/apps/{name}/auto-rollback-slo-burn", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetAutoRollbackSLOBurn))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-freeze", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppDeployFreeze))
	mux.HandleFunc("PUT /api/v1/apps/{name}/deploy-freeze", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutAppDeployFreeze))

	// Timeline and pending changes (app_timeline.go, pending_changes.go).
	mux.HandleFunc("GET /api/v1/apps/{name}/timeline", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppTimeline))
	mux.HandleFunc("GET /api/v1/apps/{name}/changes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppChanges))
	mux.HandleFunc("GET /api/v1/apps/{name}/slo-preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleSLOPreview))
	mux.HandleFunc("GET /api/v1/apps/{name}/pending-changes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePendingChanges))
	mux.HandleFunc("POST /api/v1/apps/{name}/apply-pending", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleApplyPending))
	mux.HandleFunc("PATCH /api/v1/apps/{name}/domains", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleEditAppDomains))
	mux.HandleFunc("GET /api/v1/apps/{name}/environment-domains", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppEnvironmentDomains))
	mux.HandleFunc("GET /api/v1/apps/{name}/listening-ports", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppListeningPorts))
	mux.HandleFunc("GET /api/v1/ingress/connectivity", rt.requireAbility(AbilityRead, rt.handleIngressConnectivity))

	// Streams (app_streams.go): raw TCP port forwards, host port to one
	// container port, proxied by Caddy's layer4 app
	// (internal/reconcile/ingress). AbilityWriteSensitive for create/
	// delete, the same tier as a firewall rule: a stream opens a new
	// host port to raw TCP traffic. List is ordinary AbilityRead.
	mux.HandleFunc("GET /api/v1/apps/{name}/streams", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppStreams))
	mux.HandleFunc("POST /api/v1/apps/{name}/streams", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleCreateAppStream))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/streams/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteAppStream))

	// Restart (handleRestartApp's own doc comment): AbilityDeploy, the
	// same boundary as the deploy trigger above, since forcing a
	// container recreation is the same class of action as triggering a
	// deploy, just without a new image.
	mux.HandleFunc("GET /api/v1/apps/{name}/auto-update", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetImageAutoUpdate))
	mux.HandleFunc("PUT /api/v1/apps/{name}/auto-update", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetImageAutoUpdate))
	mux.HandleFunc("POST /api/v1/apps/{name}/auto-update/check", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCheckImageAutoUpdate))
	mux.HandleFunc("POST /api/v1/apps/{name}/auto-update/webhook", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRotateImageUpdateWebhook))
	mux.HandleFunc("GET /api/v1/apps/{name}/canary", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetCanary))
	mux.HandleFunc("POST /api/v1/apps/{name}/canary", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStartCanary))
	mux.HandleFunc("PUT /api/v1/apps/{name}/canary", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetCanaryWeight))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/canary", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleAbortCanary))
	mux.HandleFunc("POST /api/v1/apps/{name}/canary/promote", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePromoteCanary))
	mux.HandleFunc("GET /api/v1/apps/{name}/split", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetSplit))
	mux.HandleFunc("POST /api/v1/apps/{name}/split", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStartSplit))
	mux.HandleFunc("PUT /api/v1/apps/{name}/split", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetSplitWeights))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/split", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStopSplit))
	mux.HandleFunc("POST /api/v1/apps/{name}/split/promote", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePromoteSplit))
	mux.HandleFunc("GET /api/v1/apps/{name}/build-hooks", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListBuildHooks))
	mux.HandleFunc("POST /api/v1/apps/{name}/build-hooks", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleCreateBuildHook))
	mux.HandleFunc("POST /api/v1/apps/{name}/build-hooks/{id}/rotate", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRotateBuildHook))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/build-hooks/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRevokeBuildHook))
	mux.HandleFunc("POST /api/v1/hooks/deploy/{id}/{token}", rt.handleBuildHookTrigger)
	mux.HandleFunc("POST /api/v1/hooks/image-update/{name}/{token}", rt.handleImageUpdateWebhook)
	mux.HandleFunc("POST /api/v1/forms/{app}/{name}", rt.handleFormSubmit)
	mux.HandleFunc("OPTIONS /api/v1/forms/{app}/{name}", rt.handleFormPreflight)
	mux.HandleFunc("GET /api/v1/apps/{name}/forms", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListForms))
	mux.HandleFunc("POST /api/v1/apps/{name}/forms", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleCreateForm))
	mux.HandleFunc("PUT /api/v1/apps/{name}/forms/{form}", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleUpdateForm))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/forms/{form}", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteForm))
	mux.HandleFunc("GET /api/v1/apps/{name}/forms/{form}/fields", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFormFields))
	mux.HandleFunc("PUT /api/v1/apps/{name}/forms/{form}/fields", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetFormFields))
	mux.HandleFunc("POST /api/v1/apps/{name}/forms/{form}/test", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSendFormTest))
	mux.HandleFunc("GET /api/v1/apps/{name}/forms/{form}/submissions", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleListFormSubmissions))
	mux.HandleFunc("GET /api/v1/apps/{name}/forms/{form}/export", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleExportFormSubmissions))
	mux.HandleFunc("PUT /api/v1/apps/{name}/forms/{form}/submissions/{id}/spam", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetFormSubmissionSpam))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/forms/{form}/submissions/{id}", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteFormSubmission))
	mux.HandleFunc("POST /api/v1/ci/oidc-exchange", rt.handleCIOIDCExchange)
	mux.HandleFunc("GET /api/v1/hooks/wake", rt.handleWakeHook)
	mux.HandleFunc("GET /api/v1/sleep", rt.requireAbility(AbilityRead, rt.handleSleepFleet))
	mux.HandleFunc("GET /api/v1/sleep/overview", rt.requireAbility(AbilityRead, rt.handleSleepOverview))
	mux.HandleFunc("GET /api/v1/apps/sleep/overview", rt.requireAbility(AbilityRead, rt.handleAppsSleepOverview))
	mux.HandleFunc("GET /api/v1/apps/{name}/sleep/history", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppSleepHistory))
	mux.HandleFunc("GET /api/v1/apps/{name}/sleep", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppSleep))
	mux.HandleFunc("PUT /api/v1/apps/{name}/sleep", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetAppSleep))
	mux.HandleFunc("POST /api/v1/apps/{name}/sleep/now", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSleepAppNow))
	mux.HandleFunc("POST /api/v1/apps/{name}/sleep/wake", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleWakeApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/restart", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleRestartApp))

	// Stop/start (handleStopApp/handleStartApp's own doc comments): same
	// AbilityDeploy tier as restart above, the same class of lifecycle
	// action.
	mux.HandleFunc("POST /api/v1/apps/{name}/stop", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStopApp))
	mux.HandleFunc("POST /api/v1/apps/{name}/start", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleStartApp))

	// One-off exec: AbilityRoot, not AbilityDeploy, because it can read the plaintext env
	// secrets that no other route ever returns.
	mux.HandleFunc("POST /api/v1/apps/{name}/exec", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleExecApp))

	// Interactive terminal (terminal.go): the same AbilityRoot tier as
	// one-off exec above, since a shell can read the same secrets. A
	// WebSocket rather than SSE, which that file's own doc comment
	// argues for at length.
	mux.HandleFunc("GET /api/v1/apps/{name}/terminal", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleAppTerminal))

	// Exec access opt-out: a second gate checked by exec and terminal on top of AbilityRoot.
	// GET is AbilityRead; PUT is AbilityRoot, since re-enabling hands back a root capability.
	mux.HandleFunc("GET /api/v1/apps/{name}/exec-access", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetExecAccess))
	mux.HandleFunc("PUT /api/v1/apps/{name}/exec-access", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetExecAccess))

	// Deploy status badge opt-in (app_badge.go): same GET-is-AbilityRead,
	// PUT-is-AbilityRoot split as exec-access just above, since enabling
	// it hands out an unauthenticated public view of this app's deploy
	// status. GET .../badge.svg itself is registered unauthenticated on
	// purpose (see publicRoutes in authz_matrix_test.go); it 404s on its
	// own when the per-app flag is off, so it never needs this file's own
	// IAM wrapper.
	mux.HandleFunc("GET /api/v1/apps/{name}/badge", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetBadgeSettings))
	mux.HandleFunc("PUT /api/v1/apps/{name}/badge", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleSetBadgeSettings))
	mux.HandleFunc("GET /api/v1/apps/{name}/badge.svg", rt.handlePublicAppBadge)

	// Real deploy-attempt history (deploy_attempts.go): a row per
	// trigger call across all three real trigger paths, additional to
	// (not a replacement for) the reconcile-conditions route above. See
	// DeployAttemptStore's own doc comment for why this is a separate
	// endpoint rather than a change to GET .../deploys's existing
	// response shape. AbilityRead, matching every other passive view of
	// an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-attempts", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListDeployAttempts))
	mux.HandleFunc("GET /api/v1/deploys/failed", rt.requireAbility(AbilityRead, rt.handleListFailedDeploys))
	mux.HandleFunc("GET /api/v1/deployments", rt.requireAbility(AbilityRead, rt.handleListDeployments))
	mux.HandleFunc("GET /api/v1/deployments/summary", rt.requireAbility(AbilityRead, rt.handleDeploymentsSummary))
	mux.HandleFunc("GET /api/v1/deployments/timeline", rt.requireAbility(AbilityRead, rt.handleDeploymentsTimeline))
	mux.HandleFunc("GET /api/v1/deployments/stream", rt.requireAbility(AbilityRead, rt.withStreamReauth(deploymentsStreamResource, rt.handleDeploymentsStream)))
	// Per-deploy event timeline (deploy_timeline.go): the whole story of one
	// deploy as ordered events, a live SSE view that ends when the deploy
	// settles, and a two-deploy comparison. Settled timelines are frozen for
	// replay until APP_DEPLOY_TIMELINE_RETENTION. AbilityRead, filtered to
	// the apps the caller can read.
	mux.HandleFunc("GET /api/v1/deployments/{id}/timeline", rt.requireAbility(AbilityRead, rt.handleGetDeployTimeline))
	mux.HandleFunc("GET /api/v1/deployments/{id}/timeline/stream", rt.requireAbility(AbilityRead, rt.withStreamReauth(timelineStreamResource, rt.handleDeployTimelineStream)))
	mux.HandleFunc("GET /api/v1/deployments/{id}/timeline/compare", rt.requireAbility(AbilityRead, rt.handleCompareDeployTimelines))

	// Deploy comparison (deploy_compare.go): a before/after diff between
	// two attempts, or one attempt against the app's current live state
	// when ?to is omitted. ?pending=true defaults from to the last
	// successful deploy and drops env values, the diff shown before a
	// deploy. AbilityRead, same sensitivity as the deploy-attempts list.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/compare", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleCompareDeploys))
	// Release notes (release_notes.go): the commits between two deploys,
	// grouped by conventional-commit type, plus a markdown rendering.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/release-notes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleReleaseNotes))

	// Promotion (promote.go): move a known-good image from this app to a
	// sibling app tagged with another environment in the same project,
	// through the exact same deploy path a plain trigger uses. Preview is
	// AbilityRead like the comparison view above; the trigger itself is
	// AbilityDeploy, matching POST .../deploys.
	mux.HandleFunc("GET /api/v1/apps/{name}/promote/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePromotePreview))
	mux.HandleFunc("POST /api/v1/apps/{name}/promote", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.idempotent(rt.handlePromoteApp)))

	// Deploy-attempt build/log stream (deploy_attempts.go): SSE, serving
	// either a live tail (attempt still running) or a full persisted
	// replay (attempt already finished), the exact contract
	// web/src/hooks/useDeployLogStream.ts was built against. AbilityRead:
	// this is a read of one attempt's own output, the same sensitivity
	// as the deploy-attempts list above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/logs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.withStreamReauth(appResourceFromPath, rt.handleDeployLogStream)))

	// Deploy-attempt step stream (deploy_steps.go): SSE, named
	// pipeline-phase transitions (detecting/building/pushing/deploying)
	// rather than raw log lines, for a checklist-style progress view.
	// Same AbilityRead boundary as the log stream above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/steps", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.withStreamReauth(appResourceFromPath, rt.handleDeployStepStream)))

	// Deploy-attempt readiness-probe attempts (deploy_probes.go): plain
	// polling JSON, not SSE, the individual probe attempts
	// (internal/probe.WithOnAttempt) one deploy's cutover made. Same
	// AbilityRead boundary as the step/log streams above.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/probes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListProbeAttempts))

	// Deploy-attempt log download (deploy_log_download.go): the same
	// attempt's full log as a plain-text attachment instead of an SSE
	// stream, mirroring /apps/{name}/logs/download for runtime logs.
	mux.HandleFunc("GET /api/v1/apps/{name}/deploys/{deployId}/logs/download", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDownloadDeployLog))

	// Read-only failure diagnosis (diagnose.go): synthesizes the app's
	// newest (or ?deploy_id=-pinned) deploy attempt, current reconcile
	// conditions, and crashloop state into a deterministic explanation.
	// AbilityRead, same sensitivity as the routes above; never writes
	// anything.
	mux.HandleFunc("GET /api/v1/apps/{name}/diagnose", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDiagnoseApp))

	// Deterministic "why is this not running" for any reconciled resource.
	mux.HandleFunc("GET /api/v1/diagnose/{kind}/{name}", rt.requireAbilityForResource(AbilityRead, stuckResourceFromPath, rt.handleDiagnoseStuck))

	// Preflight checks (preflight.go): read-only probes of an existing
	// app's stored config (AbilityRead, resource-scoped), or of a not yet
	// created app described in the body (AbilityWrite, since the body
	// names hosts the server will contact).
	mux.HandleFunc("POST /api/v1/apps/{name}/preflight", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePreflightApp))
	mux.HandleFunc("POST /api/v1/preflight", rt.requireAbility(AbilityWrite, rt.handlePreflightNew))

	// Live app.yaml validation (apps_validate_spec.go): read-only, same
	// AbilityRead/POST shape as preflight above. Never saves or deploys
	// anything, only runs the submitted body through internal/spec's own
	// Parse-time checks for an editor's earlier, friendlier feedback.
	mux.HandleFunc("POST /api/v1/apps/{name}/validate-spec", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleValidateSpec))

	// Read-only resource right-sizing suggestion
	// (resource_recommendation.go): synthesizes the app's historical
	// CPU/memory usage and current limits into a deterministic
	// raise/lower/keep suggestion per dimension. AbilityRead, same
	// sensitivity as diagnose above; never writes anything, never applied
	// automatically.
	mux.HandleFunc("GET /api/v1/apps/{name}/resource-recommendation", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppResourceRecommendation))

	// Read-only "what this would cost elsewhere" estimate
	// (cost_estimate.go): compares the app's declared or observed
	// CPU/memory against illustrative reference pricing. AbilityRead,
	// same sensitivity as resource-recommendation above; never writes
	// anything.
	mux.HandleFunc("GET /api/v1/apps/{name}/cost-estimate", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppCostEstimate))

	// Manual build trigger (see Builder/WithBuilder above and
	// handleTriggerBuild's own doc comment): builds an image from a git
	// source through the same internal/deploy.Pipeline the webhook
	// receiver uses, for an operator with no working git webhook
	// configured. AbilityDeploy, the same boundary as the image-tag
	// trigger above: this also ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/builds", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleTriggerBuild))
	mux.HandleFunc("POST /api/v1/apps/{name}/uploads", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleUploadSource))
	mux.HandleFunc("GET /api/v1/apps/{name}/uploads", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListUploads))

	// Multi-service fan-out (handleDeploySpec's own doc comment,
	// apps_multi.go): one app.yaml's services: map, built and deployed as
	// N independent services under one store.App named {name}. Same
	// AbilityDeploy boundary as the manual build trigger above: this also
	// ultimately writes desired state.
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-spec", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeploySpec))

	// Branch listing for an arbitrary public git remote (handleListGitBranches's
	// own doc comment): not scoped to an existing app, since the create-app-
	// from-git wizard needs this before an app exists to attach a build
	// to. AbilityDeploy, the same tier the build trigger above requires:
	// listing what a repo could be built from is a strict subset of
	// actually triggering that build.
	mux.HandleFunc("POST /api/v1/git/branches", rt.requireAbility(AbilityDeploy, rt.handleListGitBranches))

	// Framework pre-flight detection (handleDetectFramework's own doc
	// comment): also not scoped to an existing app, same AbilityDeploy
	// boundary as the branch listing above.
	mux.HandleFunc("POST /api/v1/build/detect", rt.requireAbility(AbilityDeploy, rt.handleDetectFramework))

	mux.HandleFunc("GET /api/v1/frameworks", rt.requireAbility(AbilityRead, rt.handleListFrameworks))
	mux.HandleFunc("GET /api/v1/frameworks/{id}", rt.requireAbility(AbilityRead, rt.handleGetFramework))
	mux.HandleFunc("GET /api/v1/frameworks/{id}/dockerfile", rt.requireAbility(AbilityRead, rt.handleGetFrameworkDockerfile))

	// Import front door: classifies pasted input and returns a plan preview,
	// creating nothing. AbilityWrite because it makes outbound fetches.
	mux.HandleFunc("POST /api/v1/imports/plan", rt.requireAbility(AbilityWrite, rt.handleImportPlan))
	mux.HandleFunc("POST /api/v1/imports/analyze", rt.requireAbility(AbilityWrite, rt.handleAnalyzeImport))
	mux.HandleFunc("POST /api/v1/git-sources/analyze", rt.requireAbility(AbilityWrite, rt.handleAnalyzeImport))
	mux.HandleFunc("POST /api/v1/git-sources/preview", rt.requireAbility(AbilityWrite, rt.handlePreviewRepo))

	// Previously-built image tags for this app's repo, so the deploy
	// trigger form can offer a dropdown instead of a hand-typed tag
	// (see ImageLister above). AbilityRead like every other passive
	// view of an app's own state.
	mux.HandleFunc("GET /api/v1/apps/{name}/images", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListImages))

	// Live traffic path (network.go): declared container port plus the
	// current Docker-assigned host port Caddy is actually proxying to, for
	// the dashboard's Network tab. AbilityRead, same tier as images above.
	mux.HandleFunc("GET /api/v1/apps/{name}/network", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppNetwork))
	// Exposure: port, direct host port and its tagged firewall rule. The write
	// opens a host port, so it is AbilityWriteSensitive like firewall rules.
	mux.HandleFunc("GET /api/v1/apps/{name}/urls", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppURLs))
	mux.HandleFunc("GET /api/v1/apps/{name}/exposure", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppExposure))
	mux.HandleFunc("POST /api/v1/apps/{name}/exposure/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePreviewAppExposure))
	mux.HandleFunc("PUT /api/v1/apps/{name}/exposure", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppExposure))
	mux.HandleFunc("POST /api/v1/apps/{name}/exposure/check", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleCheckAppExposure))
}
