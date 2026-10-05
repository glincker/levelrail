package backup

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// envPITRBaseBackupKeep sets how many succeeded base backups are kept per
// database; older ones, their bucket objects and the WAL only they needed
// are removed. 0 disables pruning.
const envPITRBaseBackupKeep = "APP_PITR_BASE_BACKUP_KEEP"

const defaultPITRBaseBackupKeep = 3

// PITRMaintenanceStore is the store surface PITRMaintainer needs.
type PITRMaintenanceStore interface {
	ListBaseBackupHistory(ctx context.Context, databaseName string) ([]store.BaseBackupHistory, error)
	DeleteBaseBackupHistory(ctx context.Context, id string) error
}

// PITRMaintainer keeps a PITR-enabled database's disk and bucket bounded:
// without it the local WAL archive and the base backups grow forever.
type PITRMaintainer struct {
	Store   PITRMaintenanceStore
	Resolve func(ctx context.Context, targetID string) (Destination, error)
	Deleter Deleter
	Runtime Runtime
	// Keep overrides the env default when positive.
	Keep   int
	Logger *slog.Logger
}

func (m *PITRMaintainer) keep() int {
	if m.Keep > 0 {
		return m.Keep
	}
	if v := os.Getenv(envPITRBaseBackupKeep); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultPITRBaseBackupKeep
}

// Prune deletes succeeded base backups beyond the newest keep(), then the
// archived WAL older than the oldest base backup that remains: a restore can
// never start before that backup, so earlier segments are dead weight.
func (m *PITRMaintainer) Prune(ctx context.Context, databaseName, containerName string) error {
	keep := m.keep()
	if keep == 0 {
		return nil
	}
	all, err := m.Store.ListBaseBackupHistory(ctx, databaseName)
	if err != nil {
		return fmt.Errorf("list base backups for %q: %w", databaseName, err)
	}
	var succeeded []store.BaseBackupHistory
	for _, h := range all {
		if h.Status == store.BackupStatusSucceeded {
			succeeded = append(succeeded, h)
		}
	}
	if len(succeeded) == 0 {
		return nil
	}

	cut := min(keep, len(succeeded))
	remaining := append([]store.BaseBackupHistory(nil), succeeded[:cut]...)
	for _, h := range succeeded[cut:] {
		if err := m.deleteBaseBackup(ctx, h); err != nil {
			// A base backup a PITR restore record still references cannot
			// be deleted: it stays, and so does the WAL it needs.
			m.log().Info("backup: base backup kept by retention", slog.String("database", databaseName), slog.String("id", h.ID), slog.String("reason", err.Error()))
			remaining = append(remaining, h)
		}
	}

	oldestKept := remaining[len(remaining)-1]
	if oldestKept.LSN == "" {
		return nil
	}
	if err := PruneWALArchive(ctx, m.Runtime, containerName, oldestKept.LSN); err != nil {
		return fmt.Errorf("prune wal archive for %q: %w", databaseName, err)
	}
	return nil
}

// deleteBaseBackup removes the row first: a foreign key from restore history
// can refuse it, and the bucket object must not be lost in that case.
func (m *PITRMaintainer) deleteBaseBackup(ctx context.Context, h store.BaseBackupHistory) error {
	if err := m.Store.DeleteBaseBackupHistory(ctx, h.ID); err != nil {
		return err
	}
	if m.Deleter != nil && m.Resolve != nil {
		dest, err := m.Resolve(ctx, h.TargetID)
		if err != nil {
			m.log().Warn("backup: base backup object left in bucket", slog.String("id", h.ID), slog.String("error", err.Error()))
			return nil
		}
		if err := m.Deleter.Delete(ctx, dest, h.ObjectKey); err != nil {
			m.log().Warn("backup: base backup object left in bucket", slog.String("id", h.ID), slog.String("key", h.ObjectKey), slog.String("error", err.Error()))
		}
	}
	return nil
}

func (m *PITRMaintainer) log() *slog.Logger {
	if m.Logger != nil {
		return m.Logger
	}
	return slog.Default()
}

var lsnPattern = regexp.MustCompile(`^[0-9A-Fa-f]+/[0-9A-Fa-f]+$`)

// PruneWALArchive removes archived WAL segments older than the one holding
// startLSN. Compares the 16 hex digit segment number so a timeline change
// (which only alters the 8 digit prefix) cannot delete segments a base
// backup on an older timeline still needs.
func PruneWALArchive(ctx context.Context, rt Runtime, containerName, startLSN string) error {
	if rt == nil {
		return nil
	}
	if !lsnPattern.MatchString(startLSN) {
		return fmt.Errorf("refusing to prune with malformed LSN %q", startLSN)
	}
	script := `cut=$(psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT pg_walfile_name('` + startLSN + `')") || exit 1
cut=${cut#????????}
[ ${#cut} -eq 16 ] || exit 1
for f in ` + postgresWALArchivePath + `/????????????????????????; do
  [ -f "$f" ] || continue
  seg=${f##*/}
  case $seg in *[!0-9A-F]*) continue;; esac
  seg=${seg#????????}
  if expr "$seg" "<" "$cut" >/dev/null; then rm -f "$f"; fi
done`
	rc, err := rt.Exec(ctx, containerName, []string{"sh", "-c", script})
	if err != nil {
		return fmt.Errorf("exec wal prune: %w", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("wal prune: %w", err)
	}
	return nil
}
