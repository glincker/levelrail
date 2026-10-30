package api

import "net/http"

// registerAppRuntimeRoutes registers the functions, telemetry, alerting, scheduled tasks, flags, tags and notifications routes.
func (rt *Router) registerAppRuntimeRoutes(mux *http.ServeMux) {
	// Functions: apps that sleep to zero and hold the waking request, with call count and p95 duration, plus starter handlers per runtime.
	mux.HandleFunc("GET /api/v1/functions", rt.requireAbility(AbilityRead, rt.handleListFunctions))
	mux.HandleFunc("GET /api/v1/functions/runtimes", rt.requireAbility(AbilityRead, rt.handleFunctionRuntimes))
	mux.HandleFunc("GET /api/v1/functions/starters", rt.requireAbility(AbilityRead, rt.handleFunctionStarters))
	// Function controls: request timeout and concurrency limit, deploy versions, and rollback or pin through the app rollback.
	mux.HandleFunc("GET /api/v1/functions/{name}/settings", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFunctionSettings))
	mux.HandleFunc("PUT /api/v1/functions/{name}/settings", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutFunctionSettings))
	mux.HandleFunc("GET /api/v1/functions/{name}/versions", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListFunctionVersions))
	mux.HandleFunc("POST /api/v1/functions/{name}/rollback", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleFunctionRollback))
	// Function paths: serve a function at <domain>/<path> through a forwarder rule on the domain.
	mux.HandleFunc("GET /api/v1/functions/{name}/route", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListFunctionRoutes))
	mux.HandleFunc("PUT /api/v1/functions/{name}/route", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetFunctionRoute))
	mux.HandleFunc("DELETE /api/v1/functions/{name}/route", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteFunctionRoute))
	// Function observe and try: recent edge calls, a test call through the real route, replay of stored input, and the per-function capture opt-in.
	mux.HandleFunc("GET /api/v1/functions/{name}/invocations", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListFunctionInvocations))
	mux.HandleFunc("POST /api/v1/functions/{name}/invoke", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleInvokeFunction))
	mux.HandleFunc("POST /api/v1/functions/{name}/invocations/{id}/replay", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleReplayFunctionInvocation))
	mux.HandleFunc("GET /api/v1/functions/{name}/capture", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFunctionCapture))
	mux.HandleFunc("PUT /api/v1/functions/{name}/capture", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleSetFunctionCapture))
	// Function triggers: signed webhook per function, async invocation queue with retries, dead-letter list and retry.
	mux.HandleFunc("GET /api/v1/functions/{name}/triggers", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFunctionTriggers))
	mux.HandleFunc("PUT /api/v1/functions/{name}/triggers/webhook", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutFunctionWebhook))
	mux.HandleFunc("DELETE /api/v1/functions/{name}/triggers/webhook", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteFunctionWebhook))
	mux.HandleFunc("POST /api/v1/functions/{name}/async", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleEnqueueFunctionInvocation))
	mux.HandleFunc("GET /api/v1/function-invocations", rt.requireAbility(AbilityRead, rt.handleListQueuedFunctionCalls))
	mux.HandleFunc("POST /api/v1/function-invocations/{id}/retry", rt.requireAbility(AbilityDeploy, rt.handleRetryFunctionInvocation))
	mux.HandleFunc("DELETE /api/v1/function-invocations/{id}", rt.requireAbility(AbilityDeploy, rt.handleDeleteFunctionInvocation))
	mux.HandleFunc("POST /api/v1/webhooks/functions/{name}", rt.handleFunctionWebhook)
	// Function key-value store: browse and edit from the dashboard and CLI, and the bearer-token entry points a function itself calls.
	mux.HandleFunc("GET /api/v1/functions/{name}/kv", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListFunctionKV))
	mux.HandleFunc("GET /api/v1/functions/{name}/kv/value", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFunctionKVValue))
	mux.HandleFunc("PUT /api/v1/functions/{name}/kv/value", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handlePutFunctionKVValue))
	mux.HandleFunc("DELETE /api/v1/functions/{name}/kv/value", rt.requireAbilityForResource(AbilityDeploy, appResourceFromPath, rt.handleDeleteFunctionKVValue))
	mux.HandleFunc("POST /api/v1/functions/{name}/kv/token", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleRotateFunctionKVToken))
	mux.HandleFunc("GET /api/v1/function-kv/entry", rt.handleFunctionKVEntry)
	mux.HandleFunc("PUT /api/v1/function-kv/entry", rt.handleFunctionKVEntry)
	mux.HandleFunc("DELETE /api/v1/function-kv/entry", rt.handleFunctionKVEntry)
	mux.HandleFunc("GET /api/v1/function-kv/keys", rt.handleFunctionKVKeys)
	mux.HandleFunc("GET /api/v1/apps/{name}/requests", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleQueryRequests))
	// Traffic: RED totals, per-replica latency, slowest route templates, slow or failed request samples and SLO burn over window=5m, 1h, 24h or 7d, plus top referrers and countries.
	mux.HandleFunc("GET /api/v1/apps/{name}/traffic", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppTraffic))
	mux.HandleFunc("GET /api/v1/apps/{name}/investigate", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleInvestigate))
	mux.HandleFunc("GET /api/v1/apps/{name}/failure-context", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleFailureContext))
	mux.HandleFunc("GET /api/v1/apps/{name}/logs", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleQueryLogs))
	// Cross-app resource usage ranking (app_resource_usage.go): a
	// literal segment, so Go's ServeMux resolves it ahead of the
	// {name} wildcard on GET /api/v1/apps/{name} in routes.go.
	mux.HandleFunc("GET /api/v1/apps-metrics", rt.requireAbility(AbilityRead, rt.handleBatchAppMetrics))
	mux.HandleFunc("GET /api/v1/apps/resource-usage", rt.requireAbility(AbilityRead, rt.handleAppResourceUsage))
	// Live log tail (additive to the historical search route just above,
	// see handleLiveLogStream's own doc comment): AbilityRead, the same
	// passive-visibility boundary as every other view of telemetry data
	// in this router, including the deploy-log stream at
	// GET .../deploys/{deployId}/logs above.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/stream", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.withStreamReauth(appResourceFromPath, rt.handleLiveLogStream)))
	// Log export (a plain-text attachment of a bounded window, see
	// logs_download.go): same AbilityRead boundary as the query/stream
	// routes above, since it reads the same store through the same
	// telemetry.QueryLogs call, nothing more sensitive than either.
	mux.HandleFunc("GET /api/v1/apps/{name}/logs/download", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDownloadLogs))

	// Alerting: threshold and crashloop rules scoped
	// to one app, fanned through a *alerting.DB when configured (see
	// WithAlertRules).
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateAlertRule))
	mux.HandleFunc("GET /api/v1/apps/{name}/alerts", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAlertRules))
	mux.HandleFunc("PUT /api/v1/apps/{name}/alerts/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleUpdateAlertRule))
	mux.HandleFunc("GET /api/v1/alert-templates", rt.requireAbility(AbilityRead, rt.handleListAlertTemplates))
	mux.HandleFunc("POST /api/v1/apps/{name}/alerts/apply-template", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleApplyAlertTemplate))
	mux.HandleFunc("GET /api/v1/notification-channels/{id}/quiet-hours", rt.requireAbility(AbilityRead, rt.handleGetChannelQuietHours))
	mux.HandleFunc("PUT /api/v1/notification-channels/{id}/quiet-hours", rt.requireAbility(AbilityWrite, rt.handlePutChannelQuietHours))
	mux.HandleFunc("DELETE /api/v1/notification-channels/{id}/quiet-hours", rt.requireAbility(AbilityWrite, rt.handleDeleteChannelQuietHours))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/alerts/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAlertRule))

	// Health & readiness score: a read-only synthesis of signals this
	// router already exposes separately (deploys, crashloop, TLS,
	// secrets, backups, health checks, alerting). See
	// app_health_score.go's own doc comment.
	mux.HandleFunc("GET /api/v1/apps/{name}/health-score", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppHealthScore))

	// Scheduled tasks: run an arbitrary command inside this app's
	// container on a cron schedule (internal/scheduledtask). CRUD sits at
	// AbilityWrite, the same config-mutation tier alert rules above and
	// env var updates already use; "run now" sits one tier up at
	// AbilityDeploy, matching restart/stop/start above, since it has the
	// identical immediate side effect (a real command actually runs
	// inside a running container right now), not just a config change.
	rt.registerJobRoutes(mux)
	rt.registerOneShotJobRoutes(mux)

	// Feature flags: CRUD at AbilityWrite/AbilityRead. Evaluate is a flat AbilityRead route an app
	// calls at runtime with a read-only token.
	mux.HandleFunc("POST /api/v1/apps/{name}/flags", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateFeatureFlag))
	mux.HandleFunc("GET /api/v1/apps/{name}/flags", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListFeatureFlags))
	mux.HandleFunc("GET /api/v1/apps/{name}/flags/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetFeatureFlag))
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
	mux.HandleFunc("GET /api/v1/apps/{name}/tags", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppTags))
	mux.HandleFunc("POST /api/v1/apps/{name}/tags", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleAttachAppTag))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/tags/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDetachAppTag))

	// Integrations (app_integrations.go): the curated internal/integrations
	// catalog (PostHog, Sentry, etc), env-var injection only. The catalog
	// itself is global and read-only (AbilityRead, no store involved,
	// same shape service-templates uses); attach/detach store a field
	// value through the app's own secrets namespace, AbilityWriteSensitive
	// like PUT .../secrets/{key} above.
	mux.HandleFunc("GET /api/v1/integrations", rt.requireAbility(AbilityRead, rt.handleListIntegrationCatalog))
	mux.HandleFunc("GET /api/v1/apps/{name}/integrations", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppIntegrations))
	mux.HandleFunc("POST /api/v1/apps/{name}/integrations", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleAttachAppIntegration))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/integrations/{id}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDetachAppIntegration))

	// Deploy-outcome notifications (wave-2 roadmap item #5): a Slack/
	// Discord/Telegram/generic-webhook/email ping fired once per deploy
	// attempt reaching a terminal state, distinct from the threshold/
	// crashloop alert rules just above (see
	// internal/alerting/deploy_notify.go's own doc comment). Also fanned
	// through *alerting.DB when configured (see WithDeployNotifyTargets).
	mux.HandleFunc("POST /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateDeployNotifyTarget))
	mux.HandleFunc("GET /api/v1/apps/{name}/deploy-notify-targets", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListDeployNotifyTargets))
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

	// Loki-compatible read API over the node-local log store (loki.go),
	// same AbilityRead gate; Basic auth password is accepted as the token.
	rt.registerLokiRoutes(mux)
}
