package store

import (
	"context"
	"fmt"
)

// StackOwnerKey is the lookup key used by ListStackOwnerNames.
func StackOwnerKey(kind, key string) string { return kind + "\x00" + key }

// ListStackOwnerNames maps every owned resource (see StackOwnerKey) to the
// name of the stack owning it, in one query, so list responses avoid N+1.
func (db *DB) ListStackOwnerNames(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT r.kind, r.key, s.name FROM stack_resources r JOIN stacks s ON s.id = r.stack_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list stack owners: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var kind, key, name string
		if err := rows.Scan(&kind, &key, &name); err != nil {
			return nil, fmt.Errorf("store: scan stack owner: %w", err)
		}
		out[StackOwnerKey(kind, key)] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list stack owners: %w", err)
	}
	return out, nil
}
