package deploytimeline

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variables that tune the timeline. Each has a fallback so an
// unset variable never disables the feature.
const (
	EnvRetention    = "APP_DEPLOY_TIMELINE_RETENTION"
	EnvMaxLogLines  = "APP_DEPLOY_TIMELINE_MAX_LOG_LINES"
	EnvImpactWindow = "APP_DEPLOY_TIMELINE_IMPACT_WINDOW"
	EnvActorWindow  = "APP_DEPLOY_TIMELINE_ACTOR_WINDOW"
	EnvSettleIdle   = "APP_DEPLOY_TIMELINE_SETTLE_IDLE"
)

const (
	defaultRetention    = 30 * 24 * time.Hour
	defaultMaxLogLines  = 500
	defaultImpactWindow = 5 * time.Minute
	defaultActorWindow  = time.Minute
	defaultSettleIdle   = 5 * time.Minute
)

// Config is the resolved tuning for one process.
type Config struct {
	Retention    time.Duration
	MaxLogLines  int
	ImpactWindow time.Duration
	ActorWindow  time.Duration
	SettleIdle   time.Duration
}

// ConfigFromEnv reads the tuning variables.
func ConfigFromEnv() Config {
	return Config{
		Retention:    envDuration(EnvRetention, defaultRetention),
		MaxLogLines:  envInt(EnvMaxLogLines, defaultMaxLogLines),
		ImpactWindow: envDuration(EnvImpactWindow, defaultImpactWindow),
		ActorWindow:  envDuration(EnvActorWindow, defaultActorWindow),
		SettleIdle:   envDuration(EnvSettleIdle, defaultSettleIdle),
	}
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name))); err == nil && d > 0 {
		return d
	}
	return def
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && n > 0 {
		return n
	}
	return def
}
