package store

import (
	"context"
	"database/sql"
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
	Since       time.Time
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

// SetAppSleeping records that serviceName went to sleep or woke, and resets the idle baseline.
func (db *DB) SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error {
	ts := now.UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, `UPDATE app_sleep SET sleeping = ?, since = ?, updated_at = ? WHERE service_name = ?`, sleeping, ts, ts, serviceName)
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
	a := AppSleep{ServiceName: serviceName}
	var sleeping int
	var since string
	err := db.QueryRowContext(ctx, `SELECT idle_minutes, sleeping, since FROM app_sleep WHERE service_name = ?`, serviceName).Scan(&a.IdleMinutes, &sleeping, &since)
	if errors.Is(err, sql.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return AppSleep{}, fmt.Errorf("store: get app sleep for %q: %w", serviceName, err)
	}
	a.Sleeping = sleeping != 0
	if a.Since, err = time.Parse(time.RFC3339, since); err != nil {
		return AppSleep{}, fmt.Errorf("store: get app sleep for %q: %w", serviceName, err)
	}
	return a, nil
}

// ListAppSleep returns every app with sleeping enabled.
func (db *DB) ListAppSleep(ctx context.Context) ([]AppSleep, error) {
	rows, err := db.QueryContext(ctx, `SELECT service_name, idle_minutes, sleeping, since FROM app_sleep WHERE idle_minutes > 0 ORDER BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list app sleep: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppSleep
	for rows.Next() {
		var a AppSleep
		var sleeping int
		var since string
		if err := rows.Scan(&a.ServiceName, &a.IdleMinutes, &sleeping, &since); err != nil {
			return nil, fmt.Errorf("store: list app sleep: %w", err)
		}
		a.Sleeping = sleeping != 0
		if a.Since, err = time.Parse(time.RFC3339, since); err != nil {
			return nil, fmt.Errorf("store: list app sleep: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
