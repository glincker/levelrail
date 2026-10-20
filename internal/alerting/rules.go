package alerting

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Kind distinguishes what a Rule evaluates.
type Kind string

// Kinds other than threshold and crashloop ignore the threshold fields and watch platform-wide or backup-schedule resources.
// For most of them ResourceID is only a display label.
const (
	KindThreshold            Kind = "threshold"
	KindCrashloop            Kind = "crashloop"
	KindCertExpiry           Kind = "cert_expiry"
	KindPatchStatus          Kind = "patch_status"
	KindScheduledTaskFailure Kind = "scheduled_task_failure"
	// KindScheduledTaskMissed and KindScheduledTaskRunningLong watch one
	// job's schedule health: skipped occurrences and a run in flight past
	// its warning threshold (scheduled_task_health.go).
	KindScheduledTaskMissed      Kind = "scheduled_task_missed"
	KindScheduledTaskRunningLong Kind = "scheduled_task_running_long"
	KindNodeDiskSpace            Kind = "node_disk_space"
	// KindNodeHostGuardrail is registered for rules that watch per-node disk, inode, memory and swap guardrails.
	KindNodeHostGuardrail Kind = "node_host_guardrail"
	KindNodeResourceUsage Kind = "node_resource_usage"
	KindDomainHealth      Kind = "domain_health"
	KindBackupMissing     Kind = "backup_missing"

	// KindControlPlaneBackupStale is platform-wide: it watches the newest control plane snapshot's age.
	KindControlPlaneBackupStale Kind = "control_plane_backup_stale"
	KindNodeOffline             Kind = "node_offline"

	// KindNodeCertExpiring is platform-wide: it fires while any node's agent
	// certificate is close to expiry or expired (node_cert_expiring.go).
	KindNodeCertExpiring Kind = "node_cert_expiring"

	// KindLogArchiveStale is platform-wide: it fires when a log archive policy fails or stops succeeding.
	KindLogArchiveStale Kind = "log_archive_stale"
	// KindExportBehind is platform-wide: it fires when an export destination (audit or log sink) keeps failing.
	KindExportBehind Kind = "export_behind"
	// KindRestoreDrillFailed is platform-wide: it fires while a resource's latest restore drill is failed.
	KindRestoreDrillFailed Kind = "restore_drill_failed"

	// KindVersionSkew is platform-wide: it fires while the running build
	// is behind the configured update channel's latest release
	// (update_settings.channel, migrations/0258_update_settings.sql).
	KindVersionSkew Kind = "version_skew"

	// KindBudgetThreshold is platform-wide: it fires while any project or
	// team budget is at its warn level, over its limit, or forecast to
	// overrun (budget_threshold.go).
	KindBudgetThreshold Kind = "budget_threshold"

	// KindEmailDeliveryFailing is platform-wide: it fires while most recent
	// outbound emails are failing (email_delivery_failing.go).
	KindEmailDeliveryFailing Kind = "email_delivery_failing"
)

// Comparator is how a threshold Rule compares the latest sample value
// against its Threshold.
type Comparator string

// The four comparators a threshold Rule can use.
const (
	GreaterThan    Comparator = ">"
	LessThan       Comparator = "<"
	GreaterOrEqual Comparator = ">="
	LessOrEqual    Comparator = "<="
)

// NotifyKind selects the notification payload shape (notify.go).
type NotifyKind string

// NotifyKind payload shapes. Kinds whose NotifyURL is not a webhook URL (email, PagerDuty, Resend, Opsgenie, Webpush) are documented in notify.go.
const (
	NotifyGeneric    NotifyKind = "generic"
	NotifySlack      NotifyKind = "slack"
	NotifyDiscord    NotifyKind = "discord"
	NotifyTelegram   NotifyKind = "telegram"
	NotifyEmail      NotifyKind = "email"
	NotifyPushover   NotifyKind = "pushover"
	NotifyPagerDuty  NotifyKind = "pagerduty"
	NotifyTeams      NotifyKind = "teams"
	NotifyResend     NotifyKind = "resend"
	NotifyNtfy       NotifyKind = "ntfy"
	NotifyGotify     NotifyKind = "gotify"
	NotifyMattermost NotifyKind = "mattermost"
	NotifyLark       NotifyKind = "lark"
	NotifyRocketChat NotifyKind = "rocketchat"
	NotifyOpsgenie   NotifyKind = "opsgenie"
	NotifyWebex      NotifyKind = "webex"
	NotifyGoogleChat NotifyKind = "googlechat"
	NotifyWebpush    NotifyKind = "webpush"
)

