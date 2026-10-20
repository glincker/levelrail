// Package loadbalancer is the product-level load balancer model layered on
// the embedded Caddy reverse_proxy upstream pool (internal/ingress).
package loadbalancer

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Algorithms selectable in Config.Algorithm.
const (
	AlgoRoundRobin = "round_robin"
	AlgoLeastConn  = "least_conn"
	AlgoIPHash     = "ip_hash"
	AlgoURIHash    = "uri_hash"
	AlgoCookie     = "cookie"
	AlgoWeighted   = "weighted"
)

// Algorithms lists every valid algorithm in display order.
func Algorithms() []string {
	return []string{AlgoRoundRobin, AlgoLeastConn, AlgoIPHash, AlgoURIHash, AlgoCookie, AlgoWeighted}
}

// MaxWeight caps a per-upstream weight.
const MaxWeight = 100

// Config is one service's load balancer settings. A nil *Config means the
// service has no load balancer and keeps its single-upstream route.
type Config struct {
	Algorithm  string `yaml:"algorithm,omitempty" json:"algorithm,omitempty"`
	CookieName string `yaml:"cookie_name,omitempty" json:"cookie_name,omitempty"`
	// Weights is indexed by replica number; missing entries default to 1.
	Weights        []int          `yaml:"weights,omitempty" json:"weights,omitempty"`
	ActiveHealth   *ActiveHealth  `yaml:"active_health,omitempty" json:"active_health,omitempty"`
	PassiveHealth  *PassiveHealth `yaml:"passive_health,omitempty" json:"passive_health,omitempty"`
	Retries        *Retries       `yaml:"retries,omitempty" json:"retries,omitempty"`
	SlowStart      string         `yaml:"slow_start,omitempty" json:"slow_start,omitempty"`
	DrainTimeout   string         `yaml:"drain_timeout,omitempty" json:"drain_timeout,omitempty"`
	RequestTimeout string         `yaml:"request_timeout,omitempty" json:"request_timeout,omitempty"`
	RateLimit      *RateLimit     `yaml:"rate_limit,omitempty" json:"rate_limit,omitempty"`
	UpstreamTLS    *UpstreamTLS   `yaml:"upstream_tls,omitempty" json:"upstream_tls,omitempty"`
}

