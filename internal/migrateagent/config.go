package migrateagent

import (
	"os"
	"time"
)

// Config holds the env-tunable limits of the migration agent.
type Config struct {
	TokenTTL       time.Duration
	SessionMax     time.Duration
	RequestTimeout time.Duration
	VolumesDir     string
}

const (
	defaultTokenTTL       = time.Hour
	defaultSessionMax     = 72 * time.Hour
	defaultRequestTimeout = 15 * time.Minute
	minTokenTTL           = time.Minute
	maxTokenTTL           = 24 * time.Hour
	minSessionMax         = time.Hour
	maxSessionMax         = 7 * 24 * time.Hour
)

func clamp(d, lo, hi time.Duration) time.Duration { return min(max(d, lo), hi) }

func envDuration(key string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(key)); err == nil && d > 0 {
		return d
	}
	return def
}

// ConfigFromEnv reads APP_MIGRATION_TOKEN_TTL, APP_MIGRATION_SESSION_MAX,
// APP_MIGRATION_REQUEST_TIMEOUT and APP_DOCKER_VOLUMES_DIR, clamping the
// durations so a typo can neither disable expiry nor make tokens unusable.
func ConfigFromEnv() Config {
	dir := os.Getenv("APP_DOCKER_VOLUMES_DIR")
	if dir == "" {
		dir = "/var/lib/docker/volumes"
	}
	return Config{
		TokenTTL:       clamp(envDuration("APP_MIGRATION_TOKEN_TTL", defaultTokenTTL), minTokenTTL, maxTokenTTL),
		SessionMax:     clamp(envDuration("APP_MIGRATION_SESSION_MAX", defaultSessionMax), minSessionMax, maxSessionMax),
		RequestTimeout: envDuration("APP_MIGRATION_REQUEST_TIMEOUT", defaultRequestTimeout),
		VolumesDir:     dir,
	}
}

func (c Config) withDefaults() Config {
	if c.TokenTTL <= 0 {
		c.TokenTTL = defaultTokenTTL
	}
	if c.SessionMax <= 0 {
		c.SessionMax = defaultSessionMax
	}
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = defaultRequestTimeout
	}
	return c
}