// Rule is one alert rule, either a threshold check or a crashloop check, plus its current evaluation state.
type Rule struct {
	ID         string
	Name       string
	Kind       Kind
	ResourceID string

	// Threshold-kind fields.
	Metric      string
	Comparator  Comparator
	Threshold   float64
	ForDuration time.Duration

	// RestartCountThreshold also means consecutive failed runs for KindScheduledTaskFailure, where RestartWindow is unused.
	RestartCountThreshold int
	RestartWindow         time.Duration

	// ScheduledTaskID is the KindScheduledTaskFailure-only field: which
	// of ResourceID's app's scheduled tasks this rule watches. Empty for
	// every other kind.
	ScheduledTaskID string

	// Backup* fields pick the schedule a KindBackupMissing rule watches. ForDuration is the grace period past the expected interval.
	BackupResourceKind string
	BackupDatabaseName string
	BackupServiceName  string
	BackupVolumeName   string

	// ChannelID attaches an already-connected NotificationChannel; empty
	// for legacy rules, which use NotifyURL/NotifyKind below directly.
	// SLO is the KindSLOBurn-only request-based SLO; nil for every other kind.
	SLO *SLOConfig
	// LogRule is the KindLogPattern/KindLogLevelRate-only match config; nil for every other kind.
	LogRule *LogRuleConfig

	ChannelID  string
	NotifyURL  string
	NotifyKind NotifyKind

	// Enabled reflects both this rule's own flag and, once resolved by a
	// read (GetRule/ListRules*), its attached channel's: a disabled
	// channel silences the rule too.
	Enabled bool

	// Noise control: Severity and Labels feed silence matchers;
	// ConsecutiveFailures, FlapThreshold and FlapWindow override the
	// control plane defaults when non-zero.
	Severity            string
	Labels              map[string]string
	ConsecutiveFailures int
	FlapThreshold       int
	FlapWindow          time.Duration

	// Evaluation state, read-only from a caller's perspective: only the
	// evaluator (evaluate.go, crashloop.go) writes these, via
	// UpdateState below.
	PendingSince    *time.Time
	Firing          bool
	FiringSince     *time.Time
	LastEvaluatedAt *time.Time
	LastValue       *float64
}

// ErrRuleNotFound is returned by GetRule when no rule has that ID.
var ErrRuleNotFound = errors.New("alerting: rule not found")

