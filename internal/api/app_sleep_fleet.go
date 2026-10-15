package api

import (
	"log/slog"
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
)

func init() {
	tenantRoutes["GET /api/v1/sleep"] = true
}

type sleepFleetApp struct {
	Name        string `json:"name"`
	IdleMinutes int    `json:"idle_minutes"`
	Sleeping    bool   `json:"sleeping"`
	// MemoryLimitBytes is the app's configured limit, 0 when it has none.
	MemoryLimitBytes int64 `json:"memory_limit_bytes"`
}

type sleepFleetResource struct {
	Enabled  int `json:"enabled"`
	Sleeping int `json:"sleeping"`
	// ReclaimedBytes sums the memory limits of apps that are asleep right now.
	ReclaimedBytes int64           `json:"reclaimed_bytes"`
	Apps           []sleepFleetApp `json:"apps"`
}

// handleSleepFleet handles GET /api/v1/sleep: every app with sleeping on that
// the caller can read, and what the asleep ones are not using.
func (rt *Router) handleSleepFleet(w http.ResponseWriter, r *http.Request) {
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: sleep fleet: resolve caller", err)
		return
	}
	rows, err := rt.appSleep.ListAppSleep(r.Context())
	if err != nil {
		rt.internalError(w, "api: sleep fleet: list sleep settings", err)
		return
	}
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.internalError(w, "api: sleep fleet: list apps", err)
		return
	}
	out := buildSleepFleet(rows, svcs, canRead)
	rt.logger.Debug("api: sleep fleet", slog.Int("enabled", out.Enabled), slog.Int("sleeping", out.Sleeping))
	writeJSON(w, http.StatusOK, out)
}

func buildSleepFleet(rows []store.AppSleep, svcs []store.DesiredService, canRead func(string) bool) sleepFleetResource {
	limits := make(map[string]int64, len(svcs))
	for _, s := range svcs {
		if s.Resources != nil {
			limits[s.Name] = s.Resources.MemoryBytes
		}
	}
	out := sleepFleetResource{Apps: []sleepFleetApp{}}
	for _, a := range rows {
		if a.IdleMinutes <= 0 || !canRead(a.ServiceName) {
			continue
		}
		item := sleepFleetApp{Name: a.ServiceName, IdleMinutes: a.IdleMinutes, Sleeping: a.Sleeping, MemoryLimitBytes: limits[a.ServiceName]}
		out.Enabled++
		if a.Sleeping {
			out.Sleeping++
			out.ReclaimedBytes += item.MemoryLimitBytes
		}
		out.Apps = append(out.Apps, item)
	}
	sort.Slice(out.Apps, func(i, j int) bool {
		if out.Apps[i].Sleeping != out.Apps[j].Sleeping {
			return out.Apps[i].Sleeping
		}
		return out.Apps[i].Name < out.Apps[j].Name
	})
	return out
}

// appListSleep is the sleep state the apps list carries per row.
type appListSleep struct {
	IdleMinutes int  `json:"idle_minutes"`
	Sleeping    bool `json:"sleeping"`
}

func sleepByApp(rows []store.AppSleep) map[string]appListSleep {
	m := make(map[string]appListSleep, len(rows))
	for _, a := range rows {
		if a.IdleMinutes > 0 {
			m[a.ServiceName] = appListSleep{IdleMinutes: a.IdleMinutes, Sleeping: a.Sleeping}
		}
	}
	return m
}
