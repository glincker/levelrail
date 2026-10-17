package plugin

import (
	"strconv"
	"strings"
	"time"
)

// Environment variables that tune the plugin runtime.
const (
	envMemoryMB        = "APP_PLUGIN_MEMORY_MB"
	envCPUs            = "APP_PLUGIN_CPUS"
	envCallTimeout     = "APP_PLUGIN_CALL_TIMEOUT"
	envStartTimeout    = "APP_PLUGIN_START_TIMEOUT"
	envReconcile       = "APP_PLUGIN_RECONCILE_INTERVAL"
	envMaxPayloadKB    = "APP_PLUGIN_MAX_PAYLOAD_KB"
	envMaxConcurrent   = "APP_PLUGIN_MAX_CONCURRENT"
	envBackoffMax      = "APP_PLUGIN_BACKOFF_MAX"
	envBreakerFailures = "APP_PLUGIN_BREAKER_FAILURES"
)

// Config holds the runtime limits. Zero fields take the defaults in withDefaults.
type Config struct {
	Namespace string
	DataDir   string
	UID, GID  int

	MemoryBytes int64
	NanoCPUs    int64
	PidsLimit   int64

	CallTimeout       time.Duration
	StartTimeout      time.Duration
	HealthTimeout     time.Duration
	ReconcileInterval time.Duration
	StopTimeout       time.Duration
	BackoffBase       time.Duration
	BackoffMax        time.Duration
	BreakerCooldown   time.Duration

	HealthFailures  int
	BreakerFailures int
	MaxConcurrent   int

	MaxEventBytes    int
	MaxConfigBytes   int
	MaxSecretBytes   int
	MaxResponseBytes int
}

func (c Config) withDefaults() Config {
	setI64(&c.MemoryBytes, 128<<20)
	setI64(&c.NanoCPUs, 500_000_000)
	setI64(&c.PidsLimit, 128)
	setDur(&c.CallTimeout, 10*time.Second)
	setDur(&c.StartTimeout, 20*time.Second)
	setDur(&c.HealthTimeout, 3*time.Second)
	setDur(&c.ReconcileInterval, 15*time.Second)
	setDur(&c.StopTimeout, 5*time.Second)
	setDur(&c.BackoffBase, 2*time.Second)
	setDur(&c.BackoffMax, 5*time.Minute)
	setDur(&c.BreakerCooldown, 30*time.Second)
	setInt(&c.HealthFailures, 3)
	setInt(&c.BreakerFailures, 5)
	setInt(&c.MaxConcurrent, 4)
	setInt(&c.MaxEventBytes, 64<<10)
	setInt(&c.MaxConfigBytes, 16<<10)
	setInt(&c.MaxSecretBytes, 4<<10)
	setInt(&c.MaxResponseBytes, 1<<10)
	if c.Namespace == "" {
		c.Namespace = "plugin"
	}
	return c
}

func setI64(p *int64, d int64) {
	if *p <= 0 {
		*p = d
	}
}

func setInt(p *int, d int) {
	if *p <= 0 {
		*p = d
	}
}

func setDur(p *time.Duration, d time.Duration) {
	if *p <= 0 {
		*p = d
	}
}

// ConfigFromEnv builds a Config from APP_PLUGIN_* variables. Unparseable
// values are ignored so a typo keeps the safe default.
func ConfigFromEnv(namespace, dataDir string, lookup func(string) (string, bool)) Config {
	c := Config{Namespace: namespace, DataDir: dataDir}
	get := func(k string) string {
		v, _ := lookup(k)
		return strings.TrimSpace(v)
	}
	if n, err := strconv.ParseInt(get(envMemoryMB), 10, 64); err == nil && n > 0 {
		c.MemoryBytes = n << 20
	}
	if f, err := strconv.ParseFloat(get(envCPUs), 64); err == nil && f > 0 {
		c.NanoCPUs = int64(f * 1e9)
	}
	for env, dst := range map[string]*time.Duration{
		envCallTimeout: &c.CallTimeout, envStartTimeout: &c.StartTimeout, envReconcile: &c.ReconcileInterval, envBackoffMax: &c.BackoffMax,
	} {
		if d, err := time.ParseDuration(get(env)); err == nil && d > 0 {
			*dst = d
		}
	}
	if n, err := strconv.Atoi(get(envMaxPayloadKB)); err == nil && n > 0 {
		c.MaxEventBytes = n << 10
	}
	if n, err := strconv.Atoi(get(envMaxConcurrent)); err == nil && n > 0 {
		c.MaxConcurrent = n
	}
	if n, err := strconv.Atoi(get(envBreakerFailures)); err == nil && n > 0 {
		c.BreakerFailures = n
	}
	return c
}
