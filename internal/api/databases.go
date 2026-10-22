package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseControllerName duplicates the controller's name format, so the two must stay in sync.
func databaseControllerName(dbName string) string {
	return "database/" + dbName
}

// databaseResource NodeID is response-only on update; placement changes go through PUT .../node.
type databaseResource struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
	NodeID  string `json:"node_id,omitempty"`
	// NodeName and NodeIsLocal are response-only display fields, see
	// appResource.
	NodeName    string `json:"node_name,omitempty"`
	NodeIsLocal bool   `json:"node_is_local,omitempty"`
	// AutoPlaced is response-only and true only when create picked a non-local node.
	AutoPlaced bool `json:"auto_placed,omitempty"`
	// ProjectID is response-only except at create; change it via PUT .../project.
	ProjectID string `json:"project_id,omitempty"`
	// Backup fields are response-only; set them via PUT and DELETE .../backup-schedule.
	BackupTargetID   string `json:"backup_target_id,omitempty"`
	BackupSchedule   string `json:"backup_schedule,omitempty"`
	BackupRetain     int    `json:"backup_retain,omitempty"`
	BackupRetainDays int    `json:"backup_retain_days,omitempty"`
	// Public access fields are response-only; set them via PUT and DELETE .../public-access.
	PubliclyAccessible bool   `json:"publicly_accessible,omitempty"`
	PublicPort         int    `json:"public_port,omitempty"`
	PublicBindAddress  string `json:"public_bind_address,omitempty"`
	// Resources is set through PUT .../resources, since there is no general update route for databases.
	Resources *store.ServiceResources `json:"resources,omitempty"`
	// ExistingVolume is request-only: "reuse" or "discard" what a deleted
	// database of the same name left behind (see database_leftover_volume.go).
	// It is stored with the record and applied by the reconciler.
	ExistingVolume string `json:"existing_volume,omitempty"`
	// ProvisionState is response-only: "pending" or "failed" while the
	// database has no running container yet, empty once it is created.
	ProvisionState  string `json:"provision_state,omitempty"`
	ProvisionReason string `json:"provision_reason,omitempty"`
	// ResourcesAppliedLive: see appResource's identically-named field
	// doc comment. Only ever set by handleSetDatabaseResources; every
	// other handler returning a databaseResource leaves it false.
	ResourcesAppliedLive bool `json:"resources_applied_live,omitempty"`
	// Suspended is response-only; set it via POST .../stop and .../start.
	Suspended bool `json:"suspended,omitempty"`
	// TLSEnabled is computed on read and not settable, since TLS activates automatically at creation.
	TLSEnabled bool `json:"tls_enabled,omitempty"`
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
		PublicBindAddress:  d.PublicBindAddress,
		Resources:          d.Resources,
		Suspended:          d.Suspended,
	}
}

// toDatabaseResourceWithStatus adds TLSEnabled, which needs a secrets lookup the pure converter cannot do.
func (rt *Router) toDatabaseResourceWithStatus(ctx context.Context, d store.DesiredDatabase) databaseResource {
	res := toDatabaseResource(d)
	rt.placementResolver(ctx).stampDatabase(&res)
	res.TLSEnabled = rt.databaseTLSEnabled(ctx, d)
	rt.stampProvisionState(ctx, &res)
	return res
}

// A lookup error reports TLS as not enabled rather than failing the request, since this is display-only.
func (rt *Router) databaseTLSEnabled(ctx context.Context, d store.DesiredDatabase) bool {
	if rt.secrets == nil || !database.ServerTLSEngine(d.Engine) {
		return false
	}
	exists, err := rt.secrets.Exists(ctx, d.Name, database.TLSCertEnvKey)
	if err != nil {
		return false
	}
	return exists
}

// databaseTLSStatuses is databaseTLSEnabled batched over every
// TLS-capable database in dbs, via one ExistsForServices call instead of
// an Exists call per database (the N+1 handleListDatabases used to make).
func (rt *Router) databaseTLSStatuses(ctx context.Context, dbs []store.DesiredDatabase) map[string]bool {
	if rt.secrets == nil {
		return nil
	}
	names := make([]string, 0, len(dbs))
	for _, d := range dbs {
		if database.ServerTLSEngine(d.Engine) {
			names = append(names, d.Name)
		}
	}
	statuses, err := rt.secrets.ExistsForServices(ctx, names, database.TLSCertEnvKey)
	if err != nil {
		return nil
	}
	return statuses
}

