package api

import "net/http"

func (rt *Router) registerCloudCFSSMRoutes(mux *http.ServeMux) {
	db := cloudAuthzRef{rt: rt}
	acct := cloudAccountFromPath
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudfront/distributions", rt.requireCloudAccess(db, "view", acct, rt.handleAWSCFList))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudfront/distributions/{did}", rt.requireCloudAccess(db, "view", acct, rt.handleAWSCFGet))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/cloudfront/distributions/{did}/invalidations", rt.requireCloudAccess(db, "view", acct, rt.handleAWSCFInvalidations))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/cloudfront/distributions/{did}/invalidations", rt.requireCloudAccess(db, "operate", acct, rt.handleAWSCFInvalidate))
	mux.HandleFunc("GET /api/v1/cloud-accounts/{id}/aws/ssm/instances", rt.requireCloudAccess(db, "view", acct, rt.handleAWSSSMInstances))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ssm/commands", rt.requireAbility(AbilityRoot, rt.handleAWSSSMSend))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ssm/commands/{cid}/invocation", rt.requireAbility(AbilityRoot, rt.handleAWSSSMInvocation))
	mux.HandleFunc("POST /api/v1/cloud-accounts/{id}/aws/ssm/sessions", rt.requireAbility(AbilityRoot, rt.handleAWSSSMSession))
}
