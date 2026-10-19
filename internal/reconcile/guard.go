package reconcile

import (
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Environment variables that tune storm protection.
const (
	EnvGlobalBurst     = "APP_RECONCILE_PASS_BURST"
	EnvGlobalRate      = "APP_RECONCILE_PASSES_PER_SEC"
	EnvResourceBurst   = "APP_RECONCILE_RESOURCE_BURST"
	EnvResourcePerSec  = "APP_RECONCILE_RESOURCE_PER_SEC"
	EnvBreakerFailures = "APP_RECONCILE_BREAKER_FAILURES"
	EnvBreakerBase     = "APP_RECONCILE_BREAKER_BASE"
	EnvBreakerMax      = "APP_RECONCILE_BREAKER_MAX"
)

const (
	recentWindow = 5 * time.Minute
	recentCap    = 1024
)

// Breaker states.
const (
	BreakerClosed   = "closed"
	BreakerOpen     = "open"
	BreakerHalfOpen = "half_open"
)

// GuardConfig bounds how much reconcile work a burst of events can cause.
type GuardConfig struct {
	PassBurst       int
	PassesPerSec    float64
	ResourceBurst   int
	ResourcePerSec  float64
	BreakerFailures int
	BreakerBase     time.Duration
	BreakerMax      time.Duration
}

// GuardConfigFromEnv reads the APP_RECONCILE_* overrides over generous defaults.
func GuardConfigFromEnv(getenv func(string) string) GuardConfig {
	if getenv == nil {
		getenv = os.Getenv
	}
	num := func(key string, def float64) float64 {
		if v, err := strconv.ParseFloat(getenv(key), 64); err == nil && v > 0 {
			return v
		}
		return def
	}
	dur := func(key string, def time.Duration) time.Duration {
		if v, err := time.ParseDuration(getenv(key)); err == nil && v > 0 {
			return v
		}
		return def
	}
	return GuardConfig{
		PassBurst:       int(num(EnvGlobalBurst, 20)),
		PassesPerSec:    num(EnvGlobalRate, 4),
		ResourceBurst:   int(num(EnvResourceBurst, 40)),
		ResourcePerSec:  num(EnvResourcePerSec, 4),
		BreakerFailures: int(num(EnvBreakerFailures, 5)),
		BreakerBase:     dur(EnvBreakerBase, 15*time.Second),
		BreakerMax:      dur(EnvBreakerMax, 10*time.Minute),
	}
}

type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) refill(now time.Time, burst int, rate float64) {
	if b.last.IsZero() {
		b.tokens = float64(burst)
	} else if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens += el * rate
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
	}
	b.last = now
}

