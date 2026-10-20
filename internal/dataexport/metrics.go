package dataexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// MetricSource is the slice of the node-local telemetry store the metric
// exporters read: the newest sample per resource for one metric name.
type MetricSource interface {
	LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error)
}

// Label is one metric label. Only app, database, node and destination are
// ever emitted, which keeps cardinality bounded by what the operator runs.
type Label struct{ Name, Value string }

// Series is one exported sample.
type Series struct {
	Name    string
	Help    string
	Counter bool
	Labels  []Label
	Value   float64
	Time    time.Time
}

type metricDesc struct {
	metric  string
	help    string
	counter bool
}

var resourceMetrics = []metricDesc{
	{"cpu_percent", "CPU usage of the container, percent of one core.", false},
	{"memory_usage_bytes", "Memory used by the container.", false},
	{"memory_limit_bytes", "Memory limit of the container.", false},
	{"network_rx_bytes", "Bytes received by the container since it started.", true},
	{"network_tx_bytes", "Bytes sent by the container since it started.", true},
	{"disk_read_bytes", "Bytes read from disk by the container since it started.", true},
	{"disk_write_bytes", "Bytes written to disk by the container since it started.", true},
}

// SanitizePrefix turns a brand short name into a Prometheus metric prefix.
func SanitizePrefix(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "platform_"
	}
	return b.String() + "_"
}

// ResourceFilter decides whether a resource may appear in the output, so a
// restricted caller of the scrape endpoint only sees what it may read.
type ResourceFilter func(kind, name string) bool

// CollectResourceSeries returns the latest per-resource container metrics
// labelled by app, database or node. Resources of other kinds are skipped.
func CollectResourceSeries(ctx context.Context, src MetricSource, prefix string, allow ResourceFilter) ([]Series, error) {
	var out []Series
	for _, d := range resourceMetrics {
		samples, err := src.LatestByMetric(ctx, d.metric)
		if err != nil {
			return out, fmt.Errorf("dataexport: collect %s: %w", d.metric, err)
		}
		for _, s := range samples {
			pfx, name, ok := strings.Cut(s.ResourceID, ":")
			if !ok || name == "" {
				continue
			}
			label := ""
			switch pfx {
			case "service":
				label, pfx = "app", "app"
			case "database":
				label = "database"
			case "node":
				label = "node"
			default:
				continue
			}
			if allow != nil && !allow(pfx, name) {
				continue
			}
			out = append(out, Series{
				Name: prefix + d.metric, Help: d.help, Counter: d.counter,
				Labels: []Label{{label, name}}, Value: s.Value, Time: s.Timestamp,
			})
		}
	}
	return out, nil
}

func escapeLabel(v string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(v)
}

// WritePrometheus renders series in the Prometheus text exposition format,
// grouped by metric name in a stable order.
func WritePrometheus(w io.Writer, series []Series) error {
	byName := map[string][]Series{}
	var names []string
	for _, s := range series {
		if _, ok := byName[s.Name]; !ok {
			names = append(names, s.Name)
		}
		byName[s.Name] = append(byName[s.Name], s)
	}
	sort.Strings(names)
	for _, n := range names {
		group := byName[n]
		kind := "gauge"
		if group[0].Counter {
			kind = "counter"
		}
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", n, group[0].Help, n, kind); err != nil {
			return err
		}
		sort.SliceStable(group, func(i, j int) bool { return labelKey(group[i]) < labelKey(group[j]) })
		for _, s := range group {
			var lb strings.Builder
			for i, l := range s.Labels {
				if i > 0 {
					lb.WriteByte(',')
				}
				lb.WriteString(l.Name + `="` + escapeLabel(l.Value) + `"`)
			}
			line := n
			if lb.Len() > 0 {
				line += "{" + lb.String() + "}"
			}
			if _, err := fmt.Fprintf(w, "%s %s\n", line, strconv.FormatFloat(s.Value, 'g', -1, 64)); err != nil {
				return err
			}
		}
	}
	return nil
}

func labelKey(s Series) string {
	var b strings.Builder
	for _, l := range s.Labels {
		b.WriteString(l.Name + "=" + l.Value + ",")
	}
	return b.String()
}

type otlpAttr struct {
	Key   string            `json:"key"`
	Value map[string]string `json:"value"`
}

type otlpPoint struct {
	Attributes []otlpAttr `json:"attributes,omitempty"`
	TimeUnix   string     `json:"timeUnixNano"`
	AsDouble   float64    `json:"asDouble"`
}

type otlpMetric struct {
	Name  string     `json:"name"`
	Desc  string     `json:"description,omitempty"`
	Gauge *otlpGauge `json:"gauge,omitempty"`
	Sum   *otlpSum   `json:"sum,omitempty"`
}

type otlpGauge struct {
	DataPoints []otlpPoint `json:"dataPoints"`
}

type otlpSum struct {
	DataPoints             []otlpPoint `json:"dataPoints"`
	AggregationTemporality int         `json:"aggregationTemporality"`
	IsMonotonic            bool        `json:"isMonotonic"`
}

// EncodeOTLP renders series as an OTLP/HTTP JSON ExportMetricsServiceRequest.
// Counters become cumulative monotonic sums, everything else gauges.
func EncodeOTLP(series []Series, serviceName string, now time.Time) ([]byte, error) {
	byName := map[string]*otlpMetric{}
	var order []string
	for _, s := range series {
		m, ok := byName[s.Name]
		if !ok {
			m = &otlpMetric{Name: s.Name, Desc: s.Help}
			if s.Counter {
				m.Sum = &otlpSum{AggregationTemporality: 2, IsMonotonic: true}
			} else {
				m.Gauge = &otlpGauge{}
			}
			byName[s.Name] = m
			order = append(order, s.Name)
		}
		ts := s.Time
		if ts.IsZero() {
			ts = now
		}
		p := otlpPoint{TimeUnix: strconv.FormatInt(ts.UnixNano(), 10), AsDouble: s.Value}
		for _, l := range s.Labels {
			p.Attributes = append(p.Attributes, otlpAttr{Key: l.Name, Value: map[string]string{"stringValue": l.Value}})
		}
		if m.Sum != nil {
			m.Sum.DataPoints = append(m.Sum.DataPoints, p)
		} else {
			m.Gauge.DataPoints = append(m.Gauge.DataPoints, p)
		}
	}
	sort.Strings(order)
	metrics := make([]*otlpMetric, 0, len(order))
	for _, n := range order {
		metrics = append(metrics, byName[n])
	}
	doc := map[string]any{"resourceMetrics": []any{map[string]any{
		"resource": map[string]any{"attributes": []otlpAttr{
			{Key: "service.name", Value: map[string]string{"stringValue": serviceName}},
		}},
		"scopeMetrics": []any{map[string]any{
			"scope":   map[string]string{"name": serviceName},
			"metrics": metrics,
		}},
	}}}
	return json.Marshal(doc)
}

// OTLPURL appends the standard metrics path when the endpoint has none.
func OTLPURL(endpoint string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	idx := strings.Index(trimmed, "://")
	if idx >= 0 && !strings.Contains(trimmed[idx+3:], "/") {
		return trimmed + "/v1/metrics"
	}
	return endpoint
}
