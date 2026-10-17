package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appGroupResource is the GET /api/v1/apps/{name}/group response: a service, its siblings, and one rollup status.
type appGroupResource struct {
	AppID    string           `json:"app_id,omitempty"`
	Services []appResource    `json:"services"`
	Status   appStatusSummary `json:"status"`
}

// handleGetAppGroup also accepts an app name, since multi-service deploys return that instead of a service name.
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

// appGroupByName resolves a group by app name when no single service matches.
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
