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

// The two rule kinds this package evaluates; see Rule's own doc comment
// for which fields each uses.
const (
	KindThreshold Kind = "threshold"
	KindCrashloop Kind = "crashloop"
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

// The three payload shapes NewNotifier knows how to build; an unknown
// or empty NotifyKind falls back to NotifyGeneric.
const (
	NotifyGeneric NotifyKind = "generic"
	NotifySlack   NotifyKind = "slack"
	NotifyDiscord NotifyKind = "discord"
)

// Rule is one alert rule: either a threshold check (Kind ==
// KindThreshold, using Metric/Comparator/Threshold/ForDuration) or a
// crashloop check (Kind == KindCrashloop, using
// RestartCountThreshold/RestartWindow), plus its own current
// evaluation state. See migrations/0001_alert_rules.sql for why the two
// kinds share one table instead of two.
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

	// Crashloop-kind fields.
	RestartCountThreshold int
	RestartWindow         time.Duration

	NotifyURL  string
	NotifyKind NotifyKind

	Enabled bool

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

// NewRuleID generates a random, URL-safe rule identifier. Exported so
// internal/api can assign one when creating a rule from a request body
// that doesn't include one, matching how a rule's ID is never
// caller-chosen data (avoids a client picking a colliding or
// predictable ID).
func NewRuleID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate rule id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SaveRule creates or fully replaces a rule's configuration. Does not
// touch evaluation state (Firing/FiringSince/etc.): those are only ever
// written by UpdateState, so an operator editing a rule's threshold
// doesn't accidentally reset its current firing status.
func (db *DB) SaveRule(ctx context.Context, r Rule) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO alert_rules (
			id, name, kind, resource_id,
			metric, comparator, threshold, for_duration_seconds,
			restart_count_threshold, restart_window_seconds,
			notify_url, notify_kind, enabled,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			notify_url = excluded.notify_url,
			notify_kind = excluded.notify_kind,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`,
		r.ID, r.Name, string(r.Kind), r.ResourceID,
		r.Metric, string(r.Comparator), r.Threshold, int64(r.ForDuration.Seconds()),
		r.RestartCountThreshold, int64(r.RestartWindow.Seconds()),
		r.NotifyURL, string(r.NotifyKind), boolToInt(r.Enabled),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("alerting: save rule %q: %w", r.ID, err)
	}
	return nil
}

// GetRule returns the rule with this ID, or ErrRuleNotFound.
func (db *DB) GetRule(ctx context.Context, id string) (*Rule, error) {
	row := db.QueryRowContext(ctx, ruleSelectColumns+` FROM alert_rules WHERE id = ?`, id)
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
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` FROM alert_rules ORDER BY name`)
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

// ListEnabledRules is what the evaluator (evaluate.go, crashloop.go)
// actually loops over each tick: a disabled rule is skipped entirely,
// not evaluated-but-not-notified, so a paused rule doesn't even pay for
// a metrics query.
func (db *DB) ListEnabledRules(ctx context.Context) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` FROM alert_rules WHERE enabled = 1 ORDER BY name`)
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

// ListRulesForResource returns every rule scoped to resourceID, ordered
// by name, regardless of enabled state. internal/api's alert-rule
// handlers (TASKS.md 2.5) use this to list only one app's own rules
// rather than every rule in the database; alert_rules already carries
// idx_alert_rules_resource (migrations/0001_alert_rules.sql) for
// exactly this lookup.
func (db *DB) ListRulesForResource(ctx context.Context, resourceID string) ([]Rule, error) {
	rows, err := db.QueryContext(ctx, ruleSelectColumns+` FROM alert_rules WHERE resource_id = ? ORDER BY name`, resourceID)
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

const ruleSelectColumns = `
	SELECT id, name, kind, resource_id,
		metric, comparator, threshold, for_duration_seconds,
		restart_count_threshold, restart_window_seconds,
		notify_url, notify_kind, enabled,
		pending_since, firing, firing_since, last_evaluated_at, last_value`

func scanRule(scan func(dest ...any) error) (*Rule, error) {
	var (
		r                            Rule
		kind, comparator, notifyKind string
		forDurationSeconds           int64
		restartWindowSeconds         int64
		enabledInt, firingInt        int
		pendingSince, firingSince    sql.NullString
		lastEvaluatedAt              sql.NullString
		lastValue                    sql.NullFloat64
	)
	err := scan(
		&r.ID, &r.Name, &kind, &r.ResourceID,
		&r.Metric, &comparator, &r.Threshold, &forDurationSeconds,
		&r.RestartCountThreshold, &restartWindowSeconds,
		&r.NotifyURL, &notifyKind, &enabledInt,
		&pendingSince, &firingInt, &firingSince, &lastEvaluatedAt, &lastValue,
	)
	if err != nil {
		return nil, err
	}

	r.Kind = Kind(kind)
	r.Comparator = Comparator(comparator)
	r.NotifyKind = NotifyKind(notifyKind)
	r.ForDuration = time.Duration(forDurationSeconds) * time.Second
	r.RestartWindow = time.Duration(restartWindowSeconds) * time.Second
	r.Enabled = enabledInt != 0
	r.Firing = firingInt != 0

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
