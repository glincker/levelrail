package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// diagnosticLogWindow and diagnosticLogLines bound how much log context
// handleDiagnoseApp pulls in for diagnose's log-based signatures (OOM,
// dependency failures): enough to catch a recent crash, not a full
// log-viewer-sized query.
const (
	diagnosticLogWindow = 10 * time.Minute
	diagnosticLogLines  = 50
)

type diagnosisSignalResource struct {
	Source  string `json:"source"`
	Excerpt string `json:"excerpt"`
}

type diagnosisResource struct {
	Explanation     string                    `json:"explanation"`
	Suggestion      string                    `json:"suggestion"`
	Confidence      string                    `json:"confidence"`
	MatchedSignals  []diagnosisSignalResource `json:"matched_signals"`
	DeployAttemptID string                    `json:"deploy_attempt_id,omitempty"`
}

func toDiagnosisResource(res diagnose.Result, attemptID string) diagnosisResource {
	signals := make([]diagnosisSignalResource, 0, len(res.MatchedSignals))
	for _, s := range res.MatchedSignals {
		signals = append(signals, diagnosisSignalResource{Source: s.Source, Excerpt: s.Excerpt})
	}
	return diagnosisResource{
		Explanation:     res.Explanation,
		Suggestion:      res.Suggestion,
		Confidence:      res.Confidence,
		MatchedSignals:  signals,
		DeployAttemptID: attemptID,
	}
}

// handleDiagnoseApp handles GET /api/v1/apps/{name}/diagnose: a
// read-only, deterministic explanation (internal/diagnose) of why this
// app's most recent deploy attempt (or a specific past one, via
// ?deploy_id=) failed, or why it's crashlooping, synthesized from
// signals the platform already collects. Per CLAUDE.md section 4.11,
// this is a read-and-suggest layer only: it never writes to any
// resource and never calls an external model.
func (rt *Router) handleDiagnoseApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	_, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: diagnose app: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	attempt, err := rt.resolveDiagnosisAttempt(ctx, name, r.URL.Query().Get("deploy_id"))
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: diagnose app: load deploy attempt failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	conditions, err := rt.deploys.GetConditions(ctx, applicationControllerName(name))
	if err != nil {
		rt.logger.Error("api: diagnose app: load conditions failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	in := diagnose.Input{
		ServiceName:    name,
		Attempt:        toAttemptSignal(attempt),
		Conditions:     toConditionSignals(conditions),
		Crashloop:      rt.diagnoseCrashloop(ctx, name),
		RecentLogLines: rt.diagnoseRecentLogs(ctx, name),
	}

	attemptID := ""
	if attempt != nil {
		attemptID = attempt.ID
	}
	writeJSON(w, http.StatusOK, toDiagnosisResource(diagnose.Diagnose(in), attemptID))
}

// resolveDiagnosisAttempt returns deployID's attempt if given (scoped to
// name, the same cross-app guard handleDeployLogStream already
// establishes), otherwise name's newest attempt, or nil if it has none
// yet: an app with no deploy history can still be diagnosed on its
// conditions and crashloop state alone.
func (rt *Router) resolveDiagnosisAttempt(ctx context.Context, name, deployID string) (*store.DeployAttempt, error) {
	if deployID != "" {
		attempt, err := rt.deployAttempts.GetDeployAttempt(ctx, deployID)
		if err != nil {
			return nil, err
		}
		if attempt.ServiceName != name {
			return nil, store.ErrDeployAttemptNotFound
		}
		return attempt, nil
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(attempts) == 0 {
		return nil, nil
	}
	return &attempts[0], nil
}

func toAttemptSignal(attempt *store.DeployAttempt) *diagnose.AttemptInput {
	if attempt == nil {
		return nil
	}
	return &diagnose.AttemptInput{Status: attempt.Status, Error: attempt.Error}
}

func toConditionSignals(conditions []reconcile.Condition) []diagnose.ConditionInput {
	out := make([]diagnose.ConditionInput, 0, len(conditions))
	for _, c := range conditions {
		out = append(out, diagnose.ConditionInput{Type: c.Type, Status: string(c.Status), Reason: c.Reason, Message: c.Message})
	}
	return out
}

// diagnoseCrashloop is best-effort: no alertRules configured, no
// crashloop rule for this app, or a lookup failure all just mean this
// one signal is unavailable, never a reason to fail the whole
// diagnosis.
func (rt *Router) diagnoseCrashloop(ctx context.Context, name string) *diagnose.CrashloopInput {
	if rt.alertRules == nil {
		return nil
	}
	rules, err := rt.alertRules.ListRulesForResource(ctx, resourceIDForApp(name))
	if err != nil {
		rt.logger.Warn("api: diagnose app: list alert rules failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	for _, rule := range rules {
		if rule.Kind != alerting.KindCrashloop {
			continue
		}
		restartCount := 0
		if rule.LastValue != nil {
			restartCount = int(*rule.LastValue)
		}
		return &diagnose.CrashloopInput{
			Firing:                rule.Firing,
			RestartCount:          restartCount,
			RestartCountThreshold: rule.RestartCountThreshold,
			RestartWindow:         rule.RestartWindow.String(),
		}
	}
	return nil
}

// diagnoseRecentLogs is best-effort, same reasoning as diagnoseCrashloop:
// no telemetry configured or a query failure just means the log-based
// signatures (OOM, dependency failures) have nothing to match against.
func (rt *Router) diagnoseRecentLogs(ctx context.Context, name string) []string {
	if rt.telemetry == nil {
		return nil
	}
	now := time.Now()
	entries, err := rt.telemetry.QueryLogs(ctx, resourceIDForApp(name), now.Add(-diagnosticLogWindow), now, "")
	if err != nil {
		rt.logger.Warn("api: diagnose app: query logs failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	if len(entries) > diagnosticLogLines {
		entries = entries[len(entries)-diagnosticLogLines:]
	}
	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		lines = append(lines, e.Message)
	}
	return lines
}