func (d databaseResource) toDesiredDatabase() store.DesiredDatabase {
	return store.DesiredDatabase{
		Name:    d.Name,
		Engine:  d.Engine,
		Version: d.Version,
		// Size preset and leftover-volume choice are desired state: stored now, applied by the reconciler.
		Resources:      d.Resources,
		ExistingVolume: d.ExistingVolume,
	}
}

// databaseListResource batches status so the list needs no per-database conditions call.
type databaseListResource struct {
	databaseResource
	Status   appStatusSummary          `json:"status"`
	External *externalDatabaseResource `json:"external,omitempty"`
}

// Engines are checked against the embedded registry, so adding an engine needs no change here.
func validateDatabaseResource(d databaseResource) error {
	if d.Name == "" {
		return errors.New("name is required")
	}
	supported, err := store.IsSupportedEngine(d.Engine)
	if err != nil {
		// A registry parse failure is a build-time invariant, so it is an error rather than treating every engine as valid.
		return fmt.Errorf("check supported database engines: %w", err)
	}
	if !supported {
		return errors.New("unsupported engine")
	}
	if d.Version == "" {
		return errors.New("version is required")
	}
	return database.ValidateEngineVersion(d.Engine, d.Version)
}

// handleListDatabases batches status and TLS lookups, avoiding a query per database.
func (rt *Router) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	dbs, err := rt.databases.ListDesiredDatabases(r.Context())
	if err != nil {
		rt.logger.Error("api: list databases failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	canSee, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list databases: visibility", err)
		return
	}
	visibleDBs := dbs[:0:0]
	for _, d := range dbs {
		if canSee(d.Name) {
			visibleDBs = append(visibleDBs, d)
		}
	}
	dbs = visibleDBs
	if dbs, err = rt.filterDatabasesByEnvironment(r, dbs); err != nil {
		rt.internalError(w, "api: list databases: environment filter", err)
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

	tlsStatuses := rt.databaseTLSStatuses(r.Context(), dbs)

	placement := rt.placementResolver(r.Context())
	pending := rt.pendingDatabases(r.Context())
	out := make([]databaseListResource, 0, len(dbs))
	for _, d := range dbs {
		res := toDatabaseResource(d)
		placement.stampDatabase(&res)
		res.TLSEnabled = tlsStatuses[d.Name]
		if pending[d.Name] {
			rt.stampProvisionState(r.Context(), &res)
		}
		out = append(out, databaseListResource{
			databaseResource: res,
			Status:           summarizeAppConditions(conditionsByController[databaseControllerName(d.Name)]),
		})
	}
	out = rt.appendExternalDatabases(r, canSee, out)
	writeJSON(w, http.StatusOK, pageQueryDefault(w, r, out, appListMaxLimit))
}

