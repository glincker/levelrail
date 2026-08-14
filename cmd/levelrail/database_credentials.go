package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// postgresPasswordEnvKey is the envKey postgresCredentialsFor stores a
// database's generated Postgres password under, in internal/secrets'
// per-(serviceName, envKey) keyspace. Reused here with a database's own
// name in place of a service name: that keyspace was designed for app
// env vars, but nothing about its shape is app-specific, and adding a
// parallel storage path just for this one value would be new surface
// for no real benefit.
const postgresPasswordEnvKey = "postgres_password"

// postgresCredentialsFor returns the Postgres credentials
// database.WithPostgresCredentials needs for dbName, generating and
// persisting a random password on first call and returning the same
// one on every subsequent call, so a reconcile pass never rotates a
// running database's password out from under it (TASKS.md 1.7's
// envelope encryption is exactly what makes "persisted, but only this
// process can read it back" possible here).
//
// Returns (nil, nil), not an error, when mgr is nil: TASKS.md 1.7's
// secrets support is itself optional (no APP_MASTER_KEY configured),
// and a Postgres database on an instance without secrets configured
// stays exactly as blocked as it already was, database.Controller's own
// credentialsBlockedResult condition explains why to the operator.
//
// The username is dbName itself, not a generated or brand-derived
// value: Postgres' own "role name equals database name" convention,
// and it never needs envelope encryption since it isn't a secret, only
// the password is.
func postgresCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.PostgresCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, postgresPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing postgres credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate postgres password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, postgresPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store postgres password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, postgresPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve postgres password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved postgres password is empty")
	}

	return &database.PostgresCredentials{Username: dbName, Password: password}, nil
}
