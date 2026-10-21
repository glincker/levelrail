package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AlertRules is the narrow surface the alert-rule handlers need from alerting.DB.
type AlertRules interface {
	SaveRule(ctx context.Context, r alerting.Rule) error
	GetRule(ctx context.Context, id string) (*alerting.Rule, error)
	ListRulesForResource(ctx context.Context, resourceID string) ([]alerting.Rule, error)
	DeleteRule(ctx context.Context, id string) error
}

// ruleResource is the wire shape for an alert rule, with durations as strings such as 2m.
// ID and ResourceID are server-assigned; ResourceID always comes from the app name in the URL, never the body.
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

	// KindBackupMissing-only fields: which database or service volume the
	// rule watches. ForDuration above doubles as its overdue grace period;
	// see alerting.Rule's own doc comment.
	BackupResourceKind string `json:"backup_resource_kind,omitempty"`
	BackupDatabaseName string `json:"backup_database_name,omitempty"`
	BackupServiceName  string `json:"backup_service_name,omitempty"`
	BackupVolumeName   string `json:"backup_volume_name,omitempty"`

	// KindSLOBurn-only field: the request-based SLO the rule watches.
	SLO *alerting.SLOConfig `json:"slo,omitempty"`

	// KindLogPattern and KindLogLevelRate only: what the rule matches and how much.
	LogRule *alerting.LogRuleConfig `json:"log_rule,omitempty"`

	NotifyURL  string `json:"notify_url,omitempty"`
	NotifyKind string `json:"notify_kind,omitempty"`
	Enabled    bool   `json:"enabled"`

	// Response-only on create and update: problems that do not block the
	// save but mean the rule will not behave as the caller probably expects.
	Warnings []string `json:"warnings,omitempty"`

	// Noise control. Severity and Labels feed silence matchers;
	// ConsecutiveFailures, FlapThreshold and FlapWindow override the
	// control plane defaults (zero keeps the default).
	Severity            string            `json:"severity,omitempty"`
	Labels              map[string]string `json:"labels,omitempty"`
	ConsecutiveFailures int               `json:"consecutive_failures,omitempty"`
	FlapThreshold       int               `json:"flap_threshold,omitempty"`
	FlapWindow          string            `json:"flap_window,omitempty"`

	// Response-only: set when an active silence or maintenance window
	// currently mutes this rule.
	Silenced   bool   `json:"silenced,omitempty"`
	SilencedBy string `json:"silenced_by,omitempty"`

	// Evaluation state is response-only: toRule ignores it, and only the evaluator writes it.
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
		BackupResourceKind:    r.BackupResourceKind,
		BackupDatabaseName:    r.BackupDatabaseName,
		BackupServiceName:     r.BackupServiceName,
		BackupVolumeName:      r.BackupVolumeName,
		SLO:                   r.SLO,
		LogRule:               r.LogRule,
		NotifyURL:             r.NotifyURL,
		NotifyKind:            string(r.NotifyKind),
		Enabled:               r.Enabled,
		Severity:              r.Severity,
		Labels:                r.Labels,
		ConsecutiveFailures:   r.ConsecutiveFailures,
		FlapThreshold:         r.FlapThreshold,
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
	if r.FlapWindow > 0 {
		out.FlapWindow = r.FlapWindow.String()
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
	case alerting.KindThreshold, alerting.KindBudgetThreshold, alerting.KindCrashloop, alerting.KindCertExpiry, alerting.KindPatchStatus, alerting.KindScheduledTaskFailure, alerting.KindScheduledTaskMissed, alerting.KindScheduledTaskRunningLong, alerting.KindNodeDiskSpace, alerting.KindNodeHostGuardrail, alerting.KindNodeResourceUsage, alerting.KindDomainHealth, alerting.KindBackupMissing, alerting.KindDatabaseConnections, alerting.KindDatabaseDisk, alerting.KindDatabaseBackupFailing, alerting.KindDatabaseDown, alerting.KindDatabaseBackupStale, alerting.KindNodeOffline, alerting.KindNodeCertExpiring, alerting.KindControlPlaneBackupStale, alerting.KindLogArchiveStale, alerting.KindExportBehind, alerting.KindRestoreDrillFailed, alerting.KindSLOBurn, alerting.KindVersionSkew, alerting.KindEmailDeliveryFailing, alerting.KindBackupFailed, alerting.KindDeployFailed,
		alerting.KindLogPattern, alerting.KindLogLevelRate,
		alerting.KindCertExpiring, alerting.KindCertRenewalStalled, alerting.KindDomainNotResolving:
	default:
		return alerting.Rule{}, fmt.Errorf("kind %q is not a supported rule kind", a.Kind)
	}

	forDuration, err := parseOptionalDuration(a.ForDuration)
	if err != nil {
		return alerting.Rule{}, fmt.Errorf("for_duration: %w", err)
	}
	restartWindow, err := parseOptionalDuration(a.RestartWindow)
	if err != nil {
		return alerting.Rule{}, fmt.Errorf("restart_window: %w", err)
	}

	flapWindow, err := parseOptionalDuration(a.FlapWindow)
	if err != nil {
		return alerting.Rule{}, fmt.Errorf("flap_window: %w", err)
	}
	if a.Severity != "" && a.Severity != alerting.SeverityInfo && a.Severity != alerting.SeverityWarning && a.Severity != alerting.SeverityCritical {
		return alerting.Rule{}, fmt.Errorf("severity must be %q, %q or %q", alerting.SeverityInfo, alerting.SeverityWarning, alerting.SeverityCritical)
	}
	if a.ConsecutiveFailures < 0 || a.FlapThreshold < 0 {
		return alerting.Rule{}, errors.New("consecutive_failures and flap_threshold must not be negative")
	}

	r := alerting.Rule{
		Severity:              a.Severity,
		Labels:                a.Labels,
		ConsecutiveFailures:   a.ConsecutiveFailures,
		FlapThreshold:         a.FlapThreshold,
		FlapWindow:            flapWindow,
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
		BackupResourceKind:    a.BackupResourceKind,
		BackupDatabaseName:    a.BackupDatabaseName,
		BackupServiceName:     a.BackupServiceName,
		BackupVolumeName:      a.BackupVolumeName,
		ChannelID:             a.ChannelID,
		SLO:                   a.SLO,
		LogRule:               a.LogRule,
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
	case alerting.KindScheduledTaskMissed, alerting.KindScheduledTaskRunningLong:
		if r.ScheduledTaskID == "" {
			return alerting.Rule{}, fmt.Errorf("scheduled_task_id is required for a %s rule", r.Kind)
		}
	case alerting.KindCertExpiring:
		if r.Threshold < 0 {
			return alerting.Rule{}, errors.New("threshold (days) must not be negative for a cert_expiring rule")
		}
	case alerting.KindSLOBurn:
		if r.SLO == nil {
			return alerting.Rule{}, errors.New("slo is required for an slo_burn rule")
		}
		if err := r.SLO.Validate(); err != nil {
			return alerting.Rule{}, fmt.Errorf("slo: %w", err)
		}
	case alerting.KindLogPattern, alerting.KindLogLevelRate:
		if r.LogRule == nil {
			return alerting.Rule{}, fmt.Errorf("log_rule is required for a %s rule", r.Kind)
		}
		if err := r.LogRule.Validate(r.Kind, alerting.LogRulePolicyFromEnv()); err != nil {
			return alerting.Rule{}, fmt.Errorf("log_rule: %w", err)
		}
	case alerting.KindDatabaseConnections, alerting.KindDatabaseDisk, alerting.KindDatabaseBackupFailing, alerting.KindDatabaseDown, alerting.KindDatabaseBackupStale:
		if r.BackupDatabaseName == "" {
			return alerting.Rule{}, fmt.Errorf("backup_database_name is required for a %s rule", r.Kind)
		}
		if r.Kind == alerting.KindDatabaseBackupStale {
			if r.Threshold < 0 || r.Threshold > 8760 {
				return alerting.Rule{}, errors.New("threshold is an age in hours between 0 and 8760")
			}
		} else if r.Threshold < 0 || r.Threshold > 100 {
			return alerting.Rule{}, errors.New("threshold is a percentage between 0 and 100")
		}
		r.BackupResourceKind = store.BackupResourceKindDatabase
	case alerting.KindBackupMissing:
		switch r.BackupResourceKind {
		case store.BackupResourceKindDatabase:
			if r.BackupDatabaseName == "" {
				return alerting.Rule{}, errors.New("backup_database_name is required for a backup_missing rule watching a database")
			}
		case store.BackupResourceKindVolume:
			if r.BackupServiceName == "" || r.BackupVolumeName == "" {
				return alerting.Rule{}, errors.New("backup_service_name and backup_volume_name are required for a backup_missing rule watching a service volume")
			}
		default:
			return alerting.Rule{}, fmt.Errorf("backup_resource_kind must be %q or %q", store.BackupResourceKindDatabase, store.BackupResourceKindVolume)
		}
	}

	return r, nil
}

