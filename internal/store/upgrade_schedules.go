package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Upgrade schedule states. Pending, blocked and running are the active states.
const (
	UpgradeSchedulePending   = "pending"
	UpgradeScheduleBlocked   = "blocked"
	UpgradeScheduleRunning   = "running"
	UpgradeScheduleApplied   = "applied"
	UpgradeScheduleFailed    = "failed"
	UpgradeScheduleCancelled = "cancelled"
	UpgradeScheduleMissed    = "missed"
)

// Upgrade schedule sources and modes.
const (
	UpgradeSourceManual = "manual"
	UpgradeSourceAuto   = "auto"
	UpgradeModeAt       = "at"
	UpgradeModeWindow   = "window"
)

// UpgradeSchedule is one scheduled control plane upgrade (migrations/0475).
// Times are RFC3339 UTC strings so they sort lexicographically.
type UpgradeSchedule struct {
	ID            string
	TargetVersion string
	Source        string
	Mode          string
	RunAt         string
	RetryUntil    string
	Timezone      string
	State         string
	AckedJSON     string
	MajorAcked    bool
	CreatedBy     string
	CreatedAt     string
	UpdatedAt     string
	CheckCount    int
	LastCheckAt   string
	LastReason    string
	Initiator     string
	FiredAt       string
	FinishedAt    string
	// NotifiedState is the last state an operator notice was sent for, so a
	// retrying schedule alerts once per transition, not once per tick.
	NotifiedState string
}

// IsActive reports whether the schedule still has work to do.
func (s UpgradeSchedule) IsActive() bool {
	switch s.State {
	case UpgradeSchedulePending, UpgradeScheduleBlocked, UpgradeScheduleRunning:
		return true
	}
	return false
}

// ErrUpgradeScheduleActive is returned when another schedule is still active.
var ErrUpgradeScheduleActive = errors.New("store: an upgrade is already scheduled")

const upgradeScheduleColumns = `id, target_version, source, mode, run_at, retry_until, timezone, state,
	acked_json, major_acked, created_by, created_at, updated_at, check_count, last_check_at, last_reason,
	initiator, fired_at, finished_at, notified_state`

func scanUpgradeSchedule(row interface{ Scan(dest ...any) error }) (UpgradeSchedule, error) {
	var s UpgradeSchedule
	var major int
	err := row.Scan(&s.ID, &s.TargetVersion, &s.Source, &s.Mode, &s.RunAt, &s.RetryUntil, &s.Timezone, &s.State,
		&s.AckedJSON, &major, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt, &s.CheckCount, &s.LastCheckAt, &s.LastReason,
		&s.Initiator, &s.FiredAt, &s.FinishedAt, &s.NotifiedState)
	s.MajorAcked = major != 0
	return s, err
}

