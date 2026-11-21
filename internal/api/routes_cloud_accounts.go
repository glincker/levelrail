package api

import "net/http"

// Writes and provider calls are root only until grants cover them; reads filter by grant.
// Grant routes pass through requireCloudAccess(share), so a share holder can manage grants at or below own scope.
func (rt *Router) registerCloudAccountRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	mux.HandleFunc("GET /api/v1/cloud-accounts", rt.requireAbility(AbilityRead, rt.handleListCloudAccounts))
	mux.HandleFunc("POST /api/v1/cloud-accounts", rt.requireAbility(AbilityRoot, rt.handleCreateCloudAccount))
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
}
