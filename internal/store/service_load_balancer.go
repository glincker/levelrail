package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetServiceLoadBalancer returns the raw JSON load balancer config for a
// service, and false when none is configured.
func (db *DB) GetServiceLoadBalancer(ctx context.Context, service string) (string, bool, error) {
	var cfg string
	err := db.QueryRowContext(ctx, `SELECT config FROM service_load_balancers WHERE service_name = ?`, service).Scan(&cfg)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get service load balancer: %w", err)
	}
	return cfg, true, nil
}

// SetServiceLoadBalancer upserts the JSON config; the service must exist.
func (db *DB) SetServiceLoadBalancer(ctx context.Context, service, configJSON string) error {
	res, err := db.ExecContext(ctx, `
		INSERT INTO service_load_balancers (service_name, config)
		SELECT name, ? FROM desired_services WHERE name = ?
		ON CONFLICT (service_name) DO UPDATE SET
			config = excluded.config,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, configJSON, service)
	if err != nil {
		return fmt.Errorf("store: set service load balancer: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// DeleteServiceLoadBalancer removes a service's load balancer config.
func (db *DB) DeleteServiceLoadBalancer(ctx context.Context, service string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM service_load_balancers WHERE service_name = ?`, service); err != nil {
		return fmt.Errorf("store: delete service load balancer: %w", err)
	}
	return nil
}

// ListServiceLoadBalancers returns every service's raw JSON config by name.
func (db *DB) ListServiceLoadBalancers(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT service_name, config FROM service_load_balancers`)
	if err != nil {
		return nil, fmt.Errorf("store: list service load balancers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]string)
	for rows.Next() {
		var name, cfg string
		if err := rows.Scan(&name, &cfg); err != nil {
			return nil, fmt.Errorf("store: scan service load balancer: %w", err)
		}
		out[name] = cfg
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate service load balancers: %w", err)
	}
	return out, nil
}

// LoadBalancerRow is one configured balancer joined to its app.
type LoadBalancerRow struct {
	Service   string
	Config    string
	UpdatedAt string
}

// ListLoadBalancerRows returns every configured balancer joined to its
// desired service, ordered by service name, in a single query.
func (db *DB) ListLoadBalancerRows(ctx context.Context) ([]LoadBalancerRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.name, l.config, l.updated_at
		FROM service_load_balancers l
		JOIN desired_services s ON s.name = l.service_name
		ORDER BY s.name`)
	if err != nil {
		return nil, fmt.Errorf("store: list load balancer rows: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []LoadBalancerRow
	for rows.Next() {
		var r LoadBalancerRow
		if err := rows.Scan(&r.Service, &r.Config, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan load balancer row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate load balancer rows: %w", err)
	}
	return out, nil
}
