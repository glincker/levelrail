package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultDatabaseAttachmentEnvVar and defaultDatabaseAttachmentField are
// what PUT /api/v1/apps/{name}/database fills in when the request body
// leaves env_var/field blank: the common case is "give me a full
// connection string under the conventional name", not an operator
// choosing a specific sub-field every time.
const (
	defaultDatabaseAttachmentEnvVar = "DATABASE_URL"
	defaultDatabaseAttachmentField  = "url"
)

// appDatabaseEnvRef is store.DatabaseEnvRef's wire shape, used both
// inside appResource.DatabaseEnv (a map) and nowhere else: it carries no
// app/env-var name of its own, that's the map key or the request's own
// field.
type appDatabaseEnvRef struct {
	Database string `json:"database"`
	Field    string `json:"field"`
}

// appDatabaseResource is PUT/DELETE /api/v1/apps/{name}/database's
// response body and appResource.DatabaseAttachment's wire shape:
// deliberately its own small type, not a reuse of appResource, the same
// "narrow response, not a full resource echo" shape appStorageResource
// already establishes for the equivalent storage endpoint.
type appDatabaseResource struct {
	AppName      string `json:"app_name,omitempty"`
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var"`
	Field        string `json:"field"`
}

// setAppDatabaseRequest is handleSetAppDatabase's request body. EnvVar
// and Field both default when left blank (defaultDatabaseAttachmentEnvVar/
// defaultDatabaseAttachmentField), so the common case ("attach this
// database, give me DATABASE_URL") needs only database_name.
type setAppDatabaseRequest struct {
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var"`
	Field        string `json:"field"`
}

// handleSetAppDatabase handles PUT /api/v1/apps/{name}/database: attaches
// an already-created managed database to this app as a real, persisted
// connection-env-var source, the UI/CLI-facing equivalent of app.yaml's
// own { from: "<database>.<field>" } env var syntax (internal/spec.
// EnvVar.From, resolved by internal/deploy's validateEnv and
// internal/reconcile/application's resolveDatabaseField) for an app that
// was created directly rather than deployed from a spec file.
//
// The database must already exist, and field (once defaulted) must be
// one database.SupportsField's engine-aware rules allow for it: both
// fail loudly here with a 400 rather than silently being written and
// only surfacing as a reconcile failure later, the same "validate
// against the real registry first" shape handleSetAppStorage already
// establishes for storage_target_id.
func (rt *Router) handleSetAppDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DatabaseName == "" {
		writeError(w, http.StatusBadRequest, "database_name is required")
		return
	}
	envVar := req.EnvVar
	if envVar == "" {
		envVar = defaultDatabaseAttachmentEnvVar
	}
	field := req.Field
	if field == "" {
		field = defaultDatabaseAttachmentField
	}

	desiredDB, err := rt.databases.GetDesiredDatabase(r.Context(), req.DatabaseName)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusBadRequest, "unknown database_name")
		return
	} else if err != nil {
		rt.logger.Error("api: set app database: look up database failed", slog.String("error", err.Error()), slog.String("database_name", req.DatabaseName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !database.SupportsField(desiredDB.Engine, field) {
		writeError(w, http.StatusBadRequest, "field \""+field+"\" is not supported for "+desiredDB.Engine+" databases")
		return
	}

	att := &store.DatabaseAttachment{DatabaseName: req.DatabaseName, EnvVar: envVar, Field: field}
	if err := rt.apps.UpdateServiceDatabaseAttachment(r.Context(), name, att); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, appDatabaseResource{AppName: name, DatabaseName: req.DatabaseName, EnvVar: envVar, Field: field})
}

// handleClearAppDatabase handles DELETE /api/v1/apps/{name}/database: the
// reverse of handleSetAppDatabase, detaching whatever database this app
// currently resolves its attachment env var from. The next reconcile
// pass sees the attachment go back to nil and stops injecting that env
// var into freshly (re)created containers, the same "desired state
// changes, running containers converge on their own schedule" behavior
// handleClearAppStorage already documents.
func (rt *Router) handleClearAppDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceDatabaseAttachment(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear app database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
