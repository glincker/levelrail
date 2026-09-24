package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultControlPlaneBackupInterval = 24 * time.Hour
	defaultControlPlaneBackupRetain   = 7
)

func controlPlaneBackupInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CONTROL_PLANE_BACKUP_INTERVAL")
	if raw == "" {
		return defaultControlPlaneBackupInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		logger.Warn("invalid APP_CONTROL_PLANE_BACKUP_INTERVAL, using the default", slog.String("value", raw))
		return defaultControlPlaneBackupInterval
	}
	return d
}

func controlPlaneBackupRetain(logger *slog.Logger) int {
	raw := os.Getenv("APP_CONTROL_PLANE_BACKUP_RETAIN")
	if raw == "" {
		return defaultControlPlaneBackupRetain
	}
	var n int
	if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 0 {
		logger.Warn("invalid APP_CONTROL_PLANE_BACKUP_RETAIN, using the default", slog.String("value", raw))
		return defaultControlPlaneBackupRetain
	}
	return n
}

// preMigrateSnapshotHook snapshots an existing database before pending
// migrations run. A failed snapshot is logged, not fatal, so a full disk
// cannot stop an upgrade the operator asked for.
func preMigrateSnapshotHook(dataDir string) store.PreMigrateHook {
	return func(ctx context.Context, db *store.DB, from, to int) {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		info, err := cpbackup.NewManager(db, dataDir).Create(ctx)
		if err != nil {
			logger.Error("pre-migration snapshot failed, continuing with migration", slog.Int("from_version", from), slog.Int("to_version", to), slog.String("error", err.Error()))
			return
		}
		logger.Info("pre-migration snapshot taken", slog.String("name", info.Name), slog.Int("from_version", from), slog.Int("to_version", to))
	}
}

// runRestoreDB implements "levelrail restore-db <file>": an offline swap of
// the control plane database for a verified backup. The current database is
// kept beside it as a timestamped .before-restore copy.
func runRestoreDB(ctx context.Context, args []string, dataDir string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: levelrail restore-db <backup-file>")
	}
	src := args[0]

	_, _ = fmt.Fprintln(stdout, "WARNING: stop the control plane before restoring. Restoring under a running server corrupts state.")

	version, err := store.InspectSnapshot(ctx, src)
	if err != nil {
		return fmt.Errorf("backup file rejected: %w", err)
	}
	latest, err := store.MaxSchemaVersion()
	if err != nil {
		return err
	}
	if version > latest {
		return fmt.Errorf("backup is at schema version %d, newer than the %d this binary supports; use a newer release", version, latest)
	}

	live := filepath.Join(dataDir, storeFilename)
	stamp := time.Now().UTC().Format("20060102T150405Z")
	if _, err := os.Stat(live); err == nil {
		aside := live + ".before-restore-" + stamp
		if err := os.Rename(live, aside); err != nil {
			return fmt.Errorf("move current database aside: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "current database kept as %s\n", aside)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(live + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s file: %w", suffix, err)
		}
	}

	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled data dir
		return err
	}
	if err := copyFile(src, live); err != nil {
		return fmt.Errorf("put backup in place: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "restored %s (schema version %d) to %s\n", src, version, live)
	_, _ = fmt.Fprintln(stdout, "The master key is not part of a backup: secrets stay readable only with the master key that encrypted them.")
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // operator-supplied CLI argument
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // operator-controlled data dir
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
