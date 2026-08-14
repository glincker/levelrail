package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseControllerName mirrors
// internal/reconcile/database.Controller.Name()'s "database/" + dbName
// convention, the same duplicate-the-string-format tradeoff
// applicationControllerName's own doc comment explains for apps.
func databaseControllerName(dbName string) string {
	return "database/" + dbName
}

// databaseResource is the wire shape for a managed database:
// store.DesiredDatabase plus its name, the same direct-marshal choice
// appResource makes for services. NodeID is response-only, the same
// "shown but not settable through this endpoint" boundary appResource's
// own NodeID field documents; there is no PUT /databases/{name}/node yet
// (TASKS-v2's Phase 5 note), UpdateDatabaseNode exists in the store but
// has no route.
type databaseResource struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
	NodeID  string `json:"node_id,omitempty"`
}

func toDatabaseResource(d store.DesiredDatabase) databaseResource {
	return databaseResource{
		Name:    d.Name,
		Engine:  d.Engine,
		Version: d.Version,
		NodeID:  d.NodeID,
	}
}

func (d databaseResource) toDesiredDatabase() store.DesiredDatabase {
	return store.DesiredDatabase{
		Name:    d.Name,
		Engine:  d.Engine,
		Version: d.Version,
	}
}

func validateDatabaseResource(d databaseResource) error {
	if d.Name == "" {
		return errors.New("name is required")
	}
	if d.Engine != store.EnginePostgres && d.Engine != store.EngineRedis {
		return errors.New("engine must be postgres or redis")
	}
	if d.Version == "" {
		return errors.New("version is required")
	}
	return nil
}

// handleListDatabases handles GET /api/v1/databases.
func (rt *Router) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		rt.logger.Error("api: list databases failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]databaseResource, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, toDatabaseResource(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateDatabase handles POST /api/v1/databases. Rejects a name
// that already exists, the same conflict-not-overwrite convention
// handleCreateApp establishes.
//
// Creating a postgres database here always succeeds at the store layer:
// the reconciler (internal/reconcile/database) will refuse to actually
// start it until credentials exist (TASKS.md 1.7, envelope-encrypted
// secrets, not built yet), and reports that refusal as a real condition
// rather than this endpoint pretending postgres isn't an option. Read it
// back via GET /api/v1/databases/{name}/status.
func (rt *Router) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	var req databaseResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateDatabaseResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := rt.databases.GetDesiredDatabase(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "a database with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.logger.Error("api: create database: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.databases.SaveDesiredDatabase(r.Context(), req.toDesiredDatabase()); err != nil {
		rt.logger.Error("api: create database failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

// handleGetDatabase handles GET /api/v1/databases/{name}.
func (rt *Router) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResource(*d))
}

// handleDeleteDatabase handles DELETE /api/v1/databases/{name}. Same
// known gap as handleDeleteApp: removes desired state, does not itself
// stop or remove a running container.
func (rt *Router) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	err := rt.databases.DeleteDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDatabaseStatus handles GET /api/v1/databases/{name}/status: the
// database analogue of handleDeployHistory, surfacing the database
// controller's stored reconcile conditions (current status, not a
// history log, same GetConditions semantics). This is the only place a
// caller can see that a postgres database exists but isn't actually
// running yet, see handleCreateDatabase's doc comment.
func (rt *Router) handleDatabaseStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: database status: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	conditions, err := rt.deploys.GetConditions(r.Context(), databaseControllerName(name))
	if err != nil {
		rt.logger.Error("api: database status failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, conditions)
}
