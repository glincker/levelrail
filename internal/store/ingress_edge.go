package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// IngressEdgeSettings are the edge values an operator can change from the
// dashboard. A nil limit means the built-in default.
type IngressEdgeSettings struct {
	TrustedProxies        string
	MaxHeaderBytes        *int64
	MaxBodyBytes          *int64
	ReadHeaderTimeoutSecs *int64
	IdleTimeoutSecs       *int64
}

// AppIngressLimit is one app's request body cap, overriding the instance value.
type AppIngressLimit struct {
	App          string
	MaxBodyBytes int64
}

// ErrAppIngressLimitNotFound is returned when an app has no override.
var ErrAppIngressLimitNotFound = errors.New("store: app ingress limit not found")

func edgeNullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func edgePtrInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// GetIngressEdgeSettings returns the single edge settings row.
func (db *DB) GetIngressEdgeSettings(ctx context.Context) (IngressEdgeSettings, error) {
	var s IngressEdgeSettings
	var hdr, body, rht, idle sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT trusted_proxies, max_header_bytes, max_body_bytes, read_header_timeout_secs, idle_timeout_secs
		FROM ingress_edge_settings WHERE id = 1`).Scan(&s.TrustedProxies, &hdr, &body, &rht, &idle)
	if err != nil {
		return s, fmt.Errorf("store: get ingress edge settings: %w", err)
	}
	s.MaxHeaderBytes, s.MaxBodyBytes, s.ReadHeaderTimeoutSecs, s.IdleTimeoutSecs = edgePtrInt64(hdr), edgePtrInt64(body), edgePtrInt64(rht), edgePtrInt64(idle)
	return s, nil
}

// UpdateIngressEdgeSettings replaces the edge settings row.
func (db *DB) UpdateIngressEdgeSettings(ctx context.Context, s IngressEdgeSettings) error {
	_, err := db.ExecContext(ctx, `UPDATE ingress_edge_settings SET trusted_proxies = ?, max_header_bytes = ?, max_body_bytes = ?,
		read_header_timeout_secs = ?, idle_timeout_secs = ?, updated_at = ? WHERE id = 1`,
		s.TrustedProxies, edgeNullInt64(s.MaxHeaderBytes), edgeNullInt64(s.MaxBodyBytes), edgeNullInt64(s.ReadHeaderTimeoutSecs), edgeNullInt64(s.IdleTimeoutSecs),
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: update ingress edge settings: %w", err)
	}
	return nil
}

// ListAppIngressLimits returns every per-app override.
func (db *DB) ListAppIngressLimits(ctx context.Context) ([]AppIngressLimit, error) {
	rows, err := db.QueryContext(ctx, `SELECT app_name, max_body_bytes FROM app_ingress_limits ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list app ingress limits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppIngressLimit
	for rows.Next() {
		var l AppIngressLimit
		if err := rows.Scan(&l.App, &l.MaxBodyBytes); err != nil {
			return nil, fmt.Errorf("store: scan app ingress limit: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetAppIngressLimit returns one app's override, or ErrAppIngressLimitNotFound.
func (db *DB) GetAppIngressLimit(ctx context.Context, app string) (AppIngressLimit, error) {
	l := AppIngressLimit{App: app}
	err := db.QueryRowContext(ctx, `SELECT max_body_bytes FROM app_ingress_limits WHERE app_name = ?`, app).Scan(&l.MaxBodyBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrAppIngressLimitNotFound
	}
	if err != nil {
		return l, fmt.Errorf("store: get app ingress limit %q: %w", app, err)
	}
	return l, nil
}

// SetAppIngressLimit upserts an app's override.
func (db *DB) SetAppIngressLimit(ctx context.Context, l AppIngressLimit) error {
	_, err := db.ExecContext(ctx, `INSERT INTO app_ingress_limits (app_name, max_body_bytes, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET max_body_bytes = excluded.max_body_bytes, updated_at = excluded.updated_at`,
		l.App, l.MaxBodyBytes, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set app ingress limit %q: %w", l.App, err)
	}
	return nil
}

// DeleteAppIngressLimit removes an app's override.
func (db *DB) DeleteAppIngressLimit(ctx context.Context, app string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM app_ingress_limits WHERE app_name = ?`, app)
	if err != nil {
		return fmt.Errorf("store: delete app ingress limit %q: %w", app, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAppIngressLimitNotFound
	}
	return nil
}