// createDesiredDatabase is shared with restore-as-new so both creates take the same conflict and store path.
// A postgres database is created even before credentials exist; the reconciler reports that as a condition.
func (rt *Router) createDesiredDatabase(w http.ResponseWriter, r *http.Request, req databaseResource) bool {
	if err := validateDatabaseResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if choice, ok := validExistingVolume(req.ExistingVolume); ok {
		req.ExistingVolume = choice
	} else {
		writeError(w, http.StatusBadRequest, "existing_volume must be \"reuse\" or \"discard\"")
		return false
	}
	if !rt.validateCreateBackup(w, r, req) {
		return false
	}
	req.ProjectID = defaultProjectFor(r, req.ProjectID)
	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return false
		}
		rt.logger.Error("api: create database: check project failed", slog.String("error", err.Error()), slog.String("project_id", req.ProjectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}

	_, err := rt.databases.GetDesiredDatabase(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "a database with this name already exists")
		return false
	}
	if !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.logger.Error("api: create database: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}

	dbReserve := serviceReservation(req.Resources, 1)
	quotaRelease, quotaOK := rt.admitQuotaOrWrite(w, r, req.ProjectID, store.ResourceOwner{},
		quota.Amounts{Databases: 1, MemoryBytes: dbReserve.MemoryBytes, NanoCPUs: dbReserve.NanoCPUs})
	if !quotaOK {
		return false
	}
	defer quotaRelease()

	if err := rt.databases.SaveDesiredDatabase(r.Context(), req.toDesiredDatabase()); err != nil {
		rt.logger.Error("api: create database failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return false
	}
	req.ExistingVolume = ""
	req.ProvisionState = provisionStatePending
	rt.enableTLSAtCreate(r.Context(), req.Name, req.Engine)

	if req.BackupSchedule != "" {
		if err := rt.databases.SetDatabaseBackupSchedule(r.Context(), req.Name, req.BackupTargetID, req.BackupSchedule, req.BackupRetain, req.BackupRetainDays); err != nil {
			rt.internalError(w, "api: create database: store backup schedule failed", err, slog.String("name", req.Name))
			return false
		}
	}

	if req.ProjectID != "" {
		if err := rt.databases.UpdateDatabaseProject(r.Context(), req.Name, req.ProjectID); err != nil {
			rt.logger.Error("api: create database: assign project failed", slog.String("error", err.Error()), slog.String("name", req.Name), slog.String("project_id", req.ProjectID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}

	// SaveDesiredDatabase never writes node_id, so placement needs this trailing update.
	if req.NodeID != "" {
		if err := rt.databases.UpdateDatabaseNode(r.Context(), req.Name, req.NodeID); err != nil {
			rt.logger.Error("api: create database: assign node failed", slog.String("error", err.Error()), slog.String("name", req.Name), slog.String("node_id", req.NodeID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}
	rt.beginProvision(r.Context(), req.Name)
	return true
}

// handleCreateDatabase rejects an existing name and treats an explicit node_id as a placement override.
func (rt *Router) handleCreateDatabase(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req databaseResource
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Databases carry no domains, so placement never prefers the local
	// node for ingress reasons here.
	cp := &createPlacement{Kind: store.NodeMoveKindDatabase, Name: req.Name}
	if !rt.resolveCreateNodePlacement(w, r, body, &req.NodeID, &req.AutoPlaced, false, cp, "api: create database") {
		return
	}

	if !rt.createDesiredDatabase(w, r, req) {
		return
	}
	rt.recordCreatePlacement(r.Context(), cp, req.NodeID)
	rt.nudgeReconciler()
	writeJSON(w, http.StatusCreated, req)
}

// handleGetDatabase handles GET /api/v1/databases/{name}.
func (rt *Router) handleGetDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		if ext, eerr := rt.externalDatabases.GetExternalDatabase(r.Context(), name); eerr == nil {
			writeJSON(w, http.StatusOK, databaseListResource{
				databaseResource: databaseResource{Name: ext.Name, Engine: ext.Engine, NodeID: ext.NodeID, ProjectID: ext.ProjectID},
				Status:           externalStatusSummary(ext.HealthStatus),
				External:         ptrTo(rt.toExternalDatabaseResource(r.Context(), *ext)),
			})
			return
		}
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toDatabaseResourceWithStatus(r.Context(), *d))
}

// The data volume is kept on delete, so a mistaken delete never destroys data.
func (rt *Router) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	existing, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete database: load failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if r.URL.Query().Get("force") != "true" {
		users, err := rt.appsUsingDatabase(r.Context(), name)
		if err != nil {
			rt.logger.Error("api: delete database: list dependent apps failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if len(users) > 0 {
			writeError(w, http.StatusConflict, "database is used by apps ("+strings.Join(users, ", ")+"); detach them or retry with force=true")
			return
		}
	}

	purge, ok := rt.requireDeleteDataConfirm(w, r, name, existing.NodeID)
	if !ok {
		return
	}

	if rt.execRuntime != nil {
		if err := rt.recordDatabaseTeardown(r.Context(), name, existing.NodeID, purge); err != nil {
			rt.internalError(w, "api: delete database: record teardown failed", err, slog.String("name", name))
			return
		}
	}

	err = rt.databases.DeleteDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.nudgeReconciler()
	if terr := rt.runDatabaseTeardown(r.Context(), name, existing.NodeID, purge); terr != nil {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":  "teardown_pending",
			"message": "database removed from desired state, but its container could not be stopped yet and will be retried: " + terr.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appsUsingDatabase returns the sorted names of apps whose env connections
// or attachment reference databaseName.
func (rt *Router) appsUsingDatabase(ctx context.Context, databaseName string) ([]string, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	var users []string
	for _, svc := range services {
		if serviceUsesDatabase(svc, databaseName) {
			users = append(users, svc.Name)
		}
	}
	sort.Strings(users)
	return users, nil
}

func serviceUsesDatabase(svc store.DesiredService, databaseName string) bool {
	if svc.DatabaseAttachment != nil && svc.DatabaseAttachment.DatabaseName == databaseName {
		return true
	}
	for _, ref := range svc.DatabaseEnv {
		if ref.Database == databaseName {
			return true
		}
	}
	return false
}

// reloadAndWriteDatabase reloads name's desired database and writes it as
// the response, the common tail every database-mutation handler in this
// file needs. logContext names the calling handler for the error log line.
func (rt *Router) reloadAndWriteDatabase(w http.ResponseWriter, r *http.Request, name, logContext string) {
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: "+logContext+": reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.toDatabaseResourceWithStatus(r.Context(), *d))
}

// setDatabaseNodeRequest is PUT /api/v1/databases/{name}/node's body,
// identical shape to setAppNodeRequest.
type setDatabaseNodeRequest struct {
	NodeID string `json:"node_id"`
}

// handleSetDatabaseNode is the database counterpart of handleSetAppNode.
func (rt *Router) handleSetDatabaseNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseNodeRequest
	if !decodeReq(w, r, &req) {
		return
	}

	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown node_id"), nodeRef(req.NodeID)) {
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

	existing, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database node: load existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	oldNodeID := existing.NodeID

	if err := rt.databases.UpdateDatabaseNode(r.Context(), name, req.NodeID); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database node failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if oldNodeID != req.NodeID {
		rt.teardownDatabaseContainer(name, oldNodeID)
	}

	rt.reloadAndWriteDatabase(w, r, name, "set database node")
}

// teardownDatabaseContainer persists the teardown job then runs it in the
// background; a failure leaves the job for RetryDatabaseTeardowns.
func (rt *Router) teardownDatabaseContainer(name, nodeID string) {
	rt.scheduleDatabaseTeardown(name, nodeID, false)
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
	if !decodeReq(w, r, &req) {
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

	quotaRelease, quotaOK := rt.admitDatabaseMoveQuota(w, r, name, req.ProjectID)
	if !quotaOK {
		return
	}
	defer quotaRelease()

	if err := rt.databases.UpdateDatabaseProject(r.Context(), name, req.ProjectID); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database project failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteDatabase(w, r, name, "set database project")
}

// setDatabaseResourcesRequest is PUT /api/v1/databases/{name}/resources's
// body.
type setDatabaseResourcesRequest struct {
	Resources *store.ServiceResources `json:"resources"`
}

// Only Resources changes: the store's upsert leaves node, project, and backup columns untouched.
func (rt *Router) handleSetDatabaseResources(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setDatabaseResourcesRequest
	if !decodeReq(w, r, &req) {
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
	quotaRelease, quotaOK := rt.admitDatabaseResizeQuota(w, r, *existing, req.Resources)
	if !quotaOK {
		return
	}
	defer quotaRelease()
	if err := rt.databases.SaveDesiredDatabase(r.Context(), desired); err != nil {
		rt.logger.Error("api: set database resources failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	saved, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set database resources: reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := rt.toDatabaseResourceWithStatus(r.Context(), *saved)
	resp.ResourcesAppliedLive = rt.applyResourcesLive(r.Context(), saved.NodeID, databaseContainerName(name), saved.Resources)
	writeJSON(w, http.StatusOK, resp)
}

// Conditions are the only place a postgres database waiting on credentials shows up.
func (rt *Router) handleDatabaseStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		if ext, eerr := rt.externalDatabases.GetExternalDatabase(r.Context(), name); eerr == nil {
			writeJSON(w, http.StatusOK, externalConditions(*ext))
			return
		}
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
