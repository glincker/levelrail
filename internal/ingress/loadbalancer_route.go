package ingress

import "github.com/GLINCKER/levelrail/internal/loadbalancer"

// NewLBRoute translates a load balancer config and its discovered upstreams
// into Caddy route settings. It returns nil when there are no upstreams.
func NewLBRoute(cfg loadbalancer.Config, ups []loadbalancer.Upstream, weights []int) *LBRoute {
	if len(ups) == 0 {
		return nil
	}
	cfg = cfg.Defaults()
	lb := &LBRoute{Upstreams: make([]string, len(ups)), CookieName: cfg.CookieName}
	for i, u := range ups {
		lb.Upstreams[i] = u.Dial
	}
	switch cfg.Algorithm {
	case loadbalancer.AlgoLeastConn:
		lb.Policy = LBPolicyLeastConn
	case loadbalancer.AlgoIPHash:
		lb.Policy = LBPolicyIPHash
	case loadbalancer.AlgoURIHash:
		lb.Policy = LBPolicyURIHash
	case loadbalancer.AlgoCookie:
		lb.Policy = LBPolicyCookie
	case loadbalancer.AlgoWeighted:
		lb.Policy = LBPolicyWeighted
		lb.Weights = weights
	default:
		lb.Policy = LBPolicyRoundRobin
	}
	if h := cfg.ActiveHealth; h != nil {
		lb.ActiveHealth = &ActiveHealthChecks{URI: h.Path, Interval: h.Interval, Timeout: h.Timeout, Passes: h.Passes, Fails: h.Fails, ExpectStatus: h.ExpectStatus}
	}
	if p := cfg.PassiveHealth; p != nil {
		lb.PassiveHealth = &PassiveHealthChecks{FailDuration: p.FailDuration, MaxFails: p.MaxFails}
	}
	if r := cfg.Retries; r != nil {
		lb.Retries, lb.TryDuration, lb.TryInterval = r.Count, r.TryDuration, r.TryInterval
	}
	lb.StreamCloseDelay = cfg.DrainTimeout
	lb.ResponseHeaderTimeout = cfg.RequestTimeout
	if t := cfg.UpstreamTLS; t != nil {
		lb.UpstreamTLS = &UpstreamTLSConfig{InsecureSkipVerify: t.InsecureSkipVerify, ServerName: t.ServerName}
	}
	if rl := cfg.RateLimit; rl != nil {
		lb.RateLimitRPS, lb.RateLimitBurst = rl.RPS, rl.Burst
	}
	return lb
}
