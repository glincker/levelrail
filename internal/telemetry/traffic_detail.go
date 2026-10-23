package telemetry

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strconv"
	"time"
)

// Traffic detail env vars.
const (
	// EnvRequestUpstreamMax caps distinct upstream labels per host per tick.
	EnvRequestUpstreamMax = "APP_REQUEST_UPSTREAM_MAX"
	// EnvRequestSampleRate is the fraction (0..1) of slow requests kept in the
	// sample log; failed requests are always eligible. Default 0.1.
	EnvRequestSampleRate = "APP_REQUEST_SAMPLE_RATE"
	// EnvRequestSlowMS is the duration at which a request counts as slow.
	EnvRequestSlowMS = "APP_REQUEST_SLOW_MS"
	// EnvRequestSampleTickMax caps samples kept per app per sampling tick.
	EnvRequestSampleTickMax = "APP_REQUEST_SAMPLE_TICK_MAX"
	// EnvRequestSampleRetention is how long samples are kept (Go duration).
	EnvRequestSampleRetention = "APP_REQUEST_SAMPLE_RETENTION"
	// EnvRequestSampleRows caps stored samples per app.
	EnvRequestSampleRows = "APP_REQUEST_SAMPLE_ROWS"

	// OverflowUpstream collects traffic beyond the per-tick upstream cap.
	OverflowUpstream = "other"

	defaultUpstreamMax     = 32
	defaultSampleRate      = 0.1
	defaultSlowMS          = 1000.0
	defaultSampleTickMax   = 50
	defaultSampleRetention = 24 * time.Hour
	defaultSampleRows      = 5000
	maxUpstreamLabel       = 64
	maxSampleField         = 64
	maxSampleRoute         = 128
)

// UpstreamStat is one upstream's counters over a sampling tick.
type UpstreamStat struct {
	Requests       uint64
	Errors4xx      uint64
	Errors5xx      uint64
	UpstreamErrors uint64
	Latency        [LatencyBucketCount]uint64
}

// RequestSample is one slow or failed request. Route is a normalized
// template, never a raw path.
type RequestSample struct {
	Time       time.Time `json:"time"`
	Route      string    `json:"route"`
	Method     string    `json:"method"`
	Status     int       `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	Upstream   string    `json:"upstream,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
}

// SampleConfig decides which requests enter the sample log.
type SampleConfig struct {
	Rate    float64
	SlowMS  float64
	TickMax int
}

// SampleConfigFromEnv reads the sampling knobs, falling back to defaults on
// unset or invalid values.
func SampleConfigFromEnv() SampleConfig {
	c := SampleConfig{Rate: defaultSampleRate, SlowMS: defaultSlowMS, TickMax: defaultSampleTickMax}
	if v, err := strconv.ParseFloat(os.Getenv(EnvRequestSampleRate), 64); err == nil && v >= 0 && v <= 1 {
		c.Rate = v
	}
	if v, err := strconv.ParseFloat(os.Getenv(EnvRequestSlowMS), 64); err == nil && v > 0 {
		c.SlowMS = v
	}
	if v, err := strconv.Atoi(os.Getenv(EnvRequestSampleTickMax)); err == nil && v >= 0 {
		c.TickMax = v
	}
	return c
}

// UpstreamMaxFromEnv returns the per-tick upstream label cap.
func UpstreamMaxFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvRequestUpstreamMax)); err == nil && n > 0 {
		return n
	}
	return defaultUpstreamMax
}

// EnvTrafficRetention overrides how long per-app traffic aggregates are kept
// (Go duration); unset means the minute tier retention.
const EnvTrafficRetention = "APP_TRAFFIC_RETENTION"

// TrafficRetentionFromEnv returns the traffic aggregate retention, or def.
func TrafficRetentionFromEnv(def time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvTrafficRetention)); err == nil && d > 0 {
		return d
	}
	return def
}

// SampleRetentionFromEnv returns how long request samples are kept.
func SampleRetentionFromEnv() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(EnvRequestSampleRetention)); err == nil && d > 0 {
		return d
	}
	return defaultSampleRetention
}

func sampleRowsFromEnv() int {
	if n, err := strconv.Atoi(os.Getenv(EnvRequestSampleRows)); err == nil && n > 0 {
		return n
	}
	return defaultSampleRows
}

// ShouldSample reports whether a finished request belongs in the sample
// log: failures always, slow requests at Rate. The decision hashes the
// request id so it is stable and needs no random source.
func (c SampleConfig) ShouldSample(status int, durationMS float64, upstreamErr bool, requestID string) bool {
	if c.TickMax <= 0 {
		return false
	}
	if status >= 500 || upstreamErr {
		return true
	}
	if durationMS < c.SlowMS || c.Rate <= 0 {
		return false
	}
	if c.Rate >= 1 {
		return true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(requestID))
	return float64(h.Sum32()%10000) < c.Rate*10000
}

