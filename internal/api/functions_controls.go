package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/store"
)

type functionSettingsStore interface {
	GetFunctionSettings(ctx context.Context, serviceName string) (store.FunctionSettings, error)
	SetFunctionSettings(ctx context.Context, s store.FunctionSettings, now time.Time) error
}

func (rt *Router) functionSettingsStore() functionSettingsStore {
	s, _ := rt.appSleep.(functionSettingsStore)
	return s
}

func (rt *Router) loadFunctionSettings(ctx context.Context, name string) store.FunctionSettings {
	s := store.FunctionSettings{ServiceName: name}
	st := rt.functionSettingsStore()
	if st == nil {
		return s
	}
	got, err := st.GetFunctionSettings(ctx, name)
	if err != nil {
		rt.logger.Warn("api: load function settings failed", slog.String("function", name), slog.String("error", err.Error()))
		return s
	}
	return got
}

type functionLimitBounds struct {
	DefaultTimeoutSeconds int `json:"default_timeout_seconds"`
	MaxTimeoutSeconds     int `json:"max_timeout_seconds"`
	DefaultConcurrency    int `json:"default_concurrency"`
	MaxConcurrency        int `json:"max_concurrency"`
}

type functionSettingsResource struct {
	Name                    string              `json:"name"`
	Port                    int                 `json:"port"`
	IdleMinutes             int                 `json:"idle_minutes"`
	MemoryBytes             int64               `json:"memory_bytes"`
	NanoCPUs                int64               `json:"nano_cpus"`
	RequestTimeoutSeconds   int                 `json:"request_timeout_seconds"`
	MaxConcurrency          int                 `json:"max_concurrency"`
	EffectiveTimeoutSeconds int                 `json:"effective_timeout_seconds"`
	EffectiveConcurrency    int                 `json:"effective_concurrency"`
	Bounds                  functionLimitBounds `json:"bounds"`
}

type functionSettingsRequest struct {
	RequestTimeoutSeconds *int `json:"request_timeout_seconds,omitempty"`
	MaxConcurrency        *int `json:"max_concurrency,omitempty"`
}

// loadFunction answers 404 unless name is an app that sleeps and holds requests.
func (rt *Router) loadFunction(w http.ResponseWriter, r *http.Request) (*store.DesiredService, store.AppSleep, bool) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "function not found")
		return nil, store.AppSleep{}, false
	}
	if err != nil {
		rt.internalError(w, "api: function: load app failed", err, slog.String("function", name))
		return nil, store.AppSleep{}, false
	}
	sleep, err := rt.appSleep.GetAppSleep(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: function: load sleep settings failed", err, slog.String("function", name))
		return nil, store.AppSleep{}, false
	}
	if !sleep.HoldRequests || sleep.IdleMinutes <= 0 {
		writeError(w, http.StatusNotFound, "function not found")
		return nil, store.AppSleep{}, false
	}
	return svc, sleep, true
}

func functionSettingsView(svc *store.DesiredService, sleep store.AppSleep, s store.FunctionSettings) functionSettingsResource {
	lim := functions.LimitsFromEnv(nil)
	out := functionSettingsResource{
		Name: svc.Name, Port: svc.Port, IdleMinutes: sleep.IdleMinutes,
		RequestTimeoutSeconds: s.RequestTimeoutSeconds, MaxConcurrency: s.MaxConcurrency,
		EffectiveTimeoutSeconds: int(lim.Timeout(s) / time.Second), EffectiveConcurrency: lim.Concurrency(s),
		Bounds: functionLimitBounds{
			DefaultTimeoutSeconds: int(lim.DefaultTimeout / time.Second), MaxTimeoutSeconds: int(lim.TimeoutCeiling / time.Second),
			DefaultConcurrency: lim.DefaultConcurrency, MaxConcurrency: lim.ConcurrencyCeiling,
		},
	}
	if svc.Resources != nil {
		out.MemoryBytes, out.NanoCPUs = svc.Resources.MemoryBytes, svc.Resources.NanoCPUs
	}
	return out
}

// handleGetFunctionSettings handles GET /api/v1/functions/{name}/settings: idle period,
// port, memory and CPU limits, and the request timeout and concurrency limit with their bounds.
func (rt *Router) handleGetFunctionSettings(w http.ResponseWriter, r *http.Request) {
	svc, sleep, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, functionSettingsView(svc, sleep, rt.loadFunctionSettings(r.Context(), svc.Name)))
}

