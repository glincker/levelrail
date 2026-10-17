package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// Generated once and persisted, so a reconcile pass never rotates a running database's password.
// Returns nil credentials, not an error, when mgr is nil (no APP_MASTER_KEY), leaving the database blocked.
// The username is dbName, Postgres's role-name-equals-database-name convention; only the password is secret.
func postgresCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.PostgresCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.PostgresPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing postgres credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate postgres password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.PostgresPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store postgres password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, database.PostgresPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve postgres password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved postgres password is empty")
	}

	return &database.PostgresCredentials{Username: dbName, Password: password}, nil
}

// Same contract as postgresCredentialsFor, with its own storage key so same-named databases do not collide.
func mysqlCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.MySQLCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.MySQLPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing mysql credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate mysql password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.MySQLPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store mysql password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, database.MySQLPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve mysql password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved mysql password is empty")
	}

	return &database.MySQLCredentials{Username: dbName, Password: password}, nil
}

// Same contract as postgresCredentialsFor, with its own storage key so same-named databases do not collide.
func mongoCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.MongoDBCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.MongoPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing mongodb credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate mongodb password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.MongoPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store mongodb password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, database.MongoPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve mongodb password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved mongodb password is empty")
	}

	return &database.MongoDBCredentials{Username: dbName, Password: password}, nil
}

// Same contract as postgresCredentialsFor, with its own storage key so same-named databases do not collide.
func mariadbCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.MariaDBCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.MariaDBPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing mariadb credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate mariadb password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.MariaDBPasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store mariadb password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, database.MariaDBPasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve mariadb password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved mariadb password is empty")
	}

	return &database.MariaDBCredentials{Username: dbName, Password: password}, nil
}

// clickhouseCredentialsFor mirrors postgresCredentialsFor for
// database.WithClickHouseCredentials, using its own storage key
// (database.ClickHousePasswordEnvKey) to avoid colliding with other engines.
func clickhouseCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName string) (*database.ClickHouseCredentials, error) {
	if mgr == nil {
		return nil, nil
	}

	exists, err := mgr.Exists(ctx, dbName, database.ClickHousePasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("check existing clickhouse credentials for %q: %w", dbName, err)
	}
	if !exists {
		password, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate clickhouse password for %q: %w", dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, database.ClickHousePasswordEnvKey, password); err != nil {
			return nil, fmt.Errorf("store clickhouse password for %q: %w", dbName, err)
		}
	}

	password, err := mgr.Resolve(ctx, dbName, database.ClickHousePasswordEnvKey)
	if err != nil {
		return nil, fmt.Errorf("resolve clickhouse password for %q: %w", dbName, err)
	}
	if password == "" {
		return nil, errors.New("resolved clickhouse password is empty")
	}

	return &database.ClickHouseCredentials{Username: dbName, Password: password}, nil
}

// apiKeyCredentialsFor mirrors postgresCredentialsFor for the search
// engines: one generated API key per database, under the engine's own
// storage key (database.PasswordSecretKey).
func apiKeyCredentialsFor(ctx context.Context, mgr *secrets.Manager, dbName, engine string) (*database.APIKeyCredentials, error) {
	if mgr == nil {
		return nil, nil
	}
	envKey, ok := database.PasswordSecretKey(engine)
	if !ok {
		return nil, fmt.Errorf("engine %q has no api key", engine)
	}

	exists, err := mgr.Exists(ctx, dbName, envKey)
	if err != nil {
		return nil, fmt.Errorf("check existing %s api key for %q: %w", engine, dbName, err)
	}
	if !exists {
		key, err := generateRandomPassword()
		if err != nil {
			return nil, fmt.Errorf("generate %s api key for %q: %w", engine, dbName, err)
		}
		if err := mgr.SetValue(ctx, dbName, envKey, key); err != nil {
			return nil, fmt.Errorf("store %s api key for %q: %w", engine, dbName, err)
		}
	}

	key, err := mgr.Resolve(ctx, dbName, envKey)
	if err != nil {
		return nil, fmt.Errorf("resolve %s api key for %q: %w", engine, dbName, err)
	}
	if key == "" {
		return nil, errors.New("resolved api key is empty")
	}
	return &database.APIKeyCredentials{Key: key}, nil
}
