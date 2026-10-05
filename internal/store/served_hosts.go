package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ReplaceServedHosts records the hostnames the ingress layer now serves.
func (db *DB) ReplaceServedHosts(ctx context.Context, hosts []string) error {
	if hosts == nil {
		hosts = []string{}
	}
	raw, err := json.Marshal(hosts)
	if err != nil {
		return fmt.Errorf("store: marshal served hosts: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO served_hosts_state (id, hosts, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET hosts = excluded.hosts, updated_at = excluded.updated_at`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: replace served hosts: %w", err)
	}
	return nil
}

// ServedHosts returns the last recorded hostnames. known is false until the
// ingress layer has applied a config at least once.
func (db *DB) ServedHosts(ctx context.Context) (hosts []string, known bool, err error) {
	var raw string
	err = db.QueryRowContext(ctx, `SELECT hosts FROM served_hosts_state WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: read served hosts: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
		return nil, false, fmt.Errorf("store: decode served hosts: %w", err)
	}
	return hosts, true, nil
}