// parseOptionalDuration parses a Go duration string, treating an empty string as zero.
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

// validateAlertRuleReferences checks that scheduled_task_id belongs to appName and channel_id names a saved channel.
// It writes the error response and returns false on failure.
func (rt *Router) validateAlertRuleReferences(w http.ResponseWriter, r *http.Request, appName, op string, rule alerting.Rule) bool {
	if rule.ScheduledTaskID != "" {
		task, err := rt.scheduledTasks.GetScheduledTask(r.Context(), rule.ScheduledTaskID)
		if errors.Is(err, store.ErrScheduledTaskNotFound) || (err == nil && task.ServiceName != appName) {
			writeError(w, http.StatusBadRequest, "unknown scheduled_task_id")
			return false
		} else if err != nil {
			rt.logger.Error("api: "+op+": look up scheduled task failed", slog.String("error", err.Error()), slog.String("scheduled_task_id", rule.ScheduledTaskID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}

	if alerting.IsDatabaseKind(rule.Kind) {
		if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown backup_database_name"), databaseRef(rule.BackupDatabaseName)) {
			return false
		}
		if _, err := rt.databases.GetDesiredDatabase(r.Context(), rule.BackupDatabaseName); errors.Is(err, store.ErrDatabaseNotFound) {
			writeError(w, http.StatusBadRequest, "unknown backup_database_name")
			return false
		} else if err != nil {
			rt.logger.Error("api: "+op+": look up database failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}

	if rule.Kind == alerting.KindBackupMissing {
		switch rule.BackupResourceKind {
		case store.BackupResourceKindDatabase:
			if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown backup_database_name"), databaseRef(rule.BackupDatabaseName)) {
				return false
			}
			if _, err := rt.databases.GetDesiredDatabase(r.Context(), rule.BackupDatabaseName); errors.Is(err, store.ErrDatabaseNotFound) {
				writeError(w, http.StatusBadRequest, "unknown backup_database_name")
				return false
			} else if err != nil {
				rt.logger.Error("api: "+op+": look up database failed", slog.String("error", err.Error()), slog.String("backup_database_name", rule.BackupDatabaseName))
				writeError(w, http.StatusInternalServerError, "internal error")
				return false
			}
		case store.BackupResourceKindVolume:
			// A volume's owning service is always this app, since backup endpoints derive it from the URL.
			if rule.BackupServiceName != appName {
				writeError(w, http.StatusBadRequest, "unknown backup_service_name/backup_volume_name")
				return false
			}
			if _, err := rt.serviceVolumeBackupSchedule.GetServiceVolumeBackupSchedule(r.Context(), rule.BackupServiceName, rule.BackupVolumeName); errors.Is(err, store.ErrServiceVolumeBackupNotFound) {
				writeError(w, http.StatusBadRequest, "unknown backup_service_name/backup_volume_name: no backup schedule has ever been configured for that volume")
				return false
			} else if err != nil {
				rt.logger.Error("api: "+op+": look up service volume backup schedule failed", slog.String("error", err.Error()),
					slog.String("backup_service_name", rule.BackupServiceName), slog.String("backup_volume_name", rule.BackupVolumeName))
				writeError(w, http.StatusInternalServerError, "internal error")
				return false
			}
		}
	}

	if rule.ChannelID != "" {
		if rt.notificationChannels == nil {
			writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
			return false
		}
		if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), rule.ChannelID); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
			writeError(w, http.StatusBadRequest, "unknown channel_id")
			return false
		} else if err != nil {
			rt.logger.Error("api: "+op+": look up channel failed", slog.String("error", err.Error()), slog.String("channel_id", rule.ChannelID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return false
		}
	}

	return true
}

// handleCreateAlertRule handles POST /api/v1/apps/{name}/alerts. resource_id is always resourceIDForApp(name), never taken from the body.
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
	if !decodeReq(w, r, &req) {
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

	if !rt.validateAlertRuleReferences(w, r, name, "create alert rule", rule) {
		return
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

	res := toRuleResource(*saved)
	res.Warnings = ruleWarnings(*saved)
	writeJSON(w, http.StatusCreated, res)
}

// alertRuleNoDestinationWarning is returned when a rule has nowhere to notify.
const alertRuleNoDestinationWarning = "no notification channel is set (channel_id or notify_url), so this rule will fire without notifying anyone"

// ruleWarnings lists non-blocking problems with a saved rule. There is no
// default channel to fall back to, so an unset destination is always silent.
func ruleWarnings(r alerting.Rule) []string {
	if r.ChannelID == "" && r.NotifyURL == "" {
		return []string{alertRuleNoDestinationWarning}
	}
	return nil
}

// handleUpdateAlertRule handles PUT /api/v1/apps/{name}/alerts/{id} as a full replace. A rule owned by another app returns 404.
func (rt *Router) handleUpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	id := r.PathValue("id")

	existing, err := rt.alertRules.GetRule(r.Context(), id)
	if errors.Is(err, alerting.ErrRuleNotFound) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update alert rule: load rule failed", slog.String("error", err.Error()), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing.ResourceID != resourceIDForApp(name) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}

	var req ruleResource
	if !decodeReq(w, r, &req) {
		return
	}
	req.ResourceID = resourceIDForApp(name) // caller-supplied value, if any, is discarded

	rule, err := req.toRule(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !rt.validateAlertRuleReferences(w, r, name, "update alert rule", rule) {
		return
	}

	if err := rt.alertRules.SaveRule(r.Context(), rule); err != nil {
		rt.logger.Error("api: update alert rule failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	saved, err := rt.alertRules.GetRule(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update alert rule: reload after save failed", slog.String("error", err.Error()), slog.String("rule_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, toRuleResource(*saved))
}

// handleListAlertRules lists every rule for this app, including disabled ones, so a paused rule can be re-enabled.
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
		res := toRuleResource(rl)
		res.NotifyURL = rt.notifyTargetFor(r, rl.NotifyKind, rl.NotifyURL)
		out = append(out, res)
	}
	rt.annotateSilenced(r.Context(), name, rules, out)
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteAlertRule handles DELETE /api/v1/apps/{name}/alerts/{id}. A rule owned by another app returns 404, so rule IDs cannot be probed across apps.
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
