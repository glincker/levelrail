package main

import (
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
)

const (
	defaultPRCommandsPerPRPerMinute   = 5
	defaultPRCommandsPerUserPerMinute = 10
	defaultPRCommandPermissionCache   = 1 * time.Minute
	defaultPRCommandLogRetention      = 30 * 24 * time.Hour
	defaultPRCommandRunTimeout        = 30 * time.Minute
)

// prCommentCommandConfig reads the pull request comment command settings.
// APP_PR_COMMENT_COMMANDS_DISABLED is the kill switch: any true value, and any
// value that does not parse, turns the feature off for every app.
func prCommentCommandConfig(logger *slog.Logger) api.PRCommentCommandConfig {
	disabled := false
	if raw := os.Getenv("APP_PR_COMMENT_COMMANDS_DISABLED"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			logger.Warn("invalid APP_PR_COMMENT_COMMANDS_DISABLED, keeping comment commands off", slog.String("value", raw))
			v = true
		}
		disabled = v
	}
	return api.PRCommentCommandConfig{
		Disabled:           disabled,
		PerPRPerMinute:     positiveEnvIntOr(logger, "APP_PR_COMMENT_COMMANDS_PER_PR_PER_MINUTE", defaultPRCommandsPerPRPerMinute),
		PerUserPerMinute:   positiveEnvIntOr(logger, "APP_PR_COMMENT_COMMANDS_PER_USER_PER_MINUTE", defaultPRCommandsPerUserPerMinute),
		PermissionCacheTTL: envDurationOrDefault(logger, "APP_PR_COMMENT_PERMISSION_CACHE", defaultPRCommandPermissionCache),
		LogRetention:       envDurationOrDefault(logger, "APP_PR_COMMENT_LOG_RETENTION", defaultPRCommandLogRetention),
		RunTimeout:         envDurationOrDefault(logger, "APP_PR_COMMENT_RUN_TIMEOUT", defaultPRCommandRunTimeout),
	}
}

func positiveEnvIntOr(logger *slog.Logger, name string, def int) int {
	v := nonNegativeEnvInt(logger, name, def)
	if v == 0 {
		logger.Warn(name+" must be above zero, using the default", slog.Int("default", def))
		return def
	}
	return v
}
