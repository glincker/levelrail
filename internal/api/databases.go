package api

import (
	"encoding/json"
	"errors"
	"fmt"
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
	// ProjectID: see appResource's own ProjectID field doc comment,
	// identical response-only-except-at-create-time boundary. Set it on
	// an existing database via PUT /api/v1/databases/{name}/project
	// (handleSetDatabaseProject) instead.
	ProjectID string `json:"project_id,omitempty"`
	// BackupTargetID, BackupSchedule, BackupRetain: wave-2 roadmap item 6,
	// scheduled backups. Response-only here, the identical boundary
	// NodeID/ProjectID already establish: set or clear them via
	// PUT/DELETE /api/v1/databases/{name}/backup-schedule (backups.go),
	// never through this resource's own create/update body.
	BackupTargetID   string `json:"backup_target_id,omitempty"`
	BackupSchedule   string `json:"backup_schedule,omitempty"`
	BackupRetain     int    `json:"backup_retain,omitempty"`
	BackupRetainDays int    `json:"backup_retain_days,omitempty"`
	// PubliclyAccessible, PublicPort: response-only, the identical
	// boundary NodeID/ProjectID/BackupTargetID already establish. Set or
	// clear them via PUT/DELETE /api/v1/databases/{name}/public-access
	// (database_public_access.go), never through this resource's own
	// create/update body.
	PubliclyAccessible bool `json:"publicly_accessible,omitempty"`
	PublicPort         int  `json:"public_port,omitempty"`
	// Resources: unlike NodeID/ProjectID/the backup fields above, this is
	// ordinary desired state, the same appResource.Resources field
	// carries for apps, not a response-only/dedicated-route field. There
	// is no PUT /api/v1/databases/{name} to carry it the way
	// handleUpdateApp carries appResource.Resources, though, so it is set
	// through its own dedicated route instead:
	// PUT /api/v1/databases/{name}/resources (handleSetDatabaseResources),
	// mirroring the node/project routes' shape rather than appResource's.
	Resources *store.ServiceResources `json:"resources,omitempty"`
}

func toDatabaseResource(d store.DesiredDatabase) databaseResource {
	return databaseResource{
		Name:               d.Name,
		Engine:             d.Engine,
		Version:            d.Version,
		NodeID:             d.NodeID,
		ProjectID:          d.ProjectID,
		BackupTargetID:     d.BackupTargetID,
		BackupSchedule:     d.BackupSchedule,
		BackupRetain:       d.BackupRetain,
		BackupRetainDays:   d.BackupRetainDays,
		PubliclyAccessible: d.PubliclyAccessible,
		PublicPort:         d.PublicPort,
		Resources:          d.Resources,
	}
}

func (d databaseResource) toDesiredDatabase() store.DesiredDatabase {
	return store.DesiredDatabase{
		Name:    d.Name,
		Engine:  d.Engine,
		Version: d.Version,
	}
}

// databaseListResource is GET /api/v1/databases' own wire shape,
// databaseResource plus a batched status summary, mirroring
// appListResource's exact reasoning (apps.go): DatabaseRow needs enough
// to render a status dot without an N+1 GetConditions call per database.
type databaseListResource struct {
	databaseResource
	Status appStatusSummary `json:"status"`
}

// validateDatabaseResource checks d.Engine against
// store.SupportedDatabaseEngines' embedded registry rather than a
// hardcoded postgres/redis/mysql comparison chain: adding a new engine
// to that registry (internal/store/database_engines.yaml) makes it
// valid here automatically, no second edit needed in this package.
func validateDatabaseResource(d databaseResource) error {
	if d.Name == "" {
		return errors.New("name is required")
	}
	supported, err := store.IsSupportedEngine(d.Engine)
	if err != nil {
		// The embedded registry failing to parse is a build-time
		// invariant covered by internal/store's own tests, not a
		// realistic runtime condition; still checked rather than
		// ignored, since silently treating a load failure as "any
		// engine is valid" would be worse than a slightly imprecise
		// 400 here.
		return fmt.Errorf("check supported database engines: %w", err)
	}
	if !supported {
		return errors.New("unsupported engine")
	}
	if d.Version == "" {
		return errors.New("version is required")
	}
	return nil
}

