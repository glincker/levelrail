package dataexport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type otlpValue struct {
	StringValue string `json:"stringValue"`
}

type otlpKV struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpLogRecord struct {
	TimeUnixNano   string    `json:"timeUnixNano"`
	SeverityNumber int       `json:"severityNumber"`
	SeverityText   string    `json:"severityText"`
	Body           otlpValue `json:"body"`
	Attributes     []otlpKV  `json:"attributes,omitempty"`
}

type otlpScopeLogs struct {
	Scope      map[string]string `json:"scope"`
	LogRecords []otlpLogRecord   `json:"logRecords"`
}

type otlpResourceLogs struct {
	Resource  map[string][]otlpKV `json:"resource"`
	ScopeLogs []otlpScopeLogs     `json:"scopeLogs"`
}

var otlpSeverity = map[string]int{"debug": 5, "info": 9, "warn": 13, "error": 17}

// OTLPLogsURL appends the standard logs path when the endpoint has none.
func OTLPLogsURL(endpoint string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	idx := strings.Index(trimmed, "://")
	if idx >= 0 && !strings.Contains(trimmed[idx+3:], "/") {
		return trimmed + "/v1/logs"
	}
	return endpoint
}

// otlpLogSender posts OTLP/HTTP JSON ExportLogsServiceRequest bodies. Each
// app becomes one resource (service.name = app) carrying the operator's
// static labels, so a collector can route and filter on it.
type otlpLogSender struct {
	url     string
	client  *http.Client
	headers map[string]string
	static  map[string]string
	scope   string
}

func (s *otlpLogSender) payload(recs []LogRecord) ([]byte, error) {
	type key struct{ kind, app string }
	groups := map[key]*otlpScopeLogs{}
	var order []key
	for _, r := range recs {
		k := key{r.Kind, r.App}
		g, ok := groups[k]
		if !ok {
			g = &otlpScopeLogs{Scope: map[string]string{"name": s.scope}}
			groups[k] = g
			order = append(order, k)
		}
		rec := otlpLogRecord{
			TimeUnixNano:   strconv.FormatInt(r.Time.UnixNano(), 10),
			SeverityNumber: otlpSeverity[r.Level],
			SeverityText:   strings.ToUpper(r.Level),
			Body:           otlpValue{StringValue: r.Message},
			Attributes:     []otlpKV{{Key: "stream", Value: otlpValue{r.Stream}}},
		}
		if len(r.Fields) > 0 {
			rec.Attributes = append(rec.Attributes, otlpKV{Key: "fields", Value: otlpValue{string(r.Fields)}})
		}
		g.LogRecords = append(g.LogRecords, rec)
	}
	staticKeys := make([]string, 0, len(s.static))
	for k := range s.static {
		staticKeys = append(staticKeys, k)
	}
	sort.Strings(staticKeys)
	out := make([]otlpResourceLogs, 0, len(order))
	for _, k := range order {
		attrs := []otlpKV{
			{Key: "service.name", Value: otlpValue{k.app}},
			{Key: "kind", Value: otlpValue{k.kind}},
		}
		for _, sk := range staticKeys {
			attrs = append(attrs, otlpKV{Key: sk, Value: otlpValue{s.static[sk]}})
		}
		out = append(out, otlpResourceLogs{
			Resource:  map[string][]otlpKV{"attributes": attrs},
			ScopeLogs: []otlpScopeLogs{*groups[k]},
		})
	}
	return json.Marshal(map[string]any{"resourceLogs": out})
}

func (s *otlpLogSender) send(ctx context.Context, recs []LogRecord) error {
	body, err := s.payload(recs)
	if err != nil {
		return permanent(fmt.Errorf("encode otlp logs payload: %w", err))
	}
	return postBody(ctx, s.client, s.url, "application/json", s.headers, body, "otlp logs")
}
