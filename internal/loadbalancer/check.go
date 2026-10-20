package loadbalancer

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CheckResult is one upstream's outcome from a check-now run.
type CheckResult struct {
	ID         string `json:"id"`
	Dial       string `json:"dial"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int64  `json:"latency_ms"`
	Reason     string `json:"reason"`
}

// CheckOutcome is the result of CheckNow.
type CheckOutcome struct {
	Results []CheckResult
	// Note is set when the balancer has no active health check configured.
	Note   string
	Status Status
}

// DefaultCheckTimeout is the probe timeout when no active health check is
// configured, overridable with APP_LB_CHECK_TIMEOUT.
func DefaultCheckTimeout() time.Duration {
	return envDuration("APP_LB_CHECK_TIMEOUT", 2*time.Second)
}

type fixedProber map[string]ProbeResult

func (f fixedProber) Probe(_ context.Context, dial, _ string, _ *UpstreamTLS, _ time.Duration) ProbeResult {
	return f[dial]
}

// CheckNow probes every running upstream once, in parallel and bounded by the
// probe timeout, using the configured active health check (or GET / when none
// is set). It records the results into reg and returns them.
func CheckNow(ctx context.Context, reg *Registry, obs Observation, stats map[string]Stats, prober Prober) CheckOutcome {
	cfg := obs.Config.Defaults()
	var note string
	if cfg.ActiveHealth == nil {
		cfg.ActiveHealth = &ActiveHealth{Path: "/", Timeout: DefaultCheckTimeout().String()}
		note = "no active health check is configured, probed GET / with the default timeout"
	}
	h := *cfg.ActiveHealth
	timeout := ParseDuration(h.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout+time.Second)
	defer cancel()

	probes := make([]ProbeResult, len(obs.Upstreams))
	var wg sync.WaitGroup
	for i, u := range obs.Upstreams {
		if !u.Running || u.Dial == "" {
			continue
		}
		wg.Add(1)
		go func(i int, dial string) {
			defer wg.Done()
			probes[i] = prober.Probe(ctx, dial, h.Path, cfg.UpstreamTLS, timeout)
		}(i, u.Dial)
	}
	wg.Wait()

	fixed := fixedProber{}
	for i, u := range obs.Upstreams {
		if u.Running && u.Dial != "" {
			fixed[u.Dial] = probes[i]
		}
	}
	obs.Config = cfg
	st := BuildStatus(ctx, obs, stats, fixed)

	out := CheckOutcome{Note: note, Results: make([]CheckResult, len(obs.Upstreams))}
	for i, u := range obs.Upstreams {
		res := CheckResult{ID: u.ID, Dial: u.Dial}
		if !u.Running || u.Dial == "" {
			res.Reason = "container not running"
			if u.Note != "" {
				res.Reason = u.Note
			}
			out.Results[i] = res
			continue
		}
		p := probes[i]
		reason := probeReason(p, h.ExpectStatus, timeout)
		res.OK, res.StatusCode, res.LatencyMs, res.Reason = reason == "", p.StatusCode, p.Latency.Milliseconds(), reason
		out.Results[i] = res
		checked := p.CheckedAt
		st.Upstreams[i].LastCheck = &checked
		st.Upstreams[i].LatencyMs = res.LatencyMs
		st.Upstreams[i].probed = &CheckRecord{At: checked, OK: res.OK, StatusCode: res.StatusCode, LatencyMs: res.LatencyMs, Reason: reason}
	}
	reg.RecordStatus(&st)
	out.Status = st
	return out
}

// CheckGate rate limits check-now per service.
type CheckGate struct {
	mu   sync.Mutex
	min  time.Duration
	last map[string]time.Time
}

// NewCheckGate returns a gate allowing one check per service per min.
func NewCheckGate(interval time.Duration) *CheckGate {
	return &CheckGate{min: interval, last: map[string]time.Time{}}
}

// CheckGateFromEnv sizes the gate from APP_LB_CHECK_MIN_INTERVAL (default 2s).
func CheckGateFromEnv() *CheckGate {
	return NewCheckGate(envDuration("APP_LB_CHECK_MIN_INTERVAL", 2*time.Second))
}

// Allow reports whether service may run a check at now; when not, it returns
// how long to wait.
func (g *CheckGate) Allow(service string, now time.Time) (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if prev, ok := g.last[service]; ok {
		if wait := g.min - now.Sub(prev); wait > 0 {
			return wait, false
		}
	}
	g.last[service] = now
	return 0, true
}

// ParseUpstreamReplica extracts the replica index from an ID built by
// UpstreamID for service.
func ParseUpstreamReplica(service, id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, service+"#")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 0 || strconv.Itoa(n) != rest {
		return 0, false
	}
	return n, true
}
