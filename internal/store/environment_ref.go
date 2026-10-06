package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// EnvironmentRef is the environment a service or database is tagged with,
// enough for the authorization gate to evaluate environment and kind rules.
type EnvironmentRef struct {
	ID        string
	Name      string
	Kind      string
	Scope     string
	ProjectID string
	Protected bool
}

func (db *DB) environmentRef(ctx context.Context, query, name string) (*EnvironmentRef, error) {
	var ref EnvironmentRef
	var protected int
	err := db.QueryRowContext(ctx, query, name).Scan(&ref.ID, &ref.Name, &ref.Kind, &ref.Scope, &ref.ProjectID, &protected)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: environment of %q: %w", name, err)
	}
	ref.Protected = protected == 1
	return &ref, nil
}

// EnvironmentOfApp returns the environment tagged on a service, or nil when
// the service is untagged or does not exist.
func (db *DB) EnvironmentOfApp(ctx context.Context, appName string) (*EnvironmentRef, error) {
	return db.environmentRef(ctx, `
		SELECT e.id, e.name, e.kind, e.scope, e.project_id, e.protected
		FROM desired_services s JOIN environments e ON e.id = s.environment_id
		WHERE s.name = ?`, appName)
}

// EnvironmentOfDatabase returns the environment tagged on a database, or nil.
func (db *DB) EnvironmentOfDatabase(ctx context.Context, databaseName string) (*EnvironmentRef, error) {
	return db.environmentRef(ctx, `
		SELECT e.id, e.name, e.kind, e.scope, e.project_id, e.protected
		FROM desired_databases d JOIN environments e ON e.id = d.environment_id
		WHERE d.name = ?`, databaseName)
}
