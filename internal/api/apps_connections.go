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

// Sets and lists the DatabaseEnv connections an app uses, separate from the single-slot database attachment.

// defaultConnectionFieldRequest is what a blank field defaults to on
// create, mirroring apps_database.go's own defaultDatabaseAttachmentField:
// the common case is "give me a full connection string."
const defaultConnectionFieldRequest = "url"

var nonEnvVarChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// appConnectionResource is one DatabaseEnv entry plus its resolved host, so cross-node reachability is visible before a deploy.
type appConnectionResource struct {
	EnvVar       string `json:"env_var"`
	DatabaseName string `json:"database_name"`
	Field        string `json:"field"`
	// The mesh DNS name when mesh is configured, otherwise the database's bare container name.
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

// Field defaults to url and EnvVar to a generated name when left blank.
type createAppConnectionRequest struct {
	Database string `json:"database"`
	Field    string `json:"field,omitempty"`
	EnvVar   string `json:"env_var,omitempty"`
}

// Replaces an existing entry for env_var; validates database and field before saving, so failures surface at request time.
func (rt *Router) handleCreateAppConnection(w http.ResponseWriter, r *http.Request) {
	var req createAppConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rt.createAppConnection(w, r, r.PathValue("name"), req)
}

func (rt *Router) createAppConnection(w http.ResponseWriter, r *http.Request, name string, req createAppConnectionRequest) {
	if req.Database == "" {
		writeError(w, http.StatusBadRequest, "database is required")
		return
	}
	field := req.Field
	if field == "" {
		field = defaultConnectionFieldRequest
	}

	if !rt.authorizeDatabaseRefs(w, r, req.Database) {
		return
	}
	desiredDB, external, err := rt.lookupAppDatabase(r.Context(), req.Database)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusBadRequest, "unknown database")
		return
	} else if err != nil {
		rt.internalError(w, "api: create app connection: look up database failed", err, slog.String("database", req.Database))
		return
	}
	if !databaseFieldSupported(external, desiredDB.Engine, field) {
		writeError(w, http.StatusBadRequest, "field \""+field+"\" is not supported for "+desiredDB.Engine+" databases")
		return
	}

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: create app connection: load app failed", err, slog.String("name", name))
		return
	}

	envVar := req.EnvVar
	if envVar == "" {
		existing := existingServiceEnvKeys(svc)
		envVar = database.ConventionalEnvVar(desiredDB.Engine, field)
		if external || envVar == "" || existing[envVar] {
			envVar = defaultConnectionEnvVar(req.Database, field, existing)
		}
	}

	if err := rt.apps.SetServiceDatabaseEnvVar(r.Context(), name, envVar, &store.DatabaseEnvRef{Database: req.Database, Field: field}); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: create app connection failed", err, slog.String("name", name))
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.externalizeConnection(r.Context(), rt.toAppConnectionResource(envVar, store.DatabaseEnvRef{Database: req.Database, Field: field}, desiredDB, svc.NodeID), external))
}

// Idempotent: removing an already-removed connection is not an error.
func (rt *Router) handleDeleteAppConnection(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	envVar := r.PathValue("env_var")

	if err := rt.apps.SetServiceDatabaseEnvVar(r.Context(), name, envVar, nil); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: delete app connection failed", err, slog.String("name", name), slog.String("env_var", envVar))
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// Skips a connection whose database was deleted, so one stale entry does not fail the whole list.
func (rt *Router) handleListAppConnections(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list app connections: load app failed", err, slog.String("name", name))
		return
	}

	out := make([]appConnectionResource, 0, len(svc.DatabaseEnv))
	for envVar, ref := range svc.DatabaseEnv {
		desiredDB, external, err := rt.lookupAppDatabase(r.Context(), ref.Database)
		if errors.Is(err, store.ErrDatabaseNotFound) {
			continue
		} else if err != nil {
			rt.internalError(w, "api: list app connections: look up database failed", err, slog.String("database", ref.Database))
			return
		}
		out = append(out, rt.externalizeConnection(r.Context(), rt.toAppConnectionResource(envVar, ref, desiredDB, svc.NodeID), external))
	}

	writeJSON(w, http.StatusOK, out)
}

// Resolves Host and MeshDNS the same way the application controller does at deploy time.
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

// Carries placement and connection state so the UI can say whether connecting is same-node or cross-node.
type connectableDatabaseResource struct {
	Name             string   `json:"name"`
	Engine           string   `json:"engine"`
	NodeID           string   `json:"node_id,omitempty"`
	CrossNode        bool     `json:"cross_node"`
	AlreadyConnected bool     `json:"already_connected"`
	ConnectedEnvVars []string `json:"connected_env_vars,omitempty"`
}

// Scoped to the app's project when it has one; a suggestion only, since POST still accepts any existing database.
func (rt *Router) handleListConnectableDatabases(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list connectable databases: load app failed", err, slog.String("name", name))
		return
	}

	var databases []store.DesiredDatabase
	if svc.ProjectID != "" {
		databases, err = rt.databases.ListDesiredDatabasesByProject(r.Context(), svc.ProjectID)
	} else {
		databases, err = rt.databases.ListDesiredDatabases(r.Context())
	}
	if err != nil {
		rt.internalError(w, "api: list connectable databases: list databases failed", err, slog.String("name", name))
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

// Collects env var names from every source, so a generated default never overwrites an existing one at deploy time.
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

// Field url becomes <DATABASE>_DATABASE_URL, and a numeric suffix avoids collisions with existing env var names.
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
