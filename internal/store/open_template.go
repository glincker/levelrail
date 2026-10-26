package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Test binaries migrate once per process and seed each fresh database file
// from that snapshot, because running every migration per fixture dominates
// test time. Production binaries never take this path (testing.Testing is
// false); LEVELRAIL_TEST_TEMPLATE_DB=off opts out in tests.
// seedTemplateDisabled lets the store package's own tests exercise the real
// migration path.
var (
	seedTemplateDisabled bool
	seedTemplateOnce     sync.Once
	seedTemplateBytes    []byte
)

func seedFromTemplate(ctx context.Context, path string, opts []OpenOption) {
	if seedTemplateDisabled || !testing.Testing() || len(opts) > 0 || os.Getenv("LEVELRAIL_TEST_TEMPLATE_DB") == "off" {
		return
	}
	if path == "" || strings.HasPrefix(path, ":") || strings.HasPrefix(path, "file:") {
		return
	}
	if fi, err := os.Stat(path); err != nil || fi.Size() != 0 {
		return
	}
	seedTemplateOnce.Do(func() { seedTemplateBytes = buildTemplate(ctx) })
	if len(seedTemplateBytes) == 0 {
		return
	}
	_ = os.WriteFile(path, seedTemplateBytes, 0o600)
}

// buildTemplate returns a fully migrated, checkpointed database image, or nil
// so callers fall back to a normal migrating Open.
func buildTemplate(ctx context.Context) []byte {
	dir, err := os.MkdirTemp("", "lr-store-template-")
	if err != nil {
		return nil
	}
	defer func() { _ = os.RemoveAll(dir) }()
	p := filepath.Join(dir, "template.db")

	sqlDB, err := sql.Open("sqlite", dsn(p))
	if err != nil {
		return nil
	}
	sqlDB.SetMaxOpenConns(1)
	db := &DB{DB: sqlDB, m: newDBMetrics(p)}
	if err := db.migrate(ctx, openConfig{}); err != nil {
		_ = sqlDB.Close()
		return nil
	}
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = sqlDB.Close()
		return nil
	}
	if err := sqlDB.Close(); err != nil {
		return nil
	}
	b, err := os.ReadFile(p) //nolint:gosec // path is a private temp dir created above
	if err != nil {
		return nil
	}
	return b
}
