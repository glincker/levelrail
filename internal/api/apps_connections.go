package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file: POST/GET/DELETE /api/v1/apps/{name}/connections[/{env_var}]
// and GET /api/v1/apps/{name}/connectable-databases, the UI/CLI-facing
// surface for store.DesiredService.DatabaseEnv (an open-ended map, one
// env var per entry) that app.yaml's own { from: "<database>.<field>" }
// syntax already populates but that, until now, had no way to set from
// the dashboard or the CLI for an app not hand-editing a spec file.
//
// This is deliberately separate from PUT/DELETE /api/v1/apps/{name}/database
// (apps_database.go), which remains DatabaseAttachment's own single-slot
// endpoint: an app can have at most one attachment but arbitrarily many
// DatabaseEnv connections, e.g. one app talking to both its own Postgres
// and a shared Redis cache.

// defaultConnectionFieldRequest is what a blank field defaults to on
// create, mirroring apps_database.go's own defaultDatabaseAttachmentField:
// the common case is "give me a full connection string."
const defaultConnectionFieldRequest = "url"

var nonEnvVarChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// appConnectionResource is GET/POST /api/v1/apps/{name}/connections'
// wire shape for one entry: store.DatabaseEnvRef plus the env var name
// (the map key) and a resolved-host preview so an operator can tell,
// without waiting for a deploy, whether this connection is actually
// cross-node-capable.
type appConnectionResource struct {
	EnvVar       string `json:"env_var"`
	DatabaseName string `json:"database_name"`
	Field        string `json:"field"`
	// Host is the value application.DatabaseHost would resolve this
	// connection's host to at deploy time: a mesh DNS name
	// (<database>.<zone>) or, when mesh networking isn't configured on
	// this control plane, the database's own Docker container name.
	Host string `json:"host"`
	// MeshDNS is true exactly when Host is a mesh DNS name rather than a
	// container name, i.e. whether this connection actually works across
	// nodes today.
	MeshDNS bool `json:"mesh_dns"`
	// NodeID is the referenced database's own placement (store.DesiredDatabase.NodeID,
	// "" meaning the local node), response-only, for the UI to compare
	// against the app's own node.
	NodeID string `json:"node_id,omitempty"`
	// CrossNode is true when the app and the database it connects to are
	// not on the same node.
	CrossNode bool `json:"cross_node"`
}

// createAppConnectionRequest is POST /api/v1/apps/{name}/connections'
// request body. Field and EnvVar both default when left blank: Field to
// "url", EnvVar via defaultConnectionEnvVar (this file's own doc
// comment on that function explains the naming convention).
type createAppConnectionRequest struct {
	Database string `json:"database"`
	Field    string `json:"field,omitempty"`
	EnvVar   string `json:"env_var,omitempty"`
}

// handleCreateAppConnection handles POST /api/v1/apps/{name}/connections:
// adds (or replaces, if env_var already names an existing connection) one
// entry in this app's DatabaseEnv map, the UI/CLI-facing equivalent of
// app.yaml's own { from: "<database>.<field>" } env var syntax for an
// app that already exists. Validates database and field exist/are
// supported the same "fail loudly here, not at reconcile time" way
// handleSetAppDatabase already does.
func (rt *Router) handleCreateAppConnection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req createAppConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Database == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	field := req.Field
	if field == "" {
		field = defaultConnectionFieldRequest
	}

	desiredDB, err := rt.databases.GetDesiredDatabase(r.Context(), req.Database)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusBadRequest, "unknown database")
		return
	} else if err != nil {
		rt.logger.Error("api: create app connection: look up database failed", slog.String("error", err.Error()), slog.String("database", req.Database))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !database.SupportsField(desiredDB.Engine, field) {
		writeError(w, http.StatusBadRequest, "field \""+field+"\" is not supported for "+desiredDB.Engine+" databases")
		return
	}

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create app connection: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	envVar := req.EnvVar
	if envVar == "" {
		envVar = defaultConnectionEnvVar(req.Database, field, existingServiceEnvKeys(svc))
	}

	if err := rt.apps.SetServiceDatabaseEnvVar(r.Context(), name, envVar, &store.DatabaseEnvRef{Database: req.Database, Field: field}); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create app connection failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.toAppConnectionResource(envVar, store.DatabaseEnvRef{Database: req.Database, Field: field}, desiredDB, svc.NodeID))
}

