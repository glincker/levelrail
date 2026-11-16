package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Scope types a connection can carry in connection_meta.
const (
	ConnectionScopeInstance = "instance"
	ConnectionScopeOrg      = "org"
	ConnectionScopeApp      = "app"
)

// ErrConnectionMetaNotFound is returned when a ref has no sidecar row.
var ErrConnectionMetaNotFound = errors.New("store: connection meta not found")

// ConnectionMeta is the sidecar row for one connection ref. Zero times mean unknown.
type ConnectionMeta struct {
	Ref          string
	ScopeType    string
	ScopeID      string
	LastUsedAt   time.Time
	ExpiresAt    time.Time
	RotateAfter  time.Time
	Health       string
	HealthDetail string
	CheckedAt    time.Time
	UpdatedAt    time.Time
}

const connectionMetaCols = "ref, scope_type, scope_id, last_used_at, expires_at, rotate_after, health, health_detail, checked_at, updated_at"

func scanConnectionMeta(scan func(dest ...any) error) (ConnectionMeta, error) {
	var m ConnectionMeta
	var used, exp, rot, chk, upd string
	if err := scan(&m.Ref, &m.ScopeType, &m.ScopeID, &used, &exp, &rot, &m.Health, &m.HealthDetail, &chk, &upd); err != nil {
		return ConnectionMeta{}, err
	}
	m.LastUsedAt, m.ExpiresAt, m.RotateAfter = parseOptionalTime(used), parseOptionalTime(exp), parseOptionalTime(rot)
	m.CheckedAt, m.UpdatedAt = parseOptionalTime(chk), parseOptionalTime(upd)
	return m, nil
}

// UpsertConnectionMeta writes the full sidecar row for m.Ref.
func (db *DB) UpsertConnectionMeta(ctx context.Context, m ConnectionMeta) error {
	if m.ScopeType == "" {
		m.ScopeType = ConnectionScopeInstance
	}
	if m.Health == "" {
		m.Health = "unknown"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO connection_meta (`+connectionMetaCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ref) DO UPDATE SET scope_type = excluded.scope_type, scope_id = excluded.scope_id,
			last_used_at = excluded.last_used_at, expires_at = excluded.expires_at, rotate_after = excluded.rotate_after,
			health = excluded.health, health_detail = excluded.health_detail, checked_at = excluded.checked_at,
			updated_at = excluded.updated_at`,
		m.Ref, m.ScopeType, m.ScopeID, formatOptionalTime(m.LastUsedAt), formatOptionalTime(m.ExpiresAt), formatOptionalTime(m.RotateAfter),
		m.Health, m.HealthDetail, formatOptionalTime(m.CheckedAt), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: upsert connection meta %q: %w", m.Ref, err)
	}
	return nil
}

// GetConnectionMeta returns one sidecar row or ErrConnectionMetaNotFound.
func (db *DB) GetConnectionMeta(ctx context.Context, ref string) (ConnectionMeta, error) {
	m, err := scanConnectionMeta(db.QueryRowContext(ctx, "SELECT "+connectionMetaCols+" FROM connection_meta WHERE ref = ?", ref).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return ConnectionMeta{}, ErrConnectionMetaNotFound
	}
	if err != nil {
		return ConnectionMeta{}, fmt.Errorf("store: get connection meta %q: %w", ref, err)
	}
	return m, nil
}

// ListConnectionMeta returns every sidecar row keyed by ref.
func (db *DB) ListConnectionMeta(ctx context.Context) (map[string]ConnectionMeta, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+connectionMetaCols+" FROM connection_meta")
	if err != nil {
		return nil, fmt.Errorf("store: list connection meta: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ConnectionMeta{}
	for rows.Next() {
		m, err := scanConnectionMeta(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan connection meta: %w", err)
		}
		out[m.Ref] = m
	}
	return out, rows.Err()
}

// DeleteConnectionMeta removes a ref's sidecar row; a missing row is not an error.
func (db *DB) DeleteConnectionMeta(ctx context.Context, ref string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM connection_meta WHERE ref = ?`, ref); err != nil {
		return fmt.Errorf("store: delete connection meta %q: %w", ref, err)
	}
	return nil
}
