package api

import (
	"context"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	envExpensiveReadTTL = "LEVELRAIL_EXPENSIVE_READ_TTL_SECONDS"
	envExpensiveReadMax = "LEVELRAIL_EXPENSIVE_READ_MAX_CONCURRENT"
	defaultReadTTLSecs  = 5
	defaultReadMax      = 4
)

// expensiveReadTTLOverride holds the live api.expensive_read_ttl setting in seconds; 0 means use env.
var expensiveReadTTLOverride atomic.Int64

// SetExpensiveReadTTL applies the api.expensive_read_ttl setting without a restart.
func SetExpensiveReadTTL(secs int64) { expensiveReadTTLOverride.Store(secs) }

func expensiveReadTTL() time.Duration {
	if v := expensiveReadTTLOverride.Load(); v > 0 {
		return time.Duration(v) * time.Second
	}
	if n, err := strconv.Atoi(os.Getenv(envExpensiveReadTTL)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return defaultReadTTLSecs * time.Second
}

var (
	readFlightOnce sync.Once
	readFlight     *flightGroup
)

// expensiveRead runs fn once per TTL window for key, however many requests ask at once.
func (rt *Router) expensiveRead(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	readFlightOnce.Do(func() {
		limit := defaultReadMax
		if n, err := strconv.Atoi(os.Getenv(envExpensiveReadMax)); err == nil && n > 0 {
			limit = n
		}
		readFlight = newFlightGroup(limit)
	})
	return readFlight.do(ctx, key, expensiveReadTTL(), fn)
}

// forgetExpensiveRead drops a cached key so the next read recomputes after a write.
func (rt *Router) forgetExpensiveRead(key string) {
	if readFlight != nil {
		readFlight.forget(key)
	}
}
