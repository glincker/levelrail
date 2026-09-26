package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// KindSLOBurn fires on error-budget burn rate for a request-based SLO over the
// app's ingress request metrics, using the multiwindow multi-burn-rate method.
const KindSLOBurn Kind = "slo_burn"

// SLO objective kinds.
const (
	SLOAvailability = "availability"
	SLOLatency      = "latency"
)

// SLOConfig is the request-based SLO a KindSLOBurn rule watches. Target is a
// percentage of good requests (99.9). For SLOLatency a request is good when it
// finishes within LatencyMs, rounded down to the nearest histogram bound.
type SLOConfig struct {
	Objective string  `json:"objective"`
	Target    float64 `json:"target"`
	LatencyMs float64 `json:"latency_ms,omitempty"`
}

// ErrorBudget is the allowed fraction of bad requests, 0.001 for a 99.9 target.
func (c SLOConfig) ErrorBudget() float64 { return 1 - c.Target/100 }

// Validate rejects a config that cannot yield a meaningful burn rate.
func (c SLOConfig) Validate() error {
	switch c.Objective {
	case SLOAvailability:
	case SLOLatency:
		if c.LatencyMs < telemetry.LatencyBucketsMS[0] {
			return fmt.Errorf("latency_ms must be at least %v", telemetry.LatencyBucketsMS[0])
		}
	default:
		return fmt.Errorf("objective must be %q or %q", SLOAvailability, SLOLatency)
	}
	if math.IsNaN(c.Target) || c.Target < 50 || c.Target >= 100 {
		return errors.New("target must be at least 50 and below 100 (a percentage such as 99.9)")
	}
	return nil
}

func encodeSLO(c *SLOConfig) string {
	if c == nil {
		return ""
	}
	b, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeSLO(raw string) *SLOConfig {
	if raw == "" {
		return nil
	}
	var c SLOConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil
	}
	return &c
}

// BurnRate is how many times faster than sustainable the error budget is being
// spent: the observed bad ratio divided by the allowed one. No traffic burns nothing.
func BurnRate(bad, total, target float64) float64 {
	budget := 1 - target/100
	if total <= 0 || budget <= 0 || bad <= 0 {
		return 0
	}
	return math.Min(bad, total) / total / budget
}

// BudgetRemaining is the fraction of the error budget left (1 untouched, 0
// spent, negative overspent). No traffic leaves it untouched.
func BudgetRemaining(bad, total, target float64) float64 {
	budget := 1 - target/100
	if total <= 0 || budget <= 0 {
		return 1
	}
	return 1 - math.Min(math.Max(bad, 0), total)/(total*budget)
}

// RequestCounts is the good/bad split of an app's requests over a span.
type RequestCounts struct {
	Total float64
	Bad   float64
}

// CountRequests sums an app's request counters over [from, to]. Counters are
// per-tick deltas, so negative or non-finite values (a counter reset artefact)
// count as zero and Bad never exceeds Total. A window with partial data is
// counted over what exists.
func CountRequests(ctx context.Context, src MetricsSource, app string, from, to time.Time, cfg SLOConfig) (RequestCounts, error) {
	resource := "service:" + app
	sum := func(metric string) (float64, error) {
		samples, err := src.QueryMetrics(ctx, resource, metric, from, to)
		if err != nil {
			return 0, fmt.Errorf("alerting: slo query %s/%s: %w", app, metric, err)
		}
		var t float64
		for _, s := range samples {
			if s.Value > 0 && !math.IsInf(s.Value, 0) && !math.IsNaN(s.Value) {
				t += s.Value
			}
		}
		return t, nil
	}

	total, err := sum(telemetry.MetricHTTPRequests)
	if err != nil || total <= 0 {
		return RequestCounts{}, err
	}
	if cfg.Objective != SLOLatency {
		bad, err := sum(telemetry.MetricHTTPResponses5xx)
		if err != nil {
			return RequestCounts{}, err
		}
		return RequestCounts{Total: total, Bad: math.Min(bad, total)}, nil
	}

	var good float64
	for i, bound := range telemetry.LatencyBucketsMS {
		if bound > cfg.LatencyMs {
			break
		}
		n, err := sum(telemetry.LatencyBucketMetric(i))
		if err != nil {
			return RequestCounts{}, err
		}
		good += n
	}
	return RequestCounts{Total: total, Bad: math.Max(total-good, 0)}, nil
}
