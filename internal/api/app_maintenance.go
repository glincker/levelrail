package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/maintenance"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
)

type appMaintenanceDomain struct {
	domainMaintenanceResource
	ElapsedSeconds int64 `json:"elapsed_seconds,omitempty"`
}

// appMaintenanceResource is the wire shape for .../apps/{name}/maintenance.
type appMaintenanceResource struct {
	App string `json:"app"`
	// Enabled is true when any domain is in maintenance, All when every domain is.
	Enabled bool                   `json:"enabled"`
	All     bool                   `json:"all"`
	Total   int                    `json:"total_domains"`
	Active  int                    `json:"active_domains"`
	Domains []appMaintenanceDomain `json:"domains"`
}

// maintenanceFailureResource reports a change that was refused or undone.
type maintenanceFailureResource struct {
	Error  string `json:"error"`
	Domain string `json:"failed_domain,omitempty"`
	Stage  string `json:"failed_stage,omitempty"`
	// RolledBack is true when nothing was changed.
	RolledBack bool `json:"rolled_back"`
}

func (rt *Router) writeMaintenanceFailure(w http.ResponseWriter, f *maintenanceFailure) {
	if f.Status >= http.StatusInternalServerError && f.Status != http.StatusNotImplemented {
		rt.logger.Error("api: enable maintenance failed", slog.String("domain", f.Domain), slog.String("stage", f.Stage), slog.String("error", f.Err.Error()))
		writeJSON(w, f.Status, maintenanceFailureResource{Error: "could not enable maintenance for " + f.Domain + " (" + f.Stage + "); nothing was changed", Domain: f.Domain, Stage: f.Stage, RolledBack: true})
		return
	}
	writeError(w, f.Status, f.Err.Error())
}

// decorateMaintenance adds the alert silence effect to a domain's resource.
func (rt *Router) decorateMaintenance(ctx context.Context, res *domainMaintenanceResource, now time.Time) {
	sil := rt.silencer()
	if sil == nil || !res.Enabled {
		return
	}
	s, err := sil.Active(ctx, res.Domain, now)
	if err != nil {
		rt.logger.Warn("api: look up maintenance silence failed", slog.String("domain", res.Domain), slog.String("error", err.Error()))
		return
	}
	if s != nil {
		res.AlertsSilenced, res.SilenceID = true, s.ID
	}
}

func (rt *Router) appDomains(w http.ResponseWriter, r *http.Request) (app string, domains []string, ok bool) {
	app = r.PathValue("name")
	if app == standalone.Scope {
		writeError(w, http.StatusBadRequest, "maintenance for all domains is not available for standalone domains")
		return "", nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), app)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", nil, false
	}
	if err != nil {
		rt.internalError(w, "api: app maintenance: get app failed", err, slog.String("app", app))
		return "", nil, false
	}
	return app, svc.Domains, true
}

func (rt *Router) appMaintenanceState(ctx context.Context, app string, domains []string, now time.Time) (appMaintenanceResource, error) {
	out := appMaintenanceResource{App: app, Total: len(domains), Domains: []appMaintenanceDomain{}}
	ss, hasSettings := rt.domainMaintenanceSettings()
	for _, d := range domains {
		res := domainMaintenanceResource{Domain: d}
		var elapsed time.Duration
		if hasSettings {
			m, on, err := ss.GetDomainMaintenanceSettings(ctx, d, now)
			if err != nil {
				return out, err
			}
			if on {
				res = toDomainMaintenanceResource(m)
				elapsed = maintenance.Elapsed(m.StartedAt, now)
			}
		} else if on, err := rt.domainMaintenance.GetDomainMaintenance(ctx, d); err != nil {
			return out, err
		} else if on {
			res.Enabled = true
		}
		rt.decorateMaintenance(ctx, &res, now)
		if res.Enabled {
			out.Active++
		}
		out.Domains = append(out.Domains, appMaintenanceDomain{domainMaintenanceResource: res, ElapsedSeconds: int64(elapsed.Seconds())})
	}
	out.Enabled, out.All = out.Active > 0, len(domains) > 0 && out.Active == len(domains)
	return out, nil
}

// handleGetAppMaintenance handles GET /api/v1/apps/{name}/maintenance:
// maintenance state of every domain of the app. AbilityRead.
func (rt *Router) handleGetAppMaintenance(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	res, err := rt.appMaintenanceState(r.Context(), app, domains, time.Now())
	if err != nil {
		rt.internalError(w, "api: get app maintenance failed", err, slog.String("app", app))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSetAppMaintenance handles PUT /api/v1/apps/{name}/maintenance:
// enables maintenance on every domain of the app, all or none. AbilityDeploy.
func (rt *Router) handleSetAppMaintenance(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	var req domainMaintenanceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(domains) == 0 {
		writeError(w, http.StatusBadRequest, "the app has no domains")
		return
	}
	if _, fail := rt.enableMaintenance(r.Context(), app, domains, req, rt.maintenanceActor(r)); fail != nil {
		rt.writeMaintenanceFailure(w, fail)
		return
	}
	res, err := rt.appMaintenanceState(r.Context(), app, domains, time.Now())
	if err != nil {
		rt.internalError(w, "api: app maintenance state failed", err, slog.String("app", app))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// appClearFailure lists the domains an app-wide clear could not end; a retry finishes the job.
type appClearFailure struct {
	Error   string   `json:"error"`
	Cleared []string `json:"cleared"`
	Failed  []string `json:"failed"`
}

// handleClearAppMaintenance handles DELETE /api/v1/apps/{name}/maintenance:
// ends maintenance on every domain of the app. Idempotent. AbilityDeploy.
func (rt *Router) handleClearAppMaintenance(w http.ResponseWriter, r *http.Request) {
	app, domains, ok := rt.appDomains(w, r)
	if !ok {
		return
	}
	actor := rt.maintenanceActor(r)
	fail := appClearFailure{Cleared: []string{}, Failed: []string{}}
	for _, d := range domains {
		if err := rt.clearMaintenance(r.Context(), app, d, actor); err != nil {
			rt.logger.Error("api: clear app maintenance failed", slog.String("domain", d), slog.String("error", err.Error()))
			fail.Failed = append(fail.Failed, d)
			continue
		}
		fail.Cleared = append(fail.Cleared, d)
	}
	if len(fail.Failed) > 0 {
		fail.Error = "maintenance could not be ended on every domain; retry to finish"
		writeJSON(w, http.StatusInternalServerError, fail)
		return
	}
	res, err := rt.appMaintenanceState(r.Context(), app, domains, time.Now())
	if err != nil {
		rt.internalError(w, "api: app maintenance state failed", err, slog.String("app", app))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
