// Package telemetry is the node-local metrics store, decided in
// ADR 009: a dedicated SQLite database (telemetry.db, separate from
// internal/store's levelrail.db) holding container resource-usage
// samples, queried in place rather than shipped anywhere centrally.
// Keeping metrics node-local instead of centralizing them is what keeps
// idle cost near zero as the number of managed apps grows.
package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DB wraps a *sql.DB opened against telemetry.db: WAL mode, migrated to
// the latest version before Open returns. Deliberately a separate type
// from store.DB, not a shared one: ADR 009 keeps this on its own SQLite
// file precisely so its write pattern (a batch every collection tick)
// never contends with internal/store's own WAL.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies
// pragmas, and runs every pending migration. path is a plain filesystem
// path, not a DSN, matching internal/store.Open's own shape.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := dsn(path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open %s: %w", path, err)
	}

	// SQLite allows only one writer at a time; a single shared
	// connection avoids SQLITE_BUSY from this process's own concurrent
	// goroutines (the collector, a retention sweep, a query handler)
	// contending with each other, the same reasoning internal/store.Open
	// already applies.
	sqlDB.SetMaxOpenConns(1)

	db := &DB{DB: sqlDB}

	if err := db.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("telemetry: ping %s: %w", path, err)
	}

	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("telemetry: migrate %s: %w", path, err)
	}

	return db, nil
}

func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	return "file:" + path + "?" + q.Encode()
}
