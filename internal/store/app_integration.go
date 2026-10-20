package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrAppIntegrationNotFound is returned by GetAppIntegration and
// DeleteAppIntegration when id doesn't match any row.
var ErrAppIntegrationNotFound = errors.New("store: app integration not found")

// ErrAppIntegrationAlreadyAttached is returned by SaveAppIntegration
// when serviceName already has integrationKey attached (the table's own
// UNIQUE(service_name, integration_key) constraint).
var ErrAppIntegrationAlreadyAttached = errors.New("store: integration already attached to this app")

// AppIntegration is one row of internal/integrations' curated catalog
// attached to an app. Field values (DSN, API key) live in
// service_secrets/service_secret_values under
// AppIntegrationSecretsKey(ServiceName, IntegrationKey), not here: see
// migrations/0119_app_integrations.sql's own doc comment.
type AppIntegration struct {
	ID             string
	ServiceName    string
	IntegrationKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AppIntegrationSecretsKey is the internal/secrets serviceName one
// app's one attached integration's field values are stored under,
// distinct per (serviceName, integrationKey) pair so detaching one
// integration (DeleteAll on this namespace) can never touch the app's
// own secrets or a different attached integration's values, mirroring
// ProjectEnvSecretsKey's exact "distinct namespace" shape.
func AppIntegrationSecretsKey(serviceName, integrationKey string) string {
	return "app-integration/" + serviceName + "/" + integrationKey
}

// SaveAppIntegration inserts a new app integration row. ID is minted by
// the caller (internal/api), the same "generate before the INSERT"
// pattern SaveScheduledTask uses. Returns
// ErrAppIntegrationAlreadyAttached if serviceName already has
// integrationKey attached.
func (db *DB) SaveAppIntegration(ctx context.Context, ai AppIntegration) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_integrations (id, service_name, integration_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, ai.ID, ai.ServiceName, ai.IntegrationKey, formatTime(ai.CreatedAt), formatTime(ai.UpdatedAt))
	if err == nil {
		return nil
	}
	if _, getErr := db.GetAppIntegrationByKey(ctx, ai.ServiceName, ai.IntegrationKey); getErr == nil {
		return ErrAppIntegrationAlreadyAttached
	}
	return fmt.Errorf("store: save app integration %q: %w", ai.ID, err)
}

// GetAppIntegration returns the app integration with this ID, or
// ErrAppIntegrationNotFound.
func (db *DB) GetAppIntegration(ctx context.Context, id string) (AppIntegration, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, service_name, integration_key, created_at, updated_at
		FROM app_integrations
		WHERE id = ?
	`, id)
	ai, err := scanAppIntegration(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return AppIntegration{}, ErrAppIntegrationNotFound
	}
	if err != nil {
		return AppIntegration{}, fmt.Errorf("store: get app integration %q: %w", id, err)
	}
	return *ai, nil
}

// GetAppIntegrationByKey returns serviceName's attachment of
// integrationKey, or ErrAppIntegrationNotFound if it isn't attached.
func (db *DB) GetAppIntegrationByKey(ctx context.Context, serviceName, integrationKey string) (AppIntegration, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, service_name, integration_key, created_at, updated_at
		FROM app_integrations
		WHERE service_name = ? AND integration_key = ?
	`, serviceName, integrationKey)
	ai, err := scanAppIntegration(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return AppIntegration{}, ErrAppIntegrationNotFound
	}
	if err != nil {
		return AppIntegration{}, fmt.Errorf("store: get app integration for %q/%q: %w", serviceName, integrationKey, err)
	}
	return *ai, nil
}

// ListAppIntegrationsForService returns every integration attached to
// serviceName, oldest first, the same creation-order convention
// ListScheduledTasksForService already uses.
func (db *DB) ListAppIntegrationsForService(ctx context.Context, serviceName string) ([]AppIntegration, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, integration_key, created_at, updated_at
		FROM app_integrations
		WHERE service_name = ?
		ORDER BY created_at
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list app integrations for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []AppIntegration
	for rows.Next() {
		ai, err := scanAppIntegration(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan app integration row: %w", err)
		}
		out = append(out, *ai)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app integration rows: %w", err)
	}
	return out, nil
}

// DeleteAppIntegration removes an app integration row. Returns
// ErrAppIntegrationNotFound if id doesn't exist. Callers are also
// responsible for clearing the field values stored under
// AppIntegrationSecretsKey (internal/api's handleDetachAppIntegration
// does both in one request).
func (db *DB) DeleteAppIntegration(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM app_integrations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete app integration %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrAppIntegrationNotFound, "delete app integration %q", id)
}

func scanAppIntegration(scan func(dest ...any) error) (*AppIntegration, error) {
	var (
		ai                   AppIntegration
		createdAt, updatedAt string
	)
	if err := scan(&ai.ID, &ai.ServiceName, &ai.IntegrationKey, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	var err error
	ai.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	ai.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &ai, nil
}
