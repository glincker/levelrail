package deploysafety

import (
	"fmt"
	"time"
)

// Verdict is the outcome of one evaluation of a gate or watch rule.
type Verdict string

// Verdicts.
const (
	VerdictPending Verdict = "pending"
	VerdictPass    Verdict = "pass"
	VerdictFail    Verdict = "fail"
)

// GateObservation is what the reconciler saw of a new container on one
// tick of its stability gate.
type GateObservation struct {
	Running   bool
	OOMKilled bool
	ExitCode  int
	// RestartCount and StartedAt are compared against the values seen
	// when the gate began; a zero StartedAt means the runtime cannot tell.
	RestartCount         int
	BaselineRestartCount int
	StartedAt            time.Time
	BaselineStartedAt    time.Time
	ConsecutivePasses    int
	SinceStart           time.Duration
	BudgetExhausted      bool
}

// EvaluateGate decides whether a new container may take traffic yet.
func EvaluateGate(o GateObservation, cfg Effective) (Verdict, string) {
	threshold := cfg.SuccessThreshold
	if threshold < 1 {
		threshold = 1
	}
	switch {
	case o.OOMKilled:
		return VerdictFail, ReasonOOMKilledDuringWindow
	case !o.Running:
		return VerdictFail, ReasonExitedDuringWindow
	case o.RestartCount > o.BaselineRestartCount:
		return VerdictFail, ReasonRestartedDuringWindow
	case !o.StartedAt.IsZero() && !o.BaselineStartedAt.IsZero() && !o.StartedAt.Equal(o.BaselineStartedAt):
		return VerdictFail, ReasonRestartedDuringWindow
	case o.ConsecutivePasses >= threshold && o.SinceStart >= cfg.StableWindow:
		return VerdictPass, ""
	case o.BudgetExhausted:
		return VerdictFail, ReasonGateTimeout
	default:
		return VerdictPending, ""
	}
}

// WatchObservation is what the post-deploy watch saw of a live release
// since its cutover.
type WatchObservation struct {
	SinceCutover time.Duration
	Restarts     int
	Exited       bool
	OOMKilled    bool
	Requests     float64
	Errors5xx    float64
	// P95Ms is the new release's p95 since cutover; BaselineP95Ms and
	// BaselineRequests describe the release it replaced over the same span
	// before cutover. A zero baseline means there is nothing to compare to.
	P95Ms            float64
	BaselineP95Ms    float64
	BaselineRequests float64
}

// LatencyRegressed reports whether the new release's p95 is a real step up
// from the baseline: both sides need MinRequests of traffic, and the rise
// must clear both the ratio and the absolute floor.
func LatencyRegressed(o WatchObservation, cfg Effective) (bool, string) {
	minReq := float64(cfg.WatchMinRequests)
	if cfg.WatchLatencyRatio <= 0 || o.BaselineP95Ms <= 0 || o.Requests < minReq || o.BaselineRequests < minReq {
		return false, ""
	}
	floor := float64(cfg.WatchLatencyMinDelta) / float64(time.Millisecond)
	if o.P95Ms < o.BaselineP95Ms*cfg.WatchLatencyRatio || o.P95Ms-o.BaselineP95Ms < floor {
		return false, ""
	}
	return true, fmt.Sprintf("p95 latency is %.0fms after cutover against %.0fms before (limit %.1fx, %d requests)", o.P95Ms, o.BaselineP95Ms, cfg.WatchLatencyRatio, int(o.Requests))
}

// EvaluateWatch decides whether a live release must be rolled back, has
// cleared its watch window, or is still being watched.
func EvaluateWatch(o WatchObservation, cfg Effective) (Verdict, string, string) {
	if o.OOMKilled {
		return VerdictFail, ReasonWatchOOMKilled, "a container of the new release was OOM-killed after cutover"
	}
	restarts := o.Restarts
	if o.Exited {
		restarts++
	}
	if cfg.WatchMaxRestarts > 0 && restarts >= cfg.WatchMaxRestarts {
		if o.Restarts == 0 {
			return VerdictFail, ReasonWatchExited, "a container of the new release exited after cutover"
		}
		return VerdictFail, ReasonWatchCrashloop, fmt.Sprintf("the new release restarted %d times after cutover (limit %d)", restarts, cfg.WatchMaxRestarts)
	}
	if cfg.WatchErrorRate > 0 && o.Requests >= float64(cfg.WatchMinRequests) && o.Requests > 0 {
		if rate := o.Errors5xx / o.Requests; rate >= cfg.WatchErrorRate {
			return VerdictFail, ReasonWatchErrorRate, fmt.Sprintf("%.0f%% of %d requests returned 5xx after cutover (limit %.0f%%)", rate*100, int(o.Requests), cfg.WatchErrorRate*100)
		}
	}
	if o.SinceCutover >= cfg.WatchWindow {
		return VerdictPass, "", ""
	}
	return VerdictPending, "", ""
}
