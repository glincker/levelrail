// Package alerting implements TASKS.md 2.5/2.7: threshold rules over
// internal/telemetry's metrics and logs, and crashloop detection as a
// built-in rule kind sharing the same evaluate/notify path rather than
// a separate mechanism (see TASKS.md 2.5's own note on why).
package alerting

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// DB wraps a *sql.DB opened against alerting.db, a dedicated SQLite
// file separate from both internal/store's levelrail.db and
// internal/telemetry's telemetry.db, the same write-isolation reasoning
// ADR 009 already applied: alert rule writes (evaluation state updates
// on every tick) are a different, independent write pattern from
// either desired-state config or metric/log ingestion.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path, applies
// pragmas, and runs every pending migration, matching internal/store and
// internal/telemetry's own Open functions exactly.
func Open(ctx context.Context, path string) (*DB, error) {
	dsn := dsn(path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("alerting: open %s: %w", path, err)
	}

	sqlDB.SetMaxOpenConns(1)

	db := &DB{DB: sqlDB}

	if err := db.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("alerting: ping %s: %w", path, err)
	}

	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("alerting: migrate %s: %w", path, err)
	}

	return db, nil
}

func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	// notification_channels' channel_id FK (migrations/0004) needs this on
	// to actually clear on delete, matching internal/store's own pragma.
	q.Add("_pragma", "foreign_keys(ON)")
	return "file:" + path + "?" + q.Encode()
}
