package otlpingest

import (
	"errors"
	"testing"

	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func sampleRequest() *collector.ExportTraceServiceRequest {
	tid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	return &collector.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
			{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "checkout"}}},
		}},
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{
			{TraceId: tid, SpanId: []byte{1, 1, 1, 1, 1, 1, 1, 1}, Name: "GET /pay", Kind: tracepb.Span_SPAN_KIND_SERVER,
				StartTimeUnixNano: 1_000_000_000, EndTimeUnixNano: 1_500_000_000,
				Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "boom"},
				Attributes: []*commonpb.KeyValue{
					{Key: "http.status_code", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: 500}}},
				}},
			{TraceId: tid, SpanId: []byte{2, 2, 2, 2, 2, 2, 2, 2}, ParentSpanId: []byte{1, 1, 1, 1, 1, 1, 1, 1}, Name: "db.query",
				StartTimeUnixNano: 1_100_000_000, EndTimeUnixNano: 1_400_000_000},
			{TraceId: []byte{1}, SpanId: []byte{9}, Name: "malformed ids are skipped"},
		}}},
	}}}
}

func TestDecodeTracesProtobufAndJSONAgree(t *testing.T) {
	req := sampleRequest()
	pb, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	js, err := protojson.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	fromPB, err := DecodeTraces("application/x-protobuf", pb, "service:web")
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := DecodeTraces("application/json; charset=utf-8", js, "service:web")
	if err != nil {
		t.Fatal(err)
	}
	if len(fromPB) != 2 || len(fromJSON) != 2 {
		t.Fatalf("spans = %d protobuf, %d json, want 2 each (the malformed one is skipped)", len(fromPB), len(fromJSON))
	}
	root, child := fromPB[0], fromPB[1]
	if root.ResourceID != "service:web" || root.Name != "GET /pay" || root.StatusCode != 2 || root.ParentID != "" {
		t.Errorf("root = %+v", root)
	}
	if child.ParentID != "0101010101010101" || child.TraceID != "0102030405060708090a0b0c0d0e0f10" {
		t.Errorf("child = %+v", child)
	}
	if root.Attrs != `{"http.status_code":500,"service.name":"checkout"}` {
		t.Errorf("attrs = %s", root.Attrs)
	}
	if fromJSON[0].SpanID != root.SpanID || fromJSON[0].Attrs != root.Attrs {
		t.Errorf("json and protobuf decode differ: %+v vs %+v", fromJSON[0], root)
	}
}

// The OTLP/JSON spec sends trace and span ids as hex strings, not the base64
// protojson emits, so a real exporter's body must decode too.
func TestDecodeTracesAcceptsHexIDsInJSON(t *testing.T) {
	body := `{"resourceSpans":[{"scopeSpans":[{"spans":[{
		"traceId":"0102030405060708090A0B0C0D0E0F10","spanId":"0101010101010101","parentSpanId":"0202020202020202",
		"name":"GET /","startTimeUnixNano":"1000000000","endTimeUnixNano":"1500000000","kind":2,
		"status":{"code":2,"message":"boom"}}]}]}]}`
	spans, err := DecodeTraces("application/json", []byte(body), "service:web")
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 || spans[0].TraceID != "0102030405060708090a0b0c0d0e0f10" || spans[0].ParentID != "0202020202020202" {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].StatusCode != 2 || spans[0].Kind != 2 || spans[0].EndNs-spans[0].StartNs != 500_000_000 {
		t.Errorf("span = %+v", spans[0])
	}
}

func TestDecodeTracesTellsTheSenderWhenNoSpanIsUsable(t *testing.T) {
	body := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"0102","spanId":"0101010101010101","name":"x"}]}]}]}`
	if _, err := DecodeTraces("application/json", []byte(body), "service:web"); !errors.Is(err, ErrNoValidSpans) {
		t.Errorf("err = %v, want ErrNoValidSpans", err)
	}
}

func TestDecodeTracesRejectsUnknownTypeAndOversize(t *testing.T) {
	if _, err := DecodeTraces("text/plain", []byte("x"), "service:web"); !errors.Is(err, ErrUnsupportedType) {
		t.Errorf("text/plain error = %v", err)
	}
	big := &collector.ExportTraceServiceRequest{}
	spans := make([]*tracepb.Span, MaxSpansPerRequest+1)
	for i := range spans {
		spans[i] = &tracepb.Span{TraceId: make([]byte, 16), SpanId: []byte{1, 2, 3, 4, 5, 6, 7, byte(i % 250)}}
	}
	big.ResourceSpans = []*tracepb.ResourceSpans{{ScopeSpans: []*tracepb.ScopeSpans{{Spans: spans}}}}
	pb, _ := proto.Marshal(big)
	if _, err := DecodeTraces("application/x-protobuf", pb, "service:web"); err == nil {
		t.Error("an oversized export was accepted")
	}
}
