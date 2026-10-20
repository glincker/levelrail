package store

import (
	"context"
	"fmt"
)

func (db *DB) environmentMap(ctx context.Context, query string) (map[string]*EnvironmentRef, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("store: environment map: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]*EnvironmentRef{}
	for rows.Next() {
		var name string
		var ref EnvironmentRef
		var protected int
		if err := rows.Scan(&name, &ref.ID, &ref.Name, &ref.Kind, &ref.Scope, &ref.ProjectID, &protected); err != nil {
			return nil, fmt.Errorf("store: scan environment map: %w", err)
		}
		ref.Protected = protected == 1
		out[name] = &ref
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate environment map: %w", err)
	}
	return out, nil
}

// EnvironmentsOfApps maps every tagged service name to its environment in one query.
func (db *DB) EnvironmentsOfApps(ctx context.Context) (map[string]*EnvironmentRef, error) {
	return db.environmentMap(ctx, `
		SELECT s.name, e.id, e.name, e.kind, e.scope, e.project_id, e.protected
		FROM desired_services s JOIN environments e ON e.id = s.environment_id`)
}

// EnvironmentsOfDatabases maps every tagged database name to its environment in one query.
func (db *DB) EnvironmentsOfDatabases(ctx context.Context) (map[string]*EnvironmentRef, error) {
	return db.environmentMap(ctx, `
		SELECT d.name, e.id, e.name, e.kind, e.scope, e.project_id, e.protected
		FROM desired_databases d JOIN environments e ON e.id = d.environment_id`)
}