// NewRuleID returns a random URL-safe rule ID, so callers never choose one and cannot collide or predict it.
func NewRuleID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate rule id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SaveRule creates or replaces a rule's configuration. It never touches evaluation state, which only UpdateState writes.
func (db *DB) SaveRule(ctx context.Context, r Rule) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, name, kind, resource_id,
			metric, comparator, threshold, for_duration_seconds,
			restart_count_threshold, restart_window_seconds, scheduled_task_id,
			backup_resource_kind, backup_database_name, backup_service_name, backup_volume_name,
			channel_id, notify_url, notify_kind, enabled,
			severity, labels_json, consecutive_failures, flap_threshold, flap_window_seconds,
			slo_json, log_rule_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			kind = excluded.kind,
			resource_id = excluded.resource_id,
			metric = excluded.metric,
			comparator = excluded.comparator,
			threshold = excluded.threshold,
			for_duration_seconds = excluded.for_duration_seconds,
			restart_count_threshold = excluded.restart_count_threshold,
			restart_window_seconds = excluded.restart_window_seconds,
			scheduled_task_id = excluded.scheduled_task_id,
			backup_resource_kind = excluded.backup_resource_kind,
			backup_database_name = excluded.backup_database_name,
			backup_service_name = excluded.backup_service_name,
			backup_volume_name = excluded.backup_volume_name,
			channel_id = excluded.channel_id,
			notify_url = excluded.notify_url,
			notify_kind = excluded.notify_kind,
			enabled = excluded.enabled,
			severity = excluded.severity,
			labels_json = excluded.labels_json,
			consecutive_failures = excluded.consecutive_failures,
			flap_threshold = excluded.flap_threshold,
			flap_window_seconds = excluded.flap_window_seconds,
			slo_json = excluded.slo_json,
			log_rule_json = excluded.log_rule_json,
			updated_at = excluded.updated_at
	`,
		r.ID, r.Name, string(r.Kind), r.ResourceID,
		r.Metric, string(r.Comparator), r.Threshold, int64(r.ForDuration.Seconds()),
		r.RestartCountThreshold, int64(r.RestartWindow.Seconds()), r.ScheduledTaskID,
		r.BackupResourceKind, r.BackupDatabaseName, r.BackupServiceName, r.BackupVolumeName,
		nullIfEmpty(r.ChannelID), r.NotifyURL, string(r.NotifyKind), boolToInt(r.Enabled),
		severityOrDefault(r.Severity), encodeLabels(r.Labels), r.ConsecutiveFailures, r.FlapThreshold, int64(r.FlapWindow.Seconds()),
		encodeSLO(r.SLO), encodeLogRule(r.LogRule), now, now,
	)
	if err != nil {
		return fmt.Errorf("alerting: save rule %q: %w", r.ID, err)
	}
	return nil
}

// GetRule returns the rule with this ID, or ErrRuleNotFound.
func (db *DB) GetRule(ctx context.Context, id string) (*Rule, error) {
	row := db.QueryRowContext(ctx, ruleSelectColumns+` WHERE r.id = ?`, id)
	r, err := scanRule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get rule %q: %w", id, err)
	}
	return r, nil
}

// ListRules returns every rule, ordered by name.
func (db *DB) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` ORDER BY r.name`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan rule row: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate rule rows: %w", err)
	}
	return out, nil
}

// ListEnabledRules returns only enabled rules; disabled ones are skipped before any metrics query.
func (db *DB) ListEnabledRules(ctx context.Context) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` WHERE r.enabled = 1 ORDER BY r.name`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list enabled rules: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan rule row: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate rule rows: %w", err)
	}
	return out, nil
}

// ListRulesForResource returns every rule scoped to resourceID, enabled or not, ordered by name.
func (db *DB) ListRulesForResource(ctx context.Context, resourceID string) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` WHERE r.resource_id = ? ORDER BY r.name`, resourceID)
	if err != nil {
		return nil, fmt.Errorf("alerting: list rules for resource %q: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan rule row: %w", err)
		}
		out = append(out, *r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate rule rows: %w", err)
	}
	return out, nil
}

// DeleteRule removes a rule by ID. Deleting a rule that doesn't exist is
// not an error, matching internal/store.DeleteDesiredService's own
// idempotent-delete convention.
func (db *DB) DeleteRule(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("alerting: delete rule %q: %w", id, err)
	}
	return nil
}

// UpdateState persists a rule's new evaluation state after one
// evaluator pass. The only method that writes Pending/Firing/
// LastEvaluated/LastValue; SaveRule above never touches them.
func (db *DB) UpdateState(ctx context.Context, id string, pendingSince, firingSince *time.Time, firing bool, evaluatedAt time.Time, value *float64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE alert_rules SET
			pending_since = ?,
			firing = ?,
			firing_since = ?,
			last_evaluated_at = ?,
			last_value = ?,
			updated_at = ?
		WHERE id = ?
	`,
		formatNullableTime(pendingSince), boolToInt(firing), formatNullableTime(firingSince),
		evaluatedAt.UTC().Format(time.RFC3339Nano), value, time.Now().UTC().Format(time.RFC3339Nano),
		id,
	)
	if err != nil {
		return fmt.Errorf("alerting: update state for rule %q: %w", id, err)
	}
	return nil
}