// handleDeleteAppConnection handles DELETE /api/v1/apps/{name}/connections/{env_var}:
// the reverse of handleCreateAppConnection, removing one DatabaseEnv
// entry. Idempotent, the same "disconnect an already-disconnected thing
// is not an error" shape handleClearAppVaultEnv already establishes.
func (rt *Router) handleDeleteAppConnection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	envVar := r.PathValue("env_var")

	if err := rt.apps.SetServiceDatabaseEnvVar(r.Context(), name, envVar, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: delete app connection failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("env_var", envVar))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleListAppConnections handles GET /api/v1/apps/{name}/connections:
// every entry in this app's DatabaseEnv map, each resolved against its
// referenced database's real placement so the response can say whether
// it currently resolves to a mesh DNS name (cross-node-capable) or a
// bare container name (reachable only when the app and database share a
// node). A database that was since deleted is skipped rather than
// failing the whole list, the same "one entry's problem, not the
// caller's" tolerance handleListApps already extends to a single
// broken app.
func (rt *Router) handleListAppConnections(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list app connections: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]appConnectionResource, 0, len(svc.DatabaseEnv))
	for envVar, ref := range svc.DatabaseEnv {
		desiredDB, err := rt.databases.GetDesiredDatabase(r.Context(), ref.Database)
		if errors.Is(err, store.ErrDatabaseNotFound) {
			continue
		} else if err != nil {
			rt.logger.Error("api: list app connections: look up database failed", slog.String("error", err.Error()), slog.String("database", ref.Database))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, rt.toAppConnectionResource(envVar, ref, desiredDB, svc.NodeID))
	}

	writeJSON(w, http.StatusOK, out)
}

// toAppConnectionResource builds one appConnectionResource, computing
// Host/MeshDNS the exact same way application.Controller's own
// resolveDatabaseField would at deploy time (application.DatabaseHost is
// that package's exported single source of truth for the host-selection
// half of that logic).
func (rt *Router) toAppConnectionResource(envVar string, ref store.DatabaseEnvRef, desiredDB *store.DesiredDatabase, appNodeID string) appConnectionResource {
	host := application.DatabaseHost(ref.Database, rt.meshZone)
	return appConnectionResource{
		EnvVar:       envVar,
		DatabaseName: ref.Database,
		Field:        ref.Field,
		Host:         host,
		MeshDNS:      rt.meshZone != "",
		NodeID:       desiredDB.NodeID,
		CrossNode:    desiredDB.NodeID != appNodeID,
	}
}

// connectableDatabaseResource is GET
// /api/v1/apps/{name}/connectable-databases' wire shape for one
// candidate database: every field handleCreateAppConnection needs to
// offer a one-click "connect" action, plus enough placement/status
// context for the UI to explain what connecting would actually mean
// (same node vs. cross-node, already connected or not).
type connectableDatabaseResource struct {
	Name             string   `json:"name"`
	Engine           string   `json:"engine"`
	NodeID           string   `json:"node_id,omitempty"`
	CrossNode        bool     `json:"cross_node"`
	AlreadyConnected bool     `json:"already_connected"`
	ConnectedEnvVars []string `json:"connected_env_vars,omitempty"`
}

