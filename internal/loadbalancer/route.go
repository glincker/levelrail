package loadbalancer

import (
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// Upstream is one replica endpoint the balancer can send traffic to.
type Upstream struct {
	// ID is stable across passes (service name plus replica index), used to
	// track first-seen time for slow start.
	ID      string `json:"id"`
	Dial    string `json:"dial"`
	Replica int    `json:"replica"`
	// NodeID is empty for the control plane's own node.
	NodeID string `json:"node_id,omitempty"`
}

// EffectiveWeights returns one weight per upstream. When slow start is set,
// an upstream first seen less than SlowStart ago ramps linearly from 1 to its
// configured weight. Only meaningful for the weighted algorithm.
func EffectiveWeights(cfg Config, ups []Upstream, firstSeen map[string]time.Time, now time.Time) []int {
	slow := ParseDuration(cfg.SlowStart)
	out := make([]int, len(ups))
	for i, u := range ups {
		w := 1
		if u.Replica < len(cfg.Weights) && cfg.Weights[u.Replica] > 0 {
			w = cfg.Weights[u.Replica]
		}
		if seen, ok := firstSeen[u.ID]; ok && slow > 0 && now.Sub(seen) < slow {
			frac := float64(now.Sub(seen)) / float64(slow)
			ramped := 1 + int(float64(w-1)*frac)
			if ramped < w {
				w = ramped
			}
		}
		out[i] = w
	}
	return out
}

// ToRoute translates cfg and the discovered upstreams into Caddy route
// settings. It returns nil when there are no upstreams.
func ToRoute(cfg Config, ups []Upstream, weights []int) *ingress.LBRoute {
	if len(ups) == 0 {
		return nil
	}
	cfg = cfg.Defaults()
	lb := &ingress.LBRoute{Upstreams: make([]string, len(ups)), CookieName: cfg.CookieName}
	for i, u := range ups {
		lb.Upstreams[i] = u.Dial
	}
	switch cfg.Algorithm {
	case AlgoLeastConn:
		lb.Policy = ingress.LBPolicyLeastConn
	case AlgoIPHash:
		lb.Policy = ingress.LBPolicyIPHash
	case AlgoURIHash:
		lb.Policy = ingress.LBPolicyURIHash
	case AlgoCookie:
		lb.Policy = ingress.LBPolicyCookie
	case AlgoWeighted:
		lb.Policy = ingress.LBPolicyWeighted
		lb.Weights = weights
	default:
		lb.Policy = ingress.LBPolicyRoundRobin
	}
	if h := cfg.ActiveHealth; h != nil {
		lb.ActiveHealth = &ingress.ActiveHealthChecks{URI: h.Path, Interval: h.Interval, Timeout: h.Timeout, Passes: h.Passes, Fails: h.Fails, ExpectStatus: h.ExpectStatus}
	}
	if p := cfg.PassiveHealth; p != nil {
		lb.PassiveHealth = &ingress.PassiveHealthChecks{FailDuration: p.FailDuration, MaxFails: p.MaxFails}
	}
	if r := cfg.Retries; r != nil {
		lb.Retries, lb.TryDuration, lb.TryInterval = r.Count, r.TryDuration, r.TryInterval
	}
	lb.StreamCloseDelay = cfg.DrainTimeout
	lb.ResponseHeaderTimeout = cfg.RequestTimeout
	if t := cfg.UpstreamTLS; t != nil {
		lb.UpstreamTLS = &ingress.UpstreamTLSConfig{InsecureSkipVerify: t.InsecureSkipVerify, ServerName: t.ServerName}
	}
	if rl := cfg.RateLimit; rl != nil {
		lb.RateLimitRPS, lb.RateLimitBurst = rl.RPS, rl.Burst
	}
	return lb
}

// UpstreamID builds the stable ID for a service replica.
func UpstreamID(service string, replica int) string {
	return service + "#" + strconv.Itoa(replica)
}
