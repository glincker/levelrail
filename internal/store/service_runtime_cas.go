package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// UpdateServiceRuntimeIf swaps only the runtime_options column when it still holds
// expected, so a concurrent deploy's other columns are never rolled back. False means it moved.
func (db *DB) UpdateServiceRuntimeIf(ctx context.Context, name string, expected, next *spec.RuntimeOptions) (bool, error) {
	enc := func(r *spec.RuntimeOptions) (string, error) {
		if r == nil {
			return "", nil
		}
		raw, err := json.Marshal(r)
		return string(raw), err
	}
	want, err := enc(expected)
	if err != nil {
		return false, fmt.Errorf("store: marshal runtime options for service %q: %w", name, err)
	}
	set, err := enc(next)
	if err != nil {
		return false, fmt.Errorf("store: marshal runtime options for service %q: %w", name, err)
	}
	res, err := db.ExecContext(ctx, `UPDATE desired_services SET runtime_options = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ? AND runtime_options = ?`, set, name, want)
	if err != nil {
		return false, fmt.Errorf("store: update runtime options for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: update runtime options for service %q: %w", name, err)
	}
	return n == 1, nil
}
