package api

import "net/http"

// Writes and provider calls are root only until grants cover them; reads filter by grant.
// Grant routes pass through requireCloudAccess(share), so a share holder can manage grants at or below own scope.
func (rt *Router) registerCloudAccountRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	mux.HandleFunc("GET /api/v1/cloud-accounts", rt.requireAbility(AbilityRead, rt.handleListCloudAccounts))
	mux.HandleFunc("POST /api/v1/cloud-accounts", rt.requireAbility(AbilityRoot, rt.handleCreateCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/aws/control-identity", rt.requireAbility(AbilityRoot, rt.handleAWSControlIdentity))
	mux.HandleFunc("GET /api/v1/me/cloud-access", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleMeCloudAccess))
	mux.HandleFunc("POST /api/v1/cloud-accounts/aws/link-kit", rt.requireAbility(AbilityRoot, rt.handleAWSLinkKit))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}", rt.requireCloudAccess(db, "view", cloudAccountFromPath, rt.handleGetCloudAccount))
	mux.HandleFunc("PATCH /api/v1/cloud-accounts/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudAccount))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteCloudAccount))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/regions", rt.requireCloudAccess(db, "view", cloudAccountFromPath, rt.handleCloudAccountRegions))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/probe", rt.requireAbility(AbilityRoot, rt.handleProbeCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/grants", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleListCloudGrants))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/grants", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleCreateCloudGrant))
	mux.HandleFunc("PATCH /api/v1/cloud-accounts/{id}/grants/{gid}", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleUpdateCloudGrant))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/grants/{gid}", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleDeleteCloudGrant))
	rt.registerCloudInstanceRoutes(mux)
	rt.registerCloudSpendRoutes(mux)
}

func (rt *Router) registerCloudInstanceRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/instances", rt.requireCloudAccess(db, "view", acct, rt.handleListCloudInstances))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/instances/{iid}", rt.requireCloudAccess(db, "view", cloudInstanceFromPath, rt.handleGetCloudInstance))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/instances/{iid}/actions", rt.requireCloudAccess(db, "operate", cloudInstanceFromPath, rt.handleCloudInstanceAction))
	mux.HandleFunc("PUT /api/v1/cloud-accounts/{id}/instances/{iid}/tags", rt.requireCloudAccess(db, "operate", cloudInstanceFromPath, rt.handleCloudInstanceTags))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/instances/{iid}/resize", rt.requireCloudAccess(db, "build", cloudInstanceFromPath, rt.handleCloudInstanceResize))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/instances/{iid}", rt.requireCloudAccess(db, "delete", cloudInstanceFromPath, rt.handleCloudInstanceTerminate))
}

func (rt *Router) registerCloudSpendRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/spend", rt.requireCloudAccess(db, "view_spend", cloudAccountFromPath, rt.handleGetCloudSpend))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/spend/refresh", rt.requireCloudAccess(db, "view_spend", cloudAccountFromPath, rt.handleRefreshCloudSpend))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/spend/estimate", rt.requireCloudAccess(db, "view_spend", cloudAccountFromPath, rt.handleCloudSpendEstimate))
}
