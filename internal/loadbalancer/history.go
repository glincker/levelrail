package loadbalancer

import (
	"os"
	"strconv"
	"time"
)

// HistoryConfig bounds the in-memory per-upstream history.
type HistoryConfig struct {
	Checks       int
	Transitions  int
	SeriesStep   time.Duration
	SeriesWindow time.Duration
	MaxAge       time.Duration
}

// HistoryConfigFromEnv reads the history bounds, falling back to defaults.
func HistoryConfigFromEnv() HistoryConfig {
	return HistoryConfig{
		Checks:       envInt("APP_LB_HISTORY_CHECKS", 60),
		Transitions:  envInt("APP_LB_HISTORY_TRANSITIONS", 50),
		SeriesStep:   envDuration("APP_LB_HISTORY_STEP", 15*time.Second),
		SeriesWindow: envDuration("APP_LB_HISTORY_WINDOW", 30*time.Minute),
		MaxAge:       envDuration("APP_LB_HISTORY_MAX_AGE", 24*time.Hour),
	}
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// CheckRecord is one health probe outcome.
type CheckRecord struct {
	At         time.Time `json:"at"`
	OK         bool      `json:"ok"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	Reason     string    `json:"reason"`
}

// Transition is one upstream state change.
type Transition struct {
	At     time.Time `json:"at"`
	From   string    `json:"from"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
}

// SeriesPoint is one sampled value.
type SeriesPoint struct {
	At    time.Time `json:"at"`
	Value float64   `json:"value"`
}

// SeriesSet is the short per-upstream sample series.
type SeriesSet struct {
	Connections []SeriesPoint `json:"connections"`
	LatencyMs   []SeriesPoint `json:"latency_ms"`
	Fails       []SeriesPoint `json:"fails"`
}

// UpstreamHistory is the recorded history of one upstream.
type UpstreamHistory struct {
	ID          string        `json:"id"`
	Dial        string        `json:"dial"`
	AdminState  string        `json:"admin_state"`
	Checks      []CheckRecord `json:"checks"`
	Transitions []Transition  `json:"transitions"`
	Series      SeriesSet     `json:"series"`
}

type upstreamHist struct {
	checks      []CheckRecord
	transitions []Transition
	series      SeriesSet
	lastSample  time.Time
	state       string
	changedAt   time.Time
}

// NewRegistryWithHistory returns an empty Registry with explicit history bounds.
func NewRegistryWithHistory(cfg HistoryConfig) *Registry {
	return &Registry{
		obs:       map[string]Observation{},
		firstSeen: map[string]map[string]time.Time{},
		hist:      map[string]map[string]*upstreamHist{},
		histCfg:   cfg,
		now:       time.Now,
	}
}

func trimTail[T any](s []T, limit int) []T {
	if len(s) <= limit {
		return s
	}
	return append([]T(nil), s[len(s)-limit:]...)
}

func pruneOld[T any](s []T, cutoff time.Time, at func(T) time.Time) []T {
	i := 0
	for i < len(s) && at(s[i]).Before(cutoff) {
		i++
	}
	if i == 0 {
		return s
	}
	return append([]T(nil), s[i:]...)
}

func (r *Registry) seriesCap() int {
	if r.histCfg.SeriesStep <= 0 {
		return 1
	}
	return int(r.histCfg.SeriesWindow/r.histCfg.SeriesStep) + 1
}

// RecordStatus records st into the history (probe results, state
// transitions, sampled series) and fills each upstream's LastChangedAt.
// History is in memory and resets when the control plane restarts.
func (r *Registry) RecordStatus(st *Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	prev := r.hist[st.Service]
	next := make(map[string]*upstreamHist, len(st.Upstreams))
	for i := range st.Upstreams {
		u := &st.Upstreams[i]
		h := prev[u.ID]
		if h == nil {
			h = &upstreamHist{state: u.State, changedAt: now}
		}
		next[u.ID] = h
		r.recordUpstream(h, u, now)
		changed := h.changedAt
		u.LastChangedAt = &changed
	}
	r.hist[st.Service] = next
}

func (r *Registry) recordUpstream(h *upstreamHist, u *UpstreamStatus, now time.Time) {
	cfg := r.histCfg
	if u.State != h.state {
		h.transitions = append(h.transitions, Transition{At: now, From: h.state, To: u.State, Reason: transitionReason(u)})
		h.state, h.changedAt = u.State, now
	}
	if p := u.probed; p != nil && (len(h.checks) == 0 || !h.checks[len(h.checks)-1].At.Equal(p.At)) {
		h.checks = append(h.checks, *p)
	}
	if h.lastSample.IsZero() || now.Sub(h.lastSample) >= cfg.SeriesStep {
		h.lastSample = now
		h.series.Connections = append(h.series.Connections, SeriesPoint{At: now, Value: float64(u.ActiveConns)})
		h.series.Fails = append(h.series.Fails, SeriesPoint{At: now, Value: float64(u.Fails)})
		if u.probed != nil {
			h.series.LatencyMs = append(h.series.LatencyMs, SeriesPoint{At: now, Value: float64(u.LatencyMs)})
		}
	}
	h.prune(cfg, now, r.seriesCap())
}

func transitionReason(u *UpstreamStatus) string {
	if u.Reason != "" {
		return u.Reason
	}
	if u.State == StateHealthy {
		return "checks passing"
	}
	return u.State
}

func (h *upstreamHist) prune(cfg HistoryConfig, now time.Time, seriesCap int) {
	age := now.Add(-cfg.MaxAge)
	h.checks = trimTail(pruneOld(h.checks, age, func(c CheckRecord) time.Time { return c.At }), cfg.Checks)
	h.transitions = trimTail(pruneOld(h.transitions, age, func(t Transition) time.Time { return t.At }), cfg.Transitions)
	win := now.Add(-cfg.SeriesWindow)
	at := func(p SeriesPoint) time.Time { return p.At }
	h.series.Connections = trimTail(pruneOld(h.series.Connections, win, at), seriesCap)
	h.series.LatencyMs = trimTail(pruneOld(h.series.LatencyMs, win, at), seriesCap)
	h.series.Fails = trimTail(pruneOld(h.series.Fails, win, at), seriesCap)
}

// History returns the recorded history for every upstream in st, checks
// capped to limit (0 means the ring size).
func (r *Registry) History(st Status, limit int) []UpstreamHistory {
	r.mu.Lock()
	defer r.mu.Unlock()
	hs := r.hist[st.Service]
	out := make([]UpstreamHistory, 0, len(st.Upstreams))
	for _, u := range st.Upstreams {
		row := UpstreamHistory{ID: u.ID, Dial: u.Dial, AdminState: u.AdminState, Checks: []CheckRecord{}, Transitions: []Transition{}, Series: SeriesSet{Connections: []SeriesPoint{}, LatencyMs: []SeriesPoint{}, Fails: []SeriesPoint{}}}
		if h := hs[u.ID]; h != nil {
			checks := h.checks
			if limit > 0 {
				checks = trimTail(checks, limit)
			}
			row.Checks = append(row.Checks, checks...)
			row.Transitions = append(row.Transitions, h.transitions...)
			row.Series = SeriesSet{
				Connections: append([]SeriesPoint{}, h.series.Connections...),
				LatencyMs:   append([]SeriesPoint{}, h.series.LatencyMs...),
				Fails:       append([]SeriesPoint{}, h.series.Fails...),
			}
		}
		out = append(out, row)
	}
	return out
}
