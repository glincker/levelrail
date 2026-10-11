// Package dataexport sends logs, audit events and metrics to destinations an
// operator already runs. Every pipeline is opt-in, bounded in memory and
// isolated from the container log stream, the API and the reconcilers.
package dataexport

import (
	"strconv"
	"time"
)

// AllowPrivateEnv lets export destinations resolve to private addresses, for
// a Loki or collector on the same host or network as the control plane.
const AllowPrivateEnv = "APP_EXPORT_ALLOW_PRIVATE_NETWORKS"

// Config holds every interval and size the exporter uses. Zero values are
// replaced by defaults in ConfigFromEnv.
type Config struct {
	Resync          time.Duration
	QueueLines      int
	BatchLines      int
	BatchBytes      int
	FlushInterval   time.Duration
	MaxAttempts     int
	BackoffBase     time.Duration
	BackoffMax      time.Duration
	BreakerFailures int
	BreakerCooldown time.Duration
	SendTimeout     time.Duration
	StatusInterval  time.Duration
	AuditInterval   time.Duration
	AuditBatch      int
	AuditMaxRows    int
	MetricsInterval time.Duration
}

// DefaultConfig returns the built-in defaults.
func DefaultConfig() Config {
	return Config{
		Resync:          15 * time.Second,
		QueueLines:      10000,
		BatchLines:      200,
		BatchBytes:      1 << 20,
		FlushInterval:   2 * time.Second,
		MaxAttempts:     4,
		BackoffBase:     500 * time.Millisecond,
		BackoffMax:      15 * time.Second,
		BreakerFailures: 5,
		BreakerCooldown: 60 * time.Second,
		SendTimeout:     10 * time.Second,
		StatusInterval:  5 * time.Second,
		AuditInterval:   10 * time.Second,
		AuditBatch:      500,
		AuditMaxRows:    1000000,
		MetricsInterval: 60 * time.Second,
	}
}

// ConfigFromEnv overlays APP_EXPORT_* variables onto the defaults. A value
// that does not parse or is not positive is ignored.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	c := DefaultConfig()
	dur := func(name string, dst *time.Duration) {
		if v, ok := lookup(name); ok {
			if d, err := time.ParseDuration(v); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	num := func(name string, dst *int) {
		if v, ok := lookup(name); ok {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	dur("APP_EXPORT_RESYNC", &c.Resync)
	num("APP_EXPORT_QUEUE_LINES", &c.QueueLines)
	num("APP_EXPORT_BATCH_LINES", &c.BatchLines)
	num("APP_EXPORT_BATCH_BYTES", &c.BatchBytes)
	dur("APP_EXPORT_FLUSH_INTERVAL", &c.FlushInterval)
	num("APP_EXPORT_MAX_ATTEMPTS", &c.MaxAttempts)
	dur("APP_EXPORT_BACKOFF_BASE", &c.BackoffBase)
	dur("APP_EXPORT_BACKOFF_MAX", &c.BackoffMax)
	num("APP_EXPORT_BREAKER_FAILURES", &c.BreakerFailures)
	dur("APP_EXPORT_BREAKER_COOLDOWN", &c.BreakerCooldown)
	dur("APP_EXPORT_SEND_TIMEOUT", &c.SendTimeout)
	dur("APP_EXPORT_STATUS_INTERVAL", &c.StatusInterval)
	dur("APP_EXPORT_AUDIT_INTERVAL", &c.AuditInterval)
	num("APP_EXPORT_AUDIT_BATCH", &c.AuditBatch)
	num("APP_EXPORT_AUDIT_MAX_ROWS", &c.AuditMaxRows)
	dur("APP_EXPORT_METRICS_INTERVAL", &c.MetricsInterval)
	return c
}
