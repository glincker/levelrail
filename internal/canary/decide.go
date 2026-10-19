package canary

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Action is what the reconciler should do for one canary this tick.
type Action int

const (
	// Hold leaves the canary as it is.
	Hold Action = iota
	// Advance moves to the next stage.
	Advance
	// Promote deploys the canary image to the stable app.
	Promote
	// Rollback removes the canary because it breached a threshold.
	Rollback
)

func (a Action) String() string {
	switch a {
	case Advance:
		return "advance"
	case Promote:
		return "promote"
	case Rollback:
		return "rollback"
	default:
		return "hold"
	}
}

// Sample is one release's traffic over a window.
type Sample struct {
	Requests uint64
	Errors   uint64
	Latency  [telemetry.LatencyBucketCount]uint64
}

// ErrorRate is the 5xx fraction, 0 for an empty window.
func (s Sample) ErrorRate() float64 {
	if s.Requests == 0 {
		return 0
	}
	return float64(s.Errors) / float64(s.Requests)
}

// P95 is the 95th percentile latency in milliseconds, 0 for an empty window.
func (s Sample) P95() float64 {
	counts := make([]float64, len(s.Latency))
	for i, c := range s.Latency {
		counts[i] = float64(c)
	}
	return telemetry.PercentileFromBuckets(counts, 0.95)
}

// SampleFrom converts an ingress upstream counter.
func SampleFrom(u telemetry.UpstreamStat) Sample {
	return Sample{Requests: u.Requests, Errors: u.Errors5xx, Latency: u.Latency}
}

// Sub returns the traffic since earlier, clamping at zero if a counter reset.
func (s Sample) Sub(earlier Sample) Sample {
	out := Sample{Requests: sub(s.Requests, earlier.Requests), Errors: sub(s.Errors, earlier.Errors)}
	for i := range s.Latency {
		out.Latency[i] = sub(s.Latency[i], earlier.Latency[i])
	}
	return out
}

func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// Decision is the verdict for one tick and the reason shown to the operator.
type Decision struct {
	Action Action
	Reason string
}

// Input is everything Decide needs; it holds no clock or I/O.
type Input struct {
	Policy Policy
	Limits Limits
	// Stage is the index into Policy.Stages currently serving.
	Stage int
	// Elapsed is how long this stage has been observed.
	Elapsed  time.Duration
	Canary   Sample
	Baseline Sample
}

// Breach reports whether the canary is worse than the baseline beyond the
// policy, with a reason. Too few canary requests is never a breach.
func Breach(in Input) (bool, string) {
	if in.Canary.Requests < in.Limits.MinRequests {
		return false, ""
	}
	canaryErr, baseErr := in.Canary.ErrorRate(), in.Baseline.ErrorRate()
	if canaryErr-baseErr > in.Policy.MaxErrorRate {
		return true, fmt.Sprintf("error rate %.1f%% vs %.1f%% on the stable release (limit +%.1f points)", canaryErr*100, baseErr*100, in.Policy.MaxErrorRate*100)
	}
	canaryP95, baseP95 := in.Canary.P95(), in.Baseline.P95()
	if in.Baseline.Requests >= in.Limits.MinRequests && canaryP95 > in.Limits.P95FloorMS && canaryP95 > baseP95*in.Policy.MaxP95Ratio {
		return true, fmt.Sprintf("p95 latency %.0f ms vs %.0f ms on the stable release (limit %.1fx)", canaryP95, baseP95, in.Policy.MaxP95Ratio)
	}
	return false, ""
}

// Decide is the pure stage machine: roll back on a breach, hold until the
// stage has run its interval with enough traffic, then advance or promote.
func Decide(in Input) Decision {
	if breach, why := Breach(in); breach {
		return Decision{Rollback, why}
	}
	if in.Elapsed < in.Policy.Interval {
		return Decision{Hold, "observing stage"}
	}
	if in.Canary.Requests < in.Limits.MinRequests {
		return Decision{Hold, fmt.Sprintf("waiting for traffic (%d of %d requests)", in.Canary.Requests, in.Limits.MinRequests)}
	}
	next := in.Stage + 1
	if next >= len(in.Policy.Stages) || in.Policy.Stages[next] >= Full {
		return Decision{Promote, "all stages passed"}
	}
	return Decision{Advance, fmt.Sprintf("stage %d passed", in.Stage+1)}
}
