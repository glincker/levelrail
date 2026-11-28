package store

import (
	"context"
	"database/sql"
	"encoding/json"
)

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// databaseInstanceOf returns the live instance id of a managed database, or "" when it is unknown.
func databaseInstanceOf(ctx context.Context, q rowQuerier, name string) string {
	var id string
	if err := q.QueryRowContext(ctx, `SELECT instance_id FROM desired_databases WHERE name = ?`, name).Scan(&id); err != nil {
		return ""
	}
	return id
}

// stampDatabaseRefs binds refs without an instance to the database that exists now. A ref
// already stored unbound for the same database stays unbound, so a plain re-save cannot
// adopt a later same-name database.
func (db *DB) stampDatabaseRefs(ctx context.Context, service string, refs map[string]DatabaseEnvRef) map[string]DatabaseEnvRef {
	if len(refs) == 0 {
		return refs
	}
	var prior map[string]DatabaseEnvRef
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT database_env FROM desired_services WHERE name = ?`, service).Scan(&raw); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &prior)
	}
	out := make(map[string]DatabaseEnvRef, len(refs))
	for k, ref := range refs {
		if ref.Instance == "" {
			if old, ok := prior[k]; !ok || old.Database != ref.Database || old.Instance != "" {
				ref.Instance = databaseInstanceOf(ctx, db, ref.Database)
			}
		}
		out[k] = ref
	}
	return out
}
