package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/store"
)

func init() {
	tenantRoutes["GET /api/v1/apps/sleep/overview"] = true
}

const (
	maxSleepIgnorePaths   = 50
	maxSleepIgnorePathLen = 200
)

// SleepActivity is the scheduler's latest view of each app's traffic.
type SleepActivity interface {
	Snapshot(name string) (appsleep.Snapshot, bool)
}

// SetSleepRuntime wires the live port wake manager and the scheduler's
// activity view into the sleep routes. Either may be nil.
func (rt *Router) SetSleepRuntime(pw appsleep.PortWaker, act SleepActivity) {
	rt.sleepPortWake, rt.sleepActivity = pw, act
}

// AppReadyAfter reports whether name's application controller recorded a
// Ready condition after since; port wake uses it to know a woken app is up.
func (rt *Router) AppReadyAfter(ctx context.Context, name string, since time.Time) bool {
	return !rt.readyConditionAfter(ctx, applicationControllerName(name), since).IsZero()
}

type sleepFix struct {
	Kind   string         `json:"kind"`
	Label  string         `json:"label"`
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body,omitempty"`
}

type sleepItem struct {
	Code   string    `json:"code"`
	Title  string    `json:"title"`
	Detail string    `json:"detail"`
	Fix    *sleepFix `json:"fix,omitempty"`
}

type sleepSettings struct {
	IdleMinutes  int      `json:"idle_minutes"`
	HoldRequests bool     `json:"hold_requests"`
	IgnorePaths  []string `json:"ignore_paths"`
	PortWake     bool     `json:"port_wake"`
}

type sleepPortWake struct {
	Supported bool  `json:"supported"`
	Active    bool  `json:"active"`
	Ports     []int `json:"ports"`
}

type sleepActivityView struct {
	LastRequestAt         *time.Time `json:"last_request_at,omitempty"`
	LastNetworkActivityAt *time.Time `json:"last_network_activity_at,omitempty"`
	OpenConnections       int        `json:"open_connections"`
	IdleForSeconds        int64      `json:"idle_for_seconds"`
	WillSleepAt           *time.Time `json:"will_sleep_at,omitempty"`
}

func toSleepItems(in []appsleep.Item) []sleepItem {
	out := make([]sleepItem, 0, len(in))
	for _, i := range in {
		it := sleepItem{Code: i.Code, Title: i.Title, Detail: i.Detail}
		if i.Fix != nil {
			f := sleepFix(*i.Fix)
			it.Fix = &f
		}
		out = append(out, it)
	}
	return out
}

// sleepFleet loads the one-shot fleet snapshot the eligibility check reads.
func (rt *Router) sleepFleet(ctx context.Context) (*appsleep.Fleet, error) {
	return appsleep.Load(ctx, sleepFleetSource{rt})
}

type sleepFleetSource struct{ rt *Router }

func (s sleepFleetSource) ListDesiredServices(ctx context.Context) ([]store.DesiredService, error) {
	return s.rt.apps.ListDesiredServices(ctx)
}

func (s sleepFleetSource) ListNodes(ctx context.Context) ([]store.Node, error) {
	if s.rt.nodes == nil {
		return nil, nil
	}
	return s.rt.nodes.ListNodes(ctx)
}

func (s sleepFleetSource) ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error) {
	if s.rt.canaries == nil {
		return nil, nil
	}
	return s.rt.canaries.ListCanaryReleases(ctx)
}

func (s sleepFleetSource) ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error) {
	if s.rt.databases == nil {
		return nil, nil
	}
	return s.rt.databases.ListDesiredDatabases(ctx)
}

func (s sleepFleetSource) CountAppStreamsByService(ctx context.Context) (map[string]int, error) {
	if c, ok := s.rt.appSleep.(interface {
		CountAppStreamsByService(ctx context.Context) (map[string]int, error)
	}); ok {
		return c.CountAppStreamsByService(ctx)
	}
	return map[string]int{}, nil
}

