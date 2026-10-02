package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// serviceTemplateListItem is one entry in GET /api/v1/service-templates:
// deliberately without Compose, so the picker grid's initial load stays
// small; the full body is fetched per-template on selection.
type serviceTemplateListItem struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Slogan                 string `json:"slogan"`
	Category               string `json:"category"`
	DocumentationURL       string `json:"documentation_url"`
	RecommendedMemoryBytes int64  `json:"recommended_memory_bytes,omitempty"`
	RequiresGPU            bool   `json:"requires_gpu,omitempty"`
	// RequiresConfiguration reports whether this template's Compose body
	// has an env value compose.ResolveMagicVars can't resolve on its own
	// (no bash-style default, not an auto-generatable SERVICE_ kind): a
	// real secret the operator has to supply. The one-click deploy path
	// (POST .../deploy below) is only safe when this is false; the UI
	// falls back to the full pre-filled wizard step otherwise.
	RequiresConfiguration bool `json:"requires_configuration"`
}

// serviceTemplateDetail is GET /api/v1/service-templates/{id}'s response
// shape: the list item's fields plus the full compose.yaml body, ready
// to pre-fill a deploy form or send straight to
// POST /api/v1/apps/{name}/compose.
type serviceTemplateDetail struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Slogan                 string `json:"slogan"`
	Category               string `json:"category"`
	DocumentationURL       string `json:"documentation_url"`
	Compose                string `json:"compose"`
	RecommendedMemoryBytes int64  `json:"recommended_memory_bytes,omitempty"`
	RequiresGPU            bool   `json:"requires_gpu,omitempty"`
	RequiresConfiguration  bool   `json:"requires_configuration"`
}