// handlePutFunctionSettings handles PUT /api/v1/functions/{name}/settings: sets the
// request timeout (seconds) and max concurrency; 0 restores the server default.
func (rt *Router) handlePutFunctionSettings(w http.ResponseWriter, r *http.Request) {
	svc, sleep, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	st := rt.functionSettingsStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "function settings are not available on this control plane")
		return
	}
	var req functionSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cur := rt.loadFunctionSettings(r.Context(), svc.Name)
	lim := functions.LimitsFromEnv(nil)
	if req.RequestTimeoutSeconds != nil {
		v := *req.RequestTimeoutSeconds
		if v < 0 || time.Duration(v)*time.Second > lim.TimeoutCeiling {
			writeError(w, http.StatusBadRequest, "request_timeout_seconds must be 0 or between 1 and "+strconv.Itoa(int(lim.TimeoutCeiling/time.Second)))
			return
		}
		cur.RequestTimeoutSeconds = v
	}
	if req.MaxConcurrency != nil {
		v := *req.MaxConcurrency
		if v < 0 || v > lim.ConcurrencyCeiling {
			writeError(w, http.StatusBadRequest, "max_concurrency must be 0 or between 1 and "+strconv.Itoa(lim.ConcurrencyCeiling))
			return
		}
		cur.MaxConcurrency = v
	}
	cur.ServiceName = svc.Name
	if err := st.SetFunctionSettings(r.Context(), cur, time.Now()); err != nil {
		rt.internalError(w, "api: set function settings failed", err, slog.String("function", svc.Name))
		return
	}
	writeJSON(w, http.StatusOK, functionSettingsView(svc, sleep, cur))
}

type functionVersion struct {
	DeployID     string    `json:"deploy_id"`
	Image        string    `json:"image"`
	CommitSHA    string    `json:"commit_sha,omitempty"`
	Status       string    `json:"status"`
	StartedAt    time.Time `json:"started_at"`
	Live         bool      `json:"live"`
	Rollbackable bool      `json:"rollbackable"`
}

// handleListFunctionVersions handles GET /api/v1/functions/{name}/versions: the
// deploy history, newest first, marking the live one and the ones that can be rolled back to.
func (rt *Router) handleListFunctionVersions(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), svc.Name)
	if err != nil {
		rt.internalError(w, "api: list function versions failed", err, slog.String("function", svc.Name))
		return
	}
	out := make([]functionVersion, 0, len(attempts))
	liveSeen := false
	for _, a := range attempts {
		done := a.Status == store.DeployAttemptStatusSucceeded && a.Image != ""
		live := done && !liveSeen && a.Image == svc.Image
		if live {
			liveSeen = true
		}
		out = append(out, functionVersion{
			DeployID: a.ID, Image: a.Image, CommitSHA: a.CommitSHA, Status: a.Status, StartedAt: a.StartedAt,
			Live: live, Rollbackable: done && !live,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": out})
}

// handleFunctionRollback handles POST /api/v1/functions/{name}/rollback?to=<deployId>&pin=true:
// the app rollback, defaulting to the newest earlier version; pin also locks publishing.
func (rt *Router) handleFunctionRollback(w http.ResponseWriter, r *http.Request) {
	svc, _, ok := rt.loadFunction(w, r)
	if !ok {
		return
	}
	to := r.URL.Query().Get("to")
	if to == "" {
		attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), svc.Name)
		if err != nil {
			rt.internalError(w, "api: function rollback: list versions failed", err, slog.String("function", svc.Name))
			return
		}
		for _, a := range attempts {
			if a.Status == store.DeployAttemptStatusSucceeded && a.Image != "" && a.Image != svc.Image {
				to = a.ID
				break
			}
		}
		if to == "" {
			writeError(w, http.StatusConflict, "no earlier version to roll back to")
			return
		}
	}
	r.SetPathValue("deployId", to)
	pin := r.URL.Query().Get("pin") == "true"
	rt.logger.Info("api: function rollback", slog.String("function", svc.Name), slog.String("deploy_id", to), slog.Bool("pin", pin))
	if pin {
		rt.handleRollbackAndLock(w, r)
		return
	}
	rt.handleRollbackToDeploy(w, r)
}
