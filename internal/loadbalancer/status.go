package loadbalancer

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Upstream states reported by BuildStatus.
const (
	StateHealthy   = "healthy"
	StateUnhealthy = "unhealthy"
	StateDraining  = "draining"
	StateUnknown   = "unknown"
	StateDisabled  = "disabled"
)

// Operator-set admin states for one upstream.
const (
	AdminActive   = "active"
	AdminDraining = "draining"
	AdminDisabled = "disabled"
)

// ValidAdminState reports whether s is one of the admin states.
func ValidAdminState(s string) bool {
	return s == AdminActive || s == AdminDraining || s == AdminDisabled
}

// UpstreamObservation is what the reconciler saw for one upstream.
type UpstreamObservation struct {
	Upstream
	Running bool `json:"running"`
	// Draining marks a previous-release container kept in the pool while the
	// new release is not ready yet.
	Draining bool   `json:"draining,omitempty"`
	Weight   int    `json:"weight"`
	Note     string `json:"note,omitempty"`
	// AdminState is the operator's choice; empty means active.
	AdminState string `json:"admin_state,omitempty"`
}

// InPool reports whether the upstream may receive new connections.
func (u UpstreamObservation) InPool() bool {
	return u.Running && (u.AdminState == "" || u.AdminState == AdminActive)
}

// Observation is the reconciler's last view of one service's balancer.
type Observation struct {
	Service    string                `json:"service"`
	Config     Config                `json:"config"`
	Upstreams  []UpstreamObservation `json:"upstreams"`
	Reason     string                `json:"reason"`
	Message    string                `json:"message"`
	Ready      bool                  `json:"ready"`
	ObservedAt time.Time             `json:"observed_at"`
}

// Registry shares reconciler observations with the API and telemetry.
type Registry struct {
	mu        sync.Mutex
	obs       map[string]Observation
	firstSeen map[string]map[string]time.Time
	hist      map[string]map[string]*upstreamHist
	histCfg   HistoryConfig
	now       func() time.Time
}

// NewRegistry returns an empty Registry with history sized from the environment.
func NewRegistry() *Registry {
	return NewRegistryWithHistory(HistoryConfigFromEnv())
}

// FirstSeen records the first time each upstream ID was seen for service and
// forgets IDs no longer present. The first call for a service treats every
// upstream as long-running so a process restart does not restart slow start.
func (r *Registry) FirstSeen(service string, ids []string, now time.Time) map[string]time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev := r.firstSeen[service]
	next := make(map[string]time.Time, len(ids))
	for _, id := range ids {
		switch t, ok := prev[id]; {
		case ok:
			next[id] = t
		case prev == nil:
			next[id] = time.Time{}
		default:
			next[id] = now
		}
	}
	r.firstSeen[service] = next
	out := make(map[string]time.Time, len(next))
	for k, v := range next {
		out[k] = v
	}
	return out
}

// Record stores the latest observation for a service.
func (r *Registry) Record(o Observation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.obs[o.Service] = o
}

// Get returns the latest observation for service.
func (r *Registry) Get(service string) (Observation, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	o, ok := r.obs[service]
	return o, ok
}

// List returns every recorded observation.
func (r *Registry) List() []Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Observation, 0, len(r.obs))
	for _, o := range r.obs {
		out = append(out, o)
	}
	return out
}

// Retain drops state for services not in keep.
func (r *Registry) Retain(keep map[string]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for s := range r.obs {
		if !keep[s] {
			delete(r.obs, s)
			delete(r.firstSeen, s)
		}
	}
	for s := range r.hist {
		if !keep[s] {
			delete(r.hist, s)
		}
	}
}

// Stats are live counters for one upstream address from the proxy.
type Stats struct {
	NumRequests int `json:"num_requests"`
	Fails       int `json:"fails"`
}

// StatsSource reports live proxy counters keyed by dial address.
type StatsSource interface {
	UpstreamStats(ctx context.Context) (map[string]Stats, error)
}

