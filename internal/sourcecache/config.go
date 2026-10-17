package sourcecache

import (
	"os"
	"strconv"
	"time"
)

// Environment variables and defaults for the cache and the fetches that fill it.
const (
	EnvMaxBytes     = "APP_SOURCE_CACHE_MAX_BYTES"
	EnvMaxAge       = "APP_SOURCE_CACHE_MAX_AGE"
	EnvFetchTimeout = "APP_GIT_FETCH_TIMEOUT"
	EnvRetries      = "APP_GIT_FETCH_RETRIES"
	EnvBackoff      = "APP_GIT_FETCH_BACKOFF"
	EnvProbeEvery   = "APP_GIT_RECOVERY_INTERVAL"

	DefaultMaxBytes     int64 = 5 << 30
	DefaultMaxAge             = 30 * 24 * time.Hour
	DefaultFetchTimeout       = 10 * time.Minute
	DefaultRetries            = 2
	DefaultBackoff            = 2 * time.Second
	DefaultProbeEvery         = 30 * time.Second
)

// Config bounds the mirror and the network calls made to fill it. An empty
// Dir keeps no mirror: every checkout is a fresh, throwaway fetch.
type Config struct {
	Dir          string
	MaxBytes     int64
	MaxAge       time.Duration
	FetchTimeout time.Duration
	Retries      int
	Backoff      time.Duration
}

// ConfigFromEnv reads the cache and fetch limits from the environment; a
// malformed or non-positive value falls back to its default.
func ConfigFromEnv(dir string) Config {
	return Config{
		Dir:          dir,
		MaxBytes:     envInt64(EnvMaxBytes, DefaultMaxBytes),
		MaxAge:       envDuration(EnvMaxAge, DefaultMaxAge),
		FetchTimeout: envDuration(EnvFetchTimeout, DefaultFetchTimeout),
		Retries:      int(envInt64(EnvRetries, DefaultRetries)),
		Backoff:      envDuration(EnvBackoff, DefaultBackoff),
	}
}

// ProbeInterval is how often unreachable hosts are re-checked.
func ProbeInterval() time.Duration { return envDuration(EnvProbeEvery, DefaultProbeEvery) }

func (c Config) withDefaults() Config {
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.MaxAge <= 0 {
		c.MaxAge = DefaultMaxAge
	}
	if c.FetchTimeout <= 0 {
		c.FetchTimeout = DefaultFetchTimeout
	}
	if c.Retries < 0 {
		c.Retries = 0
	}
	if c.Backoff <= 0 {
		c.Backoff = DefaultBackoff
	}
	return c
}

func envInt64(key string, def int64) int64 {
	v, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || v < 0 {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(os.Getenv(key))
	if err != nil || d <= 0 {
		return def
	}
	return d
}
