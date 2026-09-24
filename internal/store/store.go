// Package store is the embedded SQLite state layer: WAL mode,
// modernc.org/sqlite (pure Go, no cgo, keeps cross-compiling the
// control plane binary trivial), forward-only versioned migrations.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DB wraps a *sql.DB opened against a single SQLite file, WAL mode and
// foreign keys on, migrated to the latest version before Open returns.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies
// pragmas, and runs every pending migration. path is a plain filesystem
// path, not a DSN; Open builds the DSN itself so callers never need to
// know the pragma query-string format.
func Open(ctx context.Context, path string, opts ...OpenOption) (*DB, error) {
	var cfg openConfig
	for _, o := range opts {
		o(&cfg)
	}
	dsn := dsn(path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	// SQLite allows only one writer at a time; a single shared connection
	// avoids SQLITE_BUSY from this process's own concurrent goroutines
	// contending with each other (separate from the busy_timeout pragma,
	// which handles contention from other processes/connections).
	sqlDB.SetMaxOpenConns(1)

	db := &DB{DB: sqlDB}

	if err := db.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}

	if err := db.migrate(ctx, cfg.preMigrate); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("store: migrate %s: %w", path, err)
	}

	return db, nil
}

func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(ON)")
	return "file:" + path + "?" + q.Encode()
}

type openConfig struct {
	preMigrate PreMigrateHook
}

// OpenOption customizes Open.
type OpenOption func(*openConfig)

// PreMigrateHook runs before pending migrations are applied to an existing
// database, with the current and target schema versions.
type PreMigrateHook func(ctx context.Context, db *DB, from, to int)

// WithPreMigrateHook registers a hook that runs only when an existing
// database has migrations pending (never for a brand new one).
func WithPreMigrateHook(h PreMigrateHook) OpenOption {
	return func(c *openConfig) { c.preMigrate = h }
}
