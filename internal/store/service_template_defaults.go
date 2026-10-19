package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ServiceTemplateDefaults records the strategy and memory limit a template or
// compose file applied to a service, with the reason for the strategy.
type ServiceTemplateDefaults struct {
	ServiceName    string
	TemplateID     string
	Strategy       string
	StrategyReason string
	MemoryBytes    int64
}

// SaveServiceTemplateDefaults replaces the row for d.ServiceName, or removes it when d carries nothing.
func (db *DB) SaveServiceTemplateDefaults(ctx context.Context, d ServiceTemplateDefaults) error {
	if d.Strategy == "" && d.StrategyReason == "" && d.MemoryBytes == 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM service_template_defaults WHERE service_name = ?`, d.ServiceName); err != nil {
			return fmt.Errorf("store: clear template defaults %q: %w", d.ServiceName, err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_template_defaults (service_name, template_id, strategy, strategy_reason, memory_bytes, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET template_id = excluded.template_id, strategy = excluded.strategy,
			strategy_reason = excluded.strategy_reason, memory_bytes = excluded.memory_bytes, updated_at = excluded.updated_at
	`, d.ServiceName, d.TemplateID, d.Strategy, d.StrategyReason, d.MemoryBytes, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save template defaults %q: %w", d.ServiceName, err)
	}
	return nil
}

// GetServiceTemplateDefaults returns the row for serviceName, or nil when none exists.
func (db *DB) GetServiceTemplateDefaults(ctx context.Context, serviceName string) (*ServiceTemplateDefaults, error) {
	d := ServiceTemplateDefaults{ServiceName: serviceName}
	err := db.QueryRowContext(ctx, `SELECT template_id, strategy, strategy_reason, memory_bytes FROM service_template_defaults WHERE service_name = ?`, serviceName).
		Scan(&d.TemplateID, &d.Strategy, &d.StrategyReason, &d.MemoryBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get template defaults %q: %w", serviceName, err)
	}
	return &d, nil
}
