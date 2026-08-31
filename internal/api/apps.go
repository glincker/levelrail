package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appResource is the wire shape for an app: store.DesiredService plus
// its name, marshaled and unmarshaled directly rather than through a
// parallel domain type, since TASKS.md 1.9 doesn't ask this endpoint to
// represent anything store.DesiredService can't already hold. Strategy
// and Replicas got a home in store.DesiredService (migration
// 0017_deploy_strategy.sql), so this resource now carries them too;
// domains closed once TASKS.md 1.6 added the column, the same way.
type appResource struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Port  int    `json:"port"`
	// HostPort pins the host-side port Docker binds Port to
	// (store.DesiredService.HostPort, migrations/0056). nil means "let
	// Docker assign one", the ordinary case; a real conflict at deploy
	// time (this host port already bound) surfaces through the existing
	// reconcile-condition/deploy-log mechanism, since Docker itself is
	// the only thing that can authoritatively know at container-create
	// time. Settable on create and update, like Port itself, not
	// response-only.
	HostPort  *int                    `json:"host_port,omitempty"`
	Domains   []string                `json:"domains,omitempty"`
	Env       map[string]string       `json:"env,omitempty"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
	Health    *store.ServiceHealth    `json:"health,omitempty"`
	// Strategy and Replicas deliberately have no `omitempty`: a real
	// store.DesiredService read back from the store is documented to
	// always carry the *resolved* value (never "" / 0, see that
	// struct's own doc comment), matching how Image/Port (also always-
	// resolved, required fields) have no omitempty either. On the way
	// in, an empty Strategy or zero Replicas is not an error, it just
	// falls through to store.SaveDesiredService's own default-filling
	// (DefaultDeployStrategy/DefaultReplicas), the same behavior a
	// service saved without these fields has always had.
	Strategy string `json:"strategy"`
	Replicas int    `json:"replicas"`
	// Labels are arbitrary operator-supplied Docker labels applied to
	// the app's container at create time (internal/spec.Service.Labels'
	// wire counterpart), exposed on the general create/update endpoints
	// rather than a dedicated one: this is fairly static, low-frequency-
	// change config, not something needing its own PUT the way a
	// restart nonce or backup schedule does. validateAppResource runs
	// internal/spec.ValidateLabels against it, the same reserved-prefix
	// and sanity-limit rules app.yaml-sourced labels get.
	Labels map[string]string `json:"labels,omitempty"`
	// NodeID is TASKS.md 3.3's placement (empty means this control
	// plane's own local node). Response-only: toDesiredService below
	// never reads it, the same "shown but not settable through this
	// endpoint" boundary ruleResource's own evaluation-state fields
	// already establish for a different resource. Set it via
	// PUT /api/v1/apps/{name}/node (handleSetAppNode) instead.
	NodeID string `json:"node_id,omitempty"`
	// ProjectID is which project (projects.go) this app is filed under,
	// empty meaning no project. Response-only on PUT (handleUpdateApp
	// echoes back the existing, unchanged value the same way it already
	// does for NodeID, never req's), the same "shown but not settable
	// through the general update endpoint" boundary NodeID establishes,
	// for the identical reasoning: an ordinary edit must never silently
	// move an app between projects. handleCreateApp is the one
	// exception, see that handler's own doc comment for why accepting
	// it there doesn't violate that boundary. Set it on an existing app
	// via PUT /api/v1/apps/{name}/project (handleSetAppProject) instead.
	ProjectID string `json:"project_id,omitempty"`
	// EnvironmentID is which environment (environments.go) this app is
	// tagged with, empty meaning none. Response-only, same boundary as
	// ProjectID: set via PUT /api/v1/apps/{name}/environment instead.
	EnvironmentID string `json:"environment_id,omitempty"`
	// StorageTargetID is which connected backup target (backup_targets.go)
	// this app's object-storage credentials resolve from, empty meaning
	// no storage attached. Response-only, the same "shown but not
	// settable through this endpoint" boundary NodeID/ProjectID already
	// establish above, for identical reasoning: an ordinary edit must
	// never silently attach or detach live bucket credentials. Set it via
	// PUT/DELETE /api/v1/apps/{name}/storage (apps_storage.go's
	// handleSetAppStorage/handleClearAppStorage) instead.
	StorageTargetID string `json:"storage_target_id,omitempty"`
	// DatabaseEnv names every env var whose value resolves from a managed
	// database's own connection details (store.DesiredService.DatabaseEnv,
	// internal/spec's { from: "<database>.<field>" } syntax), so the UI
	// can show which databases an app.yaml-deployed service is wired to
	// without hand-parsing app.yaml itself. Response-only: this comes from
	// the deploy pipeline (internal/deploy), never from this endpoint.
	DatabaseEnv map[string]appDatabaseEnvRef `json:"database_env,omitempty"`
	// DatabaseAttachment is the UI/CLI-facing attachment
	// (store.DesiredService.DatabaseAttachment), distinct from DatabaseEnv
	// above: see apps_database.go's own doc comment. Response-only, the
	// same "shown but not settable through this endpoint" boundary
	// StorageTargetID already establishes: set it via PUT/DELETE
	// /api/v1/apps/{name}/database instead.
	DatabaseAttachment *appDatabaseResource `json:"database_attachment,omitempty"`
	// Suspended is whether the app is stopped (POST .../stop), the
	// reconciler's own containers-only teardown, distinct from delete.
	// Response-only: set it via POST/DELETE-shaped
	// /api/v1/apps/{name}/stop and /start (handleStopApp/handleStartApp)
	// instead, the same boundary NodeID/ProjectID/StorageTargetID
	// already establish above.
	Suspended bool `json:"suspended"`
	// AppID is which store.App (migrations/0039_apps.sql) this service
	// belongs to; empty when none. toDesiredService never reads it back
	// in: it's assigned server-side, via ensureAppLinked (apps_multi.go),
	// from handleCreateApp here and from POST /api/v1/apps/{name}/deploy-spec
	// for a fanned-out service. See GET /api/v1/apps/{name}/group
	// (apps_group.go) for the sibling-services read this enables.
	AppID string `json:"app_id,omitempty"`
	// LogDrain is response-only, the same boundary NodeID/ProjectID/
	// StorageTargetID already establish above: set it via PUT/DELETE
	// /api/v1/apps/{name}/log-drain (apps_log_drain.go) instead.
	LogDrain *store.LogDrain `json:"log_drain,omitempty"`
	// ResourcesAppliedLive is response-only, set by handleUpdateApp:
	// whether the new Resources value was already pushed onto a
	// currently running container via the Engine API's live
	// ContainerUpdate (applyResourcesLiveToReplicas), rather than
	// waiting for the next deploy or restart to take effect. False
	// still means the value is saved and will apply next time the
	// container is recreated; it never means the save itself failed.
	ResourcesAppliedLive bool `json:"resources_applied_live,omitempty"`
}

func toAppResource(svc store.DesiredService) appResource {
	var databaseEnv map[string]appDatabaseEnvRef
	if len(svc.DatabaseEnv) > 0 {
		databaseEnv = make(map[string]appDatabaseEnvRef, len(svc.DatabaseEnv))
		for k, v := range svc.DatabaseEnv {
			databaseEnv[k] = appDatabaseEnvRef{Database: v.Database, Field: v.Field}
		}
	}
	var attachment *appDatabaseResource
	if svc.DatabaseAttachment != nil {
		attachment = &appDatabaseResource{
			AppName:      svc.Name,
			DatabaseName: svc.DatabaseAttachment.DatabaseName,
			EnvVar:       svc.DatabaseAttachment.EnvVar,
			Field:        svc.DatabaseAttachment.Field,
		}
	}

	return appResource{
		Name:               svc.Name,
		Image:              svc.Image,
		Port:               svc.Port,
		HostPort:           svc.HostPort,
		Domains:            svc.Domains,
		Env:                svc.Env,
		Resources:          svc.Resources,
		Health:             svc.Health,
		Strategy:           svc.Strategy,
		Replicas:           svc.Replicas,
		Labels:             svc.Labels,
		NodeID:             svc.NodeID,
		ProjectID:          svc.ProjectID,
		EnvironmentID:      svc.EnvironmentID,
		StorageTargetID:    svc.StorageTargetID,
		DatabaseEnv:        databaseEnv,
		DatabaseAttachment: attachment,
		Suspended:          svc.Suspended,
		AppID:              svc.AppID,
		LogDrain:           svc.LogDrain,
	}
}

func (a appResource) toDesiredService() store.DesiredService {
	return store.DesiredService{
		Name:      a.Name,
		Image:     a.Image,
		Port:      a.Port,
		HostPort:  a.HostPort,
		Domains:   a.Domains,
		Env:       a.Env,
		Resources: a.Resources,
		Health:    a.Health,
		Strategy:  a.Strategy,
		Replicas:  a.Replicas,
		Labels:    a.Labels,
	}
}

// validKnownStrategies mirrors internal/spec's own
// StrategyRolling/StrategyRecreate/StrategyBlueGreen exactly, all three
// reconciler-backed (internal/reconcile/application's controller runs
// all three).
var validKnownStrategies = map[string]bool{
	spec.StrategyRolling:   true,
	spec.StrategyRecreate:  true,
	spec.StrategyBlueGreen: true,
}

func validateAppResource(a appResource) error {
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.Image == "" {
		return errors.New("image is required")
	}
	if a.Port <= 0 {
		return errors.New("port must be a positive integer")
	}
	if a.HostPort != nil && (*a.HostPort < 1 || *a.HostPort > 65535) {
		return errors.New("host_port must be between 1 and 65535")
	}
	// Empty is valid (falls through to store.DefaultDeployStrategy), but
	// a non-empty value that isn't one of the three known spec constants
	// is almost certainly a typo, and worth rejecting loudly here rather
	// than saving it and letting the reconciler discover it later.
	if a.Strategy != "" && !validKnownStrategies[a.Strategy] {
		return fmt.Errorf("strategy must be one of %q, %q, %q", spec.StrategyRolling, spec.StrategyRecreate, spec.StrategyBlueGreen)
	}
	// Zero is valid (falls through to store.DefaultReplicas), only a
	// negative count is nonsensical.
	if a.Replicas < 0 {
		return errors.New("replicas must not be negative")
	}
	if err := spec.ValidateLabels(a.Labels); err != nil {
		return err
	}
	for _, domain := range a.Domains {
		if err := ingress.ValidateWildcardDomain(domain); err != nil {
			return err
		}
	}
	return nil
}

// handleListApps handles GET /api/v1/apps. Status is computed from one
// batched conditions query (store.GetConditionsForControllers), not a
// GetConditions call per app: see appListResource's own doc comment for
// why that matters at 50+ rows.
func (rt *Router) handleListApps(w http.ResponseWriter, r *http.Request) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.logger.Error("api: list apps failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	controllerNames := make([]string, len(svcs))
	for i, s := range svcs {
		controllerNames[i] = applicationControllerName(s.Name)
	}
	conditionsByController, err := rt.deploys.GetConditionsForControllers(r.Context(), controllerNames)
	if err != nil {
		rt.logger.Error("api: list apps: batch load conditions failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]appListResource, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, appListResource{
			appResource: toAppResource(s),
			Status:      summarizeAppConditions(conditionsByController[applicationControllerName(s.Name)]),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateApp handles POST /api/v1/apps. Rejects a name that already
// exists rather than silently overwriting it: that's what PUT
// (handleUpdateApp) is for. A domain conflict (store.ErrDomainTaken,
// enforced by SaveDesiredService itself, see that error's own doc
// comment for why) surfaces as 409 with the real conflicting domain
// named in the message, the same shape a duplicate name gets, not the
// generic 500 it fell through to before this was wired up.
//
// req.ProjectID is the one exception to appResource's own "ProjectID is
// response-only" doc comment: a brand new app has no existing project
// assignment an ordinary write could silently clobber (the concern that
// keeps ProjectID excluded from handleUpdateApp and from
// toDesiredService/SaveDesiredService's own INSERT), so accepting it
// here at create time is safe in a way it wouldn't be on an update. This
// still goes through UpdateServiceProject after the create succeeds,
// not through SaveDesiredService's own INSERT (which always writes
// project_id NULL regardless, store.DB.SaveDesiredService's own doc
// comment), so store.DB.UpdateServiceProject stays the one and only
// place project_id is ever actually written, the same narrow-mutation
// discipline UpdateServiceNode already established; this handler is
// just a convenience that calls it automatically for the common
// create-with-a-project case instead of requiring a second client round
// trip to PUT /api/v1/apps/{name}/project right after creation.
func (rt *Router) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.logger.Error("api: create app: check project failed", slog.String("error", err.Error()), slog.String("project_id", req.ProjectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	_, err := rt.apps.GetDesiredService(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: create app: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.logger.Error("api: create app failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if req.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), req.Name, req.ProjectID); err != nil {
			rt.logger.Error("api: create app: assign project failed", slog.String("error", err.Error()), slog.String("name", req.Name), slog.String("project_id", req.ProjectID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Every app gets a store.App row, single-service or not: see
	// ensureAppLinked's own doc comment for why this must not be
	// special-cased away for the common single-service case.
	appID, err := rt.ensureAppLinked(r.Context(), req.Name, req.Name)
	if err != nil {
		rt.logger.Error("api: create app: link to app row failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	req.AppID = appID

	// Skip the git-build path's ":pending" placeholder (cmd/levelrail-cli's
	// pendingImageTag): its own POST .../builds call records the real
	// history entry once a build actually succeeds.
	if !strings.HasSuffix(req.Image, ":pending") {
		rt.recordPlainDeployAttempt(r.Context(), req.Name, req.Image)
	}

	writeJSON(w, http.StatusCreated, req)
}

// handleGetApp handles GET /api/v1/apps/{name}.
func (rt *Router) handleGetApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppResource(*svc))
}

// handleUpdateApp handles PUT /api/v1/apps/{name}. Full replace, same as
// SaveDesiredService itself: there is no partial-update semantics here,
// matching how store.DesiredService is already documented to work.
func (rt *Router) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = name // the path is authoritative, not whatever the body claims

	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update app: check existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.logger.Error("api: update app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// An image change here is functionally the same redeploy action as
	// handleTriggerDeploy, which already records one: an ordinary PUT
	// that happens to change Image must not be a blind spot in deploy
	// history just because it went through this endpoint instead.
	if req.Image != existing.Image {
		rt.recordPlainDeployAttempt(r.Context(), name, req.Image)
	}
	// SaveDesiredService never touches node_id or project_id (their own
	// doc comments explain why), so the response reflects existing's
	// placement and project, not req's: req.NodeID/req.ProjectID are
	// always their zero value here anyway (the client never has a way to
	// set either on this endpoint, unlike handleCreateApp's own
	// deliberate ProjectID exception), but spelling out "carry the real,
	// unchanged value forward" is clearer than relying on that zero
	// value happening to look right.
	req.NodeID = existing.NodeID
	req.ProjectID = existing.ProjectID
	// Re-fetch rather than reusing req.toDesiredService(): NodeID and
	// RestartNonce (both needed to compute the real running container's
	// name, desiredServiceContainerNames) are response-only fields this
	// endpoint's request body never carries, the same boundary NodeID's
	// own doc comment establishes.
	if saved, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		req.ResourcesAppliedLive = rt.applyResourcesLiveToReplicas(r.Context(), *saved)
	}
	writeJSON(w, http.StatusOK, req)
}

// setAppNodeRequest is PUT /api/v1/apps/{name}/node's body.
type setAppNodeRequest struct {
	// NodeID is which node to place the app on; empty string moves it
	// back to this control plane's own local node, the same convention
	// store.DesiredService.NodeID's own doc comment establishes.
	NodeID string `json:"node_id"`
}

// handleSetAppNode handles PUT /api/v1/apps/{name}/node (TASKS.md 3.3):
// the only way an app's placement actually changes, see appResource's
// own NodeID field doc comment. A non-empty node_id is checked against
// the real node registry first, so a typo'd or already-removed node ID
// fails loudly here with a clear 400 rather than silently parking the
// service on a node that will never reconcile it (resolveNodeTransport,
// cmd/levelrail/main.go, would otherwise just skip it forever with
// nothing surfaced beyond a log line).
// reloadAndWriteApp reloads name's desired service and writes it as the
// response, the common tail every app-mutation handler in this file needs.
// logContext names the calling handler for the error log line.
func (rt *Router) reloadAndWriteApp(w http.ResponseWriter, r *http.Request, name, logContext string) {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: "+logContext+": reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppResource(*svc))
}

func (rt *Router) handleSetAppNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.validatePlacementTarget(r.Context(), req.NodeID); err != nil {
		switch {
		case errors.Is(err, store.ErrNodeNotFound):
			writeError(w, http.StatusBadRequest, "unknown node_id")
		case errors.Is(err, errNodeCordoned):
			// TASKS.md 3.7: cordon means "unschedulable for new
			// placements", and this is a new placement even when the
			// service already exists, since it's actively choosing to
			// move it here.
			writeError(w, http.StatusBadRequest, "node is cordoned and not accepting new placements")
		default:
			rt.logger.Error("api: set app node: look up node failed", slog.String("error", err.Error()), slog.String("node_id", req.NodeID))
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	if err := rt.apps.UpdateServiceNode(r.Context(), name, req.NodeID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app node failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "set app node")
}

// setAppProjectRequest is PUT /api/v1/apps/{name}/project's body, the
// project-kind counterpart to setAppNodeRequest.
type setAppProjectRequest struct {
	// ProjectID is which project to move this app into; empty string
	// moves it back to "no project", the same convention
	// DesiredService.ProjectID's own doc comment establishes.
	ProjectID string `json:"project_id"`
}

// handleSetAppProject handles PUT /api/v1/apps/{name}/project
// (projects.go): the only way an existing app's project assignment
// actually changes, see appResource's own ProjectID field doc comment.
// A non-empty project_id is checked against the real project registry
// first, the same shape handleSetAppNode already establishes for
// node_id, so a typo'd or already-deleted project id fails loudly here
// with a 400 rather than silently being written.
func (rt *Router) handleSetAppProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.validateProjectID(r.Context(), req.ProjectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusBadRequest, "unknown project_id")
			return
		}
		rt.logger.Error("api: set app project: look up project failed", slog.String("error", err.Error()), slog.String("project_id", req.ProjectID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.UpdateServiceProject(r.Context(), name, req.ProjectID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app project failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "set app project")
}

// handleRestartApp handles POST /api/v1/apps/{name}/restart: force a
// running container to be recreated with no image change. No request
// body. See store.DB.RestartService's own doc comment for the mechanism
// (a fresh restart_nonce, folded into the reconciler's container-name
// hash, makes this indistinguishable from an image change to the
// existing, already-tested blue-green/recreate cutover logic).
func (rt *Router) handleRestartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.RestartService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: restart app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "restart app")
}

// handleStopApp handles POST /api/v1/apps/{name}/stop: sets Suspended,
// distinct from handleDeleteApp, which removes desired state entirely.
// No request body. The reconciler (not this handler) is what actually
// stops containers, on its next pass; see
// store.DesiredService.Suspended's own doc comment.
func (rt *Router) handleStopApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceSuspended(r.Context(), name, true); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: stop app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "stop app")
}

// handleStartApp handles POST /api/v1/apps/{name}/start: clears
// Suspended, letting the reconciler bring containers back on its next
// pass. No request body.
func (rt *Router) handleStartApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if err := rt.apps.UpdateServiceSuspended(r.Context(), name, false); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: start app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.reloadAndWriteApp(w, r, name, "start app")
}

// handleDeleteApp handles DELETE /api/v1/apps/{name}. See
// store.DeleteDesiredService's doc comment for the known gap: this
// removes desired state, it does not itself stop the running container.
//
// If the deleted service was the last member of its store.App
// (migrations/0039_apps.sql), the now-empty App row is deleted too, via
// deleteAppIfOrphaned below. Without this, a single-service app's App
// row (set by that migration's own backfill, or by a multi-service
// deploy's own store.App.ID == store.App.Name convention) would outlive
// every service that ever belonged to it, and the per-app Docker
// network internal/reconcile/application.NetworkCleanupController tears
// down once its App row disappears would never actually go away.
func (rt *Router) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete app: load existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	appID := existing.AppID

	if err := rt.apps.DeleteDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
		rt.logger.Error("api: delete app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if appID != "" {
		rt.deleteAppIfOrphaned(r.Context(), appID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// deleteAppIfOrphaned deletes the store.App row identified by appID once
// it has no remaining member services. Best-effort and logged, not
// surfaced as handleDeleteApp's own error: the service delete above
// already succeeded, and a stale App row (or, worst case, a network
// NetworkCleanupController doesn't yet know to remove) is a lesser,
// self-healing problem, not one that should make an otherwise-successful
// delete request fail.
func (rt *Router) deleteAppIfOrphaned(ctx context.Context, appID string) {
	remaining, err := rt.appGroups.ListServicesByApp(ctx, appID)
	if err != nil {
		rt.logger.Error("api: delete app: check remaining siblings failed", slog.String("error", err.Error()), slog.String("app_id", appID))
		return
	}
	if len(remaining) > 0 {
		return
	}
	if err := rt.appGroups.DeleteApp(ctx, appID); err != nil && !errors.Is(err, store.ErrAppNotFound) {
		rt.logger.Error("api: delete app: delete orphaned app row failed", slog.String("error", err.Error()), slog.String("app_id", appID))
	}
}
