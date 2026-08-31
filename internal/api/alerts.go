package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AlertRules is the surface the alert-rule handlers need from
// internal/alerting.DB (TASKS.md 2.5/2.7). *alerting.DB satisfies this
// structurally, the same "narrow consumer-defined interface" convention
// TelemetryQuerier and SecretSetter already establish in this package.
type AlertRules interface {
	SaveRule(ctx context.Context, r alerting.Rule) error
	GetRule(ctx context.Context, id string) (*alerting.Rule, error)
	ListRulesForResource(ctx context.Context, resourceID string) ([]alerting.Rule, error)
	DeleteRule(ctx context.Context, id string) error
}

// ruleResource is the wire shape for an alert rule: alerting.Rule with
// Go time.Duration fields flattened to duration strings ("2m", "30s")
// for the same reason handleQueryMetrics' step query param and
// APP_SESSION_TTL are duration strings elsewhere in this codebase,
// rather than raw seconds a caller has to know the unit of.
//
// ID and ResourceID are never caller-supplied: ID is always generated
// server-side (alerting.NewRuleID, matching how every other ID in this
// codebase is minted, never trusted from a request body), ResourceID is
// always derived from the app name in the URL
// (resourceIDForApp, metrics.go), never taken from the body, so a
// caller cannot point a rule at a different app's resource just by
// putting a different value in the JSON. Both are still marshaled out
// on responses, so a caller can see what was actually assigned.
type ruleResource struct {
	ID         string `json:"id,omitempty"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`

	// Threshold-kind fields, ignored (left at zero value) for a
	// crashloop-kind rule, mirroring alerting.Rule's own shape.
	Metric      string  `json:"metric,omitempty"`
	Comparator  string  `json:"comparator,omitempty"`
	Threshold   float64 `json:"threshold"`
	ForDuration string  `json:"for_duration,omitempty"`

	// Crashloop-kind fields, ignored for a threshold-kind rule.
	RestartCountThreshold int    `json:"restart_count_threshold"`
	RestartWindow         string `json:"restart_window,omitempty"`

	// KindScheduledTaskFailure-only field: which of this app's scheduled
	// tasks the rule watches. RestartCountThreshold above doubles as its
	// consecutive-failure threshold; see alerting.Rule's own doc comment.
	ScheduledTaskID string `json:"scheduled_task_id,omitempty"`

	NotifyURL  string `json:"notify_url,omitempty"`
	NotifyKind string `json:"notify_kind,omitempty"`
	Enabled    bool   `json:"enabled"`

	// Evaluation state: response-only. A request body that happens to
	// set these is simply ignored, toRule below never reads them; only
	// the evaluator (internal/alerting's Engine) ever writes this state,
	// via UpdateState, exactly the boundary alerting.Rule's own doc
	// comment already draws.
	Firing          bool       `json:"firing,omitempty"`
	PendingSince    *time.Time `json:"pending_since,omitempty"`
	FiringSince     *time.Time `json:"firing_since,omitempty"`
	LastEvaluatedAt *time.Time `json:"last_evaluated_at,omitempty"`
	LastValue       *float64   `json:"last_value,omitempty"`
}

func toRuleResource(r alerting.Rule) ruleResource {
	out := ruleResource{
		ID:                    r.ID,
		Name:                  r.Name,
		Kind:                  string(r.Kind),
		ResourceID:            r.ResourceID,
		ChannelID:             r.ChannelID,
		Metric:                r.Metric,
		Comparator:            string(r.Comparator),
		Threshold:             r.Threshold,
		RestartCountThreshold: r.RestartCountThreshold,
		ScheduledTaskID:       r.ScheduledTaskID,
		NotifyURL:             r.NotifyURL,
		NotifyKind:            string(r.NotifyKind),
		Enabled:               r.Enabled,
		Firing:                r.Firing,
		PendingSince:          r.PendingSince,
		FiringSince:           r.FiringSince,
		LastEvaluatedAt:       r.LastEvaluatedAt,
		LastValue:             r.LastValue,
	}
	if r.ForDuration > 0 {
		out.ForDuration = r.ForDuration.String()
	}
	if r.RestartWindow > 0 {
		out.RestartWindow = r.RestartWindow.String()
	}
	return out
}

