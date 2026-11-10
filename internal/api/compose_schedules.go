package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
)

// registerComposeScheduleRoutes serves schedules that run a command inside one
// member service of a compose stack, through that member's job and exec path.
func (rt *Router) registerComposeScheduleRoutes(mux *http.ServeMux) {
	base := "/api/v1/apps/{name}/compose/services/{service}/schedules"
	route := func(pattern, ability string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, rt.requireAbilityForResource(ability, appResourceFromPath, rt.composeScheduleTarget(ability, h)))
	}
	mux.HandleFunc("GET /api/v1/apps/{name}/compose/schedules", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListComposeSchedules))
	route("POST "+base, AbilityRoot, rt.handleCreateScheduledTask)
	route("GET "+base, AbilityRead, rt.handleListScheduledTasks)
	route("GET "+base+"/{id}", AbilityRead, rt.handleGetScheduledTask)
	route("DELETE "+base+"/{id}", AbilityWrite, rt.handleDeleteScheduledTask)
	route("POST "+base+"/{id}/run", AbilityRoot, rt.handleRunScheduledTaskNow)
	route("POST "+base+"/{id}/enable", AbilityRoot, rt.handleSetScheduledTaskEnabled(true))
	route("POST "+base+"/{id}/disable", AbilityWrite, rt.handleSetScheduledTaskEnabled(false))
	route("GET "+base+"/{id}/runs", AbilityRead, rt.handleListScheduledTaskRuns)
	route("GET "+base+"/{id}/runs/{run}", AbilityRead, rt.handleGetScheduledTaskRun)
}

// composeScheduleCtxKey marks a request routed through composeScheduleTarget.
type composeScheduleCtxKey struct{}

// isComposeScheduleRequest reports whether r came in on a compose schedule route.
func isComposeScheduleRequest(r *http.Request) bool {
	v, _ := r.Context().Value(composeScheduleCtxKey{}).(bool)
	return v
}

// composeScheduleNotFound is shared by an unknown stack, an unknown service and
// one the caller cannot read, so a probe learns nothing about what exists.
const composeScheduleNotFound = "app not found"

// composeScheduleTarget resolves {service} to its member app, checks the caller
// may read the stack and that member and holds required on the member, then
// hands the member name to a single-app job handler.
func (rt *Router) composeScheduleTarget(required string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stack, key := r.PathValue("name"), r.PathValue("service")
		member, ok := rt.composeScheduleMember(w, r, stack, key)
		if !ok {
			return
		}
		abilities, policies, scope, err := rt.callerScope(r)
		if err != nil {
			rt.internalError(w, "api: compose schedule: resolve caller failed", err, slog.String("app", stack))
			return
		}
		if !scope.authorize(abilities, policies, required, resourcePrefixApp, member) {
			writeError(w, http.StatusForbidden, "your account lacks the required ability on service "+key+" (needs "+required+")")
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), composeScheduleCtxKey{}, true))
		r.SetPathValue("name", member)
		next(w, r)
	}
}

// composeScheduleMember returns the member app named key inside stack, writing
// the shared 404 when the stack, the key or either read check fails.
func (rt *Router) composeScheduleMember(w http.ResponseWriter, r *http.Request, stack, key string) (string, bool) {
	if rt.appGroups == nil || key == "" || !rt.callerCanReadApp(r, stack) {
		writeError(w, http.StatusNotFound, composeScheduleNotFound)
		return "", false
	}
	services, err := rt.appGroups.ListServicesByApp(r.Context(), stack)
	if err != nil {
		rt.internalError(w, "api: compose schedule: list services failed", err, slog.String("app", stack))
		return "", false
	}
	for _, s := range services {
		if s.Name != stack && strings.TrimPrefix(s.Name, stack+"-") == key && rt.callerCanReadApp(r, s.Name) {
			return s.Name, true
		}
	}
	writeError(w, http.StatusNotFound, composeScheduleNotFound)
	return "", false
}

// composeScheduleResource is one member's job, tagged with its compose key.
type composeScheduleResource struct {
	Service string `json:"service"`
	scheduledTaskResource
}

// handleListComposeSchedules serves GET /api/v1/apps/{name}/compose/schedules:
// every schedule of every member the caller can read, for the stack's card.
func (rt *Router) handleListComposeSchedules(w http.ResponseWriter, r *http.Request) {
	stack := r.PathValue("name")
	if rt.appGroups == nil || !rt.callerCanReadApp(r, stack) {
		writeError(w, http.StatusNotFound, composeScheduleNotFound)
		return
	}
	services, err := rt.appGroups.ListServicesByApp(r.Context(), stack)
	if err != nil {
		rt.internalError(w, "api: compose schedules: list services failed", err, slog.String("app", stack))
		return
	}
	if len(services) == 0 || services[0].Name == stack {
		writeError(w, http.StatusNotFound, composeScheduleNotFound)
		return
	}
	out := []composeScheduleResource{}
	for _, s := range services {
		if !rt.callerCanReadApp(r, s.Name) {
			continue
		}
		tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), s.Name)
		if err != nil {
			rt.internalError(w, "api: compose schedules: list jobs failed", err, slog.String("name", s.Name))
			return
		}
		key := strings.TrimPrefix(s.Name, stack+"-")
		for _, t := range tasks {
			if isOneShotTask(t) {
				continue
			}
			out = append(out, composeScheduleResource{Service: key, scheduledTaskResource: rt.readableJobResource(r, t)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": out})
}
