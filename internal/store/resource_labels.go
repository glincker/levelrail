package store

import (
	"context"
	"fmt"
)

// Resource kinds that carry labels in resource_labels.
const (
	LabelKindApp      = "app"
	LabelKindDatabase = "database"
	LabelKindStack    = "stack"
)

// ListResourceLabels returns every labelled resource of kind keyed by name.
func (db *DB) ListResourceLabels(ctx context.Context, kind string) (map[string]map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, key, value FROM resource_labels WHERE kind = ? ORDER BY name, key`, kind)
	if err != nil {
		return nil, fmt.Errorf("store: list %s labels: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]string{}
	for rows.Next() {
		var name, k, v string
		if err := rows.Scan(&name, &k, &v); err != nil {
			return nil, fmt.Errorf("store: scan %s label: %w", kind, err)
		}
		if out[name] == nil {
			out[name] = map[string]string{}
		}
		out[name][k] = v
	}
	return out, rows.Err()
}

// GetResourceLabels returns one resource's labels, empty (not nil) when it has none.
func (db *DB) GetResourceLabels(ctx context.Context, kind, name string) (map[string]string, error) {
	all, err := db.ListResourceLabels(ctx, kind)
	if err != nil {
		return nil, err
	}
	if l := all[name]; l != nil {
		return l, nil
	}
	return map[string]string{}, nil
}

// SetResourceLabels replaces a resource's whole label set; an empty map clears it.
func (db *DB) SetResourceLabels(ctx context.Context, kind, name string, labels map[string]string) error {
	return db.replaceKV(ctx, `DELETE FROM resource_labels WHERE kind = ? AND name = ?`, []any{kind, name},
		`INSERT INTO resource_labels (kind, name, key, value) VALUES (?, ?, ?, ?)`, []any{kind, name}, labels)
}
