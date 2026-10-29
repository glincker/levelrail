package store

import (
	"context"
	"fmt"
	"sort"
)

// ManagedField records that manager owns one spec leaf of a resource.
type ManagedField struct {
	Path    string
	Manager string
}

// ListManagedFields returns the field owners of kind/name, sorted by path then manager.
func (db *DB) ListManagedFields(ctx context.Context, kind, name string) ([]ManagedField, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT path, manager FROM resource_managed_fields WHERE kind = ? AND name = ? ORDER BY path, manager
	`, kind, name)
	if err != nil {
		return nil, fmt.Errorf("store: list %s/%s managed fields: %w", kind, name, err)
	}
	defer func() { _ = rows.Close() }()
	var out []ManagedField
	for rows.Next() {
		var f ManagedField
		if err := rows.Scan(&f.Path, &f.Manager); err != nil {
			return nil, fmt.Errorf("store: scan %s/%s managed field: %w", kind, name, err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list %s/%s managed fields: %w", kind, name, err)
	}
	return out, nil
}

// ReplaceManagedFields swaps the whole ownership set of kind/name in one transaction.
func (db *DB) ReplaceManagedFields(ctx context.Context, kind, name string, fields []ManagedField) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace %s/%s managed fields: begin: %w", kind, name, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM resource_managed_fields WHERE kind = ? AND name = ?`, kind, name); err != nil {
		return fmt.Errorf("store: replace %s/%s managed fields: %w", kind, name, err)
	}
	sorted := append([]ManagedField(nil), fields...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Manager < sorted[j].Manager
	})
	for _, f := range sorted {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO resource_managed_fields (kind, name, path, manager) VALUES (?, ?, ?, ?)
		`, kind, name, f.Path, f.Manager); err != nil {
			return fmt.Errorf("store: replace %s/%s managed fields: %w", kind, name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace %s/%s managed fields: commit: %w", kind, name, err)
	}
	return nil
}
