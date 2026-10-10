package api

import "net/http"

// registerDatabaseAccessRoutes wires database users, temporary credentials,
// who-can-access and network controls. Changes need root on the database,
// which a database scoped IAM policy can grant.
func (rt *Router) registerDatabaseAccessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/databases/{name}/users", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleListDatabaseUsers))
	mux.HandleFunc("POST /api/v1/databases/{name}/users", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleCreateDatabaseUser))
	mux.HandleFunc("POST /api/v1/databases/{name}/users/{role}/rotate", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRotateDatabaseUser))
	mux.HandleFunc("POST /api/v1/databases/{name}/users/{role}/disable", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleDisableDatabaseUser))
	mux.HandleFunc("POST /api/v1/databases/{name}/users/{role}/enable", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleEnableDatabaseUser))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/users/{role}", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleDeleteDatabaseUser))

	mux.HandleFunc("GET /api/v1/databases/{name}/access/temp", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleListDatabaseTempCredentials))
	mux.HandleFunc("POST /api/v1/databases/{name}/access/temp", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleIssueDatabaseTempCredential))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/access/temp/{id}", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRevokeDatabaseTempCredential))

	mux.HandleFunc("GET /api/v1/databases/{name}/access/principals", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleDatabaseWhoCanAccess))
	mux.HandleFunc("POST /api/v1/databases/{name}/access/grants", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleGrantDatabaseAccess))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/access/grants/{policy_id}/{principal_type}/{principal_id}", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRevokeDatabaseGrant))

	mux.HandleFunc("GET /api/v1/databases/{name}/network", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabaseNetwork))
	mux.HandleFunc("POST /api/v1/databases/{name}/network/rules/preview", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handlePreviewDatabaseRules))
	mux.HandleFunc("PUT /api/v1/databases/{name}/network/rules", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleApplyDatabaseRules))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/network/rules", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRemoveDatabaseRules))
	mux.HandleFunc("POST /api/v1/databases/{name}/network/make-private", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleMakeDatabasePrivate))
	mux.HandleFunc("PUT /api/v1/databases/{name}/network/scope", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleSetDatabaseScope))
	mux.HandleFunc("PUT /api/v1/databases/{name}/network/tls", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleSetDatabaseTLS))
}
