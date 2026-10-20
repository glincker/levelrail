package main

import "github.com/GLINCKER/levelrail/internal/apiclient"

type lbFlagValues struct {
	algorithm, cookie                         string
	weights                                   []int
	healthPath, healthInterval, healthTimeout string
	healthPasses, healthFails, healthStatus   int
	maxFails                                  int
	failDuration                              string
	retries                                   int
	tryDuration                               string
	slowStart, drain, reqTimeout              string
	rps, burst                                int
	upstreamTLS, tlsInsecure                  bool
	tlsName                                   string
}

// applyLBFlags overlays the flags named in set onto cfg.
func applyLBFlags(cfg *apiclient.LoadBalancerConfig, set map[string]bool, v lbFlagValues) {
	if set["algorithm"] {
		cfg.Algorithm = v.algorithm
		if v.algorithm != "weighted" {
			cfg.Weights = nil
		}
		if v.algorithm != "cookie" {
			cfg.CookieName = ""
		}
	}
	if set["cookie-name"] {
		cfg.CookieName = v.cookie
	}
	if set["weights"] {
		cfg.Weights = v.weights
	}

	if set["health-path"] || set["health-interval"] || set["health-timeout"] || set["health-passes"] || set["health-fails"] || set["health-status"] {
		h := &apiclient.LoadBalancerActiveHealth{}
		if cfg.ActiveHealth != nil {
			cp := *cfg.ActiveHealth
			h = &cp
		}
		if set["health-path"] {
			h.Path = v.healthPath
		}
		if set["health-interval"] {
			h.Interval = v.healthInterval
		}
		if set["health-timeout"] {
			h.Timeout = v.healthTimeout
		}
		if set["health-passes"] {
			h.Passes = v.healthPasses
		}
		if set["health-fails"] {
			h.Fails = v.healthFails
		}
		if set["health-status"] {
			h.ExpectStatus = v.healthStatus
		}
		cfg.ActiveHealth = h
	}
	if set["max-fails"] || set["fail-duration"] {
		p := &apiclient.LoadBalancerPassiveHealth{}
		if cfg.PassiveHealth != nil {
			cp := *cfg.PassiveHealth
			p = &cp
		}
		if set["max-fails"] {
			p.MaxFails = v.maxFails
		}
		if set["fail-duration"] {
			p.FailDuration = v.failDuration
		}
		cfg.PassiveHealth = p
	}
	if set["retries"] || set["try-duration"] {
		r := &apiclient.LoadBalancerRetries{}
		if cfg.Retries != nil {
			cp := *cfg.Retries
			r = &cp
		}
		if set["retries"] {
			r.Count = v.retries
		}
		if set["try-duration"] {
			r.TryDuration = v.tryDuration
		}
		cfg.Retries = r
	}
	if set["slow-start"] {
		cfg.SlowStart = v.slowStart
	}
	if set["drain-timeout"] {
		cfg.DrainTimeout = v.drain
	}
	if set["request-timeout"] {
		cfg.RequestTimeout = v.reqTimeout
	}
	if set["rate-limit-rps"] || set["rate-limit-burst"] {
		rl := &apiclient.LoadBalancerRateLimit{}
		if cfg.RateLimit != nil {
			cp := *cfg.RateLimit
			rl = &cp
		}
		if set["rate-limit-rps"] {
			rl.RPS = v.rps
		}
		if set["rate-limit-burst"] {
			rl.Burst = v.burst
		}
		cfg.RateLimit = rl
	}
	if set["upstream-tls"] && !v.upstreamTLS {
		cfg.UpstreamTLS = nil
	}
	if set["upstream-tls-insecure"] || set["upstream-tls-server-name"] || (set["upstream-tls"] && v.upstreamTLS) {
		t := &apiclient.LoadBalancerUpstreamTLS{}
		if cfg.UpstreamTLS != nil {
			cp := *cfg.UpstreamTLS
			t = &cp
		}
		if set["upstream-tls-insecure"] {
			t.InsecureSkipVerify = v.tlsInsecure
		}
		if set["upstream-tls-server-name"] {
			t.ServerName = v.tlsName
		}
		cfg.UpstreamTLS = t
	}
}
