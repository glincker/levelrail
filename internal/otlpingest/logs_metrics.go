package otlpingest

import (
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	logscol "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricscol "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// ErrTooMany is returned when a request carries more records than the limit.
var ErrTooMany = errors.New("otlp: too many records in one request")

// otlpSeverityError is the first OTLP severity number of the ERROR range.
const otlpSeverityError = 17

const maxMetricName = 120

func unmarshal(contentType string, body []byte, m proto.Message) error {
	mt, _, _ := mime.ParseMediaType(contentType)
	switch mt {
	case "application/x-protobuf", "application/protobuf":
		if err := proto.Unmarshal(body, m); err != nil {
			return fmt.Errorf("otlp: decode protobuf: %w", err)
		}
	case "application/json":
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, m); err != nil {
			return fmt.Errorf("otlp: decode json: %w", err)
		}
	default:
		return ErrUnsupportedType
	}
	return nil
}

// DecodeLogs maps an OTLP logs export onto log entries for resourceID. Only
// the body text is kept: attributes may hold secrets and have no column.
func DecodeLogs(contentType string, body []byte, resourceID string, maxRecords, maxLine int) ([]telemetry.LogEntry, error) {
	var req logscol.ExportLogsServiceRequest
	if err := unmarshal(contentType, body, &req); err != nil {
		return nil, err
	}
	var out []telemetry.LogEntry
	for _, rl := range req.GetResourceLogs() {
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				if len(out) >= maxRecords {
					return nil, ErrTooMany
				}
				msg := strings.TrimSpace(anyText(lr.GetBody()))
				if msg == "" {
					continue
				}
				ns := lr.GetTimeUnixNano()
				if ns == 0 {
					ns = lr.GetObservedTimeUnixNano()
				}
				ts := time.Now()
				if ns != 0 {
					ts = time.Unix(0, unixNano(ns))
				}
				stream := "stdout"
				if int(lr.GetSeverityNumber()) >= otlpSeverityError {
					stream = "stderr"
				}
				out = append(out, telemetry.LogEntry{
					ResourceID: resourceID, ContainerID: "otlp", Stream: stream,
					Timestamp: ts, Message: truncate(msg, maxLine),
				})
			}
		}
	}
	return out, nil
}

func anyText(v *commonpb.AnyValue) string {
	if s := v.GetStringValue(); s != "" {
		return s
	}
	if s, ok := anyValue(v).(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(anyValue(v))
}

// DecodeMetrics maps gauge, sum and histogram points onto samples named
// "otlp.<metric>". Attributes are dropped so cardinality stays bounded; two
// series that differ only by attributes share one name and the later point wins.
func DecodeMetrics(contentType string, body []byte, resourceID string, maxPoints int) ([]telemetry.Sample, error) {
	var req metricscol.ExportMetricsServiceRequest
	if err := unmarshal(contentType, body, &req); err != nil {
		return nil, err
	}
	var out []telemetry.Sample
	add := func(name string, ns uint64, v float64) error {
		if len(out) >= maxPoints {
			return ErrTooMany
		}
		ts := time.Now()
		if ns != 0 {
			ts = time.Unix(0, unixNano(ns))
		}
		out = append(out, telemetry.Sample{ResourceID: resourceID, Metric: truncate("otlp."+name, maxMetricName), Timestamp: ts, Value: v})
		return nil
	}
	for _, rm := range req.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				if m.GetName() == "" {
					continue
				}
				var pts []*metricspb.NumberDataPoint
				switch d := m.GetData().(type) {
				case *metricspb.Metric_Gauge:
					pts = d.Gauge.GetDataPoints()
				case *metricspb.Metric_Sum:
					pts = d.Sum.GetDataPoints()
				case *metricspb.Metric_Histogram:
					for _, p := range d.Histogram.GetDataPoints() {
						if err := add(m.GetName()+".count", p.GetTimeUnixNano(), float64(p.GetCount())); err != nil {
							return nil, err
						}
						if p.Sum != nil {
							if err := add(m.GetName()+".sum", p.GetTimeUnixNano(), p.GetSum()); err != nil {
								return nil, err
							}
						}
					}
				}
				for _, p := range pts {
					v := p.GetAsDouble()
					if _, isInt := p.GetValue().(*metricspb.NumberDataPoint_AsInt); isInt {
						v = float64(p.GetAsInt())
					}
					if err := add(m.GetName(), p.GetTimeUnixNano(), v); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return out, nil
}
