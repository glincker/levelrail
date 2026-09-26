package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxLBHistoryLimit = 1000

// LBAdminStateStore persists per-upstream admin state. *store.DB satisfies
// this; a LoadBalancerStore that does not implement it serves no admin state.
type LBAdminStateStore interface {
	SetLBUpstreamAdminState(ctx context.Context, service string, replica int, state string) error
	ListLBUpstreamAdminStates(ctx context.Context) (map[string]map[int]string, error)
}

type lbHistoryResponse struct {
	Upstreams []loadbalancer.UpstreamHistory `json:"upstreams"`
}

type lbCheckResponse struct {
	Results []loadbalancer.CheckResult `json:"results"`
	Note    string                     `json:"note,omitempty"`
}

type lbUpstreamStateRequest struct {
	AdminState string `json:"admin_state"`
}

// lbObservation returns the registry observation for name with the persisted
// admin states overlaid, or false when no reconcile pass has seen it yet.
func (rt *Router) lbObservation(ctx context.Context, name string, cfg *loadbalancer.Config) (loadbalancer.Observation, bool, error) {
	var obs loadbalancer.Observation
	if rt.lb.registry != nil {
		obs, _ = rt.lb.registry.Get(name)
	}
	if obs.Service == "" {
		return obs, false, nil
	}
	obs.Config = *cfg
	states := map[int]string{}
	if as, ok := rt.lb.store.(LBAdminStateStore); ok {
		all, err := as.ListLBUpstreamAdminStates(ctx)
		if err != nil {
			return obs, true, err
		}
		states = all[name]
	}
	ups := make([]loadbalancer.UpstreamObservation, len(obs.Upstreams))
	copy(ups, obs.Upstreams)
	for i := range ups {
		if !ups[i].Draining {
			ups[i].AdminState = states[ups[i].Replica]
		}
	}
	obs.Upstreams = ups
	return obs, true, nil
}

func (rt *Router) lbStats(ctx context.Context) map[string]loadbalancer.Stats {
	if rt.lb.stats == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	s, err := rt.lb.stats.UpstreamStats(ctx)
	if err != nil {
		return nil
	}
	return s
}

// lbLoaded runs the shared prelude of the load balancer status routes.
func (rt *Router) lbLoaded(w http.ResponseWriter, r *http.Request) (string, loadbalancer.Observation, bool, bool) {
	var none loadbalancer.Observation
	if !rt.lbConfigured(w) {
		return "", none, false, false
	}
	name := r.PathValue("name")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return "", none, false, false
	}
	cfg, ok := rt.loadLB(w, r, name)
	if !ok {
		return "", none, false, false
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "load balancer not configured")
		return "", none, false, false
	}
	obs, observed, err := rt.lbObservation(r.Context(), name, cfg)
	if err != nil {
		rt.internalError(w, "api: load balancer admin states failed", err)
		return "", none, false, false
	}
	return name, obs, observed, true
}

// handleLoadBalancerHistory handles GET /api/v1/apps/{name}/loadbalancer/history.
func (rt *Router) handleLoadBalancerHistory(w http.ResponseWriter, r *http.Request) {
	_, obs, observed, ok := rt.lbLoaded(w, r)
	if !ok {
		return
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, maxLBHistoryLimit)
	}
	if !observed || rt.lb.registry == nil {
		writeJSON(w, http.StatusOK, lbHistoryResponse{Upstreams: []loadbalancer.UpstreamHistory{}})
		return
	}
	st := loadbalancer.BuildStatus(r.Context(), obs, nil, nil)
	writeJSON(w, http.StatusOK, lbHistoryResponse{Upstreams: rt.lb.registry.History(st, limit)})
}

// handleLoadBalancerCheck handles POST /api/v1/apps/{name}/loadbalancer/check.
func (rt *Router) handleLoadBalancerCheck(w http.ResponseWriter, r *http.Request) {
	name, obs, observed, ok := rt.lbLoaded(w, r)
	if !ok {
		return
	}
	if rt.lb.gate != nil {
		if wait, allowed := rt.lb.gate.Allow(name, time.Now()); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "a check ran a moment ago, retry shortly")
			return
		}
	}
	if !observed || rt.lb.registry == nil {
		writeJSON(w, http.StatusOK, lbCheckResponse{Results: []loadbalancer.CheckResult{}, Note: "no reconcile pass has observed this load balancer yet"})
		return
	}
	out := loadbalancer.CheckNow(r.Context(), rt.lb.registry, obs, rt.lbStats(r.Context()), rt.lb.prober)
	writeJSON(w, http.StatusOK, lbCheckResponse{Results: out.Results, Note: out.Note})
}

// handleSetLoadBalancerUpstream handles PUT /api/v1/apps/{name}/loadbalancer/upstreams/{id}.
func (rt *Router) handleSetLoadBalancerUpstream(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	as, ok := rt.lb.store.(LBAdminStateStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "upstream admin state is not supported by this store")
		return
	}
	var req lbUpstreamStateRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if !loadbalancer.ValidAdminState(req.AdminState) {
		writeError(w, http.StatusBadRequest, "admin_state must be active, draining or disabled")
		return
	}
	name, id := r.PathValue("name"), r.PathValue("id")
	if _, ok := rt.requireLBApp(w, r, name); !ok {
		return
	}
	cfg, ok := rt.loadLB(w, r, name)
	if !ok {
		return
	}
	if cfg == nil {
		writeError(w, http.StatusNotFound, "load balancer not configured")
		return
	}
	replica, valid := loadbalancer.ParseUpstreamReplica(name, id)
	if !valid || !rt.lbHasReplica(name, replica) {
		writeError(w, http.StatusNotFound, "upstream not found")
		return
	}
	if err := as.SetLBUpstreamAdminState(r.Context(), name, replica, req.AdminState); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set upstream admin state failed", err)
		return
	}
	rt.nudgeReconciler()

	obs, _, err := rt.lbObservation(r.Context(), name, cfg)
	if err != nil {
		rt.internalError(w, "api: load balancer admin states failed", err)
		return
	}
	st := loadbalancer.BuildStatus(r.Context(), obs, rt.lbStats(r.Context()), rt.lb.prober)
	if rt.lb.registry != nil {
		rt.lb.registry.RecordStatus(&st)
	}
	for _, u := range st.Upstreams {
		if u.ID == id {
			writeJSON(w, http.StatusOK, u)
			return
		}
	}
	writeError(w, http.StatusNotFound, "upstream not found")
}

func (rt *Router) lbHasReplica(service string, replica int) bool {
	if rt.lb.registry == nil {
		return false
	}
	obs, ok := rt.lb.registry.Get(service)
	if !ok {
		return false
	}
	for _, u := range obs.Upstreams {
		if !u.Draining && u.Replica == replica {
			return true
		}
	}
	return false
}