// CaddyAdminStats reads counters from Caddy's admin /reverse_proxy/upstreams.
type CaddyAdminStats struct {
	// Addr is the admin listen address, e.g. "localhost:2019".
	Addr   string
	Client *http.Client
}

// UpstreamStats implements StatsSource.
func (c CaddyAdminStats) UpstreamStats(ctx context.Context) (map[string]Stats, error) {
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+c.Addr+"/reverse_proxy/upstreams", nil)
	if err != nil {
		return nil, fmt.Errorf("loadbalancer: build stats request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loadbalancer: fetch upstream stats: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loadbalancer: upstream stats status %d", resp.StatusCode)
	}
	var rows []struct {
		Address     string `json:"address"`
		NumRequests int    `json:"num_requests"`
		Fails       int    `json:"fails"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rows); err != nil {
		return nil, fmt.Errorf("loadbalancer: decode upstream stats: %w", err)
	}
	out := make(map[string]Stats, len(rows))
	for _, r := range rows {
		out[r.Address] = Stats{NumRequests: r.NumRequests, Fails: r.Fails}
	}
	return out, nil
}

// ProbeResult is one active health check outcome.
type ProbeResult struct {
	OK         bool
	StatusCode int
	Latency    time.Duration
	Err        string
	CheckedAt  time.Time
}

// Prober runs an active health check against one upstream.
type Prober interface {
	Probe(ctx context.Context, dial, path string, tlsCfg *UpstreamTLS, timeout time.Duration) ProbeResult
}

// HTTPProber is the real Prober.
type HTTPProber struct{}

// Probe implements Prober.
func (HTTPProber) Probe(ctx context.Context, dial, path string, upstreamTLS *UpstreamTLS, timeout time.Duration) ProbeResult {
	scheme := "http"
	client := &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if upstreamTLS != nil {
		scheme = "https"
		client.Transport = &http.Transport{TLSClientConfig: probeTLSConfig(upstreamTLS)}
	}
	res := ProbeResult{CheckedAt: time.Now()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+dial+path, nil)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	start := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(start)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	_ = resp.Body.Close()
	res.StatusCode = resp.StatusCode
	res.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !res.OK {
		res.Err = fmt.Sprintf("status %d", resp.StatusCode)
	}
	return res
}

// probeTLSConfig verifies the upstream certificate the same way the proxy
// does: against server_name, unless the operator opted out for this balancer.
func probeTLSConfig(u *UpstreamTLS) *tls.Config {
	return &tls.Config{ServerName: u.ServerName, InsecureSkipVerify: u.InsecureSkipVerify, MinVersion: tls.VersionTLS12} //nolint:gosec // operator-set upstream_tls.insecure_skip_verify, mirrors the proxy
}

// UpstreamStatus is the live view of one upstream.
type UpstreamStatus struct {
	ID          string     `json:"id"`
	Dial        string     `json:"dial"`
	NodeID      string     `json:"node_id,omitempty"`
	Replica     int        `json:"replica"`
	Weight      int        `json:"weight"`
	State       string     `json:"state"`
	Healthy     bool       `json:"healthy"`
	ActiveConns int        `json:"active_connections"`
	Fails       int        `json:"fails"`
	LastCheck   *time.Time `json:"last_check,omitempty"`
	LatencyMs   int64      `json:"latency_ms,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	AdminState  string     `json:"admin_state"`
	// LastChangedAt is when State last changed, or when it was first observed.
	LastChangedAt *time.Time `json:"last_changed_at,omitempty"`

	probed *CheckRecord
}

// Status is the API view of one service's load balancer.
type Status struct {
	Service    string           `json:"service"`
	Algorithm  string           `json:"algorithm"`
	Ready      bool             `json:"ready"`
	Reason     string           `json:"reason"`
	Message    string           `json:"message"`
	ObservedAt time.Time        `json:"observed_at"`
	Upstreams  []UpstreamStatus `json:"upstreams"`
}

