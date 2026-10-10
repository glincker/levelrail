package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
)

type databaseScopeRequest struct {
	Scope   string `json:"scope"`
	DryRun  bool   `json:"dry_run"`
	Confirm bool   `json:"confirm"`
}

type databaseScopeResponse struct {
	Scope   string `json:"scope"`
	Applied bool   `json:"applied"`
	// ConfirmRequired is true when apps would lose reachability and the
	// request did not confirm; nothing was changed.
	ConfirmRequired bool               `json:"confirm_required"`
	Verdicts        []dbaccess.Verdict `json:"verdicts"`
	Lost            []dbaccess.Verdict `json:"lost"`
	Notes           []string           `json:"notes"`
}

// handleSetDatabaseScope handles PUT /api/v1/databases/{name}/network/scope.
// dry_run lists the apps that would lose reachability and changes nothing;
// cutting apps off needs confirm, and the reconciler then detaches them.
func (rt *Router) handleSetDatabaseScope(w http.ResponseWriter, r *http.Request) {
	if rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	var req databaseScopeRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	scope := dbaccess.Scope(req.Scope)
	if !scope.Valid() {
		writeError(w, http.StatusBadRequest, "scope must be platform, project or environment")
		return
	}
	clients, place, err := rt.databaseClients(r.Context(), d, scope)
	if err != nil {
		rt.internalError(w, "api: database scope: clients failed", err, slog.String("name", d.Name))
		return
	}
	apps := make([]dbaccess.Placement, 0, len(clients))
	for _, c := range clients {
		apps = append(apps, dbaccess.Placement{
			Name: c.App, ProjectID: c.ProjectID, ProjectName: c.ProjectName, EnvironmentID: c.EnvironmentID, EnvironmentName: c.Environment,
		})
	}
	verdicts, err := dbaccess.DryRun(scope, place, apps)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, dbaccess.ErrScopeUnplaced) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	lost := dbaccess.Lost(verdicts)
	resp := databaseScopeResponse{
		Scope: string(scope), Verdicts: verdicts, Lost: lost,
		Notes: []string{"Apps that lose reachability are detached from this database on the next reconcile and show why on their status."},
	}
	if resp.Lost == nil {
		resp.Lost = []dbaccess.Verdict{}
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if len(lost) > 0 && !req.Confirm {
		resp.ConfirmRequired = true
		writeJSON(w, http.StatusOK, resp)
		return
	}
	cur, err := rt.dbAccess.GetDatabaseAccessSettings(r.Context(), d.Name)
	if err != nil {
		rt.internalError(w, "api: database scope: settings failed", err, slog.String("name", d.Name))
		return
	}
	cur.Scope, cur.UpdatedAt = string(scope), time.Now().UTC().Format(time.RFC3339)
	if err := rt.dbAccess.SaveDatabaseAccessSettings(r.Context(), cur); err != nil {
		rt.internalError(w, "api: database scope: save failed", err, slog.String("name", d.Name))
		return
	}
	resp.Applied = true
	rt.auditDatabaseAccess(r, dbaccess.ActionScopeSet, d.Name, string(scope), http.StatusOK)
	rt.logger.Info("api: database network scope set", slog.String("database", d.Name), slog.String("scope", string(scope)), slog.Int("apps_losing_access", len(lost)))
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, resp)
}

type databaseTLSRequest struct {
	Require bool `json:"require"`
}

// handleSetDatabaseTLS handles PUT /api/v1/databases/{name}/network/tls.
func (rt *Router) handleSetDatabaseTLS(w http.ResponseWriter, r *http.Request) {
	var req databaseTLSRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	pg, d, ok := rt.dbAccessTarget(w, r)
	if !ok {
		return
	}
	if req.Require && !rt.databaseTLSEnabled(r.Context(), *d) {
		writeError(w, http.StatusConflict, "TLS is not enabled for this database yet, so it cannot be required")
		return
	}
	if err := pg.SetRequireTLS(r.Context(), req.Require); err != nil {
		rt.writeDatabaseAccessError(w, "api: set database tls failed", d.Name, err)
		return
	}
	cur, err := rt.dbAccess.GetDatabaseAccessSettings(r.Context(), d.Name)
	if err != nil {
		rt.internalError(w, "api: database tls: settings failed", err, slog.String("name", d.Name))
		return
	}
	cur.RequireTLS, cur.UpdatedAt = req.Require, time.Now().UTC().Format(time.RFC3339)
	if err := rt.dbAccess.SaveDatabaseAccessSettings(r.Context(), cur); err != nil {
		rt.internalError(w, "api: database tls: save failed", err, slog.String("name", d.Name))
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionTLSSet, d.Name, boolWord(req.Require), http.StatusNoContent)
	rt.logger.Info("api: database tls requirement set", slog.String("database", d.Name), slog.Bool("require", req.Require))
	w.WriteHeader(http.StatusNoContent)
}

func boolWord(b bool) string {
	if b {
		return "required"
	}
	return "optional"
}