// sleepResource builds the full GET response for svc under setting.
func (rt *Router) sleepResource(ctx context.Context, fleet *appsleep.Fleet, svc *store.DesiredService, setting store.AppSleep, now time.Time) appSleepResource {
	res := toAppSleepResource(setting)
	elig := appsleep.Evaluate(fleet.Facts(svc, setting, rt.sleepPortWake))
	res.Blockers, res.Warnings, res.Eligible = toSleepItems(elig.Blockers), toSleepItems(elig.Warnings), elig.Eligible()
	if len(elig.Blockers) > 0 {
		res.BlockedReason = elig.Blockers[0].Detail
	}
	ignore := setting.IgnorePaths
	if ignore == nil {
		ignore = []string{}
	}
	res.Settings = sleepSettings{IdleMinutes: setting.IdleMinutes, HoldRequests: setting.HoldRequests, IgnorePaths: appsleep.EffectiveIgnorePaths(svc, ignore), PortWake: setting.PortWake}
	res.PortWake = sleepPortWake{Ports: []int{}}
	if ports := appsleep.PinnedPorts(svc); len(ports) > 0 && rt.sleepPortWake != nil {
		res.PortWake.Supported, _ = rt.sleepPortWake.Supported(svc)
		st := rt.sleepPortWake.Status(svc.Name)
		res.PortWake.Active, res.PortWake.Ports = st.Active, ports
	}
	res.Activity = rt.sleepActivityFor(setting, now)
	if svc.Resources != nil && svc.Resources.MemoryBytes > 0 {
		v := svc.Resources.MemoryBytes
		res.EstimatedMemoryFreedBytes = &v
	}
	if s := rt.sleepEvents(); s != nil {
		if st, err := s.GetSleepStats(ctx, setting.ServiceName, now); err == nil {
			res.ColdStartMsP50 = st.P50WakeLatencyMs
		}
	}
	return res
}

func (rt *Router) sleepActivityFor(setting store.AppSleep, now time.Time) sleepActivityView {
	var v sleepActivityView
	if setting.IdleMinutes <= 0 {
		return v
	}
	if setting.Sleeping {
		v.IdleForSeconds = int64(now.Sub(setting.Since) / time.Second)
		return v
	}
	if rt.sleepActivity != nil {
		if snap, ok := rt.sleepActivity.Snapshot(setting.ServiceName); ok {
			if !snap.LastRequest.IsZero() {
				t := snap.LastRequest
				v.LastRequestAt = &t
			}
			if !snap.LastNetwork.IsZero() {
				t := snap.LastNetwork
				v.LastNetworkActivityAt = &t
			}
			v.OpenConnections = snap.OpenConnections
			v.IdleForSeconds = int64(snap.Decision.IdleFor / time.Second)
			if !snap.Decision.WillSleepAt.IsZero() {
				t := snap.Decision.WillSleepAt
				v.WillSleepAt = &t
			}
			return v
		}
	}
	v.IdleForSeconds = int64(now.Sub(setting.Since) / time.Second)
	t := setting.Since.Add(time.Duration(setting.IdleMinutes) * time.Minute)
	v.WillSleepAt = &t
	return v
}

// validateIgnorePaths trims and checks the operator's probe routes.
func validateIgnorePaths(in []string) ([]string, string) {
	if len(in) > maxSleepIgnorePaths {
		return nil, "ignore_paths can hold at most 50 paths"
	}
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
			continue
		case len(p) > maxSleepIgnorePathLen:
			return nil, "an ignore path is too long"
		case !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "? \t\n"):
			return nil, "ignore_paths must be URL paths such as /healthz, without a query string"
		}
		out = append(out, p)
	}
	return out, ""
}