// handleListServiceTemplates handles GET /api/v1/service-templates: the
// full catalog.Templates catalog, static and in-memory, no store
// involved.
func (rt *Router) handleListServiceTemplates(w http.ResponseWriter, _ *http.Request) {
	out := make([]serviceTemplateListItem, 0, len(catalog.Templates))
	for _, tpl := range catalog.Templates {
		out = append(out, serviceTemplateListItem{
			ID:                     tpl.ID,
			Name:                   tpl.Name,
			Slogan:                 tpl.Slogan,
			Category:               tpl.Category,
			DocumentationURL:       tpl.DocumentationURL,
			RecommendedMemoryBytes: tpl.RecommendedMemoryBytes,
			RequiresGPU:            tpl.RequiresGPU,
			RequiresConfiguration:  rt.templateRequiresConfig[tpl.ID],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetServiceTemplate handles GET /api/v1/service-templates/{id}:
// one catalog.Templates entry, including its full Compose body.
func (rt *Router) handleGetServiceTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tpl, ok := catalog.TemplateByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "service template not found")
		return
	}
	writeJSON(w, http.StatusOK, serviceTemplateDetail{
		ID:                     tpl.ID,
		Name:                   tpl.Name,
		Slogan:                 tpl.Slogan,
		Category:               tpl.Category,
		DocumentationURL:       tpl.DocumentationURL,
		Compose:                tpl.Compose,
		RecommendedMemoryBytes: tpl.RecommendedMemoryBytes,
		RequiresGPU:            tpl.RequiresGPU,
		RequiresConfiguration:  rt.templateRequiresConfig[tpl.ID],
	})
}

// handleDeployServiceTemplateNow handles
// POST /api/v1/service-templates/{id}/deploy: the one-click fast path
// for a template that doesn't need any operator-supplied configuration
// first (Router.templateRequiresConfig below). It deploys the
// template's own Compose body under a freshly generated app name and
// reuses deployComposeBody, the same create-app-from-template core
// POST /api/v1/apps/{name}/compose already uses (apps_compose.go), so
// this is not a second deploy pathway.
//
// A template that does need configuration (a required secret with no
// default) deliberately has no one-click path: this returns 409 rather
// than deploying with an empty or fake secret, and the frontend falls
// back to BrowseTemplatesFields' existing pre-filled wizard step.
func (rt *Router) handleDeployServiceTemplateNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tpl, ok := catalog.TemplateByID(id)
	if !ok {
		writeError(w, http.StatusNotFound, "service template not found")
		return
	}
	if rt.templateRequiresConfig[tpl.ID] {
		writeError(w, http.StatusConflict, "this template needs configuration (a secret with no default) before it can deploy; use the full setup flow instead")
		return
	}

	name, err := rt.generateOneClickAppName(r.Context(), tpl.ID)
	if err != nil {
		rt.internalError(w, "api: deploy service template now: generate app name failed", err, slog.String("template", tpl.ID))
		return
	}

	// isTrial: true, the one-click path's whole point is an obviously-
	// temporary instance the operator tears down from the app detail
	// page's trial banner, not a deploy meant to stick around.
	resp, ok := rt.deployComposeBody(w, r, name, []byte(tpl.Compose), true)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// oneClickNameMaxAttempts bounds generateOneClickAppName's collision
// retry loop: a random 5-char lowercase-alphanumeric suffix over a
// template ID that's already unique gives an astronomically small
// collision chance, so a handful of attempts is generous, not tight.
const oneClickNameMaxAttempts = 5

// generateOneClickAppName builds a slugified, collision-checked app
// name for a one-click template deploy: the template's own ID (already
// a lowercase-hyphenated slug, see catalog's own ID convention) plus a
// short random suffix, so repeat one-click deploys of the same template
// land as separate apps instead of colliding (SaveApp upserts by ID,
// which would silently overwrite an existing app of the same name).
func (rt *Router) generateOneClickAppName(ctx context.Context, templateID string) (string, error) {
	base := templateID
	if len(base) > 50 {
		base = base[:50]
	}
	for i := 0; i < oneClickNameMaxAttempts; i++ {
		suffix, err := compose.GenerateValue("USER", 5)
		if err != nil {
			return "", fmt.Errorf("generate app name suffix: %w", err)
		}
		candidate := base + "-" + suffix
		if _, err := rt.appCompose.GetAppByName(ctx, candidate); err != nil {
			if errors.Is(err, store.ErrAppNotFound) {
				return candidate, nil
			}
			return "", fmt.Errorf("check app name %q availability: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("could not generate a unique app name for template %q after %d attempts", templateID, oneClickNameMaxAttempts)
}

// computeTemplateRequiresConfig builds Router.templateRequiresConfig:
// one bool per catalog.Templates entry, keyed by ID, reporting whether
// that template's Compose body has any SERVICE_ magic var
// compose.ResolveMagicVars can't resolve on its own. That's the exact
// same signal handleDeployCompose already turns into a hard "unresolved
// template variable(s)" error at real deploy time, computed once here
// (catalog.Templates is static for the process lifetime, so NewRouter
// calling this once is instance state, not a package-level cache) with
// a detection-only generate/persist pair that never touches real secret
// storage.
func computeTemplateRequiresConfig() map[string]bool {
	out := make(map[string]bool, len(catalog.Templates))
	for _, tpl := range catalog.Templates {
		out[tpl.ID] = templateNeedsConfiguration(tpl)
	}
	return out
}

func templateNeedsConfiguration(tpl catalog.Template) bool {
	f, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		return true
	}
	_, unresolved, err := compose.ResolveMagicVars(f, detectionGenerate, detectionPersist)
	if err != nil {
		return true
	}
	return len(unresolved) > 0
}

// detectionGenerate/detectionPersist stand in for the real
// generate/persist callbacks handleDeployCompose passes to
// compose.ResolveMagicVars: templateNeedsConfiguration only needs to
// know whether resolution *would* succeed, never a real generated value
// or a real secret write.
func detectionGenerate(_, _ string, _ int) (string, error) { return "x", nil }
func detectionPersist(_, _, _ string) error                { return nil }
