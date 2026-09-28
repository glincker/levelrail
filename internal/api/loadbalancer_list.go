package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	lbListDefaultLimit = 100
	lbListMaxLimit     = 500
)

// Summary states for GET /api/v1/loadbalancers.
const (
	lbStateBalancing = "balancing"
	lbStateDegraded  = "degraded"
	lbStateNone      = "none"
)

type loadBalancerSummary struct {
	App               string     `json:"app"`
	Service           string     `json:"service"`
	Algorithm         string     `json:"algorithm"`
	State             string     `json:"state"`
	UpstreamsTotal    int        `json:"upstreams_total"`
	UpstreamsHealthy  int        `json:"upstreams_healthy"`
	Reason            string     `json:"reason,omitempty"`
	LastCheck         *time.Time `json:"last_check,omitempty"`
	ConfigUpdatedAt   string     `json:"config_updated_at"`
	ActiveHealthCheck bool       `json:"active_health_check"`
}

type loadBalancerListResponse struct {
	Items  []loadBalancerSummary `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// summarizeLoadBalancer derives the list row from the stored config and the
// reconciler's last observation, without probing any upstream.
func summarizeLoadBalancer(row store.LoadBalancerRow, cfg loadbalancer.Config, obs *loadbalancer.Observation) loadBalancerSummary {
	sum := loadBalancerSummary{
		App: row.Service, Service: row.Service, Algorithm: cfg.EffectiveAlgorithm(),
		State: lbStateNone, ConfigUpdatedAt: row.UpdatedAt, ActiveHealthCheck: cfg.ActiveHealth != nil,
	}
	if obs == nil {
		sum.Reason = "Pending"
		return sum
	}
	sum.Reason = obs.Reason
	if !obs.ObservedAt.IsZero() {
		at := obs.ObservedAt
		sum.LastCheck = &at
	}
	sum.UpstreamsTotal = len(obs.Upstreams)
	for _, u := range obs.Upstreams {
		if u.Running && !u.Draining {
			sum.UpstreamsHealthy++
		}
	}
	switch {
	case sum.UpstreamsTotal == 0:
	case !obs.Ready || sum.UpstreamsHealthy < sum.UpstreamsTotal:
		sum.State = lbStateDegraded
	default:
		sum.State = lbStateBalancing
	}
	return sum
}

func (rt *Router) readableAppFilter(r *http.Request) (func(app string) bool, error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, err
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return nil, err
	}
	return func(app string) bool {
		return authorizeResource(abilities, policies, AbilityRead, "app:"+app)
	}, nil
}

func lbListPage(r *http.Request) (limit, offset int) {
	limit = lbListDefaultLimit
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, lbListMaxLimit)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n > 0 {
		offset = n
	}
	return limit, offset
}

// handleListLoadBalancers handles GET /api/v1/loadbalancers: one row per
// configured balancer the caller may read, from one store query plus the
// in-memory status registry. Supports q (name substring), state, limit, offset.
func (rt *Router) handleListLoadBalancers(w http.ResponseWriter, r *http.Request) {
	if !rt.lbConfigured(w) {
		return
	}
	canRead, err := rt.readableAppFilter(r)
	if err != nil {
		rt.internalError(w, "api: resolve caller for load balancer list failed", err)
		return
	}
	rows, err := rt.lb.store.ListLoadBalancerRows(r.Context())
	if err != nil {
		rt.internalError(w, "api: list load balancers failed", err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	wantState := r.URL.Query().Get("state")

	items := make([]loadBalancerSummary, 0, len(rows))
	for _, row := range rows {
		if !canRead(row.Service) || (q != "" && !strings.Contains(strings.ToLower(row.Service), q)) {
			continue
		}
		var cfg loadbalancer.Config
		if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
			rt.logger.Warn("api: skip undecodable load balancer config", slog.String("app", row.Service), slog.String("error", err.Error()))
			continue
		}
		var obs *loadbalancer.Observation
		if rt.lb.registry != nil {
			if o, ok := rt.lb.registry.Get(row.Service); ok {
				obs = &o
			}
		}
		sum := summarizeLoadBalancer(row, cfg, obs)
		if wantState != "" && sum.State != wantState {
			continue
		}
		items = append(items, sum)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].App < items[j].App })

	limit, offset := lbListPage(r)
	total := len(items)
	offset = min(offset, total)
	end := min(offset+limit, total)
	writeJSON(w, http.StatusOK, loadBalancerListResponse{Items: items[offset:end], Total: total, Limit: limit, Offset: offset})
}
