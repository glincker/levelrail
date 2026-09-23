package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/integrations"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AppIntegrationStore is the store surface the app integration handlers
// need, mirroring ScheduledTaskStore's own shape for a different
// per-app child resource.
type AppIntegrationStore interface {
	SaveAppIntegration(ctx context.Context, ai store.AppIntegration) error
	GetAppIntegration(ctx context.Context, id string) (store.AppIntegration, error)
	ListAppIntegrationsForService(ctx context.Context, serviceName string) ([]store.AppIntegration, error)
	DeleteAppIntegration(ctx context.Context, id string) error
}

type integrationCatalogEnvVar struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

// integrationCatalogEntry is GET /api/v1/integrations' wire shape: the
// full internal/integrations.Catalog, static and in-memory, no store
// involved, the same "no store at all" shape handleListServiceTemplates
// already establishes for a different static catalog.
type integrationCatalogEntry struct {
	Key         string                     `json:"key"`
	Name        string                     `json:"name"`
	Description string                     `json:"description"`
	DocsURL     string                     `json:"docs_url"`
	EnvVars     []integrationCatalogEnvVar `json:"env_vars"`
	Frameworks  []string                   `json:"frameworks,omitempty"`
}

func toIntegrationCatalogEntry(i integrations.Integration) integrationCatalogEntry {
	envVars := make([]integrationCatalogEnvVar, len(i.EnvVars))
	for idx, v := range i.EnvVars {
		envVars[idx] = integrationCatalogEnvVar{
			Name:        v.Name,
			Type:        v.Type,
			Required:    v.Required,
			Default:     v.Default,
			Placeholder: v.Placeholder,
		}
	}
	return integrationCatalogEntry{
		Key:         i.Key,
		Name:        i.Name,
		Description: i.Description,
		DocsURL:     i.DocsURL,
		EnvVars:     envVars,
		Frameworks:  i.Frameworks,
	}
}

// handleListIntegrationCatalog handles GET /api/v1/integrations.
func (rt *Router) handleListIntegrationCatalog(w http.ResponseWriter, _ *http.Request) {
	out := make([]integrationCatalogEntry, 0, len(integrations.Catalog))
	for _, i := range integrations.Catalog {
		out = append(out, toIntegrationCatalogEntry(i))
	}
	writeJSON(w, http.StatusOK, out)
}

