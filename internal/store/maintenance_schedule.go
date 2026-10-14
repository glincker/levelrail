package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Maintenance window scopes.
const (
	MaintenanceScopeAll     = "all"
	MaintenanceScopeProject = "project"
	MaintenanceScopeApp     = "app"
)

// ErrMaintenanceWindowNotFound is returned when a window id does not exist.
var ErrMaintenanceWindowNotFound = errors.New("store: maintenance window not found")

// MaintenanceWindow is one scheduled, one-off maintenance period.
type MaintenanceWindow struct {
	ID               string
	Title            string
	Message          string
	Scope            string
	Target           string
	StartsAt         time.Time
	EndsAt           time.Time
	Freeze           bool
	AnnounceLeadMins int
	AnnouncedAt      *time.Time
	CreatedBy        string
	CreatedAt        time.Time
}

const maintenanceColumns = `id, title, message, scope, target, starts_at, ends_at, freeze,
	announce_lead_mins, announced_at, created_by, created_at`

func scanMaintenanceWindow(row interface{ Scan(dest ...any) error }) (MaintenanceWindow, error) {
	var (
		w                                    MaintenanceWindow
		freeze                               int
		startsAt, endsAt, announced, created string
	)
	if err := row.Scan(&w.ID, &w.Title, &w.Message, &w.Scope, &w.Target, &startsAt, &endsAt, &freeze,
		&w.AnnounceLeadMins, &announced, &w.CreatedBy, &created); err != nil {
		return MaintenanceWindow{}, err
	}
	w.Freeze = freeze != 0
	w.StartsAt, _ = parseStatusTime(startsAt)
	w.EndsAt, _ = parseStatusTime(endsAt)
	w.CreatedAt, _ = parseStatusTime(created)
	if announced != "" {
		if t, err := parseStatusTime(announced); err == nil {
			w.AnnouncedAt = &t
		}
	}
	return w, nil
}

// InsertMaintenanceWindow stores w, minting its id.
func (db *DB) InsertMaintenanceWindow(ctx context.Context, w MaintenanceWindow) (MaintenanceWindow, error) {
	id, err := NewStatusID("mw_")
	if err != nil {
		return MaintenanceWindow{}, err
	}
	w.ID = id
	w.CreatedAt = time.Now().UTC()
	freeze := 0
	if w.Freeze {
		freeze = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO maintenance_windows (`+maintenanceColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)
	`, w.ID, w.Title, w.Message, w.Scope, w.Target, statusTime(w.StartsAt), statusTime(w.EndsAt), freeze,
		w.AnnounceLeadMins, w.CreatedBy, statusTime(w.CreatedAt))
	if err != nil {
		return MaintenanceWindow{}, fmt.Errorf("store: insert maintenance window: %w", err)
	}
	return w, nil
}

// ListMaintenanceWindows returns windows ending at or after since, soonest first.
func (db *DB) ListMaintenanceWindows(ctx context.Context, since time.Time) ([]MaintenanceWindow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+maintenanceColumns+` FROM maintenance_windows
		WHERE ends_at >= ? ORDER BY starts_at ASC, id ASC
	`, statusTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: list maintenance windows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MaintenanceWindow
	for rows.Next() {
		w, err := scanMaintenanceWindow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan maintenance window: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate maintenance windows: %w", err)
	}
	return out, nil
}

// DeleteMaintenanceWindow removes a window.
func (db *DB) DeleteMaintenanceWindow(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM maintenance_windows WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete maintenance window: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrMaintenanceWindowNotFound
	}
	return nil
}

// MarkMaintenanceAnnounced records that the lead-time notice went out.
func (db *DB) MarkMaintenanceAnnounced(ctx context.Context, id string, at time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE maintenance_windows SET announced_at = ? WHERE id = ?`, statusTime(at), id)
	if err != nil {
		return fmt.Errorf("store: mark maintenance announced: %w", err)
	}
	return nil
}

// FreezeMaintenanceWindows returns the freeze-enabled windows that apply to
// serviceName and have not ended, as one-shot cron windows the existing
// freeze evaluation understands.
func (db *DB) FreezeMaintenanceWindows(ctx context.Context, serviceName string, now time.Time) ([]DeployFreezeWindow, error) {
	var project sql.NullString
	err := db.QueryRowContext(ctx, `SELECT project_id FROM desired_services WHERE name = ?`, serviceName).Scan(&project)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("store: freeze maintenance project lookup: %w", err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+maintenanceColumns+` FROM maintenance_windows
		WHERE freeze = 1 AND ends_at > ?
		  AND (scope = 'all' OR (scope = 'app' AND target = ?) OR (scope = 'project' AND target = ? AND target != ''))
	`, statusTime(now), serviceName, project.String)
	if err != nil {
		return nil, fmt.Errorf("store: list freeze maintenance windows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployFreezeWindow
	for rows.Next() {
		w, err := scanMaintenanceWindow(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan freeze maintenance window: %w", err)
		}
		s := w.StartsAt.UTC()
		out = append(out, DeployFreezeWindow{
			ID:       w.ID,
			Scope:    DeployFreezeScopeGlobal,
			Cron:     fmt.Sprintf("%d %d %d %d *", s.Minute(), s.Hour(), s.Day(), int(s.Month())),
			Duration: w.EndsAt.Sub(s).Truncate(time.Minute),
			Timezone: "UTC",
			Reason:   "maintenance: " + w.Title,
		})
	}
	return out, rows.Err()
}

// ListAllDeployFreezeWindows returns every recurring freeze window, any scope.
func (db *DB) ListAllDeployFreezeWindows(ctx context.Context) ([]DeployFreezeWindow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, cron, duration_seconds, timezone, reason, created_at
		FROM deploy_freeze_windows ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: list all deploy freeze windows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployFreezeWindow
	for rows.Next() {
		var (
			w         DeployFreezeWindow
			seconds   int64
			createdAt string
		)
		if err := rows.Scan(&w.ID, &w.Scope, &w.Cron, &seconds, &w.Timezone, &w.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan deploy freeze window: %w", err)
		}
		w.Duration = time.Duration(seconds) * time.Second
		w.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		out = append(out, w)
	}
	return out, rows.Err()
}
