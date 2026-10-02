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

// handleValidateSpec handles POST /api/v1/apps/{name}/validate-spec: runs
// the submitted app.yaml body through internal/spec.ValidateDocument,
// the same checks Parse uses at real deploy time, without deploying or
// saving anything. One issue per problem, not a single flat error, so a
// live editor gets earlier feedback; the deploy-time check is unchanged.
// Mirrors handleValidatePipeline's (pipelines.go) existing shape.
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