// Effective notify_url and notify_kind come from the attached channel when set, else from the row's own columns (legacy rows or a deleted channel).
const ruleSelectColumns = `
	SELECT r.id, r.name, r.kind, r.resource_id,
		r.metric, r.comparator, r.threshold, r.for_duration_seconds,
		r.restart_count_threshold, r.restart_window_seconds, r.scheduled_task_id,
		r.backup_resource_kind, r.backup_database_name, r.backup_service_name, r.backup_volume_name,
		r.channel_id, COALESCE(c.notify_url, r.notify_url), COALESCE(c.kind, r.notify_kind),
		r.enabled, c.enabled,
		r.severity, r.labels_json, r.consecutive_failures, r.flap_threshold, r.flap_window_seconds,
		r.slo_json, r.log_rule_json,
		r.pending_since, r.firing, r.firing_since, r.last_evaluated_at, r.last_value
	FROM alert_rules r
	LEFT JOIN notification_channels c ON c.id = r.channel_id`

func scanRule(scan func(dest ...any) error) (*Rule, error) {
	var (
		r                            Rule
		kind, comparator, notifyKind string
		forDurationSeconds           int64
		restartWindowSeconds         int64
		channelID                    sql.NullString
		enabledInt, firingInt        int
		channelEnabled               sql.NullInt64
		pendingSince, firingSince    sql.NullString
		lastEvaluatedAt              sql.NullString
		lastValue                    sql.NullFloat64
		labelsJSON                   string
		flapWindowSeconds            int64
		sloJSON, logRuleJSON         string
	)
	err := scan(
		&r.ID, &r.Name, &kind, &r.ResourceID,
		&r.Metric, &comparator, &r.Threshold, &forDurationSeconds,
		&r.RestartCountThreshold, &restartWindowSeconds, &r.ScheduledTaskID,
		&r.BackupResourceKind, &r.BackupDatabaseName, &r.BackupServiceName, &r.BackupVolumeName,
		&channelID, &r.NotifyURL, &notifyKind,
		&enabledInt, &channelEnabled,
		&r.Severity, &labelsJSON, &r.ConsecutiveFailures, &r.FlapThreshold, &flapWindowSeconds,
		&sloJSON, &logRuleJSON,
		&pendingSince, &firingInt, &firingSince, &lastEvaluatedAt, &lastValue,
	)
	if err != nil {
		return nil, err
	}

	r.Kind = Kind(kind)
	r.Comparator = Comparator(comparator)
	r.NotifyKind = NotifyKind(notifyKind)
	r.ChannelID = channelID.String
	r.ForDuration = time.Duration(forDurationSeconds) * time.Second
	r.RestartWindow = time.Duration(restartWindowSeconds) * time.Second
	// A disabled channel silences every rule attached to it too, not
	// just ones explicitly disabled themselves.
	r.Enabled = enabledInt != 0 && (!channelEnabled.Valid || channelEnabled.Int64 != 0)
	r.Firing = firingInt != 0
	r.Labels = decodeLabels(labelsJSON)
	r.FlapWindow = time.Duration(flapWindowSeconds) * time.Second
	r.SLO = decodeSLO(sloJSON)
	r.LogRule = decodeLogRule(logRuleJSON)

	pendingT, err := parseNullableTime(pendingSince)
	if err != nil {
		return nil, fmt.Errorf("parse pending_since: %w", err)
	}
	r.PendingSince = pendingT

	firingT, err := parseNullableTime(firingSince)
	if err != nil {
		return nil, fmt.Errorf("parse firing_since: %w", err)
	}
	r.FiringSince = firingT

	lastEvalT, err := parseNullableTime(lastEvaluatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_evaluated_at: %w", err)
	}
	r.LastEvaluatedAt = lastEvalT
	if lastValue.Valid {
		v := lastValue.Float64
		r.LastValue = &v
	}

	return &r, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func formatNullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseNullableTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
