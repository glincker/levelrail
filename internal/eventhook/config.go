package eventhook

import (
	"strconv"
	"time"
)

// Environment variables that tune delivery.
const (
	EnvMaxAttempts  = "APP_EVENT_WEBHOOK_MAX_ATTEMPTS"
	EnvBackoffBase  = "APP_EVENT_WEBHOOK_BACKOFF_BASE"
	EnvBackoffMax   = "APP_EVENT_WEBHOOK_BACKOFF_MAX"
	EnvSendTimeout  = "APP_EVENT_WEBHOOK_TIMEOUT"
	EnvRetention    = "APP_EVENT_WEBHOOK_RETENTION"
	EnvQueueSize    = "APP_EVENT_WEBHOOK_QUEUE_SIZE"
	defaultAttempts = 8
)

// Config bounds queueing, retries and retention.
type Config struct {
	MaxAttempts int
	BackoffBase time.Duration
	BackoffMax  time.Duration
	SendTimeout time.Duration
	Retention   time.Duration
	Tick        time.Duration
	BatchSize   int
	QueueSize   int
}

// DefaultConfig is the config used when no env override is set.
func DefaultConfig() Config {
	return Config{
		MaxAttempts: defaultAttempts,
		BackoffBase: 30 * time.Second,
		BackoffMax:  time.Hour,
		SendTimeout: 10 * time.Second,
		Retention:   30 * 24 * time.Hour,
		Tick:        2 * time.Second,
		BatchSize:   50,
		QueueSize:   1024,
	}
}

// ConfigFromEnv applies env overrides on top of DefaultConfig. Unparseable
// or non-positive values are ignored.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	c := DefaultConfig()
	if v, ok := lookup(EnvMaxAttempts); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxAttempts = n
		}
	}
	if v, ok := lookup(EnvQueueSize); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.QueueSize = n
		}
	}
	for env, dst := range map[string]*time.Duration{
		EnvBackoffBase: &c.BackoffBase, EnvBackoffMax: &c.BackoffMax,
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
