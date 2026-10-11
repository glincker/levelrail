package dataexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Log levels in ascending severity, used by the min_level filter.
var levelRank = map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}

// LogRecord is one container log line prepared for export.
type LogRecord struct {
	Time    time.Time
	Kind    string
	App     string
	Stream  string
	Level   string
	Message string
	Fields  json.RawMessage
}

// NewLogRecord derives the export shape of a stored log line. The level is
// the structured "level" or "severity" field when the line is JSON, else
// "error" for stderr and "info" for stdout.
func NewLogRecord(e telemetry.LogEntry) LogRecord {
	kind, name, _ := strings.Cut(e.ResourceID, ":")
	switch kind {
	case "service":
		kind = "app"
	case "":
		kind, name = "app", e.ResourceID
	}
	r := LogRecord{Time: e.Timestamp.UTC(), Kind: kind, App: name, Stream: e.Stream, Message: e.Message}
	r.Level = "info"
	if e.Stream == "stderr" {
		r.Level = "error"
	}
	if e.Structured && e.FieldsJSON != "" {
		r.Fields = json.RawMessage(e.FieldsJSON)
		var f map[string]any
		if json.Unmarshal([]byte(e.FieldsJSON), &f) == nil {
			for _, k := range []string{"level", "severity", "lvl"} {
				if v, ok := f[k].(string); ok {
					if l := normalizeLevel(v); l != "" {
						r.Level = l
						break
					}
				}
			}
		}
	}
	return r
}

func normalizeLevel(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug", "trace", "dbg":
		return "debug"
	case "info", "information", "notice":
		return "info"
	case "warn", "warning":
		return "warn"
	case "error", "err", "fatal", "critical", "crit", "panic":
		return "error"
	}
	return ""
}

// ValidLevel reports whether v is an accepted min_level filter value.
func ValidLevel(v string) bool {
	_, ok := levelRank[v]
	return v == "" || ok
}

// matches applies the app and level filters of a destination.
func (r LogRecord) matches(apps map[string]struct{}, minLevel string) bool {
	if len(apps) > 0 {
		if _, ok := apps[r.App]; !ok || r.Kind != "app" {
			return false
		}
	}
	if minLevel != "" && levelRank[r.Level] < levelRank[minLevel] {
		return false
	}
	return true
}

func (r LogRecord) size() int { return len(r.Message) + len(r.Fields) + 96 }

type httpLine struct {
	Time    string          `json:"time"`
	Kind    string          `json:"kind"`
	App     string          `json:"app"`
	Stream  string          `json:"stream"`
	Level   string          `json:"level"`
	Message string          `json:"message"`
	Fields  json.RawMessage `json:"fields,omitempty"`
}

// httpStatusError carries a non-2xx response; 4xx other than 408 and 429 are
// permanent.
func statusError(what string, code int, body io.Reader) error {
	snippet, _ := io.ReadAll(io.LimitReader(body, 256))
	err := fmt.Errorf("%s returned status %d: %s", what, code, strings.TrimSpace(string(snippet)))
	if code >= 400 && code < 500 && code != http.StatusRequestTimeout && code != http.StatusTooManyRequests {
		return permanent(err)
	}
	return err
}

func postBody(ctx context.Context, client *http.Client, url, contentType string, headers map[string]string, body []byte, what string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return permanent(fmt.Errorf("build %s request: %w", what, err))
	}
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", what, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(what, resp.StatusCode, resp.Body)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

// httpLogSender posts newline-delimited JSON, one object per log line.
type httpLogSender struct {
	url     string
	client  *http.Client
	headers map[string]string
}

func (s *httpLogSender) send(ctx context.Context, recs []LogRecord) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range recs {
		if err := enc.Encode(httpLine{
			Time: r.Time.Format(time.RFC3339Nano), Kind: r.Kind, App: r.App, Stream: r.Stream,
			Level: r.Level, Message: r.Message, Fields: r.Fields,
		}); err != nil {
			return permanent(fmt.Errorf("encode log line: %w", err))
		}
	}
	return postBody(ctx, s.client, s.url, "application/x-ndjson", s.headers, buf.Bytes(), "http log destination")
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// lokiSender pushes to Loki's /loki/api/v1/push JSON endpoint. Labels are
// bounded to job, app, kind, stream, level plus operator static labels.
type lokiSender struct {
	url     string
	client  *http.Client
	headers map[string]string
	static  map[string]string
	job     string
}

// LokiPushURL appends Loki's push path when the endpoint has none.
func LokiPushURL(endpoint string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	idx := strings.Index(trimmed, "://")
	if idx >= 0 && !strings.Contains(trimmed[idx+3:], "/") {
		return trimmed + "/loki/api/v1/push"
	}
	return endpoint
}

func (s *lokiSender) payload(recs []LogRecord) ([]byte, error) {
	groups := map[string]*lokiStream{}
	var order []string
	for _, r := range recs {
		labels := map[string]string{"job": s.job, "app": r.App, "kind": r.Kind, "stream": r.Stream, "level": r.Level}
		for k, v := range s.static {
			labels[k] = v
		}
		keys := make([]string, 0, len(labels))
		for k := range labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var id strings.Builder
		for _, k := range keys {
			id.WriteString(k + "=" + labels[k] + "\x00")
		}
		g, ok := groups[id.String()]
		if !ok {
			g = &lokiStream{Stream: labels}
			groups[id.String()] = g
			order = append(order, id.String())
		}
		g.Values = append(g.Values, [2]string{strconv.FormatInt(r.Time.UnixNano(), 10), r.Message})
	}
	streams := make([]*lokiStream, 0, len(order))
	for _, k := range order {
		streams = append(streams, groups[k])
	}
	return json.Marshal(map[string]any{"streams": streams})
}

func (s *lokiSender) send(ctx context.Context, recs []LogRecord) error {
	body, err := s.payload(recs)
	if err != nil {
		return permanent(fmt.Errorf("encode loki payload: %w", err))
	}
	return postBody(ctx, s.client, s.url, "application/json", s.headers, body, "loki")
}
