package telemetry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ManyMetricsSource is the optional batch read a MetricsSource can add:
// every (resource, metric) series in one round trip instead of one query
// per series.
type ManyMetricsSource interface {
	QueryMany(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]Sample, error)
}

// ManyMetricsQuerier is the federated counterpart of ManyMetricsSource.
type ManyMetricsQuerier interface {
	QueryMetricsMany(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]Sample, error)
}

// QueryMany returns every sample for the resource and metric cross
// product in [from, to] as one statement. Ranges the raw tier does not
// serve fall back to per series Query so tier selection stays in one place.
func (db *DB) QueryMany(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]Sample, error) {
	if len(resourceIDs) == 0 || len(metrics) == 0 {
		return nil, nil
	}
	if db.pickTier(from, to) != 0 {
		return db.queryManyTiered(ctx, resourceIDs, metrics, from, to)
	}
	args := make([]any, 0, len(resourceIDs)+len(metrics)+2)
	for _, id := range resourceIDs {
		args = append(args, id)
	}
	for _, m := range metrics {
		args = append(args, m)
	}
	args = append(args, from.Unix(), to.Unix())
	q := `SELECT resource_id, metric, ts, value FROM metric_samples
		WHERE resource_id IN (` + placeholders(len(resourceIDs)) + `)
		  AND metric IN (` + placeholders(len(metrics)) + `)
		  AND ts BETWEEN ? AND ?
		ORDER BY resource_id, metric, ts`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("telemetry: query many: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Sample
	for rows.Next() {
		var s Sample
		var ts int64
		if err := rows.Scan(&s.ResourceID, &s.Metric, &ts, &s.Value); err != nil {
			return nil, fmt.Errorf("telemetry: scan query many row: %w", err)
		}
		s.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate query many rows: %w", err)
	}
	return out, nil
}

func (db *DB) queryManyTiered(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]Sample, error) {
	var out []Sample
	for _, id := range resourceIDs {
		for _, m := range metrics {
			ss, err := db.Query(ctx, id, m, from, to)
			if err != nil {
				return nil, err
			}
			for i := range ss {
				ss[i].ResourceID, ss[i].Metric = id, m
			}
			out = append(out, ss...)
		}
	}
	return out, nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// QueryMetricsMany fans a batch read out to every source. A source
// without the batch method is read series by series. Errors from one
// source do not drop the others' samples, matching QueryMetrics.
func (f *Federator) QueryMetricsMany(ctx context.Context, resourceIDs, metrics []string, from, to time.Time) ([]Sample, error) {
	n := len(f.metrics)
	results := make([][]Sample, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i, src := range f.metrics {
		go func(i int, src MetricsSource) {
			defer wg.Done()
			if many, ok := src.(ManyMetricsSource); ok {
				results[i], errs[i] = many.QueryMany(ctx, resourceIDs, metrics, from, to)
				return
			}
			for _, id := range resourceIDs {
				for _, m := range metrics {
					ss, err := src.Query(ctx, id, m, from, to)
					if err != nil {
						errs[i] = err
						return
					}
					for j := range ss {
						ss[j].ResourceID, ss[j].Metric = id, m
					}
					results[i] = append(results[i], ss...)
				}
			}
		}(i, src)
	}
	wg.Wait()

	var all []Sample
	var finalErrs []error
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			finalErrs = append(finalErrs, errs[i])
		}
		all = append(all, results[i]...)
	}
	return all, errors.Join(finalErrs...)
}

// RequestOverview is one app's request summary over a window plus its
// per step rate series, both derived from one shared sample read.
type RequestOverview struct {
	Summary RequestSummary
	Points  []RequestPoint
}

// RequestOverviews computes SummarizeRequests and QueryRequests for every
// app over [now-window, now] from one batched sample read when q supports
// it, falling back to the per app functions otherwise.
func RequestOverviews(ctx context.Context, q MetricsQuerier, apps []string, window, step time.Duration, now time.Time) (map[string]RequestOverview, error) {
	out := make(map[string]RequestOverview, len(apps))
	from := now.Add(-window)
	if step < MinRequestStep {
		step = MinRequestStep
	}
	many, ok := q.(ManyMetricsQuerier)
	if !ok {
		var firstErr error
		for _, app := range apps {
			sum, err := SummarizeRequests(ctx, q, app, window, now)
			if err != nil && !sum.HasTraffic && firstErr == nil {
				firstErr = err
			}
			pts, _ := QueryRequests(ctx, q, app, from, now, step)
			out[app] = RequestOverview{Summary: sum, Points: pts}
		}
		return out, firstErr
	}

	ids := make([]string, len(apps))
	for i, a := range apps {
		ids[i] = "service:" + a
	}
	samples, err := many.QueryMetricsMany(ctx, ids, requestMetricNames(), from, now)
	if len(samples) == 0 && err != nil {
		return out, fmt.Errorf("telemetry: request overviews: %w", err)
	}

	type key struct{ res, metric string }
	winSums := make(map[key]float64)
	stepSums := make(map[key]map[int64]float64)
	for _, s := range samples {
		k := key{s.ResourceID, s.Metric}
		winSums[k] += s.Value
		m := stepSums[k]
		if m == nil {
			m = make(map[int64]float64)
			stepSums[k] = m
		}
		m[int64(s.Timestamp.Sub(from)/step)] += s.Value
	}
	for i, app := range apps {
		ov := RequestOverview{Summary: RequestSummary{WindowSeconds: window.Seconds()}}
		whole := make(map[string]map[int64]float64)
		series := make(map[string]map[int64]float64)
		for _, m := range requestMetricNames() {
			k := key{ids[i], m}
			whole[m] = map[int64]float64{0: winSums[k]}
			series[m] = stepSums[k]
		}
		if p, ok := requestPointAt(whole, 0, from, window); ok {
			ov.Summary = RequestSummary{
				WindowSeconds: window.Seconds(), HasTraffic: true, Requests: p.Requests,
				RatePerSec: p.RatePerSec, ErrorRate4xx: p.ErrorRate4xx, ErrorRate5xx: p.ErrorRate5xx,
				P95Ms: p.P95Ms, UpstreamErrors: p.UpstreamErrors,
			}
		}
		for idx := int64(0); idx <= int64(window/step); idx++ {
			if p, ok := requestPointAt(series, idx, from.Add(time.Duration(idx)*step), step); ok {
				ov.Points = append(ov.Points, p)
			}
		}
		out[app] = ov
	}
	return out, nil
}
