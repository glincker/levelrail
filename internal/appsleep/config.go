package appsleep

import (
	"os"
	"strconv"
	"time"
)

// Environment variables that tune sleep detection and port wake.
const (
	// EnvNetworkFloor is the container network traffic, in bytes per minute
	// (rx plus tx), below which an app still counts as idle.
	EnvNetworkFloor = "APP_SLEEP_NETWORK_FLOOR_BYTES_PER_MIN"
	// EnvWakeCooldown is how long after a wake an app cannot be put back to sleep.
	EnvWakeCooldown = "APP_SLEEP_WAKE_COOLDOWN"
	// EnvPortWakeTimeout bounds how long a held connection waits for the app.
	EnvPortWakeTimeout = "APP_SLEEP_PORT_WAKE_TIMEOUT"
	// EnvPortWakeRetry is how often a missing port wake listener is retried.
	EnvPortWakeRetry = "APP_SLEEP_PORT_WAKE_RETRY"
	// EnvPortWakeCollect is how long the listener keeps accepting after the
	// first connection, so a burst of clients is held rather than refused.
	EnvPortWakeCollect = "APP_SLEEP_PORT_WAKE_COLLECT"
	// EnvPortWakeMinPort is the lowest host port port wake will ever bind.
	EnvPortWakeMinPort = "APP_SLEEP_PORT_WAKE_MIN_PORT"
	// EnvIdleMin, EnvIdleMax and EnvIdleDefault bound and default the idle
	// time, in minutes.
	EnvIdleMin     = "APP_SLEEP_IDLE_MIN_MINUTES"
	EnvIdleMax     = "APP_SLEEP_IDLE_MAX_MINUTES"
	EnvIdleDefault = "APP_SLEEP_IDLE_DEFAULT_MINUTES"
	// EnvHoldMin and EnvHoldMax are the floor and cap of how long a request is
	// held while its app wakes; EnvHoldDefault applies with no wake history and
	// EnvHoldHeadroom multiplies the measured p95 wake.
	EnvHoldMin      = "APP_SLEEP_HOLD_MIN"
	EnvHoldMax      = "APP_SLEEP_HOLD_MAX"
	EnvHoldDefault  = "APP_SLEEP_HOLD_DEFAULT"
	EnvHoldHeadroom = "APP_SLEEP_HOLD_HEADROOM"
	// EnvWakeProbeInterval is the readiness probe interval while an app wakes.
	EnvWakeProbeInterval = "APP_SLEEP_WAKE_PROBE_INTERVAL"
)

// Defaults for the variables above. 4096 bytes per minute sits above
// keepalives and probes (a few hundred bytes) and below real use.
const (
	DefaultNetworkFloor      int64 = 4096
	DefaultWakeCooldown            = 5 * time.Minute
	DefaultPortWakeTimeout         = 30 * time.Second
	DefaultPortWakeRetry           = 2 * time.Second
	DefaultPortWakeCollect         = 250 * time.Millisecond
	DefaultPortWakeMinPort         = 1024
	DefaultIdleMin                 = 5
	DefaultIdleMax                 = 7 * 24 * 60
	DefaultIdleDefault             = 30
	DefaultHoldMin                 = 10 * time.Second
	DefaultHoldMax                 = 120 * time.Second
	DefaultHoldDefault             = 30 * time.Second
	DefaultHoldHeadroom            = 1.5
	DefaultWakeProbeInterval       = 250 * time.Millisecond
)

// Config holds the environment-driven sleep thresholds.
type Config struct {
	NetworkFloorBytesPerMin int64
	WakeCooldown            time.Duration
	PortWakeTimeout         time.Duration
	PortWakeRetry           time.Duration
	PortWakeCollect         time.Duration
	PortWakeMinPort         int
	IdleMin                 int
	IdleMax                 int
	IdleDefault             int
	HoldMin                 time.Duration
	HoldMax                 time.Duration
	HoldDefault             time.Duration
	HoldHeadroom            float64
	WakeProbeInterval       time.Duration
}

// ConfigFromEnv reads the Env* variables, using the defaults for anything
// missing or malformed.
func ConfigFromEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = os.Getenv
	}
	c := Config{
		NetworkFloorBytesPerMin: envInt64(getenv, EnvNetworkFloor, DefaultNetworkFloor),
		WakeCooldown:            envDuration(getenv, EnvWakeCooldown, DefaultWakeCooldown),
		PortWakeTimeout:         envDuration(getenv, EnvPortWakeTimeout, DefaultPortWakeTimeout),
		PortWakeRetry:           envDuration(getenv, EnvPortWakeRetry, DefaultPortWakeRetry),
		PortWakeCollect:         envDuration(getenv, EnvPortWakeCollect, DefaultPortWakeCollect),
		PortWakeMinPort:         int(envInt64(getenv, EnvPortWakeMinPort, DefaultPortWakeMinPort)),
		IdleMin:                 int(envInt64(getenv, EnvIdleMin, DefaultIdleMin)),
		IdleMax:                 int(envInt64(getenv, EnvIdleMax, DefaultIdleMax)),
		IdleDefault:             int(envInt64(getenv, EnvIdleDefault, DefaultIdleDefault)),
		HoldMin:                 envDuration(getenv, EnvHoldMin, DefaultHoldMin),
		HoldMax:                 envDuration(getenv, EnvHoldMax, DefaultHoldMax),
		HoldDefault:             envDuration(getenv, EnvHoldDefault, DefaultHoldDefault),
		HoldHeadroom:            DefaultHoldHeadroom,
		WakeProbeInterval:       envDuration(getenv, EnvWakeProbeInterval, DefaultWakeProbeInterval),
	}
	if f, err := strconv.ParseFloat(getenv(EnvHoldHeadroom), 64); err == nil && f >= 1 {
		c.HoldHeadroom = f
	}
	return c.normalized()
}

// normalized repairs bounds that contradict each other, falling back to the
// defaults so a typo cannot make the idle range or hold window empty.
func (c Config) normalized() Config {
	if c.IdleMin < 1 || c.IdleMax < c.IdleMin {
		c.IdleMin, c.IdleMax = DefaultIdleMin, DefaultIdleMax
	}
	c.IdleDefault = min(max(c.IdleDefault, c.IdleMin), c.IdleMax)
	if c.HoldMin <= 0 || c.HoldMax < c.HoldMin {
		c.HoldMin, c.HoldMax = DefaultHoldMin, DefaultHoldMax
	}
	c.HoldDefault = min(max(c.HoldDefault, c.HoldMin), c.HoldMax)
	return c
}

// HoldTimeout is how long a request is held while an app wakes: the measured
// p95 wake times HoldHeadroom, clamped to the floor and cap, or HoldDefault
// with no history.
func (c Config) HoldTimeout(p95Ms *int64) time.Duration {
	if p95Ms == nil || *p95Ms <= 0 {
		return c.HoldDefault
	}
	d := time.Duration(float64(*p95Ms) * c.HoldHeadroom * float64(time.Millisecond))
	return min(max(d, c.HoldMin), c.HoldMax)
}

func envInt64(getenv func(string) string, key string, def int64) int64 {
	if n, err := strconv.ParseInt(getenv(key), 10, 64); err == nil && n >= 0 {
		return n
	}
	return def
}

func envDuration(getenv func(string) string, key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(getenv(key)); err == nil && d >= 0 {
		return d
	}
	return def
}
