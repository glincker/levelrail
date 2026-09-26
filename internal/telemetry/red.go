package telemetry

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Request (RED) metric names. All are per-tick counter deltas stored under
// the app's "service:<name>" resource id; only non-zero values are written,
// so an app with no traffic writes nothing. Every name with the "http_"
// prefix is a sum-kind metric: rollups add it up instead of averaging.
const (
	MetricHTTPRequests       = "http_requests"
	MetricHTTPResponses2xx   = "http_responses_2xx"
	MetricHTTPResponses3xx   = "http_responses_3xx"
	MetricHTTPResponses4xx   = "http_responses_4xx"
	MetricHTTPResponses5xx   = "http_responses_5xx"
	MetricHTTPUpstreamErrors = "http_upstream_errors"
	MetricHTTPBytesIn        = "http_bytes_in"
	MetricHTTPBytesOut       = "http_bytes_out"

	metricHTTPPrefix        = "http_"
	metricHTTPLatencyBucket = "http_latency_bucket_"
)

// LatencyBucketsMS are the fixed upper bounds (milliseconds) of the request
// latency histogram. A final implicit bucket holds everything above the last.
var LatencyBucketsMS = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// LatencyBucketCount is the number of histogram slots including overflow.
const LatencyBucketCount = 12

// IsSumMetric reports whether rollups must add the metric's values.
func IsSumMetric(metric string) bool {
	return len(metric) >= len(metricHTTPPrefix) && metric[:len(metricHTTPPrefix)] == metricHTTPPrefix
}

// LatencyBucketMetric is the metric name for histogram slot i.
func LatencyBucketMetric(i int) string {
	if i >= len(LatencyBucketsMS) {
		return metricHTTPLatencyBucket + "inf"
	}
	return metricHTTPLatencyBucket + strconv.FormatFloat(LatencyBucketsMS[i], 'f', -1, 64)
}

// RequestWindow is one host key's request activity over one sampling tick.
type RequestWindow struct {
	Requests       uint64
	Status2xx      uint64
	Status3xx      uint64
	Status4xx      uint64
	Status5xx      uint64
	UpstreamErrors uint64
	BytesIn        uint64
	BytesOut       uint64
	Latency        [LatencyBucketCount]uint64
}

// Samples renders the window as non-zero metric samples for resourceID.
func (w RequestWindow) Samples(resourceID string, at time.Time) []Sample {
	var out []Sample
	add := func(metric string, v uint64) {
		if v > 0 {
			out = append(out, Sample{ResourceID: resourceID, Metric: metric, Timestamp: at, Value: float64(v)})
		}
	}
	add(MetricHTTPRequests, w.Requests)
	add(MetricHTTPResponses2xx, w.Status2xx)
	add(MetricHTTPResponses3xx, w.Status3xx)
	add(MetricHTTPResponses4xx, w.Status4xx)
	add(MetricHTTPResponses5xx, w.Status5xx)
	add(MetricHTTPUpstreamErrors, w.UpstreamErrors)
	add(MetricHTTPBytesIn, w.BytesIn)
	add(MetricHTTPBytesOut, w.BytesOut)
	for i, n := range w.Latency {
		add(LatencyBucketMetric(i), n)
	}
	return out
}

// Add merges o into w.
func (w *RequestWindow) Add(o RequestWindow) {
	w.Requests += o.Requests
	w.Status2xx += o.Status2xx
	w.Status3xx += o.Status3xx
	w.Status4xx += o.Status4xx
	w.Status5xx += o.Status5xx
	w.UpstreamErrors += o.UpstreamErrors
	w.BytesIn += o.BytesIn
	w.BytesOut += o.BytesOut
	for i := range w.Latency {
		w.Latency[i] += o.Latency[i]
	}
}

// RecordRequests writes the per-app windows as samples at time at.
func (db *DB) RecordRequests(ctx context.Context, byApp map[string]RequestWindow, at time.Time) error {
	var rows []Sample
	for app, w := range byApp {
		rows = append(rows, w.Samples("service:"+app, at)...)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := db.WriteSamples(ctx, rows); err != nil {
		return fmt.Errorf("telemetry: record requests: %w", err)
	}
	return nil
}

// PercentileFromBuckets estimates the q-quantile (0..1) in milliseconds from
// histogram counts, interpolating linearly inside the containing bucket. The
// overflow bucket reports the last finite bound. Empty input returns 0.
func PercentileFromBuckets(counts []float64, q float64) float64 {
	var total float64
	for _, c := range counts {
		total += c
	}
	if total <= 0 {
		return 0
	}
	rank := q * total
	var cum float64
	for i, c := range counts {
		if c <= 0 {
			continue
		}
		if cum+c < rank {
			cum += c
			continue
		}
		if i >= len(LatencyBucketsMS) {
			return LatencyBucketsMS[len(LatencyBucketsMS)-1]
		}
		lower := 0.0
		if i > 0 {
			lower = LatencyBucketsMS[i-1]
		}
		return lower + (LatencyBucketsMS[i]-lower)*((rank-cum)/c)
	}
	return LatencyBucketsMS[len(LatencyBucketsMS)-1]
}

// RequestPoint is one step of an app's request series. Rates are per second
// over the step; error rates are fractions of that step's requests.
type RequestPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	Requests       float64   `json:"requests"`
	RatePerSec     float64   `json:"rate_per_sec"`
	ErrorRate4xx   float64   `json:"error_rate_4xx"`
	ErrorRate5xx   float64   `json:"error_rate_5xx"`
	UpstreamErrors float64   `json:"upstream_errors"`
	P50Ms          float64   `json:"p50_ms"`
	P95Ms          float64   `json:"p95_ms"`
	P99Ms          float64   `json:"p99_ms"`
	BytesInPerSec  float64   `json:"bytes_in_per_sec"`
	BytesOutPerSec float64   `json:"bytes_out_per_sec"`
}

// MetricsQuerier is the read surface the request queries need; *Federator
// satisfies it, so request metrics federate exactly like container metrics.
type MetricsQuerier interface {
	QueryMetrics(ctx context.Context, resourceID, metric string, from, to time.Time) ([]Sample, error)
}

// MinRequestStep is the finest step a request series is bucketed at.
const MinRequestStep = 15 * time.Second

func requestMetricNames() []string {
	names := []string{
		MetricHTTPRequests, MetricHTTPResponses4xx, MetricHTTPResponses5xx,
		MetricHTTPUpstreamErrors, MetricHTTPBytesIn, MetricHTTPBytesOut,
	}
	for i := 0; i < LatencyBucketCount; i++ {
		names = append(names, LatencyBucketMetric(i))
	}
	return names
}
