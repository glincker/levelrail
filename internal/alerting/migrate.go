package alerting

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one forward-only, numbered schema change, no down
// migration by design (CLAUDE.md 4.7's convention, applied here too).
//
// This runner is a deliberate near-duplicate of internal/store's and
// internal/telemetry's own migrate.go, not a shared package: those
// packages are under active concurrent development elsewhere in this
// repo, and each currently differs only by its embedded migrations
// directory. Worth extracting into one shared migration runner once all
// three stabilize, not before, the same "revisit later" note
// internal/telemetry/migrate.go already carries.
type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, err := parseMigrationFilename(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("migrations/%s: %w", entry.Name(), err)
		}
		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migrations/%s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: version, name: name, sql: string(content)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	if err := checkNoDuplicateVersions(migrations); err != nil {
		return nil, err
	}

	return migrations, nil
}

func parseMigrationFilename(filename string) (version int, name string, err error) {
	base := strings.TrimSuffix(filename, ".sql")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("expected NNNN_description.sql, got %q", filename)
	}
	version, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("expected a numeric version prefix, got %q: %w", parts[0], err)
	}
	if version <= 0 {
		return 0, "", fmt.Errorf("migration version must be positive, got %d", version)
	}
	return version, parts[1], nil
}

func checkNoDuplicateVersions(migrations []migration) error {
	seen := make(map[int]string, len(migrations))
	for _, m := range migrations {
		if existing, ok := seen[m.version]; ok {
			return fmt.Errorf("duplicate migration version %d: %q and %q", m.version, existing, m.name)
		}
		seen[m.version] = m.name
	}
	return nil
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	current, err := db.currentVersion(ctx)
	if err != nil {
		return err
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		if err := db.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", m.version, m.name, err)
		}
	}

	return nil
}

func (db *DB) currentVersion(ctx context.Context) (int, error) {
	var version int
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read current schema version: %w", err)
	}
	return version, nil
}

func (db *DB) applyMigration(ctx context.Context, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("run migration sql: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`,
		m.version, m.name,
	); err != nil {
		return fmt.Errorf("record migration version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
