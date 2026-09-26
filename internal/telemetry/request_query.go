package telemetry

import (
	"context"
	"fmt"
	"sort"
	"time"
)

func fetchRequestSums(ctx context.Context, q MetricsQuerier, app string, from, to time.Time, step time.Duration) (map[string]map[int64]float64, error) {
	resourceID := "service:" + app
	sums := make(map[string]map[int64]float64)
	var firstErr error
	for _, metric := range requestMetricNames() {
		samples, err := q.QueryMetrics(ctx, resourceID, metric, from, to)
		if err != nil && len(samples) == 0 {
			return nil, fmt.Errorf("telemetry: query requests %s/%s: %w", app, metric, err)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
		m := make(map[int64]float64)
		for _, s := range samples {
			m[int64(s.Timestamp.Sub(from)/step)] += s.Value
		}
		sums[metric] = m
	}
	return sums, firstErr
}

func requestPointAt(sums map[string]map[int64]float64, idx int64, ts time.Time, step time.Duration) (RequestPoint, bool) {
	total := sums[MetricHTTPRequests][idx]
	if total <= 0 {
		return RequestPoint{}, false
	}
	buckets := make([]float64, LatencyBucketCount)
	for i := range buckets {
		buckets[i] = sums[LatencyBucketMetric(i)][idx]
	}
	secs := step.Seconds()
	return RequestPoint{
		Timestamp:      ts,
		Requests:       total,
		RatePerSec:     total / secs,
		ErrorRate4xx:   sums[MetricHTTPResponses4xx][idx] / total,
		ErrorRate5xx:   sums[MetricHTTPResponses5xx][idx] / total,
		UpstreamErrors: sums[MetricHTTPUpstreamErrors][idx],
		P50Ms:          PercentileFromBuckets(buckets, 0.50),
		P95Ms:          PercentileFromBuckets(buckets, 0.95),
		P99Ms:          PercentileFromBuckets(buckets, 0.99),
		BytesInPerSec:  sums[MetricHTTPBytesIn][idx] / secs,
		BytesOutPerSec: sums[MetricHTTPBytesOut][idx] / secs,
	}, true
}

// QueryRequests returns the app's request series over [from, to] bucketed by
// step (at least MinRequestStep). Buckets with no traffic are omitted.
// Counters are summed across sources, so a multi-node app merges correctly.
func QueryRequests(ctx context.Context, q MetricsQuerier, app string, from, to time.Time, step time.Duration) ([]RequestPoint, error) {
	if step < MinRequestStep {
		step = MinRequestStep
	}
	sums, err := fetchRequestSums(ctx, q, app, from, to, step)
	if sums == nil {
		return nil, err
	}
	idxs := make([]int64, 0, len(sums[MetricHTTPRequests]))
	for idx := range sums[MetricHTTPRequests] {
		idxs = append(idxs, idx)
	}
	sort.Slice(idxs, func(i, j int) bool { return idxs[i] < idxs[j] })

	points := make([]RequestPoint, 0, len(idxs))
	for _, idx := range idxs {
		if p, ok := requestPointAt(sums, idx, from.Add(time.Duration(idx)*step), step); ok {
			points = append(points, p)
		}
	}
	return points, err
}

// RequestSummary is an app's request health over one window.
type RequestSummary struct {
	WindowSeconds  float64 `json:"window_seconds"`
	HasTraffic     bool    `json:"has_traffic"`
	Requests       float64 `json:"requests"`
	RatePerSec     float64 `json:"rate_per_sec"`
	ErrorRate4xx   float64 `json:"error_rate_4xx"`
	ErrorRate5xx   float64 `json:"error_rate_5xx"`
	P95Ms          float64 `json:"p95_ms"`
	UpstreamErrors float64 `json:"upstream_errors"`
}

// SummarizeRequests reduces the app's last window of traffic to one summary,
// the entry point auto-rollback and SLO features build on. A window with no
// requests yields HasTraffic false and zero values.
func SummarizeRequests(ctx context.Context, q MetricsQuerier, app string, window time.Duration, now time.Time) (RequestSummary, error) {
	sum := RequestSummary{WindowSeconds: window.Seconds()}
	if window <= 0 {
		return sum, fmt.Errorf("telemetry: summarize requests %s: window must be positive", app)
	}
	from := now.Add(-window)
	sums, err := fetchRequestSums(ctx, q, app, from, now, window+time.Second)
	if sums == nil {
		return sum, err
	}
	p, ok := requestPointAt(sums, 0, from, window)
	if !ok {
		return sum, err
	}
	sum.HasTraffic = true
	sum.Requests = p.Requests
	sum.RatePerSec = p.RatePerSec
	sum.ErrorRate4xx = p.ErrorRate4xx
	sum.ErrorRate5xx = p.ErrorRate5xx
	sum.P95Ms = p.P95Ms
	sum.UpstreamErrors = p.UpstreamErrors
	return sum, err
}
