package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
)

// customTemplateIDPrefix keeps a custom template's ID disjoint from
// every catalog.Template.ID, a hand-written lowercase slug.
const customTemplateIDPrefix = "custom-"

// saveAppAsTemplateRequest is POST /api/v1/apps/{name}/save-as-template's
// body.
type saveAppAsTemplateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// customTemplateListItem is one entry in GET /api/v1/templates/custom,
// the operator-defined counterpart to serviceTemplateListItem: no
// Compose body, so a "Your templates" grid's initial load stays small
// the same way the built-in catalog's own list endpoint does.
type customTemplateListItem struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	SourceApp             string `json:"source_app,omitempty"`
	RequiresConfiguration bool   `json:"requires_configuration"`
	CreatedAt             string `json:"created_at"`
}

// customTemplateDetail is POST /api/v1/apps/{name}/save-as-template's
// response: the list item plus the derived compose body and which env
// keys ended up required, so the save confirmation dialog has
// something to render without a second round trip.
type customTemplateDetail struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	SourceApp             string   `json:"source_app,omitempty"`
	Compose               string   `json:"compose"`
	RequiresConfiguration bool     `json:"requires_configuration"`
	RequiredEnvKeys       []string `json:"required_env_keys,omitempty"`
	CreatedAt             string   `json:"created_at"`
}

// handleSaveAppAsTemplate handles POST /api/v1/apps/{name}/save-as-template:
// derives a compose.yaml from name's current desired state
// (single-service or every member of a multi-service app, see
// resolveAppServices) and saves it as a store.CustomTemplate. No real
// secret value is ever captured: see compose.FromDesiredServices.
func (rt *Router) handleSaveAppAsTemplate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req saveAppAsTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	services, appID, err := rt.resolveAppServices(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: save app as template: resolve app failed", err, slog.String("name", name))
		return
	}

	appName := name
	if appID != "" {
		appName = appID
	}
	composeBody, err := compose.FromDesiredServices(appName, services)
	if err != nil {
		rt.internalError(w, "api: save app as template: derive compose failed", err, slog.String("name", name))
		return
	}

	id, err := newCustomTemplateID()
	if err != nil {
		rt.internalError(w, "api: save app as template: generate id failed", err, slog.String("name", name))
		return
	}

	_, principalID, _, err := rt.callerPrincipal(r)
	if err != nil {
		principalID = ""
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tpl := store.CustomTemplate{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Compose:     composeBody,
		SourceApp:   name,
		CreatedBy:   principalID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := rt.customTemplates.SaveCustomTemplate(r.Context(), tpl); err != nil {
		rt.internalError(w, "api: save app as template: save failed", err, slog.String("name", name))
		return
	}

	writeJSON(w, http.StatusCreated, customTemplateDetail{
		ID:                    tpl.ID,
		Name:                  tpl.Name,
		Description:           tpl.Description,
		SourceApp:             tpl.SourceApp,
		Compose:               tpl.Compose,
		RequiresConfiguration: composeNeedsConfiguration(tpl.Compose),
		RequiredEnvKeys:       requiredEnvKeys(tpl.Compose),
		CreatedAt:             tpl.CreatedAt,
	})
}

// handleListCustomTemplates handles GET /api/v1/templates/custom: every
// operator-defined template, oldest first, without each one's Compose
// body (matching handleListServiceTemplates' own "list is for browsing,
// get is for the full body" split).
func (rt *Router) handleListCustomTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := rt.customTemplates.ListCustomTemplates(r.Context())
	if err != nil {
		rt.internalError(w, "api: list custom templates failed", err)
		return
	}
	out := make([]customTemplateListItem, 0, len(templates))
	for _, t := range templates {
		out = append(out, customTemplateListItem{
			ID:                    t.ID,
			Name:                  t.Name,
			Description:           t.Description,
			SourceApp:             t.SourceApp,
			RequiresConfiguration: composeNeedsConfiguration(t.Compose),
			CreatedAt:             t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteCustomTemplate handles DELETE /api/v1/templates/custom/{id}.
func (rt *Router) handleDeleteCustomTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.customTemplates.DeleteCustomTemplate(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCustomTemplateNotFound) {
			writeError(w, http.StatusNotFound, "custom template not found")
			return
		}
		rt.internalError(w, "api: delete custom template failed", err, slog.String("id", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveAppServices returns every store.DesiredService behind name
// plus the owning store.App.ID, if any. Mirrors handleGetAppGroup's own
// two-step lookup (apps_group.go): service name first, then app name.
func (rt *Router) resolveAppServices(ctx context.Context, name string) ([]store.DesiredService, string, error) {
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		app, err := rt.appGroups.GetAppByName(ctx, name)
		if errors.Is(err, store.ErrAppNotFound) {
			return nil, "", store.ErrServiceNotFound
		}
		if err != nil {
			return nil, "", fmt.Errorf("get app %q: %w", name, err)
		}
		services, err := rt.appGroups.ListServicesByApp(ctx, app.ID)
		if err != nil {
			return nil, "", fmt.Errorf("list services for app %q: %w", app.ID, err)
		}
		if len(services) == 0 {
			return nil, "", store.ErrServiceNotFound
		}
		return services, app.ID, nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("get service %q: %w", name, err)
	}

	if svc.AppID == "" {
		return []store.DesiredService{*svc}, "", nil
	}
	services, err := rt.appGroups.ListServicesByApp(ctx, svc.AppID)
	if err != nil {
		return nil, "", fmt.Errorf("list services for app %q: %w", svc.AppID, err)
	}
	return services, svc.AppID, nil
}

// requiredEnvKeys lists every unresolved env key composeNeedsConfiguration
// only boils down to a bool, for the save-as-template confirmation dialog.
func requiredEnvKeys(composeBody string) []string {
	f, err := compose.Parse([]byte(composeBody))
	if err != nil {
		return nil
	}
	_, unresolved, err := compose.ResolveMagicVars(f, detectionGenerate, detectionPersist)
	if err != nil {
		return nil
	}
	keys := make([]string, 0, len(unresolved))
	seen := make(map[string]bool, len(unresolved))
	for _, u := range unresolved {
		if seen[u.EnvKey] {
			continue
		}
		seen[u.EnvKey] = true
		keys = append(keys, u.EnvKey)
	}
	return keys
}

// newCustomTemplateID mints a customTemplateIDPrefix-prefixed random ID.
func newCustomTemplateID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate custom template id: %w", err)
	}
	return customTemplateIDPrefix + hex.EncodeToString(buf), nil
}
