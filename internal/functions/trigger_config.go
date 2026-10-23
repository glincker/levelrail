package functions

import (
	"strconv"
	"time"
)

// TriggerConfig bounds the webhook trigger and the async queue. Every value is an env var.
type TriggerConfig struct {
	ReplayWindow     time.Duration
	MaxBodyBytes     int64
	MaxQueued        int
	MaxAttempts      int
	BackoffBase      time.Duration
	BackoffMax       time.Duration
	AttemptTimeout   time.Duration
	PollInterval     time.Duration
	Workers          int
	DoneRetention    time.Duration
	WebhookPerMinute int
}

// TriggerConfigFromEnv reads the APP_FUNCTION_* variables; unset or invalid values take the documented default.
func TriggerConfigFromEnv(lookup func(string) (string, bool)) TriggerConfig {
	return TriggerConfig{
		ReplayWindow:     envDuration(lookup, "APP_FUNCTION_WEBHOOK_REPLAY_WINDOW", 5*time.Minute),
		MaxBodyBytes:     int64(envInt(lookup, "APP_FUNCTION_QUEUE_MAX_BODY_BYTES", 1<<20)),
		MaxQueued:        envInt(lookup, "APP_FUNCTION_QUEUE_MAX_PER_FUNCTION", 1000),
		MaxAttempts:      envInt(lookup, "APP_FUNCTION_QUEUE_MAX_ATTEMPTS", 5),
		BackoffBase:      envDuration(lookup, "APP_FUNCTION_QUEUE_BACKOFF_BASE", 5*time.Second),
		BackoffMax:       envDuration(lookup, "APP_FUNCTION_QUEUE_BACKOFF_MAX", 15*time.Minute),
		AttemptTimeout:   envDuration(lookup, "APP_FUNCTION_QUEUE_ATTEMPT_TIMEOUT", 2*time.Minute),
		PollInterval:     envDuration(lookup, "APP_FUNCTION_QUEUE_POLL_INTERVAL", 2*time.Second),
		Workers:          envInt(lookup, "APP_FUNCTION_QUEUE_WORKERS", 2),
		DoneRetention:    envDuration(lookup, "APP_FUNCTION_QUEUE_DONE_RETENTION", 24*time.Hour),
		WebhookPerMinute: envInt(lookup, "APP_FUNCTION_WEBHOOK_RATE_LIMIT_RPM", 600),
	}
}

func envInt(lookup func(string) (string, bool), key string, def int) int {
	if v, ok := lookup(key); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func envDuration(lookup func(string) (string, bool), key string, def time.Duration) time.Duration {
	if v, ok := lookup(key); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// Backoff is the delay before the next try after attempt (1 based): base doubled per attempt, capped.
func (c TriggerConfig) Backoff(attempt int) time.Duration {
	d := c.BackoffBase
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= c.BackoffMax {
			return c.BackoffMax
		}
	}
	if d > c.BackoffMax {
		return c.BackoffMax
	}
	return d
}
