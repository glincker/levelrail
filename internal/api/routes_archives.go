package api

import "net/http"

func (rt *Router) registerArchiveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/archives", rt.requireAbility(AbilityWriteSensitive, rt.handleListArchives))
	mux.HandleFunc("POST /api/v1/archives", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateArchive))
	mux.HandleFunc("GET /api/v1/archives/policy", rt.requireAbility(AbilityRoot, rt.handleGetArchivePolicy))
	mux.HandleFunc("PUT /api/v1/archives/policy", rt.requireAbility(AbilityRoot, rt.handlePutArchivePolicy))
	mux.HandleFunc("GET /api/v1/archives/{id}", rt.requireAbility(AbilityRoot, rt.handleGetArchive))
	mux.HandleFunc("GET /api/v1/archives/{id}/rows", rt.requireAbility(AbilityRoot, rt.handleBrowseArchive))
	mux.HandleFunc("GET /api/v1/archives/{id}/download", rt.requireAbility(AbilityRoot, rt.handleDownloadArchive))
	mux.HandleFunc("POST /api/v1/archives/{id}/restore", rt.requireAbility(AbilityRoot, rt.handleRestoreArchive))
	mux.HandleFunc("POST /api/v1/archives/{id}/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyArchive))
	mux.HandleFunc("DELETE /api/v1/archives/{id}", rt.requireAbility(AbilityRoot, rt.handleDeleteArchive))
}
