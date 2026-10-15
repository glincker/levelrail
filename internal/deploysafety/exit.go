package deploysafety

import (
	"fmt"
	"time"
)

// Workload says what a stopped container was supposed to do.
type Workload string

const (
	// WorkloadService is a long-running app container: any exit, code 0
	// included, means it stopped serving.
	WorkloadService Workload = "service"
	// WorkloadJob is a one-shot container (a scheduled task or job run):
	// exit 0 is success.
	WorkloadJob Workload = "job"
)

// Exit phases, the point in a release's life a container stopped.
const (
	ExitPhaseReadiness = "readiness"
	ExitPhaseWindow    = "window"
)

// ExitObservation is one container's observed run state.
type ExitObservation struct {
	Running    bool
	OOMKilled  bool
	ExitCode   int
	StartedAt  time.Time
	FinishedAt time.Time
	// ObservedAt stands in for FinishedAt when the transport cannot report it.
	ObservedAt time.Time
}

// ExitVerdict is ClassifyExit's result.
type ExitVerdict struct {
	// Exited is true once the container stopped running.
	Exited bool
	// Failed is true when the exit is a failure for this workload.
	Failed  bool
	Reason  string
	Message string
	// Ran is how long the container ran, zero when unknown.
	Ran time.Duration
}

// ClassifyExit decides whether obs is a failure for workload w. For a
// service it is any exit; phase picks the reason (readiness or window).
func ClassifyExit(w Workload, phase string, obs ExitObservation) ExitVerdict {
	if obs.Running {
		return ExitVerdict{}
	}
	v := ExitVerdict{Exited: true, Ran: ranFor(obs)}
	if w == WorkloadJob {
		switch {
		case obs.OOMKilled:
			v.Failed, v.Message = true, "container was killed for exceeding its memory limit"
		case obs.ExitCode != 0:
			v.Failed, v.Message = true, fmt.Sprintf("exited with code %d", obs.ExitCode)
		}
		return v
	}
	v.Failed = true
	switch {
	case obs.OOMKilled && phase == ExitPhaseWindow:
		v.Reason = ReasonOOMKilledDuringWindow
	case obs.OOMKilled:
		v.Reason = ReasonOOMKilledDuringReadiness
	case phase == ExitPhaseWindow:
		v.Reason = ReasonExitedDuringWindow
	default:
		v.Reason = ReasonExitedDuringReadiness
	}
	v.Message = serviceExitMessage(obs, v.Ran, phase)
	return v
}

func ranFor(obs ExitObservation) time.Duration {
	end := obs.FinishedAt
	if end.IsZero() {
		end = obs.ObservedAt
	}
	if obs.StartedAt.IsZero() || end.IsZero() || end.Before(obs.StartedAt) {
		return 0
	}
	return end.Sub(obs.StartedAt)
}

func serviceExitMessage(obs ExitObservation, ran time.Duration, phase string) string {
	what := fmt.Sprintf("Container exited with code %d", obs.ExitCode)
	if obs.OOMKilled {
		what = "Container was killed for exceeding its memory limit"
	}
	if r := roundRan(ran); r > 0 {
		what += " after " + r.String()
	} else if ran > 0 {
		what += " immediately"
	}
	if phase == ExitPhaseWindow {
		return what + ", during its stable window after becoming ready"
	}
	return what + ", before it became ready"
}

func roundRan(d time.Duration) time.Duration {
	if d < time.Second {
		return d.Round(100 * time.Millisecond)
	}
	return d.Round(time.Second)
}
