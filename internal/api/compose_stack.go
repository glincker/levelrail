package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ComposeRevisionStore is the store surface compose stacks need for history and rollback.
type ComposeRevisionStore interface {
	AddComposeRevision(ctx context.Context, rev store.ComposeRevision) (int, error)
	ListComposeRevisions(ctx context.Context, appID string, limit int) ([]store.ComposeRevision, error)
	GetComposeRevision(ctx context.Context, appID string, revision int) (store.ComposeRevision, error)
	ListComposeApps(ctx context.Context) ([]string, error)
	DeleteComposeRevisions(ctx context.Context, appID string) error
}

// Stack phases, a rollup over every member service's Ready condition.
const (
	composePhaseRunning     = "running"
	composePhaseProgressing = "progressing"
	composePhaseDegraded    = "degraded"
	composePhaseFailed      = "failed"
	composePhaseStopped     = "stopped"
)

type composeDeployOpts struct {
	isTrial    bool
	profiles   []string
	source     string
	templateID string
}

type composeServiceStatus struct {
	Key              string   `json:"key"`
	Name             string   `json:"name"`
	Image            string   `json:"image"`
	Port             int      `json:"port,omitempty"`
	Domains          []string `json:"domains,omitempty"`
	DependsOn        []string `json:"depends_on,omitempty"`
	DependsOnHealthy []string `json:"depends_on_healthy,omitempty"`
	Volumes          []string `json:"volumes,omitempty"`
	Replicas         int      `json:"replicas"`
	Suspended        bool     `json:"suspended,omitempty"`
	Ready            string   `json:"ready"`
	Reason           string   `json:"reason,omitempty"`
	Message          string   `json:"message,omitempty"`
	Since            string   `json:"since,omitempty"`
}

type composeStackResource struct {
	AppID      string                 `json:"app_id"`
	Phase      string                 `json:"phase"`
	Revision   int                    `json:"revision,omitempty"`
	DeployedAt string                 `json:"deployed_at,omitempty"`
	Source     string                 `json:"source,omitempty"`
	TemplateID string                 `json:"template_id,omitempty"`
	Profiles   []string               `json:"profiles,omitempty"`
	Services   []composeServiceStatus `json:"services"`
}

func (rt *Router) composeRevisionStore() ComposeRevisionStore {
	s, _ := rt.apps.(ComposeRevisionStore)
	return s
}