// InsertUpgradeSchedule stores s as pending unless another schedule is
// already active, in one statement so two callers cannot both win.
func (db *DB) InsertUpgradeSchedule(ctx context.Context, s UpgradeSchedule) error {
	if s.AckedJSON == "" {
		s.AckedJSON = "[]"
	}
	if s.Timezone == "" {
		s.Timezone = "UTC"
	}
	major := 0
	if s.MajorAcked {
		major = 1
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO upgrade_schedules (`+upgradeScheduleColumns+`)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', ?, '', '', ''
		WHERE NOT EXISTS (SELECT 1 FROM upgrade_schedules WHERE state IN ('pending', 'blocked', 'running'))`,
		s.ID, s.TargetVersion, s.Source, s.Mode, s.RunAt, s.RetryUntil, s.Timezone, UpgradeSchedulePending,
		s.AckedJSON, major, s.CreatedBy, s.CreatedAt, s.CreatedAt, s.Initiator)
	if err != nil {
		return fmt.Errorf("store upgrade schedule %s: %w", s.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store upgrade schedule %s: %w", s.ID, err)
	}
	if n == 0 {
		return ErrUpgradeScheduleActive
	}
	return nil
}

// ActiveUpgradeSchedule returns the pending, blocked or running schedule, or
// nil when there is none.
func (db *DB) ActiveUpgradeSchedule(ctx context.Context) (*UpgradeSchedule, error) {
	row := db.QueryRowContext(ctx, `SELECT `+upgradeScheduleColumns+` FROM upgrade_schedules
		WHERE state IN ('pending', 'blocked', 'running') ORDER BY created_at DESC LIMIT 1`)
	s, err := scanUpgradeSchedule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: active upgrade schedule: %w", err)
	}
	return &s, nil
}

// ListUpgradeSchedules returns the newest schedules first.
func (db *DB) ListUpgradeSchedules(ctx context.Context, limit int) ([]UpgradeSchedule, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.reader().QueryContext(ctx, `SELECT `+upgradeScheduleColumns+` FROM upgrade_schedules
		ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list upgrade schedules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []UpgradeSchedule
	for rows.Next() {
		s, err := scanUpgradeSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan upgrade schedule: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateUpgradeSchedule writes the mutable fields of s only while the row is
// still in one of from. It reports whether the row changed, which is how two
// racing writers (a cancel and a fire) resolve to exactly one winner.
func (db *DB) UpdateUpgradeSchedule(ctx context.Context, s UpgradeSchedule, from ...string) (bool, error) {
	if len(from) == 0 {
		return false, errors.New("store: update upgrade schedule needs a from state")
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(from)), ", ")
	args := []any{s.State, s.RunAt, s.RetryUntil, s.CheckCount, s.LastCheckAt, s.LastReason, s.Initiator,
		s.FiredAt, s.FinishedAt, s.NotifiedState, time.Now().UTC().Format(time.RFC3339), s.ID}
	for _, f := range from {
		args = append(args, f)
	}
	res, err := db.ExecContext(ctx, `UPDATE upgrade_schedules SET state = ?, run_at = ?, retry_until = ?,
		check_count = ?, last_check_at = ?, last_reason = ?, initiator = ?, fired_at = ?, finished_at = ?,
		notified_state = ?, updated_at = ? WHERE id = ? AND state IN (`+marks+`)`, args...)
	if err != nil {
		return false, fmt.Errorf("store: update upgrade schedule %s: %w", s.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update upgrade schedule %s: %w", s.ID, err)
	}
	return n > 0, nil
}

// UpgradePolicy is the automatic upgrade policy singleton (migrations/0476).
type UpgradePolicy struct {
	Mode                  string
	WindowCron            string
	WindowDurationSeconds int
	WindowTimezone        string
	MinReleaseAgeDays     int
	NotifyChannelIDs      string
	Paused                bool
	UpdatedBy             string
	UpdatedAt             string
	DecisionTag           string
	Decision              string
	DecisionAt            string
}

// GetUpgradePolicy returns the single policy row, always present.
func (db *DB) GetUpgradePolicy(ctx context.Context) (UpgradePolicy, error) {
	var p UpgradePolicy
	var paused int
	err := db.QueryRowContext(ctx, `SELECT mode, window_cron, window_duration_seconds, window_timezone,
		min_release_age_days, notify_channel_ids, paused, updated_by, updated_at, decision_tag, decision, decision_at
		FROM upgrade_policy WHERE id = 1`).Scan(&p.Mode, &p.WindowCron, &p.WindowDurationSeconds, &p.WindowTimezone,
		&p.MinReleaseAgeDays, &p.NotifyChannelIDs, &paused, &p.UpdatedBy, &p.UpdatedAt, &p.DecisionTag, &p.Decision, &p.DecisionAt)
	if err != nil {
		return UpgradePolicy{}, fmt.Errorf("store: get upgrade policy: %w", err)
	}
	p.Paused = paused != 0
	return p, nil
}

// UpdateUpgradePolicy replaces the operator-set fields of the policy row.
// The decision columns are written separately by SetUpgradeDecision.
func (db *DB) UpdateUpgradePolicy(ctx context.Context, p UpgradePolicy) error {
	paused := 0
	if p.Paused {
		paused = 1
	}
	if p.NotifyChannelIDs == "" {
		p.NotifyChannelIDs = "[]"
	}
	_, err := db.ExecContext(ctx, `UPDATE upgrade_policy SET mode = ?, window_cron = ?, window_duration_seconds = ?,
		window_timezone = ?, min_release_age_days = ?, notify_channel_ids = ?, paused = ?, updated_by = ?, updated_at = ?
		WHERE id = 1`, p.Mode, p.WindowCron, p.WindowDurationSeconds, p.WindowTimezone, p.MinReleaseAgeDays,
		p.NotifyChannelIDs, paused, p.UpdatedBy, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: update upgrade policy: %w", err)
	}
	return nil
}

// SetUpgradeDecision records the scheduler's latest verdict on a release so
// the dashboard can say why an automatic upgrade is being held.
func (db *DB) SetUpgradeDecision(ctx context.Context, tag, decision, at string) error {
	_, err := db.ExecContext(ctx, `UPDATE upgrade_policy SET decision_tag = ?, decision = ?, decision_at = ? WHERE id = 1`,
		tag, decision, at)
	if err != nil {
		return fmt.Errorf("store: set upgrade decision: %w", err)
	}
	return nil
}
