package api

import "net/http"

// registerMailRoutes registers email templates, audiences, suppressions and broadcasts.
// Reading templates is AbilityRead; everything that names recipients or sends is AbilityRoot.
func (rt *Router) registerMailRoutes(mux *http.ServeMux) {
	rt.registerMailPublicRoutes(mux)
	mux.HandleFunc("GET /api/v1/mail/tokens", rt.requireAbility(AbilityRead, rt.handleListMailTokens))
	mux.HandleFunc("GET /api/v1/mail/templates", rt.requireAbility(AbilityRead, rt.handleListMailTemplates))
	mux.HandleFunc("GET /api/v1/mail/templates/{key}", rt.requireAbility(AbilityRead, rt.handleGetMailTemplate))
	mux.HandleFunc("PUT /api/v1/mail/templates/{key}", rt.requireAbility(AbilityRoot, rt.handleSetMailTemplate))
	mux.HandleFunc("DELETE /api/v1/mail/templates/{key}", rt.requireAbility(AbilityRoot, rt.handleResetMailTemplate))
	mux.HandleFunc("GET /api/v1/mail/templates/{key}/versions", rt.requireAbility(AbilityRead, rt.handleListMailTemplateVersions))
	mux.HandleFunc("POST /api/v1/mail/templates/{key}/versions/{version}/restore", rt.requireAbility(AbilityRoot, rt.handleRestoreMailTemplateVersion))
	mux.HandleFunc("POST /api/v1/mail/templates/{key}/preview", rt.requireAbility(AbilityRoot, rt.handlePreviewMailTemplate))
	mux.HandleFunc("POST /api/v1/mail/templates/{key}/test", rt.requireAbility(AbilityRoot, rt.handleTestMailTemplate))
	mux.HandleFunc("PUT /api/v1/mail/layout", rt.requireAbility(AbilityRoot, rt.handleSetMailLayout))

	mux.HandleFunc("GET /api/v1/mail/audience-options", rt.requireAbility(AbilityRoot, rt.handleMailAudienceOptions))
	mux.HandleFunc("POST /api/v1/mail/audience/preview", rt.requireAbility(AbilityRoot, rt.handlePreviewMailAudience))
	mux.HandleFunc("GET /api/v1/mail/groups", rt.requireAbility(AbilityRoot, rt.handleListMailGroups))
	mux.HandleFunc("POST /api/v1/mail/groups", rt.requireAbility(AbilityRoot, rt.handleCreateMailGroup))
	mux.HandleFunc("GET /api/v1/mail/groups/{id}", rt.requireAbility(AbilityRoot, rt.handleGetMailGroup))
	mux.HandleFunc("PUT /api/v1/mail/groups/{id}", rt.requireAbility(AbilityRoot, rt.handleUpdateMailGroup))
	mux.HandleFunc("DELETE /api/v1/mail/groups/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteMailGroup))
	mux.HandleFunc("GET /api/v1/mail/suppressions", rt.requireAbility(AbilityRoot, rt.handleListMailSuppressions))
	mux.HandleFunc("POST /api/v1/mail/suppressions", rt.requireAbility(AbilityRoot, rt.handleAddMailSuppression))
	mux.HandleFunc("DELETE /api/v1/mail/suppressions/{email}", rt.requireAbility(AbilityRoot, rt.handleRemoveMailSuppression))

	mux.HandleFunc("GET /api/v1/mail/broadcasts", rt.requireAbility(AbilityRoot, rt.handleListMailBroadcasts))
	mux.HandleFunc("POST /api/v1/mail/broadcasts", rt.requireAbility(AbilityRoot, rt.handleCreateMailBroadcast))
	mux.HandleFunc("POST /api/v1/mail/broadcasts/test", rt.requireAbility(AbilityRoot, rt.handleTestMailBroadcast))
	mux.HandleFunc("GET /api/v1/mail/broadcasts/{id}", rt.requireAbility(AbilityRoot, rt.handleGetMailBroadcast))
	mux.HandleFunc("POST /api/v1/mail/broadcasts/{id}/cancel", rt.requireAbility(AbilityRoot, rt.handleCancelMailBroadcast))
}
