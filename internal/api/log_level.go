package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	levelTrace = telemetry.LevelTrace
	levelDebug = telemetry.LevelDebug
	levelInfo  = telemetry.LevelInfo
	levelWarn  = telemetry.LevelWarn
	levelError = telemetry.LevelError
	levelFatal = telemetry.LevelFatal
)

func normalizeLevel(s string) string { return telemetry.NormalizeLevel(s) }

func classifyLogLevel(e telemetry.LogEntry) string { return telemetry.ClassifyLevel(e) }

// parseLevelParam parses the ?level= minimum level; empty means no filter.
func parseLevelParam(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	l := normalizeLevel(raw)
	if l == "" {
		return "", errors.New("level must be one of trace, debug, info, warn, error, fatal")
	}
	return l, nil
}

// parseLimitParam parses the ?limit= newest-entries cap; 0 means no cap.
func parseLimitParam(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errors.New("limit must be a positive integer")
	}
	return n, nil
}

// filterLogsByLevel keeps entries at or above min. Entries with no
// detectable level are dropped when a filter is set.
func filterLogsByLevel(entries []telemetry.LogEntry, minLevel string) []telemetry.LogEntry {
	if minLevel == "" {
		return entries
	}
	out := entries[:0:0]
	for _, e := range entries {
		if telemetry.LevelRank(classifyLogLevel(e)) >= telemetry.LevelRank(minLevel) {
			out = append(out, e)
		}
	}
	return out
}
