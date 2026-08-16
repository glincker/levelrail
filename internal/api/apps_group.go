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

// handleGetAppGroup handles GET /api/v1/apps/{name}/group. name is a
// service name, the only identifier that exists before stage 2 adds the
// "appName-serviceKey" naming convention; a service with no AppID
// (created after migrations/0039_apps.sql, before any stage-1+ caller
// assigns one) is its own one-service group, not a 404.
func (rt *Router) handleGetAppGroup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
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
		AppID:    svc.AppID,
		Services: out,
		Status:   summarizeAppConditions(allConditions),
	})
}
