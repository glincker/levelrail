package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Blank env_var and field default to a full connection string under the conventional name.
const (
	defaultDatabaseAttachmentEnvVar = "DATABASE_URL"
	defaultDatabaseAttachmentField  = "url"
)

// Carries no name of its own; the map key or request field supplies it.
type appDatabaseEnvRef struct {
	Database string `json:"database"`
	Field    string `json:"field"`
}

// A narrow response shape, not a full appResource echo, matching appStorageResource.
type appDatabaseResource struct {
	AppName      string `json:"app_name,omitempty"`
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var"`
	Field        string `json:"field"`
}

// Blank env_var and field default, so database_name alone is enough.
type setAppDatabaseRequest struct {
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var"`
	Field        string `json:"field"`
}

// Validates the database and field up front, so bad input fails with 400 rather than at reconcile.
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
	field := req.Field
	if field == "" {
		field = defaultDatabaseAttachmentField
	}

	if !rt.authorizeDatabaseRefs(w, r, req.DatabaseName) {
		return
	}
	desiredDB, external, err := rt.lookupAppDatabase(r.Context(), req.DatabaseName)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusBadRequest, "unknown database_name")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app database: look up database failed", err, slog.String("database_name", req.DatabaseName))
		return
	}
	if !databaseFieldSupported(external, desiredDB.Engine, field) {
		writeError(w, http.StatusBadRequest, "field \""+field+"\" is not supported for "+desiredDB.Engine+" databases")
		return
	}

	envVar := req.EnvVar
	if envVar == "" {
		envVar = defaultDatabaseAttachmentEnvVar
		if conv := database.ConventionalEnvVar(desiredDB.Engine, field); conv != "" && !external {
			envVar = conv
		}
	}

	att := &store.DatabaseAttachment{DatabaseName: req.DatabaseName, EnvVar: envVar, Field: field}
	if err := rt.apps.UpdateServiceDatabaseAttachment(r.Context(), name, att); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app database failed", err, slog.String("name", name))
		return
	}

	writeJSON(w, http.StatusOK, appDatabaseResource{AppName: name, DatabaseName: req.DatabaseName, EnvVar: envVar, Field: field})
}

// Clearing affects only freshly created containers; running ones keep the env var until recreated.
func (rt *Router) handleClearAppDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceDatabaseAttachment(r.Context(), name, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: clear app database failed", err, slog.String("name", name))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
