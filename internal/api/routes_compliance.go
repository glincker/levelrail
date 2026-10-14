package api

import "net/http"

// registerComplianceRoutes wires access reviews and evidence packs. All of
// it is root only: it lists every account and exports audit history.
func (rt *Router) registerComplianceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/compliance/access-reviews", rt.requireAbility(AbilityRoot, rt.handleListAccessReviews))
	mux.HandleFunc("POST /api/v1/compliance/access-reviews", rt.requireAbility(AbilityRoot, rt.handleCreateAccessReview))
	mux.HandleFunc("GET /api/v1/compliance/access-reviews/{id}", rt.requireAbility(AbilityRoot, rt.handleGetAccessReview))
	mux.HandleFunc("PUT /api/v1/compliance/access-reviews/{id}/items/{item}", rt.requireAbility(AbilityRoot, rt.handleDecideAccessReviewItem))
	mux.HandleFunc("POST /api/v1/compliance/access-reviews/{id}/complete", rt.requireAbility(AbilityRoot, rt.handleCompleteAccessReview))
	mux.HandleFunc("GET /api/v1/compliance/evidence-packs", rt.requireAbility(AbilityRoot, rt.handleListEvidencePacks))
	mux.HandleFunc("POST /api/v1/compliance/evidence-packs", rt.requireAbility(AbilityRoot, rt.handleCreateEvidencePack))
	mux.HandleFunc("GET /api/v1/compliance/evidence-packs/{id}", rt.requireAbility(AbilityRoot, rt.handleGetEvidencePack))
	mux.HandleFunc("GET /api/v1/compliance/evidence-packs/{id}/download", rt.requireAbility(AbilityRoot, rt.handleDownloadEvidencePack))
	mux.HandleFunc("POST /api/v1/compliance/evidence-packs/{id}/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyEvidencePack))
}