func splitProfiles(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// recordComposeRevision is best effort: the stack already deployed, a
// missing history row only costs a rollback target.
func (rt *Router) recordComposeRevision(r *http.Request, appID string, body []byte, opts composeDeployOpts) int {
	revs := rt.composeRevisionStore()
	if revs == nil {
		return 0
	}
	n, err := revs.AddComposeRevision(r.Context(), store.ComposeRevision{
		AppID: appID, Compose: string(body), Profiles: opts.profiles, Source: opts.source,
		TemplateID: opts.templateID, CreatedBy: rt.pipelineActor(r),
	})
	if err != nil {
		rt.logger.Error("api: compose: record revision failed", slog.String("error", err.Error()), slog.String("app", appID))
		return 0
	}
	return n
}

// composeStackPhase rolls member conditions into one phase.
func composeStackPhase(services []composeServiceStatus) string {
	if len(services) == 0 {
		return composePhaseStopped
	}
	var ready, failed, suspended int
	for _, s := range services {
		switch {
		case s.Suspended:
			suspended++
		case s.Ready == string(reconcile.ConditionTrue):
			ready++
		case s.Ready == string(reconcile.ConditionFalse):
			failed++
		}
	}
	switch {
	case suspended == len(services):
		return composePhaseStopped
	case ready+suspended == len(services):
		return composePhaseRunning
	case failed == len(services):
		return composePhaseFailed
	case failed > 0:
		return composePhaseDegraded
	default:
		return composePhaseProgressing
	}
}

// loadComposeStack builds the stack view; ok is false (response written)
// when appID has no member services.
func (rt *Router) loadComposeStack(w http.ResponseWriter, r *http.Request, appID string) (composeStackResource, []store.DesiredService, bool) {
	services, err := rt.appGroups.ListServicesByApp(r.Context(), appID)
	if err != nil {
		rt.internalError(w, "api: compose: list services failed", err, slog.String("app", appID))
		return composeStackResource{}, nil, false
	}
	if len(services) == 0 {
		writeError(w, http.StatusNotFound, "compose app not found")
		return composeStackResource{}, nil, false
	}
	names := make([]string, len(services))
	for i, s := range services {
		names[i] = applicationControllerName(s.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(r.Context(), names)
	if err != nil {
		rt.internalError(w, "api: compose: load conditions failed", err, slog.String("app", appID))
		return composeStackResource{}, nil, false
	}
	out := composeStackResource{AppID: appID, Services: make([]composeServiceStatus, 0, len(services))}
	for _, s := range services {
		out.Services = append(out.Services, composeServiceView(appID, s, conds[applicationControllerName(s.Name)]))
	}
	sort.Slice(out.Services, func(i, j int) bool { return out.Services[i].Key < out.Services[j].Key })
	out.Phase = composeStackPhase(out.Services)
	if revs := rt.composeRevisionStore(); revs != nil {
		if rev, err := revs.GetComposeRevision(r.Context(), appID, 0); err == nil {
			out.Revision, out.Source, out.TemplateID, out.Profiles = rev.Revision, rev.Source, rev.TemplateID, rev.Profiles
			out.DeployedAt = rev.CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	return out, services, true
}

func composeServiceView(appID string, s store.DesiredService, conds []reconcile.Condition) composeServiceStatus {
	v := composeServiceStatus{
		Key: strings.TrimPrefix(s.Name, appID+"-"), Name: s.Name, Image: s.Image, Port: s.Port, Domains: s.Domains,
		DependsOn: s.DependsOn, DependsOnHealthy: s.DependsOnHealthy, Replicas: s.Replicas, Suspended: s.Suspended,
		Ready: string(reconcile.ConditionUnknown),
	}
	for _, vol := range s.Volumes {
		v.Volumes = append(v.Volumes, vol.Name+":"+vol.ContainerPath)
	}
	for _, c := range conds {
		if c.Type != reconcile.ConditionTypeReady {
			continue
		}
		v.Ready, v.Reason, v.Message = string(c.Status), c.Reason, c.Message
		if !c.LastTransitionTime.IsZero() {
			v.Since = c.LastTransitionTime.UTC().Format(time.RFC3339)
		}
	}
	return v
}

// handleGetComposeStack handles GET /api/v1/apps/{name}/compose.
func (rt *Router) handleGetComposeStack(w http.ResponseWriter, r *http.Request) {
	stack, _, ok := rt.loadComposeStack(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, stack)
}

type composeStackListItem struct {
	AppID      string `json:"app_id"`
	Phase      string `json:"phase"`
	Revision   int    `json:"revision"`
	Services   int    `json:"services"`
	DeployedAt string `json:"deployed_at,omitempty"`
	TemplateID string `json:"template_id,omitempty"`
}

// handleListComposeStacks handles GET /api/v1/compose: every app deployed from a compose file.
func (rt *Router) handleListComposeStacks(w http.ResponseWriter, r *http.Request) {
	revs := rt.composeRevisionStore()
	if revs == nil {
		writeError(w, http.StatusNotImplemented, "compose history is not available on this store")
		return
	}
	ids, err := revs.ListComposeApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: compose: list stacks failed", err)
		return
	}
	out := make([]composeStackListItem, 0, len(ids))
	for _, id := range ids {
		services, err := rt.appGroups.ListServicesByApp(r.Context(), id)
		if err != nil {
			rt.internalError(w, "api: compose: list services failed", err, slog.String("app", id))
			return
		}
		if len(services) == 0 {
			continue
		}
		item := composeStackListItem{AppID: id, Services: len(services)}
		names := make([]string, len(services))
		for i, s := range services {
			names[i] = applicationControllerName(s.Name)
		}
		conds, err := rt.deploys.GetConditionsForControllers(r.Context(), names)
		if err != nil {
			rt.internalError(w, "api: compose: load conditions failed", err, slog.String("app", id))
			return
		}
		views := make([]composeServiceStatus, 0, len(services))
		for _, s := range services {
			views = append(views, composeServiceView(id, s, conds[applicationControllerName(s.Name)]))
		}
		item.Phase = composeStackPhase(views)
		if rev, err := revs.GetComposeRevision(r.Context(), id, 0); err == nil {
			item.Revision, item.TemplateID = rev.Revision, rev.TemplateID
			item.DeployedAt = rev.CreatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"stacks": out})
}

type composeRedeployRequest struct {
	Service string `json:"service,omitempty"`
}

// handleRedeployCompose handles POST /api/v1/apps/{name}/compose/redeploy:
// recreate every member's containers, or only body.service's.
func (rt *Router) handleRedeployCompose(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("name")
	var req composeRedeployRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	_, services, ok := rt.loadComposeStack(w, r, appID)
	if !ok {
		return
	}
	var restarted []string
	for _, s := range services {
		if req.Service != "" && s.Name != appID+"-"+req.Service {
			continue
		}
		if err := rt.apps.RestartService(r.Context(), s.Name); err != nil {
			rt.internalError(w, "api: compose: redeploy service failed", err, slog.String("service", s.Name))
			return
		}
		restarted = append(restarted, s.Name)
	}
	if len(restarted) == 0 {
		writeError(w, http.StatusNotFound, "service "+req.Service+" is not part of this compose app")
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusAccepted, map[string]any{"app_id": appID, "restarted": restarted})
}

// handleComposeDown handles DELETE /api/v1/apps/{name}/compose: removes
// every member, its containers and history. Named volumes are kept unless
// ?volumes=true, which additionally needs AbilityWriteSensitive.
func (rt *Router) handleComposeDown(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("name")
	removeVolumes := r.URL.Query().Get("volumes") == "true"
	if removeVolumes && !rt.callerHasAbility(r, AbilityWriteSensitive) {
		writeError(w, http.StatusForbidden, "removing volumes deletes data and requires the write-sensitive ability")
		return
	}
	_, services, ok := rt.loadComposeStack(w, r, appID)
	if !ok {
		return
	}
	resp := map[string]any{"app_id": appID}
	var removed, pending, volumes, volumeErrors []string
	for _, s := range services {
		teardownErr, err := rt.deleteApp(r.Context(), s.Name)
		if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.internalError(w, "api: compose: down: delete service failed", err, slog.String("service", s.Name))
			return
		}
		removed = append(removed, s.Name)
		if teardownErr != nil {
			pending = append(pending, s.Name)
		}
		for _, v := range s.Volumes {
			volumes = append(volumes, v.Name)
		}
	}
	if removeVolumes && rt.orphanedVolumes != nil {
		var kept []string
		for _, v := range volumes {
			if err := rt.orphanedVolumes.RemoveVolume(r.Context(), v); err != nil {
				volumeErrors = append(volumeErrors, v+": "+err.Error())
				continue
			}
			kept = append(kept, v)
		}
		resp["volumes_removed"] = kept
	} else {
		resp["volumes_kept"] = volumes
	}
	if revs := rt.composeRevisionStore(); revs != nil {
		if err := revs.DeleteComposeRevisions(r.Context(), appID); err != nil {
			rt.logger.Error("api: compose: down: delete revisions failed", slog.String("error", err.Error()), slog.String("app", appID))
		}
	}
	rt.nudgeReconciler()
	resp["removed"] = removed
	if len(pending) > 0 {
		resp["teardown_pending"] = pending
	}
	if len(volumeErrors) > 0 {
		resp["volume_errors"] = volumeErrors
	}
	writeJSON(w, http.StatusOK, resp)
}
