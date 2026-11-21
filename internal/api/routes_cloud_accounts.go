package api

import "net/http"

// Writes and provider calls are root only until grants cover them; reads filter by grant.
// Grant routes pass through requireCloudAccess(share), so a share holder can manage grants at or below own scope.
func (rt *Router) registerCloudAccountRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	mux.HandleFunc("GET /api/v1/cloud-accounts", rt.requireAbility(AbilityRead, rt.handleListCloudAccounts))
	mux.HandleFunc("POST /api/v1/cloud-accounts", rt.requireAbility(AbilityRoot, rt.handleCreateCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/aws/control-identity", rt.requireAbility(AbilityRoot, rt.handleAWSControlIdentity))
	mux.HandleFunc("GET /api/v1/cloud-accounts/doctor/preflight", rt.requireAbility(AbilityRoot, rt.handleCloudDoctorPreflight))
	mux.HandleFunc("GET /api/v1/me/cloud-access", rt.requireAbilityDecided(AbilityRead, allowAnyAuthenticated, rt.handleMeCloudAccess))
	mux.HandleFunc("POST /api/v1/cloud-accounts/aws/link-kit", rt.requireAbility(AbilityRoot, rt.handleAWSLinkKit))
	mux.HandleFunc("POST /api/v1/cloud-accounts/aws/vault-kit", rt.requireAbility(AbilityRoot, rt.handleAWSVaultKit))
	mux.HandleFunc("POST /api/v1/cloud-accounts/oracle/policy-kit", rt.requireAbility(AbilityRoot, rt.handleOraclePolicyKit))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}", rt.requireCloudAccess(db, "view", cloudAccountFromPath, rt.handleGetCloudAccount))
	mux.HandleFunc("PATCH /api/v1/cloud-accounts/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudAccount))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteCloudAccount))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/regions", rt.requireCloudAccess(db, "view", cloudAccountFromPath, rt.handleCloudAccountRegions))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/doctor", rt.requireCloudAccess(db, "view", cloudAccountFromPath, rt.handleCloudAccountDoctor))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/probe", rt.requireAbility(AbilityRoot, rt.handleProbeCloudAccount))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/grants", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleListCloudGrants))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/grants", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleCreateCloudGrant))
	mux.HandleFunc("PATCH /api/v1/cloud-accounts/{id}/grants/{gid}", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleUpdateCloudGrant))
	mux.HandleFunc("DELETE /api/v1/cloud-accounts/{id}/grants/{gid}", rt.requireCloudAccess(db, "share", cloudAccountFromPath, rt.handleDeleteCloudGrant))
	rt.registerCloudInstanceRoutes(mux)
	rt.registerCloudSpendRoutes(mux)
	rt.registerCloudAWSServiceRoutes(mux)
	rt.registerCloudRDSRoutes(mux)
	rt.registerCloudAWSOpsRoutes(mux)
}

func (rt *Router) registerCloudInstanceRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/instances", rt.requireCloudAccess(db, "view", acct, rt.handleListCloudInstances))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/instances/{iid}", rt.requireCloudAccess(db, "view", cloudInstanceFromPath, rt.handleGetCloudInstance))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/instances/{iid}/events", rt.requireCloudAccess(db, "view", cloudInstanceFromPath, rt.handleCloudInstanceEvents))
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

// AWS service modules sit behind per service runtime flags; secret writes and reveals map to manage_budget (write:sensitive).
func (rt *Router) registerCloudAWSServiceRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/ecr/repositories", rt.requireCloudAccess(db, "view", acct, rt.handleAWSECRList))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ecr/repositories", rt.requireCloudAccess(db, "operate", acct, rt.handleAWSECRCreate))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ecr/login-token", rt.requireCloudAccess(db, "operate", acct, rt.handleAWSECRLoginToken))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/acm/certificates", rt.requireCloudAccess(db, "view", acct, rt.handleAWSACMList))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/acm/certificates", rt.requireCloudAccess(db, "operate", acct, rt.handleAWSACMRequest))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/secrets", rt.requireCloudAccess(db, "view", acct, rt.handleAWSSecretsList))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/secrets", rt.requireCloudAccess(db, "manage_budget", acct, rt.handleAWSSecretsPut))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/secrets/reveal", rt.requireCloudAccess(db, "manage_budget", acct, rt.handleAWSSecretsReveal))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/audit", rt.requireCloudAccess(db, "view", acct, rt.handleAWSAudit))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/guard", rt.requireCloudAccess(db, "view", acct, rt.handleAWSGuardStatus))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/guard/kill-switch", rt.requireAbility(AbilityRoot, rt.handleAWSKillSwitch))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cleanup/plan", rt.requireCloudAccess(db, "manage_budget", acct, rt.handleAWSCleanupPlan))
}
