package selfupgrade

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvVerifyFreeFactor scales the backup size into the free space a scratch
// restore needs; unset or invalid means DefaultVerifyFreeFactor.
const EnvVerifyFreeFactor = "APP_UPGRADE_VERIFY_FREE_FACTOR"

// DefaultVerifyFreeFactor is the free-space multiple of the backup size.
const DefaultVerifyFreeFactor = 2

// ErrBackupVerify marks a pre-upgrade backup that did not restore cleanly.
var ErrBackupVerify = errors.New("the pre-upgrade backup did not restore cleanly")

// CoreTables are the tables whose row counts must survive a restore.
var CoreTables = []string{"apps", "users", "nodes", "runtime_settings"}

// BackupVerifyOptions describe one scratch restore of a pre-upgrade backup.
type BackupVerifyOptions struct {
	Snapshot string
	// ScratchBase is where the temporary directory is made; it should share a disk with the backup.
	ScratchBase string
	// Restore is the rollback restore code, pointed at a scratch database path.
	Restore   func(livePath, snapshot string) (string, error)
	FreeBytes func(dir string) (int64, error)
	// KeyFiles are hashed before and after to prove the check never touched them.
	KeyFiles []string
}

// VerifyBackup restores the snapshot into a scratch directory, checks it and
// removes the scratch copy. The returned text is the plain report; a skipped
// check (short disk) returns a warning and a nil error.
func VerifyBackup(ctx context.Context, o BackupVerifyOptions) (string, error) {
	fi, err := os.Stat(o.Snapshot)
	if err != nil {
		return "", fmt.Errorf("%w: backup file unreadable: %w", ErrBackupVerify, err)
	}
	need := fi.Size() * verifyFreeFactor()
	if o.FreeBytes != nil {
		if free, ferr := o.FreeBytes(o.ScratchBase); ferr == nil && free < need {
			return fmt.Sprintf("restore check skipped: %d MiB free, %d MiB needed (backup size times %d), so this backup is NOT proven to restore", free>>20, need>>20, verifyFreeFactor()), nil
		}
	}
	before := hashFiles(o.KeyFiles)
	scratch, err := os.MkdirTemp(o.ScratchBase, ".verify-backup-*")
	if err != nil {
		return "", fmt.Errorf("%w: create scratch directory: %w", ErrBackupVerify, err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	live := filepath.Join(scratch, "levelrail.db")
	if err := os.WriteFile(live, nil, 0o600); err != nil {
		return "", fmt.Errorf("%w: prepare scratch database: %w", ErrBackupVerify, err)
	}
	if _, err := o.Restore(live, o.Snapshot); err != nil {
		return "", fmt.Errorf("%w: scratch restore failed: %w", ErrBackupVerify, err)
	}
	if err := checkRestoredCopy(ctx, o.Snapshot, live); err != nil {
		return "", fmt.Errorf("%w: %w", ErrBackupVerify, err)
	}
	want, got, err := compareCoreTables(ctx, o.Snapshot, live)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrBackupVerify, err)
	}
	after := hashFiles(o.KeyFiles)
	for p, h := range before {
		if after[p] != h {
			return "", fmt.Errorf("%w: key file %s changed during the check", ErrBackupVerify, filepath.Base(p))
		}
	}
	return fmt.Sprintf("restored to a scratch copy, integrity ok, rows match (%s)%s", countSummary(want, got), keySummary(o.KeyFiles, before)), nil
}

func verifyFreeFactor() int64 {
	if n, err := strconv.ParseInt(os.Getenv(EnvVerifyFreeFactor), 10, 64); err == nil && n > 0 {
		return n
	}
	return DefaultVerifyFreeFactor
}

func checkRestoredCopy(ctx context.Context, snapshot, restored string) error {
	_, snapSum, err := store.FileSHA256(snapshot)
	if err != nil {
		return err
	}
	if raw, err := os.ReadFile(snapshot + ".sha256"); err == nil { //nolint:gosec // sidecar written next to the backup
		if rec := strings.TrimSpace(string(raw)); rec != "" && rec != snapSum {
			return errors.New("the backup file no longer matches its recorded checksum")
		}
	}
	_, gotSum, err := store.FileSHA256(restored)
	if err != nil {
		return err
	}
	if gotSum != snapSum {
		return errors.New("the restored copy differs from the backup file")
	}
	if _, err := store.InspectSnapshot(ctx, restored); err != nil {
		return fmt.Errorf("restored database failed its integrity check: %w", err)
	}
	return nil
}

func compareCoreTables(ctx context.Context, snapshot, restored string) (map[string]int64, map[string]int64, error) {
	want, err := coreCounts(ctx, snapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("read backup: %w", err)
	}
	got, err := coreCounts(ctx, restored)
	if err != nil {
		return nil, nil, fmt.Errorf("read restored copy: %w", err)
	}
	for _, t := range CoreTables {
		if want[t] != got[t] {
			return nil, nil, fmt.Errorf("table %s has %d rows in the backup but %d after restore", t, want[t], got[t])
		}
	}
	return want, got, nil
}

func coreCounts(ctx context.Context, path string) (map[string]int64, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	out := map[string]int64{}
	for _, t := range CoreTables {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, t).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var n int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&n); err != nil { //nolint:gosec // t comes from the fixed CoreTables list
			return nil, fmt.Errorf("count %s: %w", t, err)
		}
		out[t] = n
	}
	return out, nil
}

func hashFiles(paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p) //nolint:gosec // fixed key file names under the operator's data dir
		if err != nil {
			out[p] = ""
			continue
		}
		sum := sha256.Sum256(raw)
		out[p] = hex.EncodeToString(sum[:])
	}
	return out
}

func countSummary(want, got map[string]int64) string {
	parts := make([]string, 0, len(CoreTables))
	for _, t := range CoreTables {
		if _, ok := want[t]; ok {
			parts = append(parts, fmt.Sprintf("%s=%d", t, got[t]))
		}
	}
	return strings.Join(parts, " ")
}

func keySummary(paths []string, hashes map[string]string) string {
	if len(paths) == 0 {
		return ""
	}
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		if h := hashes[p]; h != "" {
			parts = append(parts, filepath.Base(p)+" present ("+h[:12]+")")
		} else {
			parts = append(parts, filepath.Base(p)+" absent")
		}
	}
	return "; " + strings.Join(parts, ", ")
}