// appIntegrationResource is the wire shape for one app's attached
// integration. Name is the catalog's display name, a convenience for
// the attached-list UI so it doesn't need a second lookup against the
// catalog for every row; it is never a persisted field. Field values
// (DSN, API key) are never included here, matching the "names only,
// never a value" convention handleListSecrets already establishes for
// the same secrets store.
type appIntegrationResource struct {
	ID             string    `json:"id"`
	AppName        string    `json:"app_name,omitempty"`
	IntegrationKey string    `json:"integration_key"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

func toAppIntegrationResource(ai store.AppIntegration, displayName string) appIntegrationResource {
	return appIntegrationResource{
		ID:             ai.ID,
		AppName:        ai.ServiceName,
		IntegrationKey: ai.IntegrationKey,
		Name:           displayName,
		CreatedAt:      ai.CreatedAt,
		UpdatedAt:      ai.UpdatedAt,
	}
}

// integrationDisplayName returns key's catalog display name, or key
// itself if the catalog entry was removed after an app already attached
// it: an attachment must keep listing even if its catalog entry
// disappears, the same "skip, don't fail" resilience resolveIntegrationEnv
// (internal/reconcile/application) already gives this same lookup.
func integrationDisplayName(key string) string {
	if def, ok := integrations.Get(key); ok {
		return def.Name
	}
	return key
}

// handleListAppIntegrations handles GET /api/v1/apps/{name}/integrations.
func (rt *Router) handleListAppIntegrations(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list app integrations: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := rt.appIntegrations.ListAppIntegrationsForService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list app integrations failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]appIntegrationResource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppIntegrationResource(row, integrationDisplayName(row.IntegrationKey)))
	}
	writeJSON(w, http.StatusOK, out)
}

// attachAppIntegrationRequest is handleAttachAppIntegration's request
// body. Fields maps a catalog env var name (e.g. "SENTRY_DSN") to the
// value the operator typed; a field the catalog marks optional with a
// Default may be left out entirely.
type attachAppIntegrationRequest struct {
	IntegrationKey string            `json:"integration_key"`
	Fields         map[string]string `json:"fields"`
}

// handleAttachAppIntegration handles POST
// /api/v1/apps/{name}/integrations: attaches a catalog integration to
// this app, storing its field values through the same envelope
// encryption every other app secret uses
// (store.AppIntegrationSecretsKey), never a second, parallel secret
// store. internal/reconcile/application's resolveIntegrationEnv resolves
// those values into the app's env at its next container start, ahead of
// the app's own literal env (so an operator's own same-named env var
// still wins).
func (rt *Router) handleAttachAppIntegration(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "integrations are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: attach app integration: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req attachAppIntegrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	def, ok := integrations.Get(req.IntegrationKey)
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown integration_key")
		return
	}
	for _, field := range def.EnvVars {
		if field.Required && strings.TrimSpace(req.Fields[field.Name]) == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s is required", field.Name))
			return
		}
	}

	id, err := randomAppIntegrationID()
	if err != nil {
		rt.logger.Error("api: attach app integration: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now().UTC()
	ai := store.AppIntegration{ID: id, ServiceName: name, IntegrationKey: def.Key, CreatedAt: now, UpdatedAt: now}
	if err := rt.appIntegrations.SaveAppIntegration(r.Context(), ai); err != nil {
		if errors.Is(err, store.ErrAppIntegrationAlreadyAttached) {
			writeError(w, http.StatusConflict, "this integration is already attached to this app")
			return
		}
		rt.logger.Error("api: attach app integration failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("integration_key", def.Key))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	namespace := store.AppIntegrationSecretsKey(name, def.Key)
	for _, field := range def.EnvVars {
		value := strings.TrimSpace(req.Fields[field.Name])
		if value == "" {
			// Optional field left blank: resolveIntegrationEnv falls
			// back to the catalog's own Default at resolve time.
			continue
		}
		if err := rt.secrets.SetValueGuarded(r.Context(), namespace, field.Name, value, true); err != nil {
			// Never leave a half-configured row: an integration whose
			// required fields fail to store shouldn't show as attached.
			if delErr := rt.appIntegrations.DeleteAppIntegration(r.Context(), id); delErr != nil {
				rt.logger.Error("api: attach app integration: rollback failed", slog.String("error", delErr.Error()), slog.String("id", id))
			}
			rt.logger.Error("api: attach app integration: store field failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("field", field.Name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusCreated, toAppIntegrationResource(ai, def.Name))
}

// loadOwnedAppIntegration loads the app integration with this id and
// verifies it actually belongs to appName, the same ownership check
// loadOwnedScheduledTask already establishes for a different globally-
// unique-ID child resource.
func (rt *Router) loadOwnedAppIntegration(w http.ResponseWriter, r *http.Request, appName, id, op string) (store.AppIntegration, bool) {
	ai, err := rt.appIntegrations.GetAppIntegration(r.Context(), id)
	if errors.Is(err, store.ErrAppIntegrationNotFound) {
		writeError(w, http.StatusNotFound, "integration attachment not found")
		return store.AppIntegration{}, false
	}
	if err != nil {
		rt.logger.Error("api: "+op+": load app integration failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.AppIntegration{}, false
	}
	if ai.ServiceName != appName {
		writeError(w, http.StatusNotFound, "integration attachment not found")
		return store.AppIntegration{}, false
	}
	return ai, true
}

// handleDetachAppIntegration handles DELETE
// /api/v1/apps/{name}/integrations/{id}: removes the attachment row and
// every field value stored under its namespace in one request, so a
// detached integration's env vars are gone by the app's next container
// start, not left as orphaned secret rows.
func (rt *Router) handleDetachAppIntegration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	ai, ok := rt.loadOwnedAppIntegration(w, r, name, id, "detach app integration")
	if !ok {
		return
	}

	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(r.Context(), store.AppIntegrationSecretsKey(ai.ServiceName, ai.IntegrationKey)); err != nil {
			rt.logger.Error("api: detach app integration: clear field values failed", slog.String("error", err.Error()), slog.String("id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.appIntegrations.DeleteAppIntegration(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrAppIntegrationNotFound) {
			writeError(w, http.StatusNotFound, "integration attachment not found")
			return
		}
		rt.logger.Error("api: detach app integration failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// randomAppIntegrationID mirrors randomScheduledTaskID's exact shape (9
// random bytes, URL-safe base64, a short type prefix). Duplicated rather
// than shared, the same "different resource, different ID space"
// reasoning randomScheduledTaskID's own doc comment gives.
func randomAppIntegrationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate app integration id: %w", err)
	}
	return "appint_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