// LatencyBucketIndex is the histogram slot for a latency in milliseconds.
func LatencyBucketIndex(ms float64) int {
	return sort.SearchFloat64s(LatencyBucketsMS, ms)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ObserveUpstream records one request against its upstream label; labels
// beyond max distinct values per window collapse into OverflowUpstream.
func (w *RequestWindow) ObserveUpstream(upstream string, status int, upstreamErr bool, latencyMS float64, limit int) {
	if upstream == "" {
		return
	}
	if w.Upstreams == nil {
		w.Upstreams = make(map[string]UpstreamStat)
	}
	upstream = clip(upstream, maxUpstreamLabel)
	st, ok := w.Upstreams[upstream]
	if !ok && limit > 0 && len(w.Upstreams) >= limit {
		upstream = OverflowUpstream
		st = w.Upstreams[upstream]
	}
	st.Requests++
	switch {
	case status >= 500:
		st.Errors5xx++
	case status >= 400:
		st.Errors4xx++
	}
	if upstreamErr {
		st.UpstreamErrors++
	}
	st.Latency[LatencyBucketIndex(latencyMS)]++
	w.Upstreams[upstream] = st
}

// AddSample keeps s unless the window already holds tickMax samples.
func (w *RequestWindow) AddSample(s RequestSample, tickMax int) {
	if tickMax > 0 && len(w.Sampled) >= tickMax {
		return
	}
	s.Route = clip(s.Route, maxSampleRoute)
	s.Method = clip(s.Method, 16)
	s.Upstream = clip(s.Upstream, maxUpstreamLabel)
	s.RequestID = clip(s.RequestID, maxSampleField)
	w.Sampled = append(w.Sampled, s)
}

func (w *RequestWindow) mergeDetail(o RequestWindow) {
	for u, s := range o.Upstreams {
		if w.Upstreams == nil {
			w.Upstreams = make(map[string]UpstreamStat)
		}
		cur := w.Upstreams[u]
		cur.Requests += s.Requests
		cur.Errors4xx += s.Errors4xx
		cur.Errors5xx += s.Errors5xx
		cur.UpstreamErrors += s.UpstreamErrors
		for i := range cur.Latency {
			cur.Latency[i] += s.Latency[i]
		}
		w.Upstreams[u] = cur
	}
	w.Sampled = append(w.Sampled, o.Sampled...)
}

const upstreamUpsert = `
	INSERT INTO request_upstreams (resource_id, upstream, ts, requests, errors_4xx, errors_5xx, upstream_errors,
		b0, b1, b2, b3, b4, b5, b6, b7, b8, b9, b10, b11)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (resource_id, upstream, ts) DO UPDATE SET
		requests = requests + excluded.requests,
		errors_4xx = errors_4xx + excluded.errors_4xx,
		errors_5xx = errors_5xx + excluded.errors_5xx,
		upstream_errors = upstream_errors + excluded.upstream_errors,
		b0 = b0 + excluded.b0, b1 = b1 + excluded.b1, b2 = b2 + excluded.b2, b3 = b3 + excluded.b3,
		b4 = b4 + excluded.b4, b5 = b5 + excluded.b5, b6 = b6 + excluded.b6, b7 = b7 + excluded.b7,
		b8 = b8 + excluded.b8, b9 = b9 + excluded.b9, b10 = b10 + excluded.b10, b11 = b11 + excluded.b11`

func (db *DB) recordTrafficDetail(ctx context.Context, byApp map[string]RequestWindow, at time.Time) error {
	has := false
	for _, w := range byApp {
		if len(w.Upstreams) > 0 || len(w.Sampled) > 0 {
			has = true
			break
		}
	}
	if !has {
		return nil
	}
	bucket := at.Truncate(breakdownBucket).Unix()
	rowCap := sampleRowsFromEnv()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin traffic detail write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for app, w := range byApp {
		rid := "service:" + app
		for up, s := range w.Upstreams {
			args := []any{rid, up, bucket, sat(s.Requests), sat(s.Errors4xx), sat(s.Errors5xx), sat(s.UpstreamErrors)}
			for _, n := range s.Latency {
				args = append(args, sat(n))
			}
			if _, err := tx.ExecContext(ctx, upstreamUpsert, args...); err != nil {
				return fmt.Errorf("telemetry: write upstream stats: %w", err)
			}
		}
		for _, s := range w.Sampled {
			if _, err := tx.ExecContext(ctx, `INSERT INTO request_samples (resource_id, ts, route, method, status, duration_ms, upstream, request_id)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, rid, s.Time.Unix(), s.Route, s.Method, s.Status, s.DurationMS, s.Upstream, s.RequestID); err != nil {
				return fmt.Errorf("telemetry: write request sample: %w", err)
			}
		}
		if len(w.Sampled) > 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM request_samples WHERE resource_id = ? AND id NOT IN (
				SELECT id FROM request_samples WHERE resource_id = ? ORDER BY id DESC LIMIT ?)`, rid, rid, rowCap); err != nil {
				return fmt.Errorf("telemetry: trim request samples: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit traffic detail: %w", err)
	}
	return nil
}

// UpstreamRow is one upstream's totals and latency histogram over a window.
type UpstreamRow struct {
	Upstream       string
	Requests       float64
	Errors4xx      float64
	Errors5xx      float64
	UpstreamErrors float64
	Latency        []float64
}

// TrafficDetailSource reads per-upstream totals and sampled requests; *DB
// satisfies it.
type TrafficDetailSource interface {
	QueryUpstreams(ctx context.Context, resourceID string, from, to time.Time) ([]UpstreamRow, error)
	QueryRequestSamples(ctx context.Context, resourceID string, from, to time.Time, limit int) ([]RequestSample, error)
}

// QueryUpstreams sums per-upstream counters for resourceID in [from, to].
func (db *DB) QueryUpstreams(ctx context.Context, resourceID string, from, to time.Time) ([]UpstreamRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT upstream, SUM(requests), SUM(errors_4xx), SUM(errors_5xx), SUM(upstream_errors),
			SUM(b0), SUM(b1), SUM(b2), SUM(b3), SUM(b4), SUM(b5), SUM(b6), SUM(b7), SUM(b8), SUM(b9), SUM(b10), SUM(b11)
		FROM request_upstreams
		WHERE resource_id = ? AND ts BETWEEN ? AND ?
		GROUP BY upstream`,
		resourceID, from.Truncate(breakdownBucket).Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("telemetry: query upstreams for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []UpstreamRow
	for rows.Next() {
		var r UpstreamRow
		var req, e4, e5, ue int64
		var b [LatencyBucketCount]int64
		dest := []any{&r.Upstream, &req, &e4, &e5, &ue}
		for i := range b {
			dest = append(dest, &b[i])
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("telemetry: scan upstream row: %w", err)
		}
		r.Requests, r.Errors4xx, r.Errors5xx, r.UpstreamErrors = float64(req), float64(e4), float64(e5), float64(ue)
		r.Latency = make([]float64, LatencyBucketCount)
		for i, n := range b {
			r.Latency[i] = float64(n)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate upstreams: %w", err)
	}
	return out, nil
}

// QueryRequestSamples returns up to limit samples in [from, to], newest first.
func (db *DB) QueryRequestSamples(ctx context.Context, resourceID string, from, to time.Time, limit int) ([]RequestSample, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT ts, route, method, status, duration_ms, upstream, request_id
		FROM request_samples
		WHERE resource_id = ? AND ts BETWEEN ? AND ?
		ORDER BY ts DESC, id DESC LIMIT ?`,
		resourceID, from.Unix(), to.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("telemetry: query request samples for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []RequestSample
	for rows.Next() {
		var s RequestSample
		var ts int64
		if err := rows.Scan(&ts, &s.Route, &s.Method, &s.Status, &s.DurationMS, &s.Upstream, &s.RequestID); err != nil {
			return nil, fmt.Errorf("telemetry: scan request sample: %w", err)
		}
		s.Time = time.Unix(ts, 0).UTC()
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate request samples: %w", err)
	}
	return out, nil
}

// QueryUpstreams merges per-upstream totals from every source that supports
// them, tolerating partial failure like QueryBreakdown.
func (f *Federator) QueryUpstreams(ctx context.Context, resourceID string, from, to time.Time) ([]UpstreamRow, error) {
	merged := map[string]*UpstreamRow{}
	var firstErr error
	for _, src := range f.metrics {
		ts, ok := src.(TrafficDetailSource)
		if !ok {
			continue
		}
		rows, err := ts.QueryUpstreams(ctx, resourceID, from, to)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, r := range rows {
			cur := merged[r.Upstream]
			if cur == nil {
				cur = &UpstreamRow{Upstream: r.Upstream, Latency: make([]float64, LatencyBucketCount)}
				merged[r.Upstream] = cur
			}
			cur.Requests += r.Requests
			cur.Errors4xx += r.Errors4xx
			cur.Errors5xx += r.Errors5xx
			cur.UpstreamErrors += r.UpstreamErrors
			for i := 0; i < len(r.Latency) && i < len(cur.Latency); i++ {
				cur.Latency[i] += r.Latency[i]
			}
		}
	}
	out := make([]UpstreamRow, 0, len(merged))
	for _, r := range merged {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Upstream < out[j].Upstream })
	return out, firstErr
}

// QueryRequestSamples merges samples from every supporting source, newest
// first, trimmed to limit.
func (f *Federator) QueryRequestSamples(ctx context.Context, resourceID string, from, to time.Time, limit int) ([]RequestSample, error) {
	var all []RequestSample
	var firstErr error
	for _, src := range f.metrics {
		ts, ok := src.(TrafficDetailSource)
		if !ok {
			continue
		}
		rows, err := ts.QueryRequestSamples(ctx, resourceID, from, to, limit)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		all = append(all, rows...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time.After(all[j].Time) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, firstErr
}