// handleListDatabases handles GET /api/v1/databases. Status is computed
// from one batched conditions query (store.GetConditionsForControllers),
// the same shape handleListApps uses, not a GetConditions call per
// database.
func (rt *Router) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		rt.logger.Error("api: list databases failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	controllerNames := make([]string, len(dbs))
	for i, d := range dbs {
		controllerNames[i] = databaseControllerName(d.Name)
	}
	conditionsByController, err := rt.deploys.GetConditionsForControllers(r.Context(), controllerNames)
	if err != nil {
		rt.logger.Error("api: list databases: batch load conditions failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]databaseListResource, 0, len(dbs))
	for _, d := range dbs {
		out = append(out, databaseListResource{
			databaseResource: toDatabaseResource(d),
			Status:           summarizeAppConditions(conditionsByController[databaseControllerName(d.Name)]),
		})
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
//
// req.ProjectID is accepted here for the identical reason and through
// the identical mechanism appResource's own ProjectID field doc comment
// and handleCreateApp's own doc comment describe: a brand new database
// has no existing project assignment to clobber, so this handler is
// allowed to call UpdateDatabaseProject as a trailing step after the
// create succeeds, while an ordinary PUT-driven update still can't.
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
	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.logger.Error("api: create database: check project failed", slog.String("error", err.Error()), slog.String("project_id", req.ProjectID))
		writeError(w, http.StatusInternalServerError, "internal error")
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

	if req.ProjectID != "" {
		if err := rt.databases.UpdateDatabaseProject(r.Context(), req.Name, req.ProjectID); err != nil {
			rt.logger.Error("api: create database: assign project failed", slog.String("error", err.Error()), slog.String("name", req.Name), slog.String("project_id", req.ProjectID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
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

// setDatabaseNodeRequest is PUT /api/v1/databases/{name}/node's body,
// identical shape to setAppNodeRequest.
type setDatabaseNodeRequest struct {
	NodeID string `json:"node_id"`
}

// handleSetDatabaseNode handles PUT /api/v1/databases/{name}/node, the
// database counterpart to handleSetAppNode: same unknown-node-id 400,
// same empty-string-means-local-node convention, same AbilityRoot
// gating at the router. UpdateDatabaseNode has existed in the store
// since TASKS.md 3.3 landed; this was the missing route over it.
func (rt *Router) handleSetDatabaseNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.NodeID != "" {
		if _, err := rt.nodes.GetNode(r.Context(), req.NodeID); errors.Is(err, store.ErrNodeNotFound) {
			writeError(w, http.StatusBadRequest, "unknown node_id")
			return
		} else if err != nil {
			rt.logger.Error("api: set database node: look up node failed", slog.String("error", err.Error()), slog.String("node_id", req.NodeID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.databases.UpdateDatabaseNode(r.Context(), name, req.NodeID); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database node failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set database node: reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResource(*d))
}

// setDatabaseProjectRequest is PUT /api/v1/databases/{name}/project's
// body, identical shape to setAppProjectRequest.
type setDatabaseProjectRequest struct {
	ProjectID string `json:"project_id"`
}

// handleSetDatabaseProject handles PUT /api/v1/databases/{name}/project
// (projects.go), the database counterpart to handleSetAppProject: same
// unknown-project-id 400, same empty-string-means-no-project convention.
func (rt *Router) handleSetDatabaseProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.logger.Error("api: set database project: look up project failed", slog.String("error", err.Error()), slog.String("project_id", req.ProjectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.databases.UpdateDatabaseProject(r.Context(), name, req.ProjectID); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database project failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set database project: reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResource(*d))
}

// setDatabaseResourcesRequest is PUT /api/v1/databases/{name}/resources's
// body.
type setDatabaseResourcesRequest struct {
	Resources *store.ServiceResources `json:"resources"`
}

// handleSetDatabaseResources handles PUT /api/v1/databases/{name}/resources.
// databaseResource's own Resources field doc comment explains why this is
// a dedicated route rather than folded into a general PUT
// /databases/{name}: no such route exists for databases, unlike
// handleUpdateApp for apps. SaveDesiredDatabase's own ON CONFLICT clause
// never touches node_id/project_id/the backup columns (only
// engine/version/resources/updated_at), so passing existing straight
// through with only Resources replaced is safe: nothing else on the row
// moves.
func (rt *Router) handleSetDatabaseResources(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseResourcesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	existing, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: set database resources: check existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	desired := *existing
	desired.Resources = req.Resources
	if err := rt.databases.SaveDesiredDatabase(r.Context(), desired); err != nil {
		rt.logger.Error("api: set database resources failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set database resources: reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDatabaseResource(*updated))
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
