// Package canarypolicy holds the stage schedule, thresholds and env defaults of
// staged canary releases. It is a leaf so the app spec can import it.
package canarypolicy

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Env vars that tune the defaults a per-app policy or start request can override.
const (
	EnvStages       = "APP_CANARY_STAGES"
	EnvInterval     = "APP_CANARY_INTERVAL"
	EnvMaxErrorRate = "APP_CANARY_MAX_ERROR_RATE"
	EnvMaxP95Ratio  = "APP_CANARY_MAX_P95_RATIO"
	EnvMinRequests  = "APP_CANARY_MIN_REQUESTS"
	EnvP95FloorMS   = "APP_CANARY_P95_FLOOR_MS"
	EnvTick         = "APP_CANARY_TICK"
	EnvSticky       = "APP_CANARY_STICKY"

	EnvMaxHealthFailures = "APP_CANARY_MAX_HEALTH_FAILURES"
	EnvAlertRuleID       = "APP_CANARY_ALERT_RULE_ID"
)

const (
	defaultInterval     = 5 * time.Minute
	defaultMaxErrorRate = 0.02
	defaultMaxP95Ratio  = 1.5
	defaultMinRequests  = 30
	defaultP95FloorMS   = 50.0
	defaultTick         = 15 * time.Second
	// A confirmed liveness failure on the canary is already past the probe threshold, so one is enough.
	defaultMaxHealthFailures = 1

	// MaxStages bounds a stage schedule.
	MaxStages = 10
	// MaxWeight is the largest share ingress gives a canary; 100 means promote.
	MaxWeight = 99
	// Full is the stage value that means "promote".
	Full = 100
)

var defaultStages = []int{10, 25, 50, 100}

// Policy is the stage schedule and breach thresholds of one staged canary.
type Policy struct {
	Stages []int
	// Interval is how long each stage observes traffic before it may advance.
	Interval time.Duration
	// MaxErrorRate is how far (as a fraction, 0.02 is two points) the canary's
	// 5xx rate may exceed the stable release's before the canary is rolled back.
	MaxErrorRate float64
	// MaxP95Ratio is how many times the stable p95 the canary's p95 may reach.
	MaxP95Ratio float64
}

// Limits are the judgement knobs that are global rather than per app.
type Limits struct {
	// MinRequests is the canary request count a stage needs before it is judged.
	MinRequests uint64
	// P95FloorMS ignores latency ratios while the canary p95 is below it.
	P95FloorMS float64
	Tick       time.Duration
	// MaxHealthFailures rolls the canary back once its liveness probe has failed this many
	// times in a row; 0 turns the check off.
	MaxHealthFailures uint64
	// AlertRuleID names an alert rule whose firing state also rolls the canary back; empty is off.
	AlertRuleID string
}

// DefaultPolicy returns the built-in defaults overridden by APP_CANARY_* env vars.
func DefaultPolicy(getenv func(string) string) Policy {
	p := Policy{Stages: append([]int(nil), defaultStages...), Interval: defaultInterval, MaxErrorRate: defaultMaxErrorRate, MaxP95Ratio: defaultMaxP95Ratio}
	if s, err := ParseStages(getenv(EnvStages)); err == nil && len(s) > 0 {
		p.Stages = s
	}
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvInterval))); err == nil && d >= time.Second {
		p.Interval = d
	}
	if f, ok := envFloat(getenv, EnvMaxErrorRate); ok && f > 0 && f <= 1 {
		p.MaxErrorRate = f
	}
	if f, ok := envFloat(getenv, EnvMaxP95Ratio); ok && f > 1 {
		p.MaxP95Ratio = f
	}
	return p
}

// DefaultLimits returns the global limits overridden by APP_CANARY_* env vars.
func DefaultLimits(getenv func(string) string) Limits {
	l := Limits{MinRequests: defaultMinRequests, P95FloorMS: defaultP95FloorMS, Tick: defaultTick, MaxHealthFailures: defaultMaxHealthFailures}
	if n, err := strconv.ParseUint(strings.TrimSpace(getenv(EnvMaxHealthFailures)), 10, 64); err == nil {
		l.MaxHealthFailures = n
	}
	l.AlertRuleID = strings.TrimSpace(getenv(EnvAlertRuleID))
	if n, err := strconv.ParseUint(strings.TrimSpace(getenv(EnvMinRequests)), 10, 64); err == nil && n > 0 {
		l.MinRequests = n
	}
	if f, ok := envFloat(getenv, EnvP95FloorMS); ok && f >= 0 {
		l.P95FloorMS = f
	}
	if d, err := time.ParseDuration(strings.TrimSpace(getenv(EnvTick))); err == nil && d >= time.Second {
		l.Tick = d
	}
	return l
}

// StickyEnabled reports whether ingress pins a visitor to one release by
// cookie. It is on unless the setting says otherwise.
func StickyEnabled(getenv func(string) string) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(getenv(EnvSticky)))
	return err != nil || v
}

var liveEnv atomic.Pointer[func(string) string]

// SetLiveEnv points the defaults at the runtime settings so a change applies
// to the next stage or release. nil goes back to the process environment alone.
func SetLiveEnv(f func(string) string) {
	if f == nil {
		liveEnv.Store(nil)
		return
	}
	liveEnv.Store(&f)
}

// OSEnv asks the live settings first, then the process environment.
func OSEnv(k string) string {
	if f := liveEnv.Load(); f != nil {
		if v := (*f)(k); v != "" {
			return v
		}
	}
	return os.Getenv(k)
}

func envFloat(getenv func(string) string, key string) (float64, bool) {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// ParseStages parses "10,25,50,100"; an empty string yields no stages.
func ParseStages(v string) ([]int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var out []int
	for _, part := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("canary stage %q is not a whole number", strings.TrimSpace(part))
		}
		out = append(out, n)
	}
	return out, ValidateStages(out)
}

// ValidateStages checks a schedule: ascending percents of 1 to 100, at least
// one below 100 so something is judged before promotion.
func ValidateStages(stages []int) error {
	if len(stages) == 0 {
		return errors.New("canary needs at least one stage")
	}
	if len(stages) > MaxStages {
		return fmt.Errorf("canary has %d stages, the most allowed is %d", len(stages), MaxStages)
	}
	prev := 0
	for _, s := range stages {
		if s < 1 || s > Full {
			return fmt.Errorf("canary stage %d must be between 1 and %d", s, Full)
		}
		if s <= prev {
			return errors.New("canary stages must be strictly ascending")
		}
		prev = s
	}
	if stages[0] >= Full {
		return errors.New("canary needs a first stage below 100 percent")
	}
	return nil
}

// Validate checks p's thresholds and schedule.
func (p Policy) Validate() error {
	if err := ValidateStages(p.Stages); err != nil {
		return err
	}
	if p.Interval < time.Second {
		return errors.New("canary interval must be at least 1s")
	}
	if p.MaxErrorRate <= 0 || p.MaxErrorRate > 1 {
		return errors.New("canary max_error_rate must be greater than 0 and at most 1")
	}
	if p.MaxP95Ratio <= 1 {
		return errors.New("canary max_p95_ratio must be greater than 1")
	}
	return nil
}

// Weight is the traffic percent ingress sends the canary at stage i.
func (p Policy) Weight(i int) int {
	if i < 0 || i >= len(p.Stages) {
		return 0
	}
	return min(p.Stages[i], MaxWeight)
}
