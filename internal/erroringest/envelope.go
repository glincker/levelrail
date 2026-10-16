package erroringest

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// MaxEventsPerEnvelope bounds the events taken from one request.
const MaxEventsPerEnvelope = 20

const maxTitle = 200

// ErrMalformed is returned when a body is neither an envelope nor an event.
var ErrMalformed = errors.New("sentry: malformed envelope or event")

var droppableKeys = []string{"breadcrumbs", "modules", "extra", "contexts", "sdk", "user", "tags"}

// ParseEnvelope returns the error events of a Sentry envelope. Items of any
// other type (sessions, transactions, attachments, ...) are skipped.
func ParseEnvelope(body []byte) ([]telemetry.ErrorEvent, error) {
	rest := body
	line, rest, ok := nextLine(rest)
	if !ok || len(bytes.TrimSpace(line)) == 0 {
		return nil, ErrMalformed
	}
	var hdr map[string]json.RawMessage
	if err := json.Unmarshal(line, &hdr); err != nil {
		return nil, ErrMalformed
	}
	var out []telemetry.ErrorEvent
	for len(bytes.TrimSpace(rest)) > 0 && len(out) < MaxEventsPerEnvelope {
		var ih []byte
		ih, rest, _ = nextLine(rest)
		if len(bytes.TrimSpace(ih)) == 0 {
			continue
		}
		var item struct {
			Type   string `json:"type"`
			Length *int   `json:"length"`
		}
		if err := json.Unmarshal(ih, &item); err != nil {
			return out, ErrMalformed
		}
		var payload []byte
		if item.Length != nil {
			n := *item.Length
			if n < 0 || n > len(rest) {
				return out, ErrMalformed
			}
			payload, rest = rest[:n], rest[n:]
			if len(rest) > 0 && rest[0] == '\n' {
				rest = rest[1:]
			}
		} else {
			payload, rest, _ = nextLine(rest)
		}
		if item.Type != "event" {
			continue
		}
		ev, err := buildEvent(payload)
		if err != nil {
			continue
		}
		out = append(out, ev)
	}
	return out, nil
}

// ParseStore decodes the body of the legacy /store/ endpoint: a JSON event,
// or the base64 + zlib form old SDKs sent.
func ParseStore(body []byte) (telemetry.ErrorEvent, error) {
	b := bytes.TrimSpace(body)
	if len(b) > 0 && b[0] != '{' {
		raw, err := base64.StdEncoding.DecodeString(string(b))
		if err != nil {
			return telemetry.ErrorEvent{}, ErrMalformed
		}
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return telemetry.ErrorEvent{}, ErrMalformed
		}
		defer func() { _ = zr.Close() }()
		if b, err = io.ReadAll(io.LimitReader(zr, 4<<20)); err != nil {
			return telemetry.ErrorEvent{}, ErrMalformed
		}
	}
	return buildEvent(b)
}

func nextLine(b []byte) (line, rest []byte, ok bool) {
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return b, nil, len(b) > 0
	}
	return b[:i], b[i+1:], true
}

func buildEvent(payload []byte) (telemetry.ErrorEvent, error) {
	var doc map[string]any
	if err := json.Unmarshal(payload, &doc); err != nil {
		return telemetry.ErrorEvent{}, ErrMalformed
	}
	var typed struct {
		EventID  string          `json:"event_id"`
		Level    string          `json:"level"`
		Message  json.RawMessage `json:"message"`
		Logentry *struct {
			Message   string `json:"message"`
			Formatted string `json:"formatted"`
		} `json:"logentry"`
		Exception   json.RawMessage `json:"exception"`
		Fingerprint []string        `json:"fingerprint"`
		Release     string          `json:"release"`
		Environment string          `json:"environment"`
	}
	if err := json.Unmarshal(payload, &typed); err != nil {
		// Mistyped optional fields should not lose the event.
		typed.Fingerprint = nil
	}
	ex := lastException(typed.Exception)
	msg := messageText(typed.Message)
	if msg == "" && typed.Logentry != nil {
		msg = typed.Logentry.Formatted
		if msg == "" {
			msg = typed.Logentry.Message
		}
	}
	if ex == nil && msg == "" {
		return telemetry.ErrorEvent{}, ErrMalformed
	}
	title := msg
	if ex != nil && ex.Type != "" {
		title = ex.Type
		if ex.Value != "" {
			title += ": " + ex.Value
		}
	}
	if len(title) > maxTitle {
		title = title[:maxTitle]
	}
	id := strings.ReplaceAll(typed.EventID, "-", "")
	if id == "" {
		var b [16]byte
		_, _ = rand.Read(b[:])
		id = hex.EncodeToString(b[:])
	}
	level := strings.ToLower(typed.Level)
	switch level {
	case "fatal", "error", "warning", "info", "debug":
	default:
		level = "error"
	}
	return telemetry.ErrorEvent{
		Fingerprint: fingerprint(typed.Fingerprint, ex, msg),
		Title:       title,
		Level:       level,
		EventID:     id,
		TimestampNs: timestampNs(doc["timestamp"]),
		Release:     typed.Release,
		Environment: typed.Environment,
		Raw:         boundedRaw(doc),
	}, nil
}

func messageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Formatted string `json:"formatted"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(raw, &o) == nil {
		if o.Formatted != "" {
			return o.Formatted
		}
		return o.Message
	}
	return ""
}

// lastException picks the outermost raised exception: SDKs list a chain
// oldest cause first, so the last value is what actually propagated.
func lastException(raw json.RawMessage) *exceptionValue {
	if len(raw) == 0 {
		return nil
	}
	var wrapped struct {
		Values []exceptionValue `json:"values"`
	}
	if json.Unmarshal(raw, &wrapped) == nil && len(wrapped.Values) > 0 {
		return &wrapped.Values[len(wrapped.Values)-1]
	}
	var list []exceptionValue
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return &list[len(list)-1]
	}
	return nil
}

func timestampNs(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x * 1e9)
	case string:
		if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
			return t.UnixNano()
		}
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return int64(f * 1e9)
		}
	}
	return time.Now().UnixNano()
}

// boundedRaw scrubs the event and shrinks it under MaxRawBytes by dropping
// bulky optional sections before giving up and keeping a stub.
func boundedRaw(doc map[string]any) string {
	scrub(doc)
	for i := 0; ; i++ {
		b, err := json.Marshal(doc)
		if err != nil {
			return "{}"
		}
		if len(b) <= MaxRawBytes {
			return string(b)
		}
		if i >= len(droppableKeys) {
			stub := map[string]any{"truncated": true}
			for _, k := range []string{"event_id", "level", "message", "exception", "release", "environment", "timestamp"} {
				if v, ok := doc[k]; ok {
					stub[k] = v
				}
			}
			if sb, err := json.Marshal(stub); err == nil && len(sb) <= MaxRawBytes {
				return string(sb)
			}
			return fmt.Sprintf(`{"truncated":true,"event_id":%q}`, doc["event_id"])
		}
		delete(doc, droppableKeys[i])
	}
}