type sleepOverviewItem struct {
	Name                      string     `json:"name"`
	Enabled                   bool       `json:"enabled"`
	Sleeping                  bool       `json:"sleeping"`
	Eligible                  bool       `json:"eligible"`
	IdleMinutes               int        `json:"idle_minutes"`
	Since                     *time.Time `json:"since,omitempty"`
	WillSleepAt               *time.Time `json:"will_sleep_at,omitempty"`
	BlockerCodes              []string   `json:"blocker_codes"`
	WarningCodes              []string   `json:"warning_codes"`
	PortWakeActive            bool       `json:"port_wake_active"`
	EstimatedMemoryFreedBytes *int64     `json:"estimated_memory_freed_bytes"`
}

type sleepAppsOverview struct {
	Enabled  int                 `json:"enabled"`
	Sleeping int                 `json:"sleeping"`
	Eligible int                 `json:"eligible"`
	Apps     []sleepOverviewItem `json:"apps"`
}

// handleAppsSleepOverview handles GET /api/v1/apps/sleep/overview: every app
// that has sleep on or could, from one pass over the fleet.
func (rt *Router) handleAppsSleepOverview(w http.ResponseWriter, r *http.Request) {
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: apps sleep overview: resolve caller", err)
		return
	}
	fleet, err := rt.sleepFleet(r.Context())
	if err != nil {
		rt.internalError(w, "api: apps sleep overview: load fleet", err)
		return
	}
	rows, err := rt.appSleep.ListAppSleep(r.Context())
	if err != nil {
		rt.internalError(w, "api: apps sleep overview: list settings", err)
		return
	}
	settings := make(map[string]store.AppSleep, len(rows))
	for _, a := range rows {
		settings[a.ServiceName] = a
	}
	writeJSON(w, http.StatusOK, rt.buildAppsSleepOverview(fleet, settings, canRead, time.Now()))
}

func (rt *Router) buildAppsSleepOverview(fleet *appsleep.Fleet, settings map[string]store.AppSleep, canRead func(string) bool, now time.Time) sleepAppsOverview {
	out := sleepAppsOverview{Apps: []sleepOverviewItem{}}
	for name, svc := range fleet.Services {
		if !canRead(name) || store.IsCanaryService(name) {
			continue
		}
		svc := svc
		setting, enabled := settings[name]
		if !enabled {
			setting = store.AppSleep{ServiceName: name, PortWake: true}
		}
		elig := appsleep.Evaluate(fleet.Facts(&svc, setting, rt.sleepPortWake))
		if !enabled && !elig.Eligible() {
			continue
		}
		item := sleepOverviewItem{Name: name, Enabled: enabled, Sleeping: setting.Sleeping, Eligible: elig.Eligible(), IdleMinutes: setting.IdleMinutes, BlockerCodes: itemCodes(elig.Blockers), WarningCodes: itemCodes(elig.Warnings)}
		if rt.sleepPortWake != nil {
			item.PortWakeActive = rt.sleepPortWake.Status(name).Active
		}
		if setting.Sleeping {
			since := setting.Since
			item.Since = &since
		}
		if enabled {
			item.WillSleepAt = rt.sleepActivityFor(setting, now).WillSleepAt
			out.Enabled++
		}
		if setting.Sleeping {
			out.Sleeping++
		}
		if item.Eligible {
			out.Eligible++
		}
		if svc.Resources != nil && svc.Resources.MemoryBytes > 0 {
			v := svc.Resources.MemoryBytes
			item.EstimatedMemoryFreedBytes = &v
		}
		out.Apps = append(out.Apps, item)
	}
	sort.Slice(out.Apps, func(i, j int) bool {
		a, b := out.Apps[i], out.Apps[j]
		if a.Sleeping != b.Sleeping {
			return a.Sleeping
		}
		if a.Enabled != b.Enabled {
			return a.Enabled
		}
		return a.Name < b.Name
	})
	return out
}

func itemCodes(in []appsleep.Item) []string {
	out := make([]string, 0, len(in))
	for _, i := range in {
		out = append(out, i.Code)
	}
	return out
}

type sleepRefusal struct {
	Error    string      `json:"error"`
	Blockers []sleepItem `json:"blockers"`
}