// ActiveHealth probes each upstream on an interval.
type ActiveHealth struct {
	Path         string `yaml:"path" json:"path"`
	Interval     string `yaml:"interval,omitempty" json:"interval,omitempty"`
	Timeout      string `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Passes       int    `yaml:"passes,omitempty" json:"passes,omitempty"`
	Fails        int    `yaml:"fails,omitempty" json:"fails,omitempty"`
	ExpectStatus int    `yaml:"expect_status,omitempty" json:"expect_status,omitempty"`
}

// PassiveHealth marks an upstream down after failures seen on live traffic.
type PassiveHealth struct {
	FailDuration string `yaml:"fail_duration,omitempty" json:"fail_duration,omitempty"`
	MaxFails     int    `yaml:"max_fails,omitempty" json:"max_fails,omitempty"`
}

// Retries controls how a failed request is retried on another upstream.
type Retries struct {
	Count       int    `yaml:"count,omitempty" json:"count,omitempty"`
	TryDuration string `yaml:"try_duration,omitempty" json:"try_duration,omitempty"`
	TryInterval string `yaml:"try_interval,omitempty" json:"try_interval,omitempty"`
}

// RateLimit caps requests per second per client address.
type RateLimit struct {
	RPS   int `yaml:"rps" json:"rps"`
	Burst int `yaml:"burst,omitempty" json:"burst,omitempty"`
}

// UpstreamTLS makes the proxy speak HTTPS to upstreams.
type UpstreamTLS struct {
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
	ServerName         string `yaml:"server_name,omitempty" json:"server_name,omitempty"`
}

// EffectiveAlgorithm returns the configured algorithm or the default.
func (c *Config) EffectiveAlgorithm() string {
	if c == nil || c.Algorithm == "" {
		return AlgoRoundRobin
	}
	return c.Algorithm
}

// Defaults returns a copy of c with unset optional fields filled in.
func (c Config) Defaults() Config {
	c.Algorithm = c.EffectiveAlgorithm()
	if c.ActiveHealth != nil {
		h := *c.ActiveHealth
		h.Interval = orDefault(h.Interval, "10s")
		h.Timeout = orDefault(h.Timeout, "5s")
		if h.Passes == 0 {
			h.Passes = 1
		}
		if h.Fails == 0 {
			h.Fails = 1
		}
		c.ActiveHealth = &h
	}
	if c.PassiveHealth != nil {
		p := *c.PassiveHealth
		p.FailDuration = orDefault(p.FailDuration, "30s")
		if p.MaxFails == 0 {
			p.MaxFails = 1
		}
		c.PassiveHealth = &p
	}
	if c.Retries != nil {
		r := *c.Retries
		r.TryDuration = orDefault(r.TryDuration, "5s")
		c.Retries = &r
	}
	if c.Algorithm == AlgoCookie {
		c.CookieName = orDefault(c.CookieName, "lb")
	}
	return c
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ValidationError lists every problem found in a Config.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid load balancer config: %s", joinSorted(e.Problems))
}

func joinSorted(in []string) string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// Validate checks c and returns a *ValidationError listing all problems.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	var probs []string
	add := func(format string, args ...any) { probs = append(probs, fmt.Sprintf(format, args...)) }

	valid := false
	for _, a := range Algorithms() {
		if c.EffectiveAlgorithm() == a {
			valid = true
		}
	}
	if !valid {
		add("algorithm %q must be one of %v", c.Algorithm, Algorithms())
	}
	if c.CookieName != "" && c.EffectiveAlgorithm() != AlgoCookie {
		add("cookie_name only applies to algorithm cookie")
	}
	if len(c.Weights) > 0 && c.EffectiveAlgorithm() != AlgoWeighted {
		add("weights only apply to algorithm weighted")
	}
	for i, w := range c.Weights {
		if w < 1 || w > MaxWeight {
			add("weights[%d]=%d must be between 1 and %d", i, w, MaxWeight)
		}
	}
	dur := func(field, v string) {
		if v == "" {
			return
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			add("%s %q must be a positive duration such as 5s", field, v)
		}
	}
	if h := c.ActiveHealth; h != nil {
		if h.Path == "" || h.Path[0] != '/' {
			add("active_health.path must start with /")
		}
		dur("active_health.interval", h.Interval)
		dur("active_health.timeout", h.Timeout)
		if h.Passes < 0 || h.Fails < 0 {
			add("active_health passes and fails must not be negative")
		}
		if h.ExpectStatus != 0 && (h.ExpectStatus < 100 || h.ExpectStatus > 599) {
			add("active_health.expect_status must be a status code")
		}
		if h.Interval != "" && h.Timeout != "" {
			i, ie := time.ParseDuration(h.Interval)
			t, te := time.ParseDuration(h.Timeout)
			if ie == nil && te == nil && t >= i {
				add("active_health.timeout must be shorter than interval")
			}
		}
	}
	if p := c.PassiveHealth; p != nil {
		dur("passive_health.fail_duration", p.FailDuration)
		if p.MaxFails < 0 {
			add("passive_health.max_fails must not be negative")
		}
	}
	if r := c.Retries; r != nil {
		if r.Count < 0 || r.Count > 10 {
			add("retries.count must be between 0 and 10")
		}
		dur("retries.try_duration", r.TryDuration)
		dur("retries.try_interval", r.TryInterval)
	}
	dur("slow_start", c.SlowStart)
	dur("drain_timeout", c.DrainTimeout)
	dur("request_timeout", c.RequestTimeout)
	if rl := c.RateLimit; rl != nil {
		if rl.RPS < 1 {
			add("rate_limit.rps must be at least 1")
		}
		if rl.Burst < 0 {
			add("rate_limit.burst must not be negative")
		}
	}
	if len(probs) > 0 {
		return &ValidationError{Problems: probs}
	}
	return nil
}

// ParseDuration parses a validated duration string, returning zero for empty.
func ParseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}
