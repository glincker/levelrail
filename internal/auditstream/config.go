package auditstream

import (
	"strconv"
	"time"
)

// Environment variables that tune streaming.
const (
	EnvBatchSize   = "APP_AUDIT_STREAM_BATCH_SIZE"
	EnvTick        = "APP_AUDIT_STREAM_TICK"
	EnvBackoffBase = "APP_AUDIT_STREAM_BACKOFF_BASE"
	EnvBackoffMax  = "APP_AUDIT_STREAM_BACKOFF_MAX"
	EnvSendTimeout = "APP_AUDIT_STREAM_TIMEOUT"
	EnvRetention   = "APP_AUDIT_STREAM_HISTORY_RETENTION"
)

// Config bounds batching, retries and history retention.
type Config struct {
	BatchSize   int
	Tick        time.Duration
	BackoffBase time.Duration
	BackoffMax  time.Duration
	SendTimeout time.Duration
	Retention   time.Duration
}

// DefaultConfig is the config used when no env override is set.
func DefaultConfig() Config {
	return Config{
		BatchSize:   200,
		Tick:        5 * time.Second,
		BackoffBase: 5 * time.Second,
		BackoffMax:  10 * time.Minute,
		SendTimeout: 15 * time.Second,
		Retention:   30 * 24 * time.Hour,
	}
}

// ConfigFromEnv applies env overrides on top of DefaultConfig. Unparseable
// or non-positive values are ignored.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	c := DefaultConfig()
	if v, ok := lookup(EnvBatchSize); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.BatchSize = n
		}
	}
	for env, dst := range map[string]*time.Duration{
		EnvTick: &c.Tick, EnvBackoffBase: &c.BackoffBase, EnvBackoffMax: &c.BackoffMax,
		EnvSendTimeout: &c.SendTimeout, EnvRetention: &c.Retention,
	} {
		if v, ok := lookup(env); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	return c
}
