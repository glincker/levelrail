package telemetry

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Log levels in ascending severity; an empty level means the line carries none.
const (
	LevelTrace = "trace"
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
	LevelFatal = "fatal"
)

var levelRank = map[string]int{LevelTrace: 1, LevelDebug: 2, LevelInfo: 3, LevelWarn: 4, LevelError: 5, LevelFatal: 6}

// LevelRank orders a normalised level; unknown or empty is 0.
func LevelRank(level string) int { return levelRank[level] }

// levelPrefixLen bounds how much of a plain line is searched for a level
// token, so a level word deep in a message body does not classify it.
const levelPrefixLen = 64

const levelTokens = `trace|debug|info|notice|warn|warning|err|error|fatal|panic|crit|critical`

var (
	upperLevelRE  = regexp.MustCompile(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERR|ERROR|FATAL|PANIC|CRIT|CRITICAL)\b`)
	markedLevelRE = regexp.MustCompile(`(?i)(?:\blevel[=:]\s*"?|\[\s*)(` + levelTokens + `)\b`)
	startLevelRE  = regexp.MustCompile(`(?i)^\s*(` + levelTokens + `)\s*:`)
)

// NormalizeLevel maps a level word to one of the Level constants, or "".
func NormalizeLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "info", "notice":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "err", "error":
		return LevelError
	case "fatal", "panic", "crit", "critical":
		return LevelFatal
	default:
		return ""
	}
}

// NumericLevel maps a pino/bunyan style numeric level to a Level constant.
func NumericLevel(n float64) string {
	switch {
	case n >= 60:
		return LevelFatal
	case n >= 50:
		return LevelError
	case n >= 40:
		return LevelWarn
	case n >= 30:
		return LevelInfo
	case n >= 20:
		return LevelDebug
	case n >= 10:
		return LevelTrace
	default:
		return ""
	}
}

// ClassifyLevel returns the entry's level: from the structured
// level/severity field when the line is JSON, else from a level token
// near the start of a plain line. Empty means unknown.
func ClassifyLevel(e LogEntry) string {
	if e.Structured && e.FieldsJSON != "" {
		var fields map[string]any
		if err := json.Unmarshal([]byte(e.FieldsJSON), &fields); err == nil {
			for _, key := range []string{"level", "severity", "lvl", "log.level"} {
				switch v := fields[key].(type) {
				case string:
					if l := NormalizeLevel(v); l != "" {
						return l
					}
				case float64:
					if l := NumericLevel(v); l != "" {
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
		return NormalizeLevel(m[1])
	}
	if m := markedLevelRE.FindStringSubmatch(head); m != nil {
		return NormalizeLevel(m[1])
	}
	if m := startLevelRE.FindStringSubmatch(head); m != nil {
		return NormalizeLevel(m[1])
	}
	return ""
}
