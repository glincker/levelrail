package main

import (
	"log/slog"
	"strings"
)

// envLogLevel selects the agent log level at startup.
const envLogLevel = "APP_LOG_LEVEL"

// parseLogLevel maps a level name to a slog level. An empty value is info
// silently; an unknown one is info with ok false so the caller can warn.
func parseLogLevel(raw string) (level slog.Level, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return slog.LevelInfo, true
	case "debug":
		return slog.LevelDebug, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}
