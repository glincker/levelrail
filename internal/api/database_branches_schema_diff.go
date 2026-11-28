package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/dbauthz"
	"github.com/glincker/levelrail/internal/dbbranch"
	"github.com/glincker/levelrail/internal/dbschema"
	"github.com/glincker/levelrail/internal/dbviewer"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// schemaDiffAgainstParent is the default comparison base.
const schemaDiffAgainstParent = "parent"

type schemaDiffResponse struct {
	Branch  string `json:"branch"`
	Against string `json:"against"`
	dbschema.Result
}

// handleGetBranchSchemaDiff handles GET /api/v1/databases/{name}/branches/{branch}/schema-diff.
// The result describes what the branch changed relative to the base; the SQL turns the base into the branch.
func (rt *Router) handleGetBranchSchemaDiff(w http.ResponseWriter, r *http.Request) {
	b, bs, ok := rt.loadPathBranch(w, r, dbauthz.ActionRead)
	if !ok {
		return
	}
	against := r.URL.Query().Get("against")
	if against == "" || against == schemaDiffAgainstParent {
		against = b.ParentName
	}
	if against == b.DatabaseName {
		writeError(w, http.StatusBadRequest, "against must be a different database")
		return
	}
	// Read access on the base comes before any answer that mentions a state.
	if !rt.requireDBAction(w, r, dbauthz.ActionRead, against) {
		return
	}
	if !dbbranch.Reachable(b.State) {
		writeCodedError(w, http.StatusConflict, "branch_not_ready", "branch "+b.DatabaseName+" is "+b.State+" and not available yet")
		return
	}
	if !rt.requireSchemaDiffBase(w, r, bs, b, against) {
		return
	}
	timeout := time.Duration(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDatabasesSchemaDiffTimeoutSeconds)) * time.Second
	maxObjects := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDatabasesSchemaDiffMaxObjects)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	headSnap, ok := rt.readSchemaSnapshot(ctx, w, r, b.DatabaseName, timeout, maxObjects)
	if !ok {
		return
	}
	baseSnap, ok := rt.readSchemaSnapshot(ctx, w, r, against, timeout, maxObjects)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, schemaDiffResponse{Branch: b.DatabaseName, Against: against, Result: dbschema.Diff(baseSnap, headSnap)})
}

// requireSchemaDiffBase accepts the root or a ready branch of the same tree, and checks db:read on it.
func (rt *Router) requireSchemaDiffBase(w http.ResponseWriter, r *http.Request, bs branchAPIStore, b store.DatabaseBranch, against string) bool {
	if against != b.RootName {
		other, err := bs.GetDatabaseBranch(r.Context(), against)
		if errors.Is(err, store.ErrDatabaseBranchNotFound) || (err == nil && other.RootName != b.RootName) {
			writeError(w, http.StatusNotFound, "branch not found")
			return false
		}
		if err != nil {
			rt.internalError(w, "api: schema diff: load base branch failed", err)
			return false
		}
		if !dbbranch.Reachable(other.State) {
			writeCodedError(w, http.StatusConflict, "branch_not_ready", "branch "+against+" is "+other.State+" and not available yet")
			return false
		}
	}
	return rt.requireDBAction(w, r, dbauthz.ActionRead, against)
}

// readSchemaSnapshot reads one database through the viewer exec path; it writes the error response itself.
func (rt *Router) readSchemaSnapshot(ctx context.Context, w http.ResponseWriter, r *http.Request, name string, timeout time.Duration, maxObjects int) (dbschema.Snapshot, bool) {
	desired, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return dbschema.Snapshot{}, false
	}
	if err != nil {
		rt.internalError(w, "api: schema diff: load database failed", err, slog.String("name", name))
		return dbschema.Snapshot{}, false
	}
	if desired.Engine != store.EnginePostgres {
		writeError(w, http.StatusBadRequest, dbschema.ErrUnsupported.Error())
		return dbschema.Snapshot{}, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return dbschema.Snapshot{}, false
	}
	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, name, desired.NodeID)
	if !ok {
		return dbschema.Snapshot{}, false
	}
	limits := rt.databaseViewerLimits(ctx)
	limits.Timeout = timeout
	target := dbviewer.Target{Exec: nodeRuntime, ContainerID: state.ID, Dialect: dbviewer.DialectPostgres, Limits: limits}
	snap, err := dbschema.Read(ctx, target, maxObjects)
	if err != nil {
		rt.writeViewerError(w, "api: schema diff read failed", name, err)
		return dbschema.Snapshot{}, false
	}
	return snap, true
}
