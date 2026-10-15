// Package otlpingest decodes OTLP/HTTP trace exports into stored spans.
package otlpingest

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"mime"
	"strings"

	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// MaxSpansPerRequest bounds one export, so a single request cannot be an
// unbounded write.
const MaxSpansPerRequest = 5000

// maxAttrValue caps one attribute value so a span cannot carry a megabyte.
const maxAttrValue = 1024

// ErrUnsupportedType is returned for a content type that is neither OTLP
// protobuf nor OTLP JSON.
var ErrUnsupportedType = errors.New("otlp: content type must be application/x-protobuf or application/json")

// ErrNoValidSpans is returned when an export held spans but none had valid
// ids, so the sender learns its ids are malformed instead of seeing success.
var ErrNoValidSpans = errors.New("otlp: no span had a 16 byte trace id and an 8 byte span id")

// hexIDs rewrites the hex trace and span ids OTLP/JSON uses into the base64
// protojson expects for bytes fields, in place.
func hexIDs(doc map[string]any) {
	fix := func(m map[string]any, key string, n int) {
		v, ok := m[key].(string)
		if !ok || len(v) != n*2 {
			return
		}
		if raw, err := hex.DecodeString(v); err == nil {
			m[key] = base64.StdEncoding.EncodeToString(raw)
		}
	}
	each := func(v any, f func(map[string]any)) {
		list, _ := v.([]any)
		for _, e := range list {
			if m, ok := e.(map[string]any); ok {
				f(m)
			}
		}
	}
	each(doc["resourceSpans"], func(rs map[string]any) {
		for _, key := range []string{"scopeSpans", "instrumentationLibrarySpans"} {
			each(rs[key], func(ss map[string]any) {
				each(ss["spans"], func(sp map[string]any) {
					fix(sp, "traceId", 16)
					fix(sp, "spanId", 8)
					fix(sp, "parentSpanId", 8)
				})
			})
		}
	})
}

// DecodeTraces parses one export request and returns its spans stamped with
// resourceID. The sender's own service.name is kept as an attribute but never
// decides which app the spans belong to.
func DecodeTraces(contentType string, body []byte, resourceID string) ([]telemetry.Span, error) {
	var req collector.ExportTraceServiceRequest
	mt, _, _ := mime.ParseMediaType(contentType)
	switch mt {
	case "application/x-protobuf", "application/protobuf":
		if err := proto.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("otlp: decode protobuf: %w", err)
		}
	case "application/json":
		var doc map[string]any
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("otlp: decode json: %w", err)
		}
		hexIDs(doc)
		fixed, err := json.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("otlp: decode json: %w", err)
		}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(fixed, &req); err != nil {
			return nil, fmt.Errorf("otlp: decode json: %w", err)
		}
	default:
		return nil, ErrUnsupportedType
	}

	var out []telemetry.Span
	seen := 0
	for _, rs := range req.GetResourceSpans() {
		service := attrString(rs.GetResource().GetAttributes(), "service.name")
		for _, ss := range rs.GetScopeSpans() {
			for _, sp := range ss.GetSpans() {
				if len(out) >= MaxSpansPerRequest {
					return nil, fmt.Errorf("otlp: more than %d spans in one request", MaxSpansPerRequest)
				}
				seen++
				if len(sp.GetTraceId()) != 16 || len(sp.GetSpanId()) != 8 {
					continue
				}
				attrs := map[string]any{}
				if service != "" {
					attrs["service.name"] = service
				}
				for _, kv := range sp.GetAttributes() {
					attrs[kv.GetKey()] = anyValue(kv.GetValue())
				}
				ab, _ := json.Marshal(attrs)
				parent := ""
				if len(sp.GetParentSpanId()) == 8 {
					parent = hex.EncodeToString(sp.GetParentSpanId())
				}
				out = append(out, telemetry.Span{
					TraceID: hex.EncodeToString(sp.GetTraceId()), SpanID: hex.EncodeToString(sp.GetSpanId()),
					ParentID: parent, ResourceID: resourceID, Name: truncate(sp.GetName(), 256),
					Kind: int(sp.GetKind()), StartNs: unixNano(sp.GetStartTimeUnixNano()), EndNs: unixNano(sp.GetEndTimeUnixNano()),
					StatusCode: int(sp.GetStatus().GetCode()), StatusMessage: truncate(sp.GetStatus().GetMessage(), 256),
					Attrs: string(ab),
				})
			}
		}
	}
	if seen > 0 && len(out) == 0 {
		return nil, ErrNoValidSpans
	}
	return out, nil
}

// unixNano converts an OTLP timestamp, clamping anything past the int64 range.
func unixNano(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v) //nolint:gosec // bounded just above
}

func attrString(kvs []*commonpb.KeyValue, key string) string {
	for _, kv := range kvs {
		if kv.GetKey() == key {
			return truncate(kv.GetValue().GetStringValue(), 128)
		}
	}
	return ""
}

func anyValue(v *commonpb.AnyValue) any {
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return truncate(x.StringValue, maxAttrValue)
	case *commonpb.AnyValue_IntValue:
		return x.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return x.DoubleValue
	case *commonpb.AnyValue_BoolValue:
		return x.BoolValue
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