// handleListConnectableDatabases handles GET
// /api/v1/apps/{name}/connectable-databases: every managed database this
// app could connect to (POST .../connections would accept it), so the
// dashboard can suggest connections instead of an operator hand-typing a
// database name. Scoped to the app's own project when it has one
// (ListDesiredDatabasesByProject), matching how every other project-
// scoped resource already limits "what's related to this app" (see
// AppStore.ListDesiredServicesByProject's own doc comment); an app with
// no project sees every database, the same lenient default
// handleSetAppDatabase itself already has (it accepts any existing
// database name regardless of project). Either way this is a
// suggestion, not an authorization boundary: POST .../connections
// itself still accepts any existing database name.
func (rt *Router) handleListConnectableDatabases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list connectable databases: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var databases []store.DesiredDatabase
	if svc.ProjectID != "" {
		databases, err = rt.databases.ListDesiredDatabasesByProject(r.Context(), svc.ProjectID)
	} else {
		databases, err = rt.databases.ListDesiredDatabases(r.Context())
	}
	if err != nil {
		rt.logger.Error("api: list connectable databases: list databases failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	connectedEnvVars := map[string][]string{}
	for envVar, ref := range svc.DatabaseEnv {
		connectedEnvVars[ref.Database] = append(connectedEnvVars[ref.Database], envVar)
	}
	if att := svc.DatabaseAttachment; att != nil {
		connectedEnvVars[att.DatabaseName] = append(connectedEnvVars[att.DatabaseName], att.EnvVar)
	}

	out := make([]connectableDatabaseResource, 0, len(databases))
	for _, d := range databases {
		envVars := connectedEnvVars[d.Name]
		out = append(out, connectableDatabaseResource{
			Name:             d.Name,
			Engine:           d.Engine,
			NodeID:           d.NodeID,
			CrossNode:        d.NodeID != svc.NodeID,
			AlreadyConnected: len(envVars) > 0,
			ConnectedEnvVars: envVars,
		})
	}

	writeJSON(w, http.StatusOK, out)
}

// existingServiceEnvKeys collects every env var name already in use on
// svc, across every source that can declare one: defaultConnectionEnvVar
// checks against this so a generated default never silently collides
// with (and overwrites at deploy time) something already there.
func existingServiceEnvKeys(svc *store.DesiredService) map[string]bool {
	keys := make(map[string]bool, len(svc.Env)+len(svc.SecretEnv)+len(svc.VaultEnv)+len(svc.DatabaseEnv))
	for k := range svc.Env {
		keys[k] = true
	}
	for _, ref := range svc.SecretEnv {
		keys[ref.Name] = true
	}
	for k := range svc.VaultEnv {
		keys[k] = true
	}
	for k := range svc.DatabaseEnv {
		keys[k] = true
	}
	if att := svc.DatabaseAttachment; att != nil {
		keys[att.EnvVar] = true
	}
	return keys
}

// defaultConnectionEnvVar picks a collision-safe env var name for a new
// connection when the request leaves env_var blank: "<DATABASE>_DATABASE_URL"
// for field "url" (the common case, namespaced by database name so two
// connections on the same app never collide the way a bare
// "DATABASE_URL" would once an app has more than one), or
// "<DATABASE>_DB_<FIELD>" for every other field (e.g. "MAIN_DB_HOST"),
// mirroring the self-documenting S3_* naming
// resolveStorageEnv/StorageEnvKeys already establish for storage
// targets. existing is every env var name already in use on this
// service (existingServiceEnvKeys); a numeric suffix (_2, _3, ...) is
// appended until the generated name is free, so a second connection to
// the same database+field never silently overwrites the first.
func defaultConnectionEnvVar(dbName, field string, existing map[string]bool) string {
	base := strings.Trim(nonEnvVarChars.ReplaceAllString(strings.ToUpper(dbName), "_"), "_")
	var name string
	if field == defaultConnectionFieldRequest {
		name = base + "_DATABASE_URL"
	} else {
		name = base + "_DB_" + strings.ToUpper(field)
	}
	if !existing[name] {
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if !existing[candidate] {
			return candidate
		}
	}
}