// toRule converts a request body into an alerting.Rule, validating it
// along the way. id is assigned by the caller (handleCreateAlertRule),
// never taken from the resource itself.
func (a ruleResource) toRule(id string) (alerting.Rule, error) {
	if a.Name == "" {
		return alerting.Rule{}, errors.New("name is required")
	}

	kind := alerting.Kind(a.Kind)
	switch kind {
	case alerting.KindThreshold, alerting.KindCrashloop, alerting.KindCertExpiry, alerting.KindPatchStatus, alerting.KindScheduledTaskFailure:
	default:
		return alerting.Rule{}, fmt.Errorf("kind must be %q, %q, %q, %q, or %q",
			alerting.KindThreshold, alerting.KindCrashloop, alerting.KindCertExpiry, alerting.KindPatchStatus, alerting.KindScheduledTaskFailure)
	}

	forDuration, err := parseOptionalDuration(a.ForDuration)
	if err != nil {
		return alerting.Rule{}, fmt.Errorf("for_duration: %w", err)
	}
	restartWindow, err := parseOptionalDuration(a.RestartWindow)
	if err != nil {
		return alerting.Rule{}, fmt.Errorf("restart_window: %w", err)
	}

	r := alerting.Rule{
		ID:                    id,
		Name:                  a.Name,
		Kind:                  kind,
		ResourceID:            a.ResourceID,
		Metric:                a.Metric,
		Comparator:            alerting.Comparator(a.Comparator),
		Threshold:             a.Threshold,
		ForDuration:           forDuration,
		RestartCountThreshold: a.RestartCountThreshold,
		RestartWindow:         restartWindow,
		ScheduledTaskID:       a.ScheduledTaskID,
		ChannelID:             a.ChannelID,
		NotifyURL:             a.NotifyURL,
		NotifyKind:            alerting.NotifyKind(a.NotifyKind),
		Enabled:               a.Enabled,
	}

	switch kind {
	case alerting.KindThreshold:
		if r.Metric == "" {
			return alerting.Rule{}, errors.New("metric is required for a threshold rule")
		}
		switch r.Comparator {
		case alerting.GreaterThan, alerting.LessThan, alerting.GreaterOrEqual, alerting.LessOrEqual:
		default:
			return alerting.Rule{}, fmt.Errorf("comparator must be one of %q, %q, %q, %q",
				alerting.GreaterThan, alerting.LessThan, alerting.GreaterOrEqual, alerting.LessOrEqual)
		}
	case alerting.KindCrashloop:
		if r.RestartCountThreshold <= 0 {
			return alerting.Rule{}, errors.New("restart_count_threshold must be a positive integer for a crashloop rule")
		}
		if r.RestartWindow <= 0 {
			return alerting.Rule{}, errors.New("restart_window must be a positive duration for a crashloop rule")
		}
	case alerting.KindScheduledTaskFailure:
		if r.ScheduledTaskID == "" {
			return alerting.Rule{}, errors.New("scheduled_task_id is required for a scheduled_task_failure rule")
		}
		if r.RestartCountThreshold <= 0 {
			return alerting.Rule{}, errors.New("restart_count_threshold must be a positive integer for a scheduled_task_failure rule")
		}
	}

	return r, nil
}

// parseOptionalDuration parses raw as a Go duration string ("2m", "30s"),
// treating an empty string as zero (the same "omitted means zero value"
// contract parseStep (metrics.go) already applies to its own optional
// duration query param.
func parseOptionalDuration(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("must be a valid duration (e.g. \"60s\"): %w", err)
	}
	return d, nil
}

