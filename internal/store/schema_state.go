package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SchemaState is what a database file says about itself, read without
// migrating or modifying it.
type SchemaState struct {
	// Schema is the highest applied migration version.
	Schema int
	// LastVersion is the control plane version recorded most recently in the
	// upgrade history, empty when there is none.
	LastVersion string
}

// ReadSchemaState reports a database file's schema version and last running
// release read-only, safe against a database newer than this binary.
func ReadSchemaState(ctx context.Context, path string) (SchemaState, error) {
	sdb, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return SchemaState{}, fmt.Errorf("open %s read-only: %w", path, err)
	}
	defer func() { _ = sdb.Close() }()
	var st SchemaState
	if err := sdb.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&st.Schema); err != nil {
		return SchemaState{}, fmt.Errorf("read schema version of %s: %w", path, err)
	}
	// The table may not exist on a database from before upgrade history.
	_ = sdb.QueryRowContext(ctx, `SELECT to_version FROM upgrade_history ORDER BY seq DESC LIMIT 1`).Scan(&st.LastVersion)
	return st, nil
}
