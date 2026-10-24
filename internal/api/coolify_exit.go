package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/coolifyexit"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func exitReportPlatform(p platformimport.Platform) bool {
	switch p {
	case platformimport.Coolify, platformimport.Dokploy, platformimport.CapRover, platformimport.Heroku, platformimport.DockerSSH:
		return true
	}
	return false
}

func (rt *Router) writeCoolifyExitReport(w http.ResponseWriter, r *http.Request, req platformImportRequest, disc *platformimport.Discovery) {
	opts, _, err := rt.importPlanOptions(r.Context(), req)
	if err != nil {
		rt.internalError(w, "api: coolify exit report: read targets failed", err)
		return
	}
	writeJSON(w, http.StatusOK, coolifyexit.Build(disc, platformimport.BuildPlan(disc, opts)))
}

// handleCoolifyExitReport handles POST /api/v1/migration/coolify/exit-report: it reads the
// source, creates nothing and stores nothing. Run again after staging it doubles as the
// verification report, because apps already imported show as imported.
func (rt *Router) handleCoolifyExitReport(w http.ResponseWriter, r *http.Request) {
	req, src, ok := rt.decodePlatformImport(w, r)
	if !ok {
		return
	}
	defer req.wipeSecrets()
	if !exitReportPlatform(platformimport.Platform(req.Platform)) {
		writeError(w, http.StatusBadRequest, "the exit report reads coolify, dokploy, caprover, heroku or docker-ssh")
		return
	}
	disc, err := src.Discover(r.Context())
	if err != nil {
		rt.writeImportError(w, err, req.Token)
		return
	}
	rt.writeCoolifyExitReport(w, r, req, disc)
}

// handleAppImportExitReport handles GET /api/v1/migration/apps/sessions/{id}/exit-report from
// the discovery a connected session holds in memory, so no source token is sent again.
func (rt *Router) handleAppImportExitReport(w http.ResponseWriter, r *http.Request) {
	sess, _, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	live := rt.appImportLive.get(sess.ID)
	if live == nil || live.disc == nil {
		writeError(w, http.StatusConflict, "the session is not connected to its source: reconnect it first")
		return
	}
	if !exitReportPlatform(live.disc.Platform) {
		writeError(w, http.StatusBadRequest, "the exit report reads coolify, dokploy, caprover, heroku or docker-ssh")
		return
	}
	rt.writeCoolifyExitReport(w, r, live.req, live.disc)
}
