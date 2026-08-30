package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

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

// mysqlCredentialsFor is postgresCredentialsFor's exact counterpart for
// database.WithMySQLCredentials: same generate-once-persist-forever
// shape, same "nil manager means nil credentials, not an error" contract,
// its own storage key (database.MySQLPasswordEnvKey) so it never collides with a
// postgres database of the same name. See postgresCredentialsFor's own
// doc comment for the full reasoning, unchanged here.
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

// mongoCredentialsFor is postgresCredentialsFor's exact counterpart for
// database.WithMongoDBCredentials: same generate-once-persist-forever
// shape, same "nil manager means nil credentials, not an error"
// contract, its own storage key (database.MongoPasswordEnvKey) so it never
// collides with a same-named database of a different engine. See
// postgresCredentialsFor's own doc comment for the full reasoning,
// unchanged here.
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

// mariadbCredentialsFor is postgresCredentialsFor's exact counterpart
// for database.WithMariaDBCredentials: same generate-once-persist-
// forever shape, same "nil manager means nil credentials, not an error"
// contract, its own storage key (database.MariaDBPasswordEnvKey) so it never
// collides with a mysql database of the same name. See
// postgresCredentialsFor's own doc comment for the full reasoning,
// unchanged here.
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
