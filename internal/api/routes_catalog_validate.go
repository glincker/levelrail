package api

import (
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// registerCatalogRoutes wires the template submission checker. It is pure
// (no store, no deploy), so AbilityRead is enough.
func (rt *Router) registerCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/service-templates/validate", rt.requireAbility(AbilityRead, rt.handleValidateTemplateSubmission))
}

// templateValidateMaxBytes caps the submission body; override with LEVELRAIL_TEMPLATE_VALIDATE_MAX_BYTES.
func templateValidateMaxBytes() int64 {
	return envInt64Positive("LEVELRAIL_TEMPLATE_VALIDATE_MAX_BYTES", 1<<18)
}

type validateTemplateRequest struct {
	TemplateYAML string `json:"template_yaml"`
	ComposeYAML  string `json:"compose_yaml"`
}

type validateTemplateResponse struct {
	Valid  bool     `json:"valid"`
	ID     string   `json:"id,omitempty"`
	Name   string   `json:"name,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

func (rt *Router) handleValidateTemplateSubmission(w http.ResponseWriter, r *http.Request) {
	var req validateTemplateRequest
	if err := decodeJSONBodyLimit(r, &req, templateValidateMaxBytes()); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.TemplateYAML) == "" || strings.TrimSpace(req.ComposeYAML) == "" {
		writeError(w, http.StatusBadRequest, "template_yaml and compose_yaml are required")
		return
	}
	m, err := catalog.CheckSubmission([]byte(req.TemplateYAML), []byte(req.ComposeYAML))
	resp := validateTemplateResponse{Valid: err == nil, ID: m.ID, Name: m.Name}
	if err != nil {
		for _, line := range strings.Split(err.Error(), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				resp.Errors = append(resp.Errors, line)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
