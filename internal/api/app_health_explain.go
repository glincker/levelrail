package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/healthpreset"
	"github.com/glincker/levelrail/internal/secretmask"
	"github.com/glincker/levelrail/internal/store"
)

// Explain states, worst last.
const (
	healthExplainHealthy  = "healthy"
	healthExplainDegraded = "degraded"
	healthExplainFailing  = "failing"
)

// Next step kinds the CLI and dashboard switch on.
const (
	nextStepApplyPreset = "apply_preset"
	nextStepRollback    = "rollback"
	nextStepViewLogs    = "view_logs"
	nextStepNone        = "none"
)

// explainLogMaxRunes keeps the log line to one readable row.
const explainLogMaxRunes = 240

type healthExplainCheck struct {
	Target     string    `json:"target"`
	Error      string    `json:"error,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	Since      time.Time `json:"since"`
	LastProbed time.Time `json:"last_probed"`
}

type healthExplainDeploy struct {
	ID        string    `json:"id"`
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Error     string    `json:"error,omitempty"`
}

type healthExplainNextStep struct {
	Kind    string `json:"kind"`
	Preset  string `json:"preset,omitempty"`
	Command string `json:"command,omitempty"`
}

// appHealthExplainResource answers "why is this red or amber" from signals the router already holds.
type appHealthExplainResource struct {
	AppName      string                `json:"app_name"`
	State        string                `json:"state"`
	FailingCheck *healthExplainCheck   `json:"failing_check,omitempty"`
	Restarts     *int                  `json:"restarts,omitempty"`
	Crashloop    bool                  `json:"crashloop"`
	LastDeploy   *healthExplainDeploy  `json:"last_deploy,omitempty"`
	LogError     string                `json:"log_error,omitempty"`
	NextStep     healthExplainNextStep `json:"next_step"`
	ComputedAt   time.Time             `json:"computed_at"`
}

// handleGetAppHealthExplain handles GET /api/v1/apps/{name}/health-explain, computed fresh per call.
func (rt *Router) handleGetAppHealthExplain(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: health explain: load app", err)
		return
	}
	out := appHealthExplainResource{AppName: name, ComputedAt: time.Now().UTC()}

	if n, ok := rt.liveRestarts(ctx, *svc); ok {
		out.Restarts = &n
	}
	if rt.alertRules != nil {
		rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(name))
		if err != nil {
			rt.logger.Warn("api: health explain: list rules failed", slog.String("app", name), slog.String("error", err.Error()))
		}
		for _, rl := range rules {
			if rl.Kind == alerting.KindCrashloop && rl.Firing {
				out.Crashloop = true
			}
		}
	}

	var attempts []store.DeployAttempt
	if rt.deployAttempts != nil {
		attempts, err = rt.deployAttempts.ListDeployAttempts(ctx, name)
		if err != nil {
			rt.logger.Warn("api: health explain: list deploys failed", slog.String("app", name), slog.String("error", err.Error()))
			attempts = nil
		}
	}
	if len(attempts) > 0 {
		a := attempts[0]
		out.LastDeploy = &healthExplainDeploy{ID: a.ID, Image: a.Image, Status: a.Status, StartedAt: a.StartedAt, Error: secretmask.Mask(a.Error)}
		out.FailingCheck = rt.explainFailingCheck(r, attempts)
	}
	out.LogError = rt.explainLogError(r, name)

	deployFailed := out.LastDeploy != nil && out.LastDeploy.Status == store.DeployAttemptStatusFailed
	switch {
	case out.Crashloop || out.FailingCheck != nil || deployFailed:
		out.State = healthExplainFailing
	case (out.Restarts != nil && *out.Restarts > 0) || out.LogError != "":
		out.State = healthExplainDegraded
	default:
		out.State = healthExplainHealthy
	}
	out.NextStep = explainNextStep(name, svc, out, attempts)
	writeJSON(w, http.StatusOK, out)
}

// explainFailingCheck reads the newest attempt that recorded probes and reports a trailing
// run of failures, since the first failure of that run is when the check went bad.
func (rt *Router) explainFailingCheck(r *http.Request, attempts []store.DeployAttempt) *healthExplainCheck {
	if rt.probeAttempts == nil {
		return nil
	}
	for _, a := range attempts {
		probes, err := rt.probeAttempts.ListProbeAttempts(r.Context(), a.ID)
		if err != nil {
			rt.logger.Warn("api: health explain: list probes failed", slog.String("deploy", a.ID), slog.String("error", err.Error()))
			return nil
		}
		if len(probes) == 0 {
			continue
		}
		last := probes[0]
		for _, p := range probes {
			if p.ProbedAt.After(last.ProbedAt) {
				last = p
			}
		}
		if last.Success {
			return nil
		}
		since := last.ProbedAt
		for _, p := range probes {
			if !p.Success && p.ProbedAt.Before(since) {
				since = p.ProbedAt
			}
		}
		return &healthExplainCheck{Target: last.Target, Error: secretmask.Mask(last.Error), StatusCode: last.StatusCode, Since: since, LastProbed: last.ProbedAt}
	}
	return nil
}

// explainLogError returns the newest error level line in the diagnostic window, masked and shortened.
func (rt *Router) explainLogError(r *http.Request, name string) string {
	if rt.telemetry == nil {
		return ""
	}
	now := time.Now()
	entries, err := rt.telemetry.QueryLogs(r.Context(), resourceIDForApp(name), now.Add(-diagnosticLogWindow), now, "")
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if classifyLogLevel(entries[i]) != "error" {
			continue
		}
		line := secretmask.Mask(strings.TrimSpace(entries[i].Message))
		if runes := []rune(line); len(runes) > explainLogMaxRunes {
			line = string(runes[:explainLogMaxRunes])
		}
		return line
	}
	return ""
}

// explainNextStep prefers a rollback when a good deploy exists behind a failed one, then a
// probe path fix, then the logs.
func explainNextStep(name string, svc *store.DesiredService, out appHealthExplainResource, attempts []store.DeployAttempt) healthExplainNextStep {
	if out.LastDeploy != nil && out.LastDeploy.Status == store.DeployAttemptStatusFailed {
		for _, a := range attempts[1:] {
			if a.Status == store.DeployAttemptStatusSucceeded {
				return healthExplainNextStep{Kind: nextStepRollback, Command: fmt.Sprintf("levelrail apps rollback %s", name)}
			}
		}
	}
	if c := out.FailingCheck; c != nil && c.StatusCode == http.StatusNotFound && svc.Health != nil && svc.Health.Readiness != nil {
		for _, id := range []string{"healthz", "health", "api-health", "ping"} {
			p, ok := healthpreset.Get(id)
			if ok && p.Path != svc.Health.Readiness.Path {
				return healthExplainNextStep{Kind: nextStepApplyPreset, Preset: id,
					Command: fmt.Sprintf("levelrail apps health set %s --probe readiness --preset %s", name, id)}
			}
		}
	}
	if out.Crashloop || out.LogError != "" || out.FailingCheck != nil {
		return healthExplainNextStep{Kind: nextStepViewLogs, Command: fmt.Sprintf("levelrail apps logs %s", name)}
	}
	return healthExplainNextStep{Kind: nextStepNone}
}
