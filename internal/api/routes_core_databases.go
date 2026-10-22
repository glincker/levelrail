package api

import "net/http"

// registerDatabaseRoutes registers the database engines, databases and external databases routes.
func (rt *Router) registerDatabaseRoutes(mux *http.ServeMux) {
	// Databases CRUD. No PUT: engine, version and name never change in place (that is a migration).
	// The engine registry list sits first since it is not scoped to one database.
	mux.HandleFunc("GET /api/v1/database-engines", rt.requireAbility(AbilityRead, rt.handleListDatabaseEngines))

	mux.HandleFunc("GET /api/v1/databases", rt.requireAbility(AbilityRead, rt.handleListDatabases))
	mux.HandleFunc("POST /api/v1/databases", rt.requireAbility(AbilityWrite, rt.handleCreateDatabase))
	// Resource-scoped, same reasoning as the apps routes above.
	mux.HandleFunc("GET /api/v1/databases/{name}", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabase))
	mux.HandleFunc("DELETE /api/v1/databases/{name}", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleDeleteDatabase))
	mux.HandleFunc("GET /api/v1/databases/{name}/provision", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabaseProvision))
	mux.HandleFunc("GET /api/v1/databases/{name}/provision/stream", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.withStreamReauth(databaseResourceFromPath, rt.handleDatabaseProvisionStream)))
	mux.HandleFunc("POST /api/v1/databases/{name}/provision/retry", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleRetryDatabaseProvision))
	mux.HandleFunc("POST /api/v1/databases/{name}/provision/discard", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleDiscardPendingDatabase))
	mux.HandleFunc("PUT /api/v1/databases/{name}/pending-settings", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleUpdatePendingDatabase))
	mux.HandleFunc("GET /api/v1/databases/{name}/status", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleDatabaseStatus))

	// Telemetry query, the database counterpart to
	// GET /apps/{name}/metrics, /logs, /logs/stream above: same
	// TelemetryQuerier/logBroadcaster gating (501 when unconfigured).
	// database_metrics.go/database_logs.go share the actual query/stream
	// logic with their app equivalents via queryResourceMetrics/
	// queryResourceLogs/streamResourceLogs (metrics.go/logs.go/
	// live_logs.go), parameterized on a resourceLookup, not a
	// hand-copied duplicate.
	mux.HandleFunc("GET /api/v1/databases/{name}/metrics", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleQueryDatabaseMetrics))
	mux.HandleFunc("GET /api/v1/databases/{name}/logs", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleQueryDatabaseLogs))
	mux.HandleFunc("GET /api/v1/databases/{name}/logs/stream", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.withStreamReauth(databaseResourceFromPath, rt.handleLiveDatabaseLogStream)))

	// Slow query log, Postgres/MySQL only (database_slow_queries.go's own
	// doc comment explains why Redis and every other engine return 400):
	// parses the same stored container log lines the routes above expose,
	// rather than a new telemetry source.
	mux.HandleFunc("GET /api/v1/databases/{name}/slow-queries", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleQueryDatabaseSlowQueries))
	// Live counts for engines with no data browser; index names can name tenants.
	mux.HandleFunc("GET /api/v1/databases/{name}/engine-stats", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleGetDatabaseEngineStats))
	// Database viewer: row data and SQL are sensitive, so reads are
	// AbilityReadSensitive; the write console is AbilityRoot (admin only).
	mux.HandleFunc("GET /api/v1/databases/{name}/schema", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleGetDatabaseSchema))
	mux.HandleFunc("GET /api/v1/databases/{name}/tables/{schema}/{table}/rows", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleGetDatabaseTableRows))
	mux.HandleFunc("GET /api/v1/databases/{name}/tables/{schema}/{table}/structure", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleGetDatabaseTableStructure))
	mux.HandleFunc("POST /api/v1/databases/{name}/query", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleDatabaseQuery))
	mux.HandleFunc("POST /api/v1/databases/{name}/query/write", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleDatabaseQueryWrite))
	mux.HandleFunc("POST /api/v1/databases/{name}/explain", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleDatabaseExplain))
	mux.HandleFunc("GET /api/v1/databases/{name}/keys", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleScanDatabaseKeys))
	mux.HandleFunc("GET /api/v1/databases/{name}/key", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleGetDatabaseKey))
	mux.HandleFunc("GET /api/v1/databases/{name}/query-history", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleListDatabaseQueryHistory))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/query-history", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleClearDatabaseQueryHistory))
	mux.HandleFunc("GET /api/v1/databases/{name}/saved-queries", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleListDatabaseSavedQueries))
	mux.HandleFunc("POST /api/v1/databases/{name}/saved-queries", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleSaveDatabaseQuery))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/saved-queries/{id}", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleDeleteDatabaseSavedQuery))

	// Resource right-sizing, the database counterpart to
	// GET /apps/{name}/resource-recommendation above
	// (database_resource_recommendation.go).
	mux.HandleFunc("GET /api/v1/databases/{name}/resource-recommendation", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleDatabaseResourceRecommendation))

	// Placement, the database counterpart to
	// PUT /apps/{name}/node above: same AbilityRoot gating.
	mux.HandleFunc("PUT /api/v1/databases/{name}/node", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleSetDatabaseNode))

	// External databases: connected, never run, by this platform. Anything
	// that dials out or reveals a credential is admin only.
	mux.HandleFunc("GET /api/v1/external-databases", rt.requireAbility(AbilityRead, rt.handleListExternalDatabases))
	mux.HandleFunc("POST /api/v1/external-databases", rt.requireAbility(AbilityRoot, rt.handleCreateExternalDatabase))
	mux.HandleFunc("POST /api/v1/external-databases/test", rt.requireAbility(AbilityRoot, rt.handleTestExternalDatabase))
	mux.HandleFunc("GET /api/v1/external-databases/candidates", rt.requireAbility(AbilityRoot, rt.handleListExternalDatabaseCandidates))
	mux.HandleFunc("POST /api/v1/external-databases/adopt", rt.requireAbility(AbilityRoot, rt.handleAdoptExternalDatabase))
	mux.HandleFunc("GET /api/v1/external-databases/{name}", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetExternalDatabase))
	mux.HandleFunc("PUT /api/v1/external-databases/{name}", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleUpdateExternalDatabase))
	mux.HandleFunc("DELETE /api/v1/external-databases/{name}", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleDeleteExternalDatabase))
	mux.HandleFunc("POST /api/v1/external-databases/{name}/probe", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleProbeExternalDatabase))
	mux.HandleFunc("GET /api/v1/external-databases/{name}/password", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRevealExternalDatabasePassword))
}
