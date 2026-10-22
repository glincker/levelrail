package trafficexport

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

const (
	otlpLogsPath    = "/v1/logs"
	otlpServiceName = "levelrail-edge"
	otlpScopeName   = "levelrail.trafficexport"
)

// OTLPConfig configures the OTLP/HTTP logs sink. Endpoint is a base URL that
// gets /v1/logs appended; URL, when set, is used as is (for example
// https://us.i.posthog.com/i/v1/logs).
type OTLPConfig struct {
	Endpoint string
	URL      string
	Headers  map[string]string
	HTTP     HTTPOptions
}

// OTLPSink posts one LogRecord per event as OTLP/HTTP protobuf.
type OTLPSink struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewOTLPSink validates the target and returns the sink.
func NewOTLPSink(cfg OTLPConfig) (*OTLPSink, error) {
	o := cfg.HTTP.withDefaults()
	target := strings.TrimSpace(cfg.URL)
	if target == "" {
		target = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/") + otlpLogsPath
	}
	if err := checkTarget(target, o); err != nil {
		return nil, fmt.Errorf("otlp sink: %w", err)
	}
	return &OTLPSink{url: target, headers: cfg.Headers, client: newClient(o)}, nil
}

// Name identifies the sink in stats and logs.
func (s *OTLPSink) Name() string { return "otlp" }

func strAttr(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

func intAttr(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}

func floatAttr(k string, v float64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}}
}

func boolAttr(k string, v bool) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}}
}

func toLogRecord(e Event) *logspb.LogRecord {
	ts := uint64(e.Time.UnixNano()) //nolint:gosec // event times are after 1970
	sev, sevText := logspb.SeverityNumber_SEVERITY_NUMBER_INFO, "INFO"
	if e.Status >= 500 {
		sev, sevText = logspb.SeverityNumber_SEVERITY_NUMBER_ERROR, "ERROR"
	}
	attrs := []*commonpb.KeyValue{
		strAttr("http.request.method", e.Method),
		strAttr("url.path", e.Path),
		intAttr("http.response.status_code", int64(e.Status)),
		strAttr("server.address", e.Host),
		floatAttr("http.server.request.duration", e.DurationMS/1000),
		intAttr("http.request.body.size", e.BytesIn),
		intAttr("http.response.body.size", e.BytesOut),
		strAttr("levelrail.app", e.App),
		strAttr("levelrail.request_id", e.RequestID),
		boolAttr("levelrail.upstream_error", e.UpstreamError),
		boolAttr("levelrail.cold_start", e.ColdStart),
	}
	for k, v := range map[string]string{
		"levelrail.visitor_id": e.VisitorID, "http.request.referrer_host": e.ReferrerHost, "user_agent.family": e.UAFamily,
	} {
		if v != "" {
			attrs = append(attrs, strAttr(k, v))
		}
	}
	rec := &logspb.LogRecord{
		TimeUnixNano: ts, ObservedTimeUnixNano: ts,
		SeverityNumber: sev, SeverityText: sevText,
		Body:       &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: fmt.Sprintf("%s %s %d", e.Method, e.Path, e.Status)}},
		Attributes: attrs,
	}
	// No span id: the edge has no span of its own, only the trace it joins.
	if tid, err := hex.DecodeString(e.TraceID); err == nil && len(tid) == 16 {
		rec.TraceId = tid
	}
	return rec
}

// Send posts one ExportLogsServiceRequest.
func (s *OTLPSink) Send(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	recs := make([]*logspb.LogRecord, 0, len(events))
	for _, e := range events {
		recs = append(recs, toLogRecord(e))
	}
	req := &collogs.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{strAttr("service.name", otlpServiceName)}},
		ScopeLogs: []*logspb.ScopeLogs{{
			Scope:      &commonpb.InstrumentationScope{Name: otlpScopeName},
			LogRecords: recs,
		}},
	}}}
	body, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("otlp sink: encode request: %w", err)
	}
	return postBody(ctx, s.client, s.Name(), s.url, "application/x-protobuf", s.headers, body)
}
