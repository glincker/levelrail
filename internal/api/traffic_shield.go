package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type trafficShieldReason struct {
	Reason string  `json:"reason"`
	Count  float64 `json:"count"`
}

type trafficShield struct {
	Blocked  float64               `json:"blocked"`
	ByReason []trafficShieldReason `json:"by_reason"`
}

func shieldCounts(ctx context.Context, q telemetry.MetricsQuerier, app string, from, to time.Time) trafficShield {
	out := trafficShield{ByReason: make([]trafficShieldReason, 0, telemetry.ShieldReasonCount)}
	for i, reason := range telemetry.ShieldReasons {
		n := sumMetric(ctx, q, app, telemetry.ShieldMetric(i), from, to)
		out.Blocked += n
		out.ByReason = append(out.ByReason, trafficShieldReason{Reason: reason, Count: n})
	}
	return out
}
