package store

import (
	"context"
	"fmt"
)

// SetPreviewEnvExclude marks key as left out of name's preview environments (or back in when exclude is false).
func (db *DB) SetPreviewEnvExclude(ctx context.Context, name, key string, exclude bool) error {
	if _, err := db.GetDesiredService(ctx, name); err != nil {
		return err
	}
	q := `INSERT OR IGNORE INTO preview_env_excludes (service_name, key) VALUES (?, ?)`
	if !exclude {
		q = `DELETE FROM preview_env_excludes WHERE service_name = ? AND key = ?`
	}
	if _, err := db.ExecContext(ctx, q, name, key); err != nil {
		return fmt.Errorf("store: set preview env exclude for %q: %w", name, err)
	}
	return nil
}

// ListPreviewEnvExcludes returns the keys name keeps out of previews.
func (db *DB) ListPreviewEnvExcludes(ctx context.Context, name string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key FROM preview_env_excludes WHERE service_name = ? ORDER BY key`, name)
	if err != nil {
		return nil, fmt.Errorf("store: list preview env excludes for %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: scan preview env exclude: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