func (b *bucket) take(now time.Time, burst int, rate float64) bool {
	b.refill(now, burst, rate)
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

type controllerGuard struct {
	bucket     bucket
	failures   int
	state      string
	openUntil  time.Time
	lastErr    string
	lastErrAt  time.Time
	trips      int64
	panics     int64
	throttled  int64
	skippedOpn int64
}

// BreakerStatus is one controller's circuit breaker and rate limit state.
type BreakerStatus struct {
	Name      string    `json:"name"`
	State     string    `json:"state"`
	Failures  int       `json:"failures"`
	OpenUntil time.Time `json:"open_until"`
	LastError string    `json:"last_error,omitempty"`
	Trips     int64     `json:"trips"`
	Panics    int64     `json:"panics"`
	Throttled int64     `json:"throttled"`
	Skipped   int64     `json:"skipped"`
}

// GuardStatus is the visible state of storm protection.
type GuardStatus struct {
	PassesDeferred int64           `json:"passes_deferred"`
	RecentLimited  int             `json:"recent_limited"`
	OpenBreakers   int             `json:"open_breakers"`
	Breakers       []BreakerStatus `json:"breakers"`
}

// Guard rate limits reconcile passes and stops hammering controllers that keep failing.
type Guard struct {
	cfg GuardConfig
	now func() time.Time

	mu       sync.Mutex
	global   bucket
	per      map[string]*controllerGuard
	deferred int64
	recent   []time.Time
}

// NewGuard builds a Guard from cfg; zero fields fall back to the defaults.
func NewGuard(cfg GuardConfig) *Guard {
	def := GuardConfigFromEnv(func(string) string { return "" })
	if cfg.PassBurst <= 0 {
		cfg.PassBurst = def.PassBurst
	}
	if cfg.PassesPerSec <= 0 {
		cfg.PassesPerSec = def.PassesPerSec
	}
	if cfg.ResourceBurst <= 0 {
		cfg.ResourceBurst = def.ResourceBurst
	}
	if cfg.ResourcePerSec <= 0 {
		cfg.ResourcePerSec = def.ResourcePerSec
	}
	if cfg.BreakerFailures <= 0 {
		cfg.BreakerFailures = def.BreakerFailures
	}
	if cfg.BreakerBase <= 0 {
		cfg.BreakerBase = def.BreakerBase
	}
	if cfg.BreakerMax <= 0 {
		cfg.BreakerMax = def.BreakerMax
	}
	return &Guard{cfg: cfg, now: time.Now, per: map[string]*controllerGuard{}}
}

func (g *Guard) noteLimited(now time.Time) {
	g.recent = append(g.recent, now)
	if len(g.recent) > recentCap {
		g.recent = g.recent[len(g.recent)-recentCap:]
	}
}

// AllowPass takes one event-triggered pass from the global budget. When it is spent it returns how long to wait, so the caller can coalesce the burst into one later pass.
func (g *Guard) AllowPass() (bool, time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if g.global.take(now, g.cfg.PassBurst, g.cfg.PassesPerSec) {
		return true, 0
	}
	g.deferred++
	g.noteLimited(now)
	need := (1 - g.global.tokens) / g.cfg.PassesPerSec
	return false, time.Duration(need*float64(time.Second)) + time.Millisecond
}

func (g *Guard) entry(name string) *controllerGuard {
	c := g.per[name]
	if c == nil {
		c = &controllerGuard{state: BreakerClosed}
		g.per[name] = c
	}
	return c
}

// Before reports whether the named controller may run now.
func (g *Guard) Before(name string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	c := g.entry(name)
	switch c.state {
	case BreakerOpen:
		if now.Before(c.openUntil) {
			c.skippedOpn++
			return false
		}
		c.state = BreakerHalfOpen
	}
	if !c.bucket.take(now, g.cfg.ResourceBurst, g.cfg.ResourcePerSec) {
		c.throttled++
		g.noteLimited(now)
		return false
	}
	return true
}

// After records the outcome of a controller run. A panic counts as a failure.
func (g *Guard) After(name string, err error, panicked bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	c := g.entry(name)
	if err == nil {
		c.failures, c.state, c.openUntil = 0, BreakerClosed, time.Time{}
		return
	}
	if panicked {
		c.panics++
	}
	c.failures++
	c.lastErr, c.lastErrAt = err.Error(), now
	if c.failures < g.cfg.BreakerFailures {
		return
	}
	over := c.failures - g.cfg.BreakerFailures
	wait := g.cfg.BreakerBase
	for i := 0; i < over && wait < g.cfg.BreakerMax; i++ {
		wait *= 2
	}
	if wait > g.cfg.BreakerMax {
		wait = g.cfg.BreakerMax
	}
	if c.state != BreakerOpen {
		c.trips++
	}
	c.state, c.openUntil = BreakerOpen, now.Add(wait)
}

// Status snapshots the guard for the health page.
func (g *Guard) Status() GuardStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	cut := now.Add(-recentWindow)
	keep := g.recent[:0]
	for _, t := range g.recent {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	g.recent = keep
	st := GuardStatus{PassesDeferred: g.deferred, RecentLimited: len(g.recent), Breakers: []BreakerStatus{}}
	for name, c := range g.per {
		if c.state == BreakerClosed && c.trips == 0 && c.throttled == 0 && c.panics == 0 {
			continue
		}
		if c.state == BreakerOpen {
			st.OpenBreakers++
		}
		st.Breakers = append(st.Breakers, BreakerStatus{
			Name: name, State: c.state, Failures: c.failures, OpenUntil: c.openUntil, LastError: c.lastErr,
			Trips: c.trips, Panics: c.panics, Throttled: c.throttled, Skipped: c.skippedOpn,
		})
	}
	sort.Slice(st.Breakers, func(i, j int) bool { return st.Breakers[i].Name < st.Breakers[j].Name })
	return st
}

// Reset closes the named breaker, or every breaker when name is empty, and clears the recent limit counters. It returns how many breakers were open.
func (g *Guard) Reset(name string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	closed := 0
	for n, c := range g.per {
		if name != "" && n != name {
			continue
		}
		if c.state == BreakerOpen {
			closed++
		}
		c.failures, c.state, c.openUntil, c.lastErr = 0, BreakerClosed, time.Time{}, ""
		c.bucket.tokens = float64(g.cfg.ResourceBurst)
	}
	if name == "" {
		g.recent = nil
		g.global.tokens = float64(g.cfg.PassBurst)
	}
	return closed
}
