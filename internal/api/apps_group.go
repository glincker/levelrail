package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appGroupResource is GET /api/v1/apps/{name}/group's wire shape: stage
// 1 of multi-service apps (migrations/0039_apps.sql), a service plus
// its sibling services under the same store.App, and one rollup status
// across all of them. A new, additive endpoint rather than a change to
// appResource/GET /api/v1/apps/{name}: that endpoint's existing callers
// depend on its single-service shape unchanged.
type appGroupResource struct {
	AppID    string           `json:"app_id,omitempty"`
	Services []appResource    `json:"services"`
	Status   appStatusSummary `json:"status"`
}

// handleGetAppGroup handles GET /api/v1/apps/{name}/group. name is
// ordinarily a member service's own name (the only identifier that
// existed before stage 2 added the "appName-serviceKey" convention), but
// every multi-service-creating endpoint (handleDeployCompose,
// handleDeploySpec) hands the caller back the app's own name instead
// (composeDeployResponse.AppID, deploySpecResponse.AppID), so a caller
// that naturally tries that name here first falls back to an app lookup
// rather than a misleading 404: see appGroupByName. A service with no
// AppID (created after migrations/0039_apps.sql, before any stage-1+
// caller assigns one) is its own one-service group, not a 404.
func (rt *Router) handleGetAppGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		rt.appGroupByName(w, r, name)
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app group: get service failed", err, slog.String("name", name))
		return
	}

	services := []store.DesiredService{*svc}
	if svc.AppID != "" {
		services, err = rt.appGroups.ListServicesByApp(r.Context(), svc.AppID)
		if err != nil {
			rt.internalError(w, "api: get app group: list services failed", err, slog.String("app_id", svc.AppID))
			return
		}
	}

	rt.writeAppGroup(w, r, svc.AppID, services)
}

// appGroupByName is handleGetAppGroup's fallback once name doesn't match
// any single service: name might still be the store.App itself (App.ID
// == App.Name, the convention every multi-service deploy path already
// uses), in which case its member services are exactly what a caller
// asking for "the group named X" actually wants. A name that matches
// neither a service nor an app is a genuine 404 either way.
func (rt *Router) appGroupByName(w http.ResponseWriter, r *http.Request, name string) {
	app, err := rt.appGroups.GetAppByName(r.Context(), name)
	if errors.Is(err, store.ErrAppNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app group: get app failed", err, slog.String("name", name))
		return
	}

	services, err := rt.appGroups.ListServicesByApp(r.Context(), app.ID)
	if err != nil {
		rt.internalError(w, "api: get app group: list services failed", err, slog.String("app_id", app.ID))
		return
	}
	if len(services) == 0 {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	rt.writeAppGroup(w, r, app.ID, services)
}

// writeAppGroup batch-loads services' reconcile conditions and writes
// the appGroupResource response, the shared tail of handleGetAppGroup's
// two lookup paths (by member service, by app name).
func (rt *Router) writeAppGroup(w http.ResponseWriter, r *http.Request, appID string, services []store.DesiredService) {
	controllerNames := make([]string, len(services))
	for i, s := range services {
		controllerNames[i] = applicationControllerName(s.Name)
	}
	conditionsByController, err := rt.deploys.GetConditionsForControllers(r.Context(), controllerNames)
	if err != nil {
		rt.internalError(w, "api: get app group: batch load conditions failed", err)
		return
	}

	out := make([]appResource, 0, len(services))
	var allConditions []reconcile.Condition
	for _, s := range services {
		out = append(out, toAppResource(s))
		allConditions = append(allConditions, conditionsByController[applicationControllerName(s.Name)]...)
	}

	writeJSON(w, http.StatusOK, appGroupResource{
		AppID:    appID,
		Services: out,
		Status:   summarizeAppConditions(allConditions),
	})
}
