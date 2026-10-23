// Package trafficexport delivers finished edge requests to analytics vendors
// through bounded, lossy queues so a slow vendor never slows a real request.
package trafficexport

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names read by this package; invalid values fall back to defaults.
const (
	EnvTimeout       = "APP_TRAFFIC_EXPORT_TIMEOUT"        // HTTP timeout per delivery, default 10s
	EnvAllowPrivate  = "APP_TRAFFIC_EXPORT_ALLOW_PRIVATE"  // 1 allows loopback and link-local targets
	EnvMaxBodyBytes  = "APP_TRAFFIC_EXPORT_MAX_BODY_BYTES" // largest request body, default 20 MiB
	EnvQueueSize     = "APP_TRAFFIC_EXPORT_QUEUE_SIZE"     // events buffered per sink, default 10000
	EnvBatchSize     = "APP_TRAFFIC_EXPORT_BATCH_SIZE"     // events per delivery, default 500
	EnvFlushInterval = "APP_TRAFFIC_EXPORT_FLUSH_INTERVAL" // longest wait for a full batch, default 5s
	EnvMaxAttempts   = "APP_TRAFFIC_EXPORT_MAX_ATTEMPTS"   // deliveries tried per batch, default 5
	EnvBackoffBase   = "APP_TRAFFIC_EXPORT_BACKOFF_BASE"   // first retry delay, doubles each try, default 500ms
	EnvBackoffMax    = "APP_TRAFFIC_EXPORT_BACKOFF_MAX"    // retry delay ceiling, default 30s
)

const (
	defaultTimeout       = 10 * time.Second
	defaultMaxBodyBytes  = 20 << 20
	defaultQueueSize     = 10000
	defaultBatchSize     = 500
	defaultFlushInterval = 5 * time.Second
	defaultMaxAttempts   = 5
	defaultBackoffBase   = 500 * time.Millisecond
	defaultBackoffMax    = 30 * time.Second
)

// HTTPOptions is the transport policy shared by every HTTP sink.
type HTTPOptions struct {
	Timeout      time.Duration
	AllowPrivate bool
	MaxBodyBytes int
	// Namespace prefixes vendor attribute names and the service name; the
	// caller passes the brand short name. Empty means "edge".
	Namespace string
}

// HTTPOptionsFromEnv reads the sink transport policy from the environment.
func HTTPOptionsFromEnv() HTTPOptions {
	return HTTPOptions{
		Timeout:      envDuration(EnvTimeout, defaultTimeout),
		AllowPrivate: envBool(EnvAllowPrivate),
		MaxBodyBytes: envInt(EnvMaxBodyBytes, defaultMaxBodyBytes),
	}
}

func (o HTTPOptions) withDefaults() HTTPOptions {
	if o.Timeout <= 0 {
		o.Timeout = defaultTimeout
	}
	if o.MaxBodyBytes <= 0 {
		o.MaxBodyBytes = defaultMaxBodyBytes
	}
	o.Namespace = strings.ToLower(strings.TrimSpace(o.Namespace))
	if o.Namespace == "" {
		o.Namespace = "edge"
	}
	return o
}

// Options tunes the per-sink queue, batching and retry behavior.
type Options struct {
	QueueSize     int
	BatchSize     int
	FlushInterval time.Duration
	MaxAttempts   int
	BackoffBase   time.Duration
	BackoffMax    time.Duration
}

// OptionsFromEnv reads the pipeline tuning from the environment.
func OptionsFromEnv() Options {
	return Options{
		QueueSize:     envInt(EnvQueueSize, defaultQueueSize),
		BatchSize:     envInt(EnvBatchSize, defaultBatchSize),
		FlushInterval: envDuration(EnvFlushInterval, defaultFlushInterval),
		MaxAttempts:   envInt(EnvMaxAttempts, defaultMaxAttempts),
		BackoffBase:   envDuration(EnvBackoffBase, defaultBackoffBase),
		BackoffMax:    envDuration(EnvBackoffMax, defaultBackoffMax),
	}
}

func (o Options) withDefaults() Options {
	if o.QueueSize <= 0 {
		o.QueueSize = defaultQueueSize
	}
	if o.BatchSize <= 0 {
		o.BatchSize = defaultBatchSize
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = defaultFlushInterval
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = defaultMaxAttempts
	}
	if o.BackoffBase <= 0 {
		o.BackoffBase = defaultBackoffBase
	}
	if o.BackoffMax < o.BackoffBase {
		o.BackoffMax = o.BackoffBase
	}
	return o
}

func envDuration(name string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func envInt(name string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envBool(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	return v == "1" || strings.EqualFold(v, "true")
}
