package deploytimeline

import (
	"strings"
)

// Explanation is a rule-based, read-only account of why a deploy did not
// succeed. It is derived from the timeline alone, so it is stable and needs
// no model call.
type Explanation struct {
	DeployID string  `json:"deploy_id"`
	App      string  `json:"app"`
	Verdict  Verdict `json:"verdict"`
	// Failed is false for a deploy that succeeded, is running or was canceled.
	Failed   bool   `json:"failed"`
	Phase    Phase  `json:"phase,omitempty"`
	Code     string `json:"code,omitempty"`
	Headline string `json:"headline"`
	// Cause is the first failed event's own words.
	Cause string `json:"cause,omitempty"`
	// Hint is the generic next step for the failed phase.
	Hint string `json:"hint,omitempty"`
	// Evidence is the warnings before the failure plus the failed event.
	Evidence []Event `json:"evidence,omitempty"`
	// LastLogs are the log lines just before the failure.
	LastLogs []string `json:"last_logs,omitempty"`
	// RolledBack is set when the platform reverted after the failure.
	RolledBack bool `json:"rolled_back,omitempty"`
}

var phaseHints = map[Phase]string{
	PhaseTrigger:   "Check the trigger source and that the repository and branch are still reachable.",
	PhaseBuild:     "Open the build log above the failing line, fix the build error and redeploy.",
	PhasePush:      "The built image could not reach the registry; check registry credentials and disk space.",
	PhaseHooks:     "A deploy hook exited non-zero; run it by hand with the same environment.",
	PhaseDeploy:    "The new release could not start; check the container exit reason and its environment.",
	PhaseReadiness: "The app started but never answered its readiness check; verify the port and health path.",
	PhaseCutover:   "Traffic could not move to the new release; check the ingress and node status.",
	PhaseWatch:     "The post-deploy watch tripped on errors or latency; compare with the previous release.",
}

// Explain summarises why tl failed. maxLogs bounds the log lines and warning
// evidence, and callers read it from settings.
func Explain(tl Timeline, maxLogs int) Explanation {
	ex := Explanation{DeployID: tl.DeployID, App: tl.App, Verdict: tl.Verdict}
	switch tl.Verdict {
	case VerdictFailed, VerdictRolledBack:
	default:
		ex.Headline = "This deploy did not fail (" + string(tl.Verdict) + ")."
		return ex
	}
	ex.Failed = true
	ex.RolledBack = tl.Verdict == VerdictRolledBack

	idx := firstFailed(tl.Events, true)
	if idx < 0 {
		idx = firstFailed(tl.Events, false)
	}
	if idx < 0 {
		ex.Headline = "The deploy failed but no step recorded the cause; read the full build log."
		return ex
	}
	first := tl.Events[idx]
	ex.Phase, ex.Code = first.Phase, first.Code
	ex.Cause = firstNonEmpty(first.Reason, first.Detail, first.Title)
	ex.Hint = phaseHints[first.Phase]
	ex.Headline = "Failed during " + string(first.Phase) + ": " + first.Title
	if ex.RolledBack {
		ex.Headline += " (rolled back)"
	}

	var logs []string
	for _, e := range tl.Events[:idx] {
		if e.Status == StatusWarning && len(ex.Evidence) < maxLogs {
			ex.Evidence = append(ex.Evidence, e)
		}
		if e.Kind == KindLog && strings.TrimSpace(e.Title) != "" {
			logs = append(logs, firstLine(e.Title))
		}
	}
	ex.Evidence = append(ex.Evidence, first)
	if maxLogs > 0 {
		if len(logs) > maxLogs {
			logs = logs[len(logs)-maxLogs:]
		}
		ex.LastLogs = logs
	}
	return ex
}

// firstFailed prefers a structural step over a log line, since a failed log
// line is often only the symptom.
func firstFailed(events []Event, skipLogs bool) int {
	for i, e := range events {
		if e.Status != StatusFailed || e.Kind == KindOutcome || (skipLogs && e.Kind == KindLog) {
			continue
		}
		return i
	}
	return -1
}
