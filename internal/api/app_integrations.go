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

// integrationCatalogEntry is the wire shape for one entry of the static integration catalog.
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

// Name is display-only and never persisted; field values are never returned, only names.
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

// Falls back to the raw key when the catalog entry was removed, so an attachment still lists.
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
		rt.internalError(w, "api: list app integrations: load app failed", err, slog.String("name", name))
		return
	}

	rows, err := rt.appIntegrations.ListAppIntegrationsForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list app integrations failed", err, slog.String("name", name))
		return
	}

	out := make([]appIntegrationResource, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAppIntegrationResource(row, integrationDisplayName(row.IntegrationKey)))
	}
	writeJSON(w, http.StatusOK, out)
}

// Fields maps catalog env var names to operator-entered values.
type attachAppIntegrationRequest struct {
	IntegrationKey string            `json:"integration_key"`
	Fields         map[string]string `json:"fields"`
}

// Values are stored with the same envelope encryption as app secrets; an app's own literal env var still wins over an integration's.
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
		rt.internalError(w, "api: attach app integration: load app failed", err, slog.String("name", name))
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
		rt.internalError(w, "api: attach app integration: generate id failed", err)
		return
	}

	now := time.Now().UTC()
	ai := store.AppIntegration{ID: id, ServiceName: name, IntegrationKey: def.Key, CreatedAt: now, UpdatedAt: now}
	if err := rt.appIntegrations.SaveAppIntegration(r.Context(), ai); err != nil {
		if errors.Is(err, store.ErrAppIntegrationAlreadyAttached) {
			writeError(w, http.StatusConflict, "this integration is already attached to this app")
			return
		}
		rt.internalError(w, "api: attach app integration failed", err, slog.String("name", name), slog.String("integration_key", def.Key))
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
			rt.internalError(w, "api: attach app integration: store field failed", err, slog.String("name", name), slog.String("field", field.Name))
			return
		}
	}

	writeJSON(w, http.StatusCreated, toAppIntegrationResource(ai, def.Name))
}

// loadOwnedAppIntegration checks that the integration actually belongs to appName.
func (rt *Router) loadOwnedAppIntegration(w http.ResponseWriter, r *http.Request, appName, id, op string) (store.AppIntegration, bool) {
	ai, err := rt.appIntegrations.GetAppIntegration(r.Context(), id)
	if errors.Is(err, store.ErrAppIntegrationNotFound) {
		writeError(w, http.StatusNotFound, "integration attachment not found")
		return store.AppIntegration{}, false
	}
	if err != nil {
		rt.internalError(w, "api: "+op+": load app integration failed", err, slog.String("id", id))
		return store.AppIntegration{}, false
	}
	if ai.ServiceName != appName {
		writeError(w, http.StatusNotFound, "integration attachment not found")
		return store.AppIntegration{}, false
	}
	return ai, true
}

// Detach removes the attachment and its stored field values together, so no orphaned secrets remain.
func (rt *Router) handleDetachAppIntegration(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	ai, ok := rt.loadOwnedAppIntegration(w, r, name, id, "detach app integration")
	if !ok {
		return
	}

	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(r.Context(), store.AppIntegrationSecretsKey(ai.ServiceName, ai.IntegrationKey)); err != nil {
			rt.internalError(w, "api: detach app integration: clear field values failed", err, slog.String("id", id))
			return
		}
	}

	if err := rt.appIntegrations.DeleteAppIntegration(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrAppIntegrationNotFound) {
			writeError(w, http.StatusNotFound, "integration attachment not found")
			return
		}
		rt.internalError(w, "api: detach app integration failed", err, slog.String("id", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Duplicated from randomScheduledTaskID on purpose: each resource has its own ID space.
func randomAppIntegrationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate app integration id: %w", err)
	}
	return "appint_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
