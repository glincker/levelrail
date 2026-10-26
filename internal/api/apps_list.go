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

// appHeadStore is the optional light-projection surface; *store.DB has it.
type appHeadStore interface {
	ListServiceHeads(ctx context.Context, f store.AppListFilter) ([]store.DesiredService, error)
}

// listAppHeads returns services with only the fields status bucketing,
// summaries and coverage need, falling back to the full load for stores
// without the projection.
func listAppHeads(ctx context.Context, apps any, f store.AppListFilter) ([]store.DesiredService, error) {
	if hs, ok := apps.(appHeadStore); ok {
		return hs.ListServiceHeads(ctx, f)
	}
	if fs, ok := apps.(appFilterStore); ok {
		svcs, _, err := fs.ListDesiredServicesFiltered(ctx, f)
		return svcs, err
	}
	return nil, errNoFilteredList
}

func parseAppListFilter(r *http.Request) store.AppListFilter {
	q := r.URL.Query()
	f := store.AppListFilter{
		Query:     q.Get("q"),
		ProjectID: q.Get("project"),
	}
	var envs []string
	for _, raw := range q["environment"] {
		for _, e := range strings.Split(raw, ",") {
			if e = strings.TrimSpace(e); e != "" {
				envs = append(envs, e)
			}
		}
	}
	if len(envs) == 1 {
		f.Environment = envs[0]
	} else {
		f.Environments = envs
	}
	f.AnyTag = q.Get("tag_match") == "any"
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
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return nil, false, err
	}
	return func(app string) bool {
		return scope.authorize(abilities, policies, AbilityRead, resourcePrefixApp, app)
	}, len(policies) > 0 || scope.restricted(), nil
}

// X-Total-Count is the match count before paging; without limit the full filtered set is returned.
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
	if bucket := r.URL.Query().Get("status"); bucket != "" {
		canRead, filtered, err = rt.restrictToStatus(r.Context(), fs, f, bucket, canRead)
		if err != nil {
			rt.internalError(w, "api: list apps: status filter failed", err)
			return
		}
	}
	rank := rt.rankedAppOrder(r, fs, f)
	var (
		svcs  []store.DesiredService
		total int
	)
	if rank != nil {
		svcs, total, err = listRankedApps(r.Context(), fs, f, rank, canRead)
	} else {
		dbFilter := f
		if filtered {
			dbFilter.Limit, dbFilter.Offset = 0, 0
		}
		svcs, total, err = fs.ListDesiredServicesFiltered(r.Context(), dbFilter)
		if err == nil && filtered {
			visible := svcs[:0:0]
			for _, s := range svcs {
				if canRead(s.Name) {
					visible = append(visible, s)
				}
			}
			total = len(visible)
			svcs = pageSlice(visible, f.Limit, f.Offset)
		}
	}
	if err != nil {
		rt.internalError(w, "api: list apps failed", err)
		return
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
	envSettings, envSettingsOK := rt.appOverridesForList(r.Context())

	var sleepRows map[string]appListSleep
	if rows, err := rt.appSleep.ListAppSleep(r.Context()); err == nil {
		sleepRows = sleepByApp(rows)
	}

	placement := rt.placementResolver(r.Context())
	owners := rt.loadStackOwners(r.Context())
	out := make([]appListResource, 0, len(svcs))
	for _, s := range svcs {
		resource := toAppResourceRaw(s)
		placement.stampApp(&resource)
		redactListedApp(&resource, s, envSettings, envSettingsOK)
		resource.Tags = tagNamesFromStoreTags(tagsByApp[s.Name])
		conditions := conditionsByController[applicationControllerName(s.Name)]
		if cond := rt.crossNodeIngressAppCondition(r.Context(), s); cond != nil {
			conditions = append(conditions, *cond)
		}
		item := appListResource{
			appResource:     resource,
			Status:          summarizeAppConditions(conditions),
			EnvironmentName: envNames[s.EnvironmentID],
			ManagedByStack:  owners.of("App", "App/"+s.Name),
		}
		if sl, ok := sleepRows[s.Name]; ok {
			item.Sleep = &sl
		}
		out = append(out, item)
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

// environmentNames maps environment IDs used by svcs to their names,
// via one batched GetEnvironmentsByIDs call instead of a GetEnvironment
// call per distinct environment (the N+1 handleListApps used to make).
func (rt *Router) environmentNames(ctx context.Context, svcs []store.DesiredService) map[string]string {
	seen := map[string]bool{}
	ids := make([]string, 0, len(svcs))
	for _, s := range svcs {
		if s.EnvironmentID == "" || seen[s.EnvironmentID] {
			continue
		}
		seen[s.EnvironmentID] = true
		ids = append(ids, s.EnvironmentID)
	}
	envs, err := rt.environments.GetEnvironmentsByIDs(ctx, ids)
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(envs))
	for id, env := range envs {
		out[id] = env.Name
	}
	return out
}

type appsSummaryResource struct {
	Total         int            `json:"total"`
	Running       int            `json:"running"`
	Failing       int            `json:"failing"`
	Deploying     int            `json:"deploying"`
	Stopped       int            `json:"stopped"`
	Attention     int            `json:"attention"`
	ByEnvironment map[string]int `json:"by_environment"`
}

// handleAppsSummary handles GET /api/v1/apps-summary: status and environment
// counts over the same filters and visibility as the list, so a dashboard
// can show tiles without downloading every app.
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
	svcs, err := listAppHeads(r.Context(), fs, f)
	if err != nil {
		rt.internalError(w, "api: apps summary failed", err)
		return
	}
	visible := svcs[:0:0]
	for _, s := range svcs {
		if canRead(s.Name) {
			visible = append(visible, s)
		}
	}
	buckets, err := rt.appStatusBuckets(r.Context(), visible)
	if err != nil {
		rt.internalError(w, "api: apps summary: batch load conditions failed", err)
		return
	}
	envNames := rt.environmentNames(r.Context(), visible)
	sum := appsSummaryResource{Total: len(visible), ByEnvironment: map[string]int{}}
	for _, s := range visible {
		switch buckets[s.Name] {
		case appBucketRunning:
			sum.Running++
		case appBucketFailing:
			sum.Failing++
		case appBucketStopped:
			sum.Stopped++
		default:
			sum.Deploying++
		}
		if name := envNames[s.EnvironmentID]; name != "" {
			sum.ByEnvironment[name]++
		}
	}
	sum.Attention = sum.Failing
	writeJSON(w, http.StatusOK, sum)
}
