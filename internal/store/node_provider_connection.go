package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NodeProviderConnection is the non-secret record of how a provider was
// connected, who did it and when it last passed a connection test.
type NodeProviderConnection struct {
	Provider       string
	Method         string
	Label          string
	Location       string
	Notes          string
	ConnectedBy    string
	ConnectedAt    time.Time
	LastTestedAt   *time.Time
	LastTestOK     bool
	LastTestDetail string
}

// ErrNodeProviderConnectionNotFound is returned when no row exists.
var ErrNodeProviderConnectionNotFound = errors.New("store: node provider connection not found")

// SaveNodeProviderConnection upserts the row, resetting the test result.
func (db *DB) SaveNodeProviderConnection(ctx context.Context, c NodeProviderConnection) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_provider_connections (provider, method, label, location, notes, connected_by, connected_at, last_tested_at, last_test_ok, last_test_detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, 0, '')
		ON CONFLICT(provider) DO UPDATE SET method = excluded.method, label = excluded.label,
			location = excluded.location, notes = excluded.notes, connected_by = excluded.connected_by,
			connected_at = excluded.connected_at, last_tested_at = NULL, last_test_ok = 0, last_test_detail = ''
	`, c.Provider, c.Method, c.Label, c.Location, c.Notes, c.ConnectedBy, formatTime(c.ConnectedAt))
	if err != nil {
		return fmt.Errorf("store: save node provider connection %q: %w", c.Provider, err)
	}
	return nil
}

// RecordNodeProviderTest stores the outcome of a connection test.
func (db *DB) RecordNodeProviderTest(ctx context.Context, provider string, ok bool, detail string, at time.Time) error {
	okInt := 0
	if ok {
		okInt = 1
	}
	res, err := db.ExecContext(ctx, `
		UPDATE node_provider_connections SET last_tested_at = ?, last_test_ok = ?, last_test_detail = ? WHERE provider = ?
	`, formatTime(at), okInt, detail, provider)
	if err != nil {
		return fmt.Errorf("store: record node provider test %q: %w", provider, err)
	}
	return rowsAffectedOrNotFound(res, ErrNodeProviderConnectionNotFound, "record node provider test %q", provider)
}

// DeleteNodeProviderConnection removes the row; absent is not an error.
func (db *DB) DeleteNodeProviderConnection(ctx context.Context, provider string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM node_provider_connections WHERE provider = ?`, provider); err != nil {
		return fmt.Errorf("store: delete node provider connection %q: %w", provider, err)
	}
	return nil
}

// ListNodeProviderConnections returns every connection row.
func (db *DB) ListNodeProviderConnections(ctx context.Context) ([]NodeProviderConnection, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT provider, method, label, location, notes, connected_by, connected_at, last_tested_at, last_test_ok, last_test_detail
		FROM node_provider_connections
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list node provider connections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeProviderConnection
	for rows.Next() {
		var (
			c        NodeProviderConnection
			at       string
			testedAt sql.NullString
			testOK   int
		)
		if err := rows.Scan(&c.Provider, &c.Method, &c.Label, &c.Location, &c.Notes, &c.ConnectedBy, &at, &testedAt, &testOK, &c.LastTestDetail); err != nil {
			return nil, fmt.Errorf("store: scan node provider connection: %w", err)
		}
		c.LastTestOK = testOK != 0
		if c.ConnectedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("store: parse node provider connected_at: %w", err)
		}
		if testedAt.Valid {
			t, err := time.Parse(time.RFC3339Nano, testedAt.String)
			if err != nil {
				return nil, fmt.Errorf("store: parse node provider last_tested_at: %w", err)
			}
			c.LastTestedAt = &t
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate node provider connections: %w", err)
	}
	return out, nil
}
