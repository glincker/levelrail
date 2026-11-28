package api

import (
	"net/http"

	"github.com/glincker/levelrail/internal/spec"
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
	req, ok := decodeJSON[validateSpecRequest](w, r, validateSpecBodyLimit)
	if !ok {
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
