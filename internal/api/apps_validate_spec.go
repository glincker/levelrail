package api

import (
	"encoding/json"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// validateSpecBodyLimit matches loadbalancer.go's own maxLBImportBytes:
// both bodies are the same kind of thing, a full app.yaml document.
const validateSpecBodyLimit = 1 << 20

type validateSpecRequest struct {
	YAML string `json:"yaml"`
}

type validateSpecResponse struct {
	Valid    bool         `json:"valid"`
	Issues   []spec.Issue `json:"issues"`
	Services int          `json:"services,omitempty"`
}

// handleValidateSpec checks an app.yaml body with the same rules as deploy time, without deploying or saving.
func (rt *Router) handleValidateSpec(w http.ResponseWriter, r *http.Request) {
	var req validateSpecRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, validateSpecBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	parsed, issues := spec.ValidateDocument([]byte(req.YAML))
	resp := validateSpecResponse{Valid: len(issues) == 0, Issues: issues}
	if resp.Issues == nil {
		resp.Issues = []spec.Issue{}
	}
	if parsed != nil {
		resp.Services = len(parsed.Services)
	}
	writeJSON(w, http.StatusOK, resp)
}
