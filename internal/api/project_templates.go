package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/projecttpl"
)

type renderProjectTemplateRequest struct {
	Params map[string]string `json:"params"`
}

type renderProjectTemplateResponse struct {
	Files []projecttpl.File `json:"files"`
}

type projectTemplateFieldErrors struct {
	Error  string                  `json:"error"`
	Fields []projecttpl.FieldError `json:"fields"`
}

// handleListProjectTemplates handles GET /api/v1/project-templates.
func (rt *Router) handleListProjectTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]projecttpl.Info{"templates": projecttpl.List()})
}

// handleRenderProjectTemplate handles POST /api/v1/project-templates/{id}/render:
// validates the parameters and returns the starter files without storing anything.
func (rt *Router) handleRenderProjectTemplate(w http.ResponseWriter, r *http.Request) {
	var req renderProjectTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	files, err := projecttpl.Render(r.PathValue("id"), req.Params)
	var verr *projecttpl.ValidationError
	switch {
	case errors.Is(err, projecttpl.ErrUnknownTemplate):
		writeError(w, http.StatusNotFound, "template not found")
	case errors.As(err, &verr):
		writeJSON(w, http.StatusUnprocessableEntity, projectTemplateFieldErrors{Error: verr.Error(), Fields: verr.Fields})
	case err != nil:
		rt.internalError(w, "api: render project template failed", err)
	default:
		writeJSON(w, http.StatusOK, renderProjectTemplateResponse{Files: files})
	}
}
