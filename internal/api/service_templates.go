package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

// serviceTemplateListItem is one entry in GET /api/v1/service-templates:
// deliberately without Compose, so the picker grid's initial load stays
// small; the full body is fetched per-template on selection.
type serviceTemplateListItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slogan           string `json:"slogan"`
	Category         string `json:"category"`
	DocumentationURL string `json:"documentation_url"`
}

// serviceTemplateDetail is GET /api/v1/service-templates/{id}'s response
// shape: the list item's fields plus the full compose.yaml body, ready
// to pre-fill a deploy form or send straight to
// POST /api/v1/apps/{name}/compose.
type serviceTemplateDetail struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slogan           string `json:"slogan"`
	Category         string `json:"category"`
	DocumentationURL string `json:"documentation_url"`
	Compose          string `json:"compose"`
}

// handleListServiceTemplates handles GET /api/v1/service-templates: the
// full catalog.Templates catalog, static and in-memory, no store
// involved.
func (rt *Router) handleListServiceTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]serviceTemplateListItem, 0, len(catalog.Templates))
	for _, tpl := range catalog.Templates {
		out = append(out, serviceTemplateListItem{
			ID:               tpl.ID,
			Name:             tpl.Name,
			Slogan:           tpl.Slogan,
			Category:         tpl.Category,
			DocumentationURL: tpl.DocumentationURL,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetServiceTemplate handles GET /api/v1/service-templates/{id}:
// one catalog.Templates entry, including its full Compose body.
func (rt *Router) handleGetServiceTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, tpl := range catalog.Templates {
		if tpl.ID == id {
			writeJSON(w, http.StatusOK, serviceTemplateDetail{
				ID:               tpl.ID,
				Name:             tpl.Name,
				Slogan:           tpl.Slogan,
				Category:         tpl.Category,
				DocumentationURL: tpl.DocumentationURL,
				Compose:          tpl.Compose,
			})
			return
		}
	}
	writeError(w, http.StatusNotFound, "service template not found")
}
