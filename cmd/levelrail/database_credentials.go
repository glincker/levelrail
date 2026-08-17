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

// mysqlPasswordEnvKey is mysqlCredentialsFor's own storage key, distinct
// from postgresPasswordEnvKey: a database named "main" reconciled as
// mysql must never share storage (and therefore a password) with a
// database of the same name reconciled as postgres.
const mysqlPasswordEnvKey = "mysql_password"

// mongoPasswordEnvKey is mongoCredentialsFor's own storage key, distinct
// from postgresPasswordEnvKey/mysqlPasswordEnvKey for the same reason
// mysqlPasswordEnvKey is distinct from postgresPasswordEnvKey: a
// database named "main" reconciled as mongodb must never share storage
// with a same-named database reconciled as a different engine.
const mongoPasswordEnvKey = "mongo_password"

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

// mysqlCredentialsFor is postgresCredentialsFor's exact counterpart for
// database.WithMySQLCredentials: same generate-once-persist-forever
// shape, same "nil manager means nil credentials, not an error" contract,
// its own storage key (mysqlPasswordEnvKey) so it never collides with a
// postgres database of the same name. See postgresCredentialsFor's own
// doc comment for the full reasoning, unchanged here.
func mysqlCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.MySQLCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, mysqlPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing mysql credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate mysql password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, mysqlPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store mysql password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, mysqlPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve mysql password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved mysql password is empty")
	}

	return &database.MySQLCredentials{Username: dbName, Password: password}, nil
}

// mongoCredentialsFor is postgresCredentialsFor's exact counterpart for
// database.WithMongoDBCredentials: same generate-once-persist-forever
// shape, same "nil manager means nil credentials, not an error"
// contract, its own storage key (mongoPasswordEnvKey) so it never
// collides with a same-named database of a different engine. See
// postgresCredentialsFor's own doc comment for the full reasoning,
// unchanged here.
func mongoCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.MongoDBCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, mongoPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing mongodb credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate mongodb password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, mongoPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store mongodb password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, mongoPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve mongodb password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved mongodb password is empty")
	}

	return &database.MongoDBCredentials{Username: dbName, Password: password}, nil
}