// BuildStatus merges an observation with proxy counters and active probes.
// stats and prober may be nil.
func BuildStatus(ctx context.Context, obs Observation, stats map[string]Stats, prober Prober) Status {
	cfg := obs.Config.Defaults()
	out := Status{
		Service: obs.Service, Algorithm: cfg.Algorithm, Ready: obs.Ready,
		Reason: obs.Reason, Message: obs.Message, ObservedAt: obs.ObservedAt,
		Upstreams: make([]UpstreamStatus, len(obs.Upstreams)),
	}
	var wg sync.WaitGroup
	for i, u := range obs.Upstreams {
		st := UpstreamStatus{ID: u.ID, Dial: u.Dial, NodeID: u.NodeID, Replica: u.Replica, Weight: u.Weight, State: StateUnknown}
		s := stats[u.Dial]
		st.ActiveConns, st.Fails = s.NumRequests, s.Fails
		checked := obs.ObservedAt
		st.LastCheck = &checked
		out.Upstreams[i] = st

		wg.Add(1)
		go func(i int, u UpstreamObservation) {
			defer wg.Done()
			out.Upstreams[i] = classify(ctx, out.Upstreams[i], u, cfg, prober)
		}(i, u)
	}
	wg.Wait()
	return out
}

func classify(ctx context.Context, st UpstreamStatus, u UpstreamObservation, cfg Config, prober Prober) UpstreamStatus {
	st.AdminState = u.AdminState
	if st.AdminState == "" {
		st.AdminState = AdminActive
	}
	switch {
	case st.AdminState == AdminDisabled:
		st.State, st.Reason = StateDisabled, "disabled by operator"
		return st
	case st.AdminState == AdminDraining:
		st.State, st.Reason = StateDraining, "draining, no new connections"
		return st
	case !u.Running:
		st.State, st.Reason = StateUnhealthy, "container not running"
		return st
	case cfg.PassiveHealth != nil && cfg.PassiveHealth.MaxFails > 0 && st.Fails >= cfg.PassiveHealth.MaxFails:
		st.State, st.Reason = StateUnhealthy, fmt.Sprintf("%d recent failures", st.Fails)
		return st
	}
	if h := cfg.ActiveHealth; h != nil && prober != nil {
		timeout := ParseDuration(h.Timeout)
		res := prober.Probe(ctx, u.Dial, h.Path, cfg.UpstreamTLS, timeout)
		checked := res.CheckedAt
		st.LastCheck = &checked
		st.LatencyMs = res.Latency.Milliseconds()
		reason := probeReason(res, h.ExpectStatus, timeout)
		st.probed = &CheckRecord{At: checked, OK: reason == "", StatusCode: res.StatusCode, LatencyMs: st.LatencyMs, Reason: reason}
		if reason != "" {
			st.State, st.Reason = StateUnhealthy, reason
			return st
		}
	}
	st.Healthy = true
	st.State = StateHealthy
	if u.Draining {
		st.State, st.Reason = StateDraining, u.Note
	}
	return st
}

// probeReason returns "" for a passing probe, else a short human reason.
func probeReason(res ProbeResult, want int, timeout time.Duration) string {
	if res.Err != "" && res.StatusCode == 0 {
		e := strings.ToLower(res.Err)
		switch {
		case strings.Contains(e, "timeout") || strings.Contains(e, "deadline exceeded"):
			return fmt.Sprintf("timeout after %s", timeout)
		case strings.Contains(e, "connection refused"):
			return "connection refused"
		case strings.Contains(e, "no such host"):
			return "host not found"
		case strings.Contains(e, "certificate") || strings.Contains(e, "tls"):
			return "TLS handshake failed"
		case len(res.Err) > 80:
			return strings.TrimSpace(res.Err[:80])
		}
		return strings.TrimSpace(res.Err)
	}
	ok := res.OK
	if want != 0 {
		ok = res.StatusCode == want
	}
	if ok {
		return ""
	}
	if want != 0 {
		return fmt.Sprintf("expected %d, got %d", want, res.StatusCode)
	}
	return fmt.Sprintf("expected 2xx or 3xx, got %d", res.StatusCode)
}
