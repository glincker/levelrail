package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by the cloud connection store.
var (
	ErrCloudConnectionExists   = errors.New("store: cloud connection already exists")
	ErrCloudConnectionNotFound = errors.New("store: cloud connection not found")
)

// CloudConnection is one named cloud account. Config is opaque JSON of non-secret
// settings; CredentialRef is a reference, never a credential value.
type CloudConnection struct {
	Name           string
	Provider       string
	TeamID         string
	AuthMethod     string
	Config         string
	CredentialRef  string
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastCheckedAt  time.Time
	LastCheckClass string
}

const cloudConnectionCols = "name, provider, team_id, auth_method, config, credential_ref, created_by, created_at, updated_at, last_checked_at, last_check_class" //nolint:gosec // column names, not credentials

func scanCloudConnection(sc interface{ Scan(...any) error }) (CloudConnection, error) {
	var c CloudConnection
	var created, updated, checked string
	if err := sc.Scan(&c.Name, &c.Provider, &c.TeamID, &c.AuthMethod, &c.Config, &c.CredentialRef, &c.CreatedBy, &created, &updated, &checked, &c.LastCheckClass); err != nil {
		return CloudConnection{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	c.LastCheckedAt, _ = time.Parse(time.RFC3339Nano, checked)
	return c, nil
}

// ListCloudConnections returns every connection ordered by name.
func (db *DB) ListCloudConnections(ctx context.Context) ([]CloudConnection, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+cloudConnectionCols+" FROM cloud_connections ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("store: list cloud connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CloudConnection{}
	for rows.Next() {
		c, err := scanCloudConnection(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan cloud connection: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCloudConnection returns one connection or ErrCloudConnectionNotFound.
func (db *DB) GetCloudConnection(ctx context.Context, name string) (CloudConnection, error) {
	c, err := scanCloudConnection(db.QueryRowContext(ctx, "SELECT "+cloudConnectionCols+" FROM cloud_connections WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return CloudConnection{}, ErrCloudConnectionNotFound
	}
	if err != nil {
		return CloudConnection{}, fmt.Errorf("store: get cloud connection: %w", err)
	}
	return c, nil
}

// CreateCloudConnection inserts c, or returns ErrCloudConnectionExists.
func (db *DB) CreateCloudConnection(ctx context.Context, c CloudConnection) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `INSERT INTO cloud_connections (name, provider, team_id, auth_method, config, credential_ref, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Provider, c.TeamID, c.AuthMethod, c.Config, c.CredentialRef, c.CreatedBy, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return ErrCloudConnectionExists
		}
		return fmt.Errorf("store: create cloud connection: %w", err)
	}
	return nil
}

// UpdateCloudConnection replaces the mutable fields; provider and creator never change.
// A changed definition clears the last check, which no longer describes it.
func (db *DB) UpdateCloudConnection(ctx context.Context, c CloudConnection) error {
	res, err := db.ExecContext(ctx, `UPDATE cloud_connections SET team_id = ?, auth_method = ?, config = ?, credential_ref = ?, updated_at = ?, last_checked_at = '', last_check_class = '' WHERE name = ?`,
		c.TeamID, c.AuthMethod, c.Config, c.CredentialRef, time.Now().UTC().Format(time.RFC3339Nano), c.Name)
	if err != nil {
		return fmt.Errorf("store: update cloud connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCloudConnectionNotFound
	}
	return nil
}

// SetCloudConnectionCheck records the outcome class of the latest health check.
func (db *DB) SetCloudConnectionCheck(ctx context.Context, name, class string, at time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE cloud_connections SET last_checked_at = ?, last_check_class = ? WHERE name = ?`,
		at.UTC().Format(time.RFC3339Nano), class, name)
	if err != nil {
		return fmt.Errorf("store: set cloud connection check: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCloudConnectionNotFound
	}
	return nil
}

// DeleteCloudConnection removes a connection; a missing one is ErrCloudConnectionNotFound.
func (db *DB) DeleteCloudConnection(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM cloud_connections WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete cloud connection: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCloudConnectionNotFound
	}
	return nil
}
