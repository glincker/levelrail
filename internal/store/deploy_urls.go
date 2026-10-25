package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DeployURL ties a past deploy attempt to the retained service that serves its own hostname.
type DeployURL struct {
	ServiceName string
	AppName     string
	AttemptID   string
	Host        string
	CreatedAt   time.Time
}

// SaveDeployURL records u, replacing any earlier row for the same service.
func (db *DB) SaveDeployURL(ctx context.Context, u DeployURL) error {
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_urls (service_name, app_name, attempt_id, host, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET app_name = excluded.app_name, attempt_id = excluded.attempt_id, host = excluded.host
	`, u.ServiceName, u.AppName, u.AttemptID, u.Host, u.CreatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save deploy url %q: %w", u.ServiceName, err)
	}
	return nil
}

// DeleteDeployURL removes the row for serviceName; a missing row is not an error.
func (db *DB) DeleteDeployURL(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_urls WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete deploy url %q: %w", serviceName, err)
	}
	return nil
}

// ListDeployURLs returns every row, or only appName's when appName is not empty.
func (db *DB) ListDeployURLs(ctx context.Context, appName string) ([]DeployURL, error) {
	q := `SELECT service_name, app_name, attempt_id, host, created_at FROM deploy_urls`
	var args []any
	if appName != "" {
		q += ` WHERE app_name = ?`
		args = append(args, appName)
	}
	q += ` ORDER BY created_at DESC, service_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy urls: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployURL
	for rows.Next() {
		var u DeployURL
		var created string
		if err := rows.Scan(&u.ServiceName, &u.AppName, &u.AttemptID, &u.Host, &created); err != nil {
			return nil, fmt.Errorf("store: scan deploy url: %w", err)
		}
		u.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeployURLApp returns the app that owns the retained service serviceName, and
// false when serviceName is not a retained deploy service.
func (db *DB) DeployURLApp(ctx context.Context, serviceName string) (string, bool, error) {
	var app string
	err := db.QueryRowContext(ctx, `SELECT app_name FROM deploy_urls WHERE service_name = ?`, serviceName).Scan(&app)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: deploy url app for %q: %w", serviceName, err)
	}
	return app, true, nil
}
