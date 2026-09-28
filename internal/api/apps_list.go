package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const appListMaxLimit = 500

var errNoFilteredList = errors.New("api: app store does not support filtered listing")

// appFilterStore is the optional filtered-list surface; *store.DB has it.
type appFilterStore interface {
	ListDesiredServicesFiltered(ctx context.Context, f store.AppListFilter) ([]store.DesiredService, int, error)
	ListAppNamesFiltered(ctx context.Context, f store.AppListFilter) ([]string, error)
}

func parseAppListFilter(r *http.Request) store.AppListFilter {
	q := r.URL.Query()
	f := store.AppListFilter{
		Query:       q.Get("q"),
		ProjectID:   q.Get("project"),
		Environment: q.Get("environment"),
	}
	seen := map[string]bool{}
	for _, raw := range q["tag"] {
		for _, t := range strings.Split(raw, ",") {
			t = strings.ToLower(strings.TrimSpace(t))
			if t != "" && !seen[t] {
				seen[t] = true
				f.Tags = append(f.Tags, t)
			}
		}
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		f.Limit = min(n, appListMaxLimit)
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n > 0 {
		f.Offset = n
	}
	return f
}

// callerAppVisibility returns a per-app read predicate and whether the
// caller has any IAM policies at all (none means every app is visible
// to a caller holding the base read ability, so DB paging is exact).
func (rt *Router) callerAppVisibility(r *http.Request) (canRead func(string) bool, filtered bool, err error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, false, err
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return nil, false, err
	}
	return func(app string) bool {
		return authorizeResource(abilities, policies, AbilityRead, "app:"+app)
	}, len(policies) > 0, nil
}

// handleListApps handles GET /api/v1/apps with optional q, project,
// environment, tag (repeatable, AND), limit and offset. Without limit the
// full filtered set returns as before; X-Total-Count always carries the
// match count before paging.
func (rt *Router) handleListApps(w http.ResponseWriter, r *http.Request) {
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		rt.internalError(w, "api: list apps: store lacks filtered listing", errNoFilteredList)
		return
	}
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: list apps: resolve caller failed", err)
		return
	}
	f := parseAppListFilter(r)
	dbFilter := f
	if filtered {
		dbFilter.Limit, dbFilter.Offset = 0, 0
	}
	svcs, total, err := fs.ListDesiredServicesFiltered(r.Context(), dbFilter)
	if err != nil {
		rt.internalError(w, "api: list apps failed", err)
		return
	}
	if filtered {
		visible := svcs[:0:0]
		for _, s := range svcs {
			if canRead(s.Name) {
				visible = append(visible, s)
			}
		}
		total = len(visible)
		svcs = pageSlice(visible, f.Limit, f.Offset)
	}

	controllerNames := make([]string, len(svcs))
	appNames := make([]string, len(svcs))
	for i, s := range svcs {
		controllerNames[i] = applicationControllerName(s.Name)
		appNames[i] = s.Name
	}
	conditionsByController, err := rt.deploys.GetConditionsForControllers(r.Context(), controllerNames)
	if err != nil {
		rt.internalError(w, "api: list apps: batch load conditions failed", err)
		return
	}
	tagsByApp, err := rt.tags.ListTagsForApps(r.Context(), appNames)
	if err != nil {
		rt.internalError(w, "api: list apps: batch load tags failed", err)
		return
	}
	envNames := rt.environmentNames(r.Context(), svcs)

	out := make([]appListResource, 0, len(svcs))
	for _, s := range svcs {
		resource := toAppResource(s)
		resource.Tags = tagNamesFromStoreTags(tagsByApp[s.Name])
		out = append(out, appListResource{
			appResource:     resource,
			Status:          summarizeAppConditions(conditionsByController[applicationControllerName(s.Name)]),
			EnvironmentName: envNames[s.EnvironmentID],
		})
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	writeJSON(w, http.StatusOK, out)
}

func pageSlice(in []store.DesiredService, limit, offset int) []store.DesiredService {
	if offset >= len(in) {
		return nil
	}
	in = in[offset:]
	if limit > 0 && limit < len(in) {
		in = in[:limit]
	}
	return in
}

// environmentNames maps environment IDs used by svcs to their names.
func (rt *Router) environmentNames(ctx context.Context, svcs []store.DesiredService) map[string]string {
	out := map[string]string{}
	for _, s := range svcs {
		if s.EnvironmentID == "" {
			continue
		}
		if _, done := out[s.EnvironmentID]; done {
			continue
		}
		env, err := rt.environments.GetEnvironment(ctx, s.EnvironmentID)
		if err != nil {
			continue
		}
		out[s.EnvironmentID] = env.Name
	}
	return out
}

type appsSummaryResource struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Failing   int `json:"failing"`
	Deploying int `json:"deploying"`
	Stopped   int `json:"stopped"`
	Unknown   int `json:"unknown"`
}

// handleAppsSummary handles GET /api/v1/apps-summary: status counts over
// the same filters as the list, from names plus one batched conditions
// query, never loading service graphs.
func (rt *Router) handleAppsSummary(w http.ResponseWriter, r *http.Request) {
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		rt.internalError(w, "api: apps summary: store lacks filtered listing", errNoFilteredList)
		return
	}
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: apps summary: resolve caller failed", err)
		return
	}
	f := parseAppListFilter(r)
	f.Limit, f.Offset = 0, 0
	names, err := fs.ListAppNamesFiltered(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: apps summary failed", err)
		return
	}
	controllers := make([]string, 0, len(names))
	visible := names[:0:0]
	for _, n := range names {
		if canRead(n) {
			visible = append(visible, n)
			controllers = append(controllers, applicationControllerName(n))
		}
	}
	conds, err := rt.deploys.GetConditionsForControllers(r.Context(), controllers)
	if err != nil {
		rt.internalError(w, "api: apps summary: batch load conditions failed", err)
		return
	}
	sum := appsSummaryResource{Total: len(visible)}
	for _, n := range visible {
		switch summarizeAppConditions(conds[applicationControllerName(n)]).Label {
		case "Healthy":
			sum.Running++
		case "Attention needed":
			sum.Failing++
		case "Reconciling":
			sum.Deploying++
		case "Stopped":
			sum.Stopped++
		default:
			sum.Unknown++
		}
	}
	writeJSON(w, http.StatusOK, sum)
}
