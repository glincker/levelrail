package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// LogDrainType selects which external sink protocol a service's log
// drain uses.
type LogDrainType string

// The two sink protocols internal/telemetry's forwarder knows how to
// build (internal/telemetry/drain.go).
const (
	LogDrainHTTP   LogDrainType = "http"
	LogDrainSyslog LogDrainType = "syslog"
)

// LogDrain is one service's external log-forwarding configuration
// (migrations/0047_service_log_drain.sql), the Coolify-parity per-app
// log_drain field: forward this service's container log stream to Target
// alongside the existing node-local store, in addition to it, never
// instead of it. Enabled lets an operator keep Target configured but
// pause forwarding without losing the value.
type LogDrain struct {
	Type    LogDrainType `json:"type"`
	Target  string       `json:"target"`
	Enabled bool         `json:"enabled"`
}

// UpdateServiceLogDrain sets or clears svc's log drain, the only way
// log_drain ever changes: SaveDesiredService's own doc comment explains
// why this is deliberately excluded from that method's full-record-
// replace semantics, the same reasoning UpdateServiceStorageTarget
// already establishes for storage_target_id. drain nil clears it.
func (db *DB) UpdateServiceLogDrain(ctx context.Context, name string, drain *LogDrain) error {
	var drainJSON sql.NullString
	if drain != nil {
		b, err := json.Marshal(drain)
		if err != nil {
			return fmt.Errorf("store: marshal log drain for service %q: %w", name, err)
		}
		drainJSON = sql.NullString{String: string(b), Valid: true}
	}

	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET log_drain = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, drainJSON, name)
	if err != nil {
		return fmt.Errorf("store: update log drain for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update log drain for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}
