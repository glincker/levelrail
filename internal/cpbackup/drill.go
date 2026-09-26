package cpbackup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DrillResult is the outcome of one restore drill.
type DrillResult struct {
	At        time.Time     `json:"at"`
	OK        bool          `json:"ok"`
	Partial   bool          `json:"partial"`
	Detail    string        `json:"detail"`
	BackupKey string        `json:"backup_key,omitempty"`
	Duration  time.Duration `json:"duration_ns"`
	Checks    []Check       `json:"checks"`
}

// RunDrill downloads the newest complete backup and proves it can be restored,
// into a temporary directory, never the live database. With a drill identity
// configured it decrypts and integrity-checks; without one it verifies only
// the ciphertext checksum and manifest and reports itself as partial.
func (s *Service) RunDrill(ctx context.Context) (DrillResult, error) {
	if !s.drillMu.TryLock() {
		return DrillResult{}, ErrBusy
	}
	defer s.drillMu.Unlock()

	start := s.now()
	res := DrillResult{At: start.UTC().Truncate(time.Second)}
	err := s.drill(ctx, &res)
	res.Duration = s.now().Sub(start)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return res, err
		}
		res.OK, res.Detail = false, err.Error()
	}
	if recErr := s.Store.RecordCPDRDrill(context.WithoutCancel(ctx), res.At, res.OK, res.Partial, res.Detail, res.Duration.Milliseconds()); recErr != nil {
		return res, fmt.Errorf("record drill: %w", recErr)
	}
	return res, err
}

func (s *Service) drill(ctx context.Context, res *DrillResult) error {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if cfg.TargetID == "" {
		return ErrNotConfigured
	}
	bucket, _, err := s.Dest.Open(ctx, cfg.TargetID)
	if err != nil {
		return fmt.Errorf("open destination: %w", err)
	}
	prefix := Prefix + "/"
	if cfg.InstallID != "" {
		prefix = installPrefix(cfg.InstallID)
	}
	key, err := LatestKey(ctx, bucket, prefix)
	if err != nil {
		return err
	}
	res.BackupKey = key
	tmp, err := s.tempDir()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	src := NewBucketSource(bucket, key)
	if len(s.DrillIdentities) == 0 {
		return partialDrill(ctx, src, tmp, res)
	}
	rep, err := Restore(ctx, src, RestoreOptions{LivePath: filepath.Join(tmp, "levelrail.db"), Identities: s.DrillIdentities, ForceInstallID: true, Now: s.Now})
	res.Checks = rep.Checks
	if err != nil {
		return err
	}
	c, err := sanity(ctx, filepath.Join(tmp, "levelrail.db"), rep.Manifest)
	res.Checks = append(res.Checks, c)
	if err != nil {
		return err
	}
	res.OK, res.Detail = true, fmt.Sprintf("restored %s and it passed every check: %s", key, c.Detail)
	return nil
}

func partialDrill(ctx context.Context, src Source, tmp string, res *DrillResult) error {
	m, err := src.ReadManifest(ctx)
	if err != nil {
		return err
	}
	dst := filepath.Join(tmp, "backup.db.age")
	if err := src.Fetch(ctx, dst, m.SizeBytes); err != nil {
		return err
	}
	size, sum, err := store.FileSHA256(dst)
	if err != nil {
		return err
	}
	if size != m.SizeBytes || sum != m.SHA256 {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, m.SHA256, sum)
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return err
	}
	if m.SchemaVersion > latest {
		return fmt.Errorf("%w: backup is at version %d, this binary supports up to %d", ErrNewerSchema, m.SchemaVersion, latest)
	}
	res.Partial, res.OK = true, true
	res.Checks = []Check{
		{Name: "manifest", OK: true, Detail: fmt.Sprintf("schema %d, install %s", m.SchemaVersion, m.InstallID)},
		{Name: "checksum", OK: true, Detail: "sha256 matches the manifest"},
	}
	res.Detail = "partial drill: checksum and manifest verified, decryption not tested because no drill identity is configured"
	return nil
}

// sanity checks that a restored database has its migrations and some data.
func sanity(ctx context.Context, path string, m Manifest) (Check, error) {
	c := Check{Name: "row_counts"}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return c, fmt.Errorf("open restored database: %w", err)
	}
	defer func() { _ = db.Close() }()
	var migrations int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&migrations); err != nil {
		return c, fmt.Errorf("count migrations: %w", err)
	}
	if migrations != m.MigrationsApplied {
		return c, fmt.Errorf("restored database has %d applied migrations, manifest says %d", migrations, m.MigrationsApplied)
	}
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return c, fmt.Errorf("list tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return c, fmt.Errorf("list tables: %w", err)
		}
		tables = append(tables, n)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return c, fmt.Errorf("list tables: %w", err)
	}
	_ = rows.Close()
	if len(tables) == 0 {
		return c, errors.New("restored database has no tables")
	}
	var total int64
	for _, t := range tables {
		var n int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+strings.ReplaceAll(t, `"`, `""`)+`"`).Scan(&n); err != nil {
			return c, fmt.Errorf("count rows in %s: %w", t, err)
		}
		total += n
	}
	c.OK, c.Detail = true, fmt.Sprintf("%d tables, %d rows, %d migrations", len(tables), total, migrations)
	return c, nil
}
