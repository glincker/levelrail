package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// trafficAbuse is the per-app ingress abuse defense tally over a traffic
// window: what it let through, what log_only flagged, what enforce blocked.
type trafficAbuse struct {
	Allowed    float64            `json:"allowed"`
	WouldBlock float64            `json:"would_block"`
	Blocked    float64            `json:"blocked"`
	FailOpen   float64            `json:"fail_open"`
	ByReason   []abuseReasonCount `json:"by_reason"`
}

// emptyAbuseTraffic is the zero tally with every reason listed, so a client
// never has to handle a missing row.
func emptyAbuseTraffic() trafficAbuse {
	out := trafficAbuse{ByReason: make([]abuseReasonCount, 0, telemetry.AbuseReasonCount)}
	for _, name := range telemetry.AbuseReasons {
		out.ByReason = append(out.ByReason, abuseReasonCount{Reason: name})
	}
	return out
}

func abuseTraffic(ctx context.Context, q telemetry.MetricsQuerier, app string, from, to time.Time) trafficAbuse {
	out := trafficAbuse{ByReason: make([]abuseReasonCount, 0, telemetry.AbuseReasonCount)}
	out.Allowed = sumMetric(ctx, q, app, telemetry.MetricAbuseAllowed, from, to)
	out.FailOpen = sumMetric(ctx, q, app, telemetry.MetricAbuseFailOpen, from, to)
	for i, name := range telemetry.AbuseReasons {
		rc := abuseReasonCount{
			Reason:     name,
			WouldBlock: sumMetric(ctx, q, app, telemetry.AbuseWouldBlockMetric(i), from, to),
			Blocked:    sumMetric(ctx, q, app, telemetry.AbuseBlockedMetric(i), from, to),
		}
		out.WouldBlock += rc.WouldBlock
		out.Blocked += rc.Blocked
		out.ByReason = append(out.ByReason, rc)
	}
	return out
}
