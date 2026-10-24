package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AppSleep is one app's sleep-when-idle setting (migrations/0379_app_sleep.sql).
// IdleMinutes 0 or a missing row means the app never sleeps.
type AppSleep struct {
	ServiceName string
	IdleMinutes int
	Sleeping    bool
	// HoldRequests makes a request that wakes the app wait for it (function mode).
	HoldRequests bool
	// IgnorePaths are request routes that never count as traffic; nil uses the defaults.
	IgnorePaths []string
	// PortWake lets a connection to a pinned host port wake the app.
	PortWake bool
	// WakeError is the last failed wake, cleared by the next successful one.
	WakeError string
	Since     time.Time
}

const appSleepColumns = `idle_minutes, sleeping, hold_requests, since, ignore_paths, port_wake, wake_error`

func scanAppSleep(a *AppSleep, scan func(dest ...any) error) error {
	var sleeping, hold, portWake int
	var since, paths string
	if err := scan(&a.IdleMinutes, &sleeping, &hold, &since, &paths, &portWake, &a.WakeError); err != nil {
		return err
	}
	a.Sleeping, a.HoldRequests, a.PortWake = sleeping != 0, hold != 0, portWake != 0
	if err := json.Unmarshal([]byte(paths), &a.IgnorePaths); err != nil {
		return fmt.Errorf("ignore_paths: %w", err)
	}
	t, err := time.Parse(time.RFC3339, since)
	if err != nil {
		return err
	}
	a.Since = t
	return nil
}

// SetAppSleepOptions updates ignore_paths and port_wake for an app that has a
// sleep setting; a nil argument leaves that field as it is.
func (db *DB) SetAppSleepOptions(ctx context.Context, serviceName string, ignorePaths []string, portWake *bool, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	if ignorePaths != nil {
		b, err := json.Marshal(ignorePaths)
		if err != nil {
			return fmt.Errorf("store: set app sleep ignore paths for %q: %w", serviceName, err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE app_sleep SET ignore_paths = ?, updated_at = ? WHERE service_name = ?`, string(b), ts, serviceName); err != nil {
			return fmt.Errorf("store: set app sleep ignore paths for %q: %w", serviceName, err)
		}
	}
	if portWake != nil {
		if _, err := db.ExecContext(ctx, `UPDATE app_sleep SET port_wake = ?, updated_at = ? WHERE service_name = ?`, *portWake, ts, serviceName); err != nil {
			return fmt.Errorf("store: set app sleep port wake for %q: %w", serviceName, err)
		}
	}
	return nil
}

// SetAppSleepWakeError records why the last wake failed, or clears it with "".
func (db *DB) SetAppSleepWakeError(ctx context.Context, serviceName, msg string, now time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE app_sleep SET wake_error = ?, updated_at = ? WHERE service_name = ?`, msg, now.UTC().Format(time.RFC3339), serviceName)
	if err != nil {
		return fmt.Errorf("store: set app sleep wake error for %q: %w", serviceName, err)
	}
	return nil
}

// ServicesWithActiveDeploy returns the apps that have a deploy, build or
// rollback running or queued right now.
func (db *DB) ServicesWithActiveDeploy(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT service_name FROM deploy_attempts WHERE status IN (?, ?)`, DeployAttemptStatusRunning, DeployAttemptStatusQueued)
	if err != nil {
		return nil, fmt.Errorf("store: list active deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: list active deploys: %w", err)
		}
		out[n] = true
	}
	return out, rows.Err()
}

// SetAppSleepIdle enables (minutes > 0) or disables (0) sleeping and resets the idle baseline.
func (db *DB) SetAppSleepIdle(ctx context.Context, serviceName string, minutes int, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_sleep (service_name, idle_minutes, sleeping, since, updated_at) VALUES (?, ?, 0, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET idle_minutes = excluded.idle_minutes, since = excluded.since, updated_at = excluded.updated_at
	`, serviceName, minutes, ts, ts)
	if err != nil {
		return fmt.Errorf("store: set app sleep for %q: %w", serviceName, err)
	}
	return nil
}

// SetAppSleepHold turns function mode on or off for an app that has a sleep setting.
func (db *DB) SetAppSleepHold(ctx context.Context, serviceName string, hold bool, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE app_sleep SET hold_requests = ?, updated_at = ? WHERE service_name = ?`, hold, now.UTC().Format(time.RFC3339), serviceName)
	if err != nil {
		return fmt.Errorf("store: set app sleep hold for %q: %w", serviceName, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: set app sleep hold for %q: no sleep setting", serviceName)
	}
	return nil
}

// SetAppSleeping records that serviceName went to sleep or woke, and resets the idle baseline.
func (db *DB) SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `UPDATE app_sleep SET sleeping = ?, since = ?, updated_at = ?, wake_error = CASE WHEN ? THEN wake_error ELSE '' END WHERE service_name = ?`, sleeping, ts, ts, sleeping, serviceName)
	if err != nil {
		return fmt.Errorf("store: set app sleeping for %q: %w", serviceName, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("store: set app sleeping for %q: no sleep setting", serviceName)
	}
	return nil
}

// GetAppSleep returns serviceName's row, or a disabled zero value when none exists.
func (db *DB) GetAppSleep(ctx context.Context, serviceName string) (AppSleep, error) {
	a := AppSleep{ServiceName: serviceName, PortWake: true}
	err := scanAppSleep(&a, db.QueryRowContext(ctx, `SELECT `+appSleepColumns+` FROM app_sleep WHERE service_name = ?`, serviceName).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return AppSleep{}, fmt.Errorf("store: get app sleep for %q: %w", serviceName, err)
	}
	return a, nil
}

// ListAppSleep returns every app with sleeping enabled.
func (db *DB) ListAppSleep(ctx context.Context) ([]AppSleep, error) {
	rows, err := db.reader().QueryContext(ctx, `SELECT service_name, `+appSleepColumns+` FROM app_sleep WHERE idle_minutes > 0 ORDER BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list app sleep: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppSleep
	for rows.Next() {
		var a AppSleep
		err := scanAppSleep(&a, func(dest ...any) error { return rows.Scan(append([]any{&a.ServiceName}, dest...)...) })
		if err != nil {
			return nil, fmt.Errorf("store: list app sleep: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAppStreamsByService returns how many TCP/UDP streams target each app.
func (db *DB) CountAppStreamsByService(ctx context.Context) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT service_name, COUNT(*) FROM app_streams GROUP BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("store: count app streams: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var n string
		var c int
		if err := rows.Scan(&n, &c); err != nil {
			return nil, fmt.Errorf("store: count app streams: %w", err)
		}
		out[n] = c
	}
	return out, rows.Err()
}
