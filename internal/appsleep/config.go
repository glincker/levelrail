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
)

// Defaults for the variables above. 4096 bytes per minute sits above
// keepalives and probes (a few hundred bytes) and below real use.
const (
	DefaultNetworkFloor    int64 = 4096
	DefaultWakeCooldown          = 5 * time.Minute
	DefaultPortWakeTimeout       = 30 * time.Second
	DefaultPortWakeRetry         = 2 * time.Second
	DefaultPortWakeCollect       = 250 * time.Millisecond
	DefaultPortWakeMinPort       = 1024
)

// Config holds the environment-driven sleep thresholds.
type Config struct {
	NetworkFloorBytesPerMin int64
	WakeCooldown            time.Duration
	PortWakeTimeout         time.Duration
	PortWakeRetry           time.Duration
	PortWakeCollect         time.Duration
	PortWakeMinPort         int
}

// ConfigFromEnv reads the Env* variables, using the defaults for anything
// missing or malformed.
func ConfigFromEnv(getenv func(string) string) Config {
	if getenv == nil {
		getenv = os.Getenv
	}
	return Config{
		NetworkFloorBytesPerMin: envInt64(getenv, EnvNetworkFloor, DefaultNetworkFloor),
		WakeCooldown:            envDuration(getenv, EnvWakeCooldown, DefaultWakeCooldown),
		PortWakeTimeout:         envDuration(getenv, EnvPortWakeTimeout, DefaultPortWakeTimeout),
		PortWakeRetry:           envDuration(getenv, EnvPortWakeRetry, DefaultPortWakeRetry),
		PortWakeCollect:         envDuration(getenv, EnvPortWakeCollect, DefaultPortWakeCollect),
		PortWakeMinPort:         int(envInt64(getenv, EnvPortWakeMinPort, DefaultPortWakeMinPort)),
	}
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
