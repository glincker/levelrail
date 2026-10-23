package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Database TLS modes: default follows the engine, on and off are operator choices.
const (
	DatabaseTLSModeDefault = "default"
	DatabaseTLSModeOn      = "on"
	DatabaseTLSModeOff     = "off"
)

// GetDatabaseTLSMode returns a database's TLS mode, or default when none was set.
func (db *DB) GetDatabaseTLSMode(ctx context.Context, database string) (string, error) {
	var mode string
	err := db.QueryRowContext(ctx, `SELECT mode FROM database_tls_settings WHERE database_name = ?`, database).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseTLSModeDefault, nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get database tls mode for %q: %w", database, err)
	}
	return mode, nil
}

// SetDatabaseTLSMode stores a database's TLS mode.
func (db *DB) SetDatabaseTLSMode(ctx context.Context, database, mode string) error {
	switch mode {
	case DatabaseTLSModeDefault, DatabaseTLSModeOn, DatabaseTLSModeOff:
	default:
		return fmt.Errorf("store: database tls mode %q is not default, on or off", mode)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_tls_settings (database_name, mode, updated_at)
		VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (database_name) DO UPDATE SET mode = excluded.mode, updated_at = excluded.updated_at
	`, database, mode)
	if err != nil {
		return fmt.Errorf("store: set database tls mode for %q: %w", database, err)
	}
	return nil
}

// DatabaseTLSEffective reports whether a database of engine runs TLS under mode.
func DatabaseTLSEffective(engine, mode string) bool {
	switch mode {
	case DatabaseTLSModeOn:
		return true
	case DatabaseTLSModeOff:
		return false
	}
	return engine == EnginePostgres || engine == EngineRedis
}

// DatabaseTLSToggleable reports whether the operator can switch TLS for engine.
func DatabaseTLSToggleable(engine string) bool {
	switch engine {
	case EngineMySQL, EngineMariaDB, EngineMongoDB, EngineRedis:
		return true
	}
	return false
}

// DatabaseTLSOnAtCreate reports whether a newly created database of engine starts with
// TLS on. Databases created before this existed have no stored mode and keep their behaviour.
func DatabaseTLSOnAtCreate(engine string) bool {
	return engine == EngineMySQL || engine == EngineMariaDB || engine == EngineMongoDB
}
