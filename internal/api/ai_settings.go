package api

import (
	"encoding/json"
	"net/http"
)

// handleGetAIAssistantSettings handles GET /api/v1/settings/ai-assistant.
// configured reports whether a usable provider chain exists; keys never
// appear in the response.
func (rt *Router) handleGetAIAssistantSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	out, err := rt.aiSettingsResource(r.Context())
	if err != nil {
		rt.writeAIVaultError(w, "api: get ai assistant settings failed", err, "")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUpdateAIAssistantSettings handles PUT /api/v1/settings/ai-assistant:
// it sets the active model and/or the fallback chain.
func (rt *Router) handleUpdateAIAssistantSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	var req aiChoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Active == nil && req.Fallback == nil {
		writeError(w, http.StatusBadRequest, "active or fallback is required")
		return
	}
	rt.updateAIChoice(w, r, req)
}
