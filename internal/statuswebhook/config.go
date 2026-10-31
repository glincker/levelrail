// Package statuswebhook delivers signed status page events to subscriber URLs.
package statuswebhook

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variables that tune delivery. Every value has a default; none is a secret.
const (
	EnvMaxSubscribers = "APP_STATUS_WEBHOOK_MAX_SUBSCRIBERS"
	EnvTimeout        = "APP_STATUS_WEBHOOK_TIMEOUT"
	EnvMaxAttempts    = "APP_STATUS_WEBHOOK_MAX_ATTEMPTS"
	EnvBackoffBase    = "APP_STATUS_WEBHOOK_BACKOFF_BASE"
	EnvBackoffMax     = "APP_STATUS_WEBHOOK_BACKOFF_MAX"
	EnvPauseAfter     = "APP_STATUS_WEBHOOK_PAUSE_AFTER"
	EnvPoll           = "APP_STATUS_WEBHOOK_POLL_INTERVAL"
	EnvLogKeep        = "APP_STATUS_WEBHOOK_LOG_KEEP"
	EnvAllowHTTP      = "APP_STATUS_WEBHOOK_ALLOW_HTTP"
	EnvAllowPrivate   = "APP_STATUS_WEBHOOK_ALLOW_PRIVATE"
)

// Config holds the delivery limits.
type Config struct {
	MaxSubscribers int
	Timeout        time.Duration
	MaxAttempts    int
	BackoffBase    time.Duration
	BackoffMax     time.Duration
	PauseAfter     int
	Poll           time.Duration
	LogKeep        int
	AllowHTTP      bool
	AllowPrivate   bool
}

// ConfigFromEnv reads Config, falling back to defaults for unset or invalid values.
func ConfigFromEnv() Config {
	return Config{
		MaxSubscribers: envInt(EnvMaxSubscribers, 20),
		Timeout:        envDur(EnvTimeout, 10*time.Second),
		MaxAttempts:    envInt(EnvMaxAttempts, 5),
		BackoffBase:    envDur(EnvBackoffBase, 30*time.Second),
		BackoffMax:     envDur(EnvBackoffMax, time.Hour),
		PauseAfter:     envInt(EnvPauseAfter, 10),
		Poll:           envDur(EnvPoll, 5*time.Second),
		LogKeep:        envInt(EnvLogKeep, 50),
		AllowHTTP:      envBool(EnvAllowHTTP),
		AllowPrivate:   envBool(EnvAllowPrivate),
	}
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && n > 0 {
		return n
	}
	return def
}

func envDur(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name))); err == nil && d > 0 {
		return d
	}
	return def
}

func envBool(name string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(name)))
	return err == nil && b
}
