package main

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/statuspage"
)

const defaultStatusPageRateLimitRPM = 120

func warnEnvInt(logger *slog.Logger, name string, def int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		logger.Warn("invalid "+name+", using the default", slog.String("value", raw))
		return def
	}
	return n
}

func warnEnvDuration(logger *slog.Logger, name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid "+name+", using the default", slog.String("value", raw))
		return def
	}
	return d
}

// alertNoiseConfig reads the alert noise control defaults. Zero disables
// consecutive-failure holding, flapping detection, grouping, the channel
// rate limit or history pruning respectively; a rule's own fields
// override the first three.
func alertNoiseConfig(logger *slog.Logger) alerting.NoiseConfig {
	return alerting.NoiseConfig{
		ConsecutiveFailures: warnEnvInt(logger, "APP_ALERT_CONSECUTIVE_FAILURES", 1),
		FlapThreshold:       warnEnvInt(logger, "APP_ALERT_FLAP_THRESHOLD", alerting.DefaultFlapThreshold),
		FlapWindow:          warnEnvDuration(logger, "APP_ALERT_FLAP_WINDOW", alerting.DefaultFlapWindow),
		GroupWindow:         warnEnvDuration(logger, "APP_ALERT_GROUP_WINDOW", 0),
		RateLimit:           warnEnvInt(logger, "APP_ALERT_CHANNEL_RATE_LIMIT", alerting.DefaultRateLimit),
		RateWindow:          warnEnvDuration(logger, "APP_ALERT_CHANNEL_RATE_WINDOW", alerting.DefaultRateWindow),
		HistoryRetention:    warnEnvDuration(logger, "APP_ALERT_HISTORY_RETENTION", alerting.DefaultHistoryRetention),
	}
}

func statusPageConfig(logger *slog.Logger) statuspage.Config {
	return statuspage.Config{
		SampleInterval: warnEnvDuration(logger, "APP_STATUS_PAGE_SAMPLE_INTERVAL", statuspage.DefaultSampleInterval),
		CacheTTL:       warnEnvDuration(logger, "APP_STATUS_PAGE_CACHE_TTL", statuspage.DefaultCacheTTL),
		CheckTimeout:   warnEnvDuration(logger, "APP_STATUS_PAGE_CHECK_TIMEOUT", statuspage.DefaultCheckTimeout),
	}
}

func statusPageRateLimit(logger *slog.Logger) int {
	return warnEnvInt(logger, "APP_STATUS_PAGE_RATE_LIMIT_RPM", defaultStatusPageRateLimitRPM)
}
