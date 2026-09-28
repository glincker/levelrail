package alerting

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // window timezones must resolve on minimal container images

	"github.com/GLINCKER/levelrail/internal/cronexpr"
)

// Maintenance window scopes.
const (
	ScopeAll  = "all"
	ScopeApp  = "app"
	ScopeNode = "node"
)

// MaxMaintenanceDuration bounds a single window so a typo cannot silence alerts for months.
const MaxMaintenanceDuration = 30 * 24 * time.Hour

// MaintenanceWindow is a recurring silence: it starts at each cron match
// (evaluated in Timezone) and lasts Duration.
type MaintenanceWindow struct {
	ID        string
	Name      string
	Cron      string
	Duration  time.Duration
	Timezone  string
	Scope     string
	Targets   []string
	Enabled   bool
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WindowState is a window's status at one instant.
type WindowState struct {
	Active    bool
	Start     time.Time
	End       time.Time
	NextStart time.Time
}

// Validate checks every field of the window.
func (w MaintenanceWindow) Validate() error {
	if w.Name == "" {
		return errors.New("name is required")
	}
	if _, err := cronexpr.Parse(w.Cron); err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	if w.Duration <= 0 || w.Duration > MaxMaintenanceDuration {
		return fmt.Errorf("duration must be between 1s and %s", MaxMaintenanceDuration)
	}
	if _, err := time.LoadLocation(w.tz()); err != nil {
		return fmt.Errorf("timezone %q: %w", w.Timezone, err)
	}
	switch w.Scope {
	case ScopeAll:
	case ScopeApp, ScopeNode:
		if len(w.Targets) == 0 {
			return fmt.Errorf("targets are required for scope %q", w.Scope)
		}
	default:
		return fmt.Errorf("scope must be %q, %q or %q", ScopeAll, ScopeApp, ScopeNode)
	}
	return nil
}

func (w MaintenanceWindow) tz() string {
	if w.Timezone == "" {
		return "UTC"
	}
	return w.Timezone
}

// StateAt evaluates the cron schedule in the window's timezone. Cron
// fields are matched against wall-clock time, so durations are measured
// on the wall clock too (a window spanning a DST change keeps its
// nominal length).
func (w MaintenanceWindow) StateAt(now time.Time) (WindowState, error) {
	sched, err := cronexpr.Parse(w.Cron)
	if err != nil {
		return WindowState{}, fmt.Errorf("cron: %w", err)
	}
	loc, err := time.LoadLocation(w.tz())
	if err != nil {
		return WindowState{}, fmt.Errorf("timezone %q: %w", w.Timezone, err)
	}
	local := now.In(loc)
	wall := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), local.Second(), 0, time.UTC)
	toReal := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc)
	}

	st := WindowState{}
	if next := sched.Next(wall); !next.IsZero() {
		st.NextStart = toReal(next)
	}
	start := sched.Next(wall.Add(-w.Duration))
	if start.IsZero() || start.After(wall) {
		return st, nil
	}
	st.Active = true
	st.Start = toReal(start)
	st.End = toReal(start.Add(w.Duration))
	return st, nil
}

// Covers reports whether the window's scope includes the alert c.
func (w MaintenanceWindow) Covers(c AlertContext) bool {
	switch w.Scope {
	case ScopeAll:
		return true
	case ScopeApp:
		return containsString(w.Targets, c.App)
	case ScopeNode:
		return containsString(w.Targets, c.NodeID) || containsString(w.Targets, c.NodeName)
	default:
		return false
	}
}

// ErrMaintenanceWindowNotFound is returned when no window has that ID.
var ErrMaintenanceWindowNotFound = errors.New("alerting: maintenance window not found")

// NewMaintenanceWindowID generates a random window identifier.
func NewMaintenanceWindowID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate maintenance window id: %w", err)
	}
	return "mw_" + hex.EncodeToString(b), nil
}

// SaveMaintenanceWindow creates or fully replaces a window.
func (db *DB) SaveMaintenanceWindow(ctx context.Context, w MaintenanceWindow) error {
	targets, err := json.Marshal(nonNilStrings(w.Targets))
	if err != nil {
		return fmt.Errorf("alerting: encode window targets: %w", err)
	}
	now := fmtTime(time.Now())
	_, err = db.ExecContext(ctx, `
		INSERT INTO alert_maintenance_windows
			(id, name, cron, duration_seconds, timezone, scope, targets_json, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name, cron = excluded.cron, duration_seconds = excluded.duration_seconds,
			timezone = excluded.timezone, scope = excluded.scope, targets_json = excluded.targets_json,
			enabled = excluded.enabled, updated_at = excluded.updated_at`,
		w.ID, w.Name, w.Cron, int64(w.Duration.Seconds()), w.tz(), w.Scope, string(targets), boolToInt(w.Enabled), w.CreatedBy, now, now)
	if err != nil {
		return fmt.Errorf("alerting: save maintenance window %q: %w", w.ID, err)
	}
	return nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

const windowColumns = `SELECT id, name, cron, duration_seconds, timezone, scope, targets_json, enabled, created_by, created_at, updated_at FROM alert_maintenance_windows`

func scanWindow(scan func(dest ...any) error) (*MaintenanceWindow, error) {
	var (
		w                MaintenanceWindow
		seconds          int64
		targets, cr, upd string
		enabled          int
	)
	if err := scan(&w.ID, &w.Name, &w.Cron, &seconds, &w.Timezone, &w.Scope, &targets, &enabled, &w.CreatedBy, &cr, &upd); err != nil {
		return nil, err
	}
	w.Duration = time.Duration(seconds) * time.Second
	w.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(targets), &w.Targets); err != nil {
		return nil, fmt.Errorf("decode targets: %w", err)
	}
	var err error
	if w.CreatedAt, err = time.Parse(time.RFC3339Nano, cr); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if w.UpdatedAt, err = time.Parse(time.RFC3339Nano, upd); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &w, nil
}

// GetMaintenanceWindow returns one window or ErrMaintenanceWindowNotFound.
func (db *DB) GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error) {
	w, err := scanWindow(db.QueryRowContext(ctx, windowColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMaintenanceWindowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get maintenance window %q: %w", id, err)
	}
	return w, nil
}

// ListMaintenanceWindows returns every window ordered by name.
func (db *DB) ListMaintenanceWindows(ctx context.Context) ([]MaintenanceWindow, error) {
	rows, err := db.QueryContext(ctx, windowColumns+` ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list maintenance windows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []MaintenanceWindow
	for rows.Next() {
		w, err := scanWindow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan maintenance window row: %w", err)
		}
		out = append(out, *w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate maintenance window rows: %w", err)
	}
	return out, nil
}

// DeleteMaintenanceWindow removes a window; deleting a missing one is a no-op.
func (db *DB) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM alert_maintenance_windows WHERE id = ?`, id); err != nil {
		return fmt.Errorf("alerting: delete maintenance window %q: %w", id, err)
	}
	return nil
}