// handleCreateAlertRule handles POST /api/v1/apps/{name}/alerts
// (TASKS.md 2.5). resource_id is always resourceIDForApp(name),
// regardless of what the request body carries: an app's own URL is the
// only thing that gets to say which resource a rule it creates through
// that URL is scoped to.
func (rt *Router) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create alert rule: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req ruleResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.ResourceID = resourceIDForApp(name) // caller-supplied value, if any, is discarded

	id, err := alerting.NewRuleID()
	if err != nil {
		rt.logger.Error("api: create alert rule: generate id failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rule, err := req.toRule(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// A scheduled_task_id must actually belong to this app, the same
	// ownership check loadOwnedScheduledTask (scheduled_tasks.go) applies
	// when a caller reaches a task through its own app URL: without it, a
	// caller who can guess another app's task ID could point a rule
	// created through this app's URL at that task instead.
	if rule.ScheduledTaskID != "" {
		task, err := rt.scheduledTasks.GetScheduledTask(r.Context(), rule.ScheduledTaskID)
		if errors.Is(err, store.ErrScheduledTaskNotFound) || (err == nil && task.ServiceName != name) {
			writeError(w, http.StatusBadRequest, "unknown scheduled_task_id")
			return
		} else if err != nil {
			rt.logger.Error("api: create alert rule: look up scheduled task failed", slog.String("error", err.Error()), slog.String("scheduled_task_id", rule.ScheduledTaskID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	// Validated against the real registry first, same as
	// handleCreateDeployNotifyTarget does for its own channel_id.
	if rule.ChannelID != "" {
		if rt.notificationChannels == nil {
			writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
			return
		}
		if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), rule.ChannelID); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
			writeError(w, http.StatusBadRequest, "unknown channel_id")
			return
		} else if err != nil {
			rt.logger.Error("api: create alert rule: look up channel failed", slog.String("error", err.Error()), slog.String("channel_id", rule.ChannelID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.alertRules.SaveRule(r.Context(), rule); err != nil {
		rt.logger.Error("api: create alert rule failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Re-fetched so the response carries resolved notify_url/notify_kind,
	// which rule itself doesn't have until read back through the join.
	saved, err := rt.alertRules.GetRule(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: create alert rule: reload after save failed", slog.String("error", err.Error()), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toRuleResource(*saved))
}

// handleListAlertRules handles GET /api/v1/apps/{name}/alerts
// (TASKS.md 2.5): every rule scoped to this app's resource, including
// disabled ones, so the UI can show and let an operator re-enable a
// paused rule.
func (rt *Router) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list alert rules: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rules, err := rt.alertRules.ListRulesForResource(r.Context(), resourceIDForApp(name))
	if err != nil {
		rt.logger.Error("api: list alert rules failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]ruleResource, 0, len(rules))
	for _, rl := range rules {
		out = append(out, toRuleResource(rl))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteAlertRule handles DELETE /api/v1/apps/{name}/alerts/{id}
// (TASKS.md 2.5). A rule ID is globally unique and never itself encodes
// which app it belongs to, but the URL implies app-scoping, so this
// verifies the rule's own ResourceID actually matches
// resourceIDForApp(name) before deleting it: without that check, a
// caller who can guess or enumerate another app's rule ID could delete
// it through an app they do have write access to. Returns 404 either
// way (rule doesn't exist, or belongs to a different app), the same
// information-hiding reasoning handleGetApp's own 404 already applies:
// a caller with write access to app A learns nothing about whether some
// ID belongs to app B.
func (rt *Router) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	id := r.PathValue("id")

	rule, err := rt.alertRules.GetRule(r.Context(), id)
	if errors.Is(err, alerting.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete alert rule: load rule failed", slog.String("error", err.Error()), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rule.ResourceID != resourceIDForApp(name) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}

	if err := rt.alertRules.DeleteRule(r.Context(), id); err != nil {
		rt.logger.Error("api: delete alert rule failed", slog.String("error", err.Error()), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
