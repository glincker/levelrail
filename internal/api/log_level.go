package api

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Log levels in ascending severity; an empty level means the line
// carries none.
const (
	levelTrace = "trace"
	levelDebug = "debug"
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
	levelFatal = "fatal"
)

var levelRank = map[string]int{levelTrace: 1, levelDebug: 2, levelInfo: 3, levelWarn: 4, levelError: 5, levelFatal: 6}

// levelPrefixLen bounds how much of a plain line is searched for a level
// token, so a level word deep in a message body does not classify it.
const levelPrefixLen = 64

const levelTokens = `trace|debug|info|notice|warn|warning|err|error|fatal|panic|crit|critical`

// Upper case tokens anywhere in the prefix, or a lower case token in an
// explicit level=, [level] or "error:" position.
var (
	upperLevelRE  = regexp.MustCompile(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERR|ERROR|FATAL|PANIC|CRIT|CRITICAL)\b`)
	markedLevelRE = regexp.MustCompile(`(?i)(?:\blevel[=:]\s*"?|\[\s*)(` + levelTokens + `)\b`)
	startLevelRE  = regexp.MustCompile(`(?i)^\s*(` + levelTokens + `)\s*:`)
)

func normalizeLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return levelTrace
	case "debug":
		return levelDebug
	case "info", "notice":
		return levelInfo
	case "warn", "warning":
		return levelWarn
	case "err", "error":
		return levelError
	case "fatal", "panic", "crit", "critical":
		return levelFatal
	default:
		return ""
	}
}

func numericLevel(n float64) string {
	switch {
	case n >= 60:
		return levelFatal
	case n >= 50:
		return levelError
	case n >= 40:
		return levelWarn
	case n >= 30:
		return levelInfo
	case n >= 20:
		return levelDebug
	case n >= 10:
		return levelTrace
	default:
		return ""
	}
}

// classifyLogLevel returns the entry's level: from the structured
// level/severity field when the line is JSON, else from a level token
// near the start of a plain line. Empty means unknown.
func classifyLogLevel(e telemetry.LogEntry) string {
	if e.Structured && e.FieldsJSON != "" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(e.FieldsJSON), &fields); err == nil {
			for _, key := range []string{"level", "severity", "lvl", "log.level"} {
				switch v := fields[key].(type) {
				case string:
					if l := normalizeLevel(v); l != "" {
						return l
					}
				case float64:
					if l := numericLevel(v); l != "" {
						return l
					}
				}
			}
		}
		return ""
	}
	head := e.Message
	if len(head) > levelPrefixLen {
		head = head[:levelPrefixLen]
		for !utf8.ValidString(head) && len(head) > 0 {
			head = head[:len(head)-1]
		}
	}
	if m := upperLevelRE.FindStringSubmatch(head); m != nil {
		return normalizeLevel(m[1])
	}
	if m := markedLevelRE.FindStringSubmatch(head); m != nil {
		return normalizeLevel(m[1])
	}
	if m := startLevelRE.FindStringSubmatch(head); m != nil {
		return normalizeLevel(m[1])
	}
	return ""
}

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
		if levelRank[classifyLogLevel(e)] >= levelRank[minLevel] {
			out = append(out, e)
		}
	}
	return out
}
