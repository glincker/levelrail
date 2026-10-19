package api

import "net/http"

// registerAuditRoutes registers the storage env keys, audit log, OTLP, AI settings and global environments routes.
func (rt *Router) registerAuditRoutes(mux *http.ServeMux) {
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
	// Walks the audit hash chain and reports edited, removed or truncated rows.
	mux.HandleFunc("GET /api/v1/audit-log/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyAuditLog))
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
	mux.HandleFunc("POST /api/v1/otlp/v1/traces", rt.handleOTLPTraces)
	mux.HandleFunc("GET /api/v1/apps/{name}/tracing", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetTracing))
	mux.HandleFunc("POST /api/v1/apps/{name}/tracing", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleEnableTracing))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/tracing", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDisableTracing))
	mux.HandleFunc("GET /api/v1/apps/{name}/traces", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListTraces))
	mux.HandleFunc("POST /api/v1/sentry/api/{project}/envelope/", rt.handleSentryEnvelope)
	mux.HandleFunc("POST /api/v1/sentry/api/{project}/store/", rt.handleSentryStore)
	mux.HandleFunc("OPTIONS /api/v1/sentry/api/{project}/envelope/", rt.handleSentryPreflight)
	mux.HandleFunc("OPTIONS /api/v1/sentry/api/{project}/store/", rt.handleSentryPreflight)
	mux.HandleFunc("GET /api/v1/apps/{name}/error-tracking", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetErrorTracking))
	mux.HandleFunc("POST /api/v1/apps/{name}/error-tracking", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleEnableErrorTracking))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/error-tracking", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDisableErrorTracking))
	mux.HandleFunc("GET /api/v1/apps/{name}/errors", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListErrors))
	mux.HandleFunc("GET /api/v1/apps/{name}/errors/{fingerprint}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetError))
	mux.HandleFunc("PATCH /api/v1/apps/{name}/errors/{fingerprint}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handlePatchError))
	mux.HandleFunc("GET /api/v1/apps/{name}/log-pipeline", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetLogPipeline))
	mux.HandleFunc("PUT /api/v1/apps/{name}/log-pipeline", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetLogPipeline))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/log-pipeline", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteLogPipeline))
	mux.HandleFunc("POST /api/v1/apps/{name}/log-pipeline/preview", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handlePreviewLogPipeline))
	mux.HandleFunc("GET /api/v1/logs/usage", rt.requireAbility(AbilityRead, rt.handleLogUsage))
	mux.HandleFunc("GET /api/v1/apps/{name}/log-budget", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetLogBudget))
	mux.HandleFunc("PUT /api/v1/apps/{name}/log-budget", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetLogBudget))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/log-budget", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteLogBudget))
	mux.HandleFunc("POST /api/v1/apps/{name}/log-budget/trim", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleTrimLogBudget))
	mux.HandleFunc("GET /api/v1/apps/{name}/access", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleAppAccess))
	mux.HandleFunc("GET /api/v1/apps/{name}/traces/{id}", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetTrace))
	mux.HandleFunc("GET /api/v1/apps/{name}/log-drain", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppLogDrain))
	mux.HandleFunc("PUT /api/v1/apps/{name}/log-drain", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppLogDrain))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/log-drain", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppLogDrain))

	// BYOK AI assistant settings (ai_settings.go): GET is AbilityRead;
	// PUT is AbilityRoot, the same tier PUT /api/v1/settings/
	// email uses for any other platform-wide credential-bearing config.
	mux.HandleFunc("GET /api/v1/settings/ai-assistant", rt.requireAbility(AbilityRead, rt.handleGetAIAssistantSettings))
	mux.HandleFunc("PUT /api/v1/settings/ai-assistant", rt.requireAbility(AbilityRoot, rt.handleUpdateAIAssistantSettings))
	mux.HandleFunc("GET /api/v1/settings/ai-assistant/providers", rt.requireAbility(AbilityRead, rt.handleListAIProviders))
	mux.HandleFunc("PUT /api/v1/settings/ai-assistant/providers/{id}", rt.requireAbility(AbilityRoot, rt.handleSetAIProvider))
	mux.HandleFunc("POST /api/v1/settings/ai-assistant/providers/{id}/test", rt.requireAbility(AbilityRoot, rt.handleTestAIProvider))
	mux.HandleFunc("DELETE /api/v1/settings/ai-assistant/providers/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteAIProvider))
	mux.HandleFunc("GET /api/v1/settings/ai-assistant/providers/{id}/models", rt.requireAbility(AbilityRoot, rt.handleListAIProviderModels))
	mux.HandleFunc("GET /api/v1/ai/usage", rt.requireAbility(AbilityRead, rt.handleAIUsage))
	mux.HandleFunc("GET /api/v1/settings/ai-control", rt.requireAbility(AbilityRead, rt.handleGetAIControl))
	mux.HandleFunc("PUT /api/v1/settings/ai-control", rt.requireAbility(AbilityRoot, rt.handleUpdateAIControl))
	mux.HandleFunc("POST /api/v1/settings/ai-control/revoke-agent-tokens", rt.requireAbility(AbilityRoot, rt.handleRevokeAgentTokens))

	// AI assistant chat sessions (ai_chat.go): AbilityRoot throughout,
	// not a lower tier, because a confirmed message can execute any
	// mutating tool the platform exposes (deploy, rollback, restart, and
	// so on) once a human approves it, the same blast radius as the
	// scoped token itself would need to reach those actions directly.
	mux.HandleFunc("POST /api/v1/ai/sessions", rt.requireAbility(AbilityRoot, rt.handleCreateAIChatSession))
	mux.HandleFunc("GET /api/v1/ai/sessions", rt.requireAbility(AbilityRoot, rt.handleListAIChatSessions))
	mux.HandleFunc("GET /api/v1/ai/sessions/{id}", rt.requireAbility(AbilityRoot, rt.handleGetAIChatSession))
	mux.HandleFunc("DELETE /api/v1/ai/sessions/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteAIChatSession))
	mux.HandleFunc("POST /api/v1/ai/sessions/{id}/messages", rt.requireAbility(AbilityRoot, rt.handleCreateAIChatMessage))
	mux.HandleFunc("POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}", rt.requireAbility(AbilityRoot, rt.handleResolveAIChatConfirmation))

	// Global typed environments (environments_global.go, environment_move.go),
	// behind the global-environments experimental feature.
	mux.HandleFunc("GET /api/v1/environments", rt.requireAbility(AbilityRead, rt.handleListAllEnvironments))
	mux.HandleFunc("POST /api/v1/environments", rt.requireAbility(AbilityWrite, rt.handleCreateGlobalEnvironment))
	mux.HandleFunc("PUT /api/v1/databases/{name}/environment", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleSetDatabaseEnvironment))
}
