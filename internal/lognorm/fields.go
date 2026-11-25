// Package lognorm turns raw container log lines into clusterable records:
// it picks the useful JSON fields, joins stack traces and masks variable parts.
// All functions are pure; nothing here touches the store or the API.
package lognorm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Record is one logical log entry after field picking.
type Record struct {
	Message    string
	Level      string
	Error      string
	Structured bool
}

// Pick lists, matched case-insensitively and in priority order. Wiring slice:
// expose these as a runtime setting if operators need custom fields.
var (
	messageKeys = []string{"message", "msg", "log", "text", "event"}
	levelKeys   = []string{"level", "severity", "lvl", "levelname", "log.level"}
	errorKeys   = []string{"error", "err", "exception", "error_message", "stack_trace", "stacktrace"}
)

// Extract picks message, level and error from a JSON log line. A line that is
// not a JSON object comes back unchanged as the message.
func Extract(line string) Record {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return Record{Message: line}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return Record{Message: line}
	}
	lower := make(map[string]any, len(obj))
	for k, v := range obj {
		lower[strings.ToLower(k)] = v
	}
	rec := Record{
		Message:    pick(lower, messageKeys),
		Level:      strings.ToLower(pick(lower, levelKeys)),
		Error:      pick(lower, errorKeys),
		Structured: true,
	}
	if rec.Message == "" && rec.Error == "" {
		// No known field: keep the raw line so nothing is silently dropped.
		rec.Message = line
	}
	return rec
}

// Text renders the record as one line suitable for clustering.
func (r Record) Text() string {
	switch {
	case r.Message != "" && r.Error != "" && !strings.Contains(r.Message, r.Error):
		return r.Message + ": " + r.Error
	case r.Message != "":
		return r.Message
	default:
		return r.Error
	}
}

func pick(m map[string]any, keys []string) string {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		if s := stringify(v); s != "" {
			return s
		}
	}
	return ""
}

func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64, bool:
		return fmt.Sprint(t)
	case map[string]any:
		// Nested error objects usually carry the text under message.
		for _, k := range []string{"message", "msg"} {
			if s, ok := t[k].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
