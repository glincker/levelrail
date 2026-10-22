package functions

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Environment settings read by the control plane. A per-function value of 0
// falls back to the Default* variable; the Max* variables bound what a
// function may ask for.
const (
	EnvRequestTimeout        = "APP_FUNCTION_REQUEST_TIMEOUT"
	EnvRequestTimeoutMax     = "APP_FUNCTION_REQUEST_TIMEOUT_MAX"
	EnvMaxConcurrency        = "APP_FUNCTION_MAX_CONCURRENCY"
	EnvMaxConcurrencyCeiling = "APP_FUNCTION_MAX_CONCURRENCY_CEILING"
)

const (
	fallbackRequestTimeoutMax     = 10 * time.Minute
	fallbackMaxConcurrencyCeiling = 10000
)

// Limits are the server-wide defaults and ceilings for per-function limits.
type Limits struct {
	DefaultTimeout     time.Duration
	TimeoutCeiling     time.Duration
	DefaultConcurrency int
	ConcurrencyCeiling int
}

// LimitsFromEnv reads Limits from the environment; getenv nil uses os.Getenv.
func LimitsFromEnv(getenv func(string) string) Limits {
	if getenv == nil {
		getenv = os.Getenv
	}
	l := Limits{TimeoutCeiling: fallbackRequestTimeoutMax, ConcurrencyCeiling: fallbackMaxConcurrencyCeiling}
	if d, err := time.ParseDuration(getenv(EnvRequestTimeoutMax)); err == nil && d > 0 {
		l.TimeoutCeiling = d
	}
	if d, err := time.ParseDuration(getenv(EnvRequestTimeout)); err == nil && d > 0 {
		l.DefaultTimeout = min(d, l.TimeoutCeiling)
	}
	if n, err := strconv.Atoi(getenv(EnvMaxConcurrencyCeiling)); err == nil && n > 0 {
		l.ConcurrencyCeiling = n
	}
	if n, err := strconv.Atoi(getenv(EnvMaxConcurrency)); err == nil && n > 0 {
		l.DefaultConcurrency = min(n, l.ConcurrencyCeiling)
	}
	return l
}

// Timeout is the held-request wait for s, or 0 when neither the function nor the server sets one.
func (l Limits) Timeout(s store.FunctionSettings) time.Duration {
	if s.RequestTimeoutSeconds > 0 {
		return min(time.Duration(s.RequestTimeoutSeconds)*time.Second, l.TimeoutCeiling)
	}
	return l.DefaultTimeout
}

// Concurrency is the most requests held at once for s, or 0 for no limit.
func (l Limits) Concurrency(s store.FunctionSettings) int {
	if s.MaxConcurrency > 0 {
		return min(s.MaxConcurrency, l.ConcurrencyCeiling)
	}
	return l.DefaultConcurrency
}

// Limiter counts requests held per function. The zero value is ready to use.
type Limiter struct {
	mu sync.Mutex
	in map[string]int
}

// Acquire takes a slot for name; ok is false when limit slots are already taken.
// A limit of 0 or less never refuses.
func (l *Limiter) Acquire(name string, limit int) (release func(), ok bool) {
	if limit <= 0 {
		return func() {}, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.in == nil {
		l.in = map[string]int{}
	}
	if l.in[name] >= limit {
		return func() {}, false
	}
	l.in[name]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.in[name]--; l.in[name] <= 0 {
				delete(l.in, name)
			}
		})
	}, true
}

// Held reports how many requests are held for name.
func (l *Limiter) Held(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.in[name]
}
