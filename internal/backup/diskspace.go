package backup

import (
	"fmt"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/diskspace"
)

// envMinDiskSpaceMB overrides defaultMinDiskSpaceMB, the free-space floor
// Runner/RestoreRunner check before starting a dump/upload or
// download/restore.
const envMinDiskSpaceMB = "APP_MIN_BACKUP_DISK_MB"

// defaultMinDiskSpaceMB is smaller than internal/build's own default
// (1GiB): a database dump and a volume archive both stream directly
// between the source (a container) and the destination bucket, never
// touching local disk (see runDumpAndUpload/downloadAndRestore), so this
// only guards the one real local write a backup or restore attempt makes:
// its own store.BackupHistory/RestoreHistory bookkeeping row, in the
// control plane's SQLite database.
const defaultMinDiskSpaceMB int64 = 256

// diskFreeFunc is diskspace.Free by default; tests substitute a fake to
// exercise checkDiskSpace's blocking/allowing branches without needing a
// real near-full filesystem.
var diskFreeFunc = diskspace.Free

func minDiskSpaceBytes() int64 {
	mb := defaultMinDiskSpaceMB
	if v := os.Getenv(envMinDiskSpaceMB); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
			mb = parsed
		}
	}
	return mb * 1024 * 1024
}

// checkDiskSpace fails fast with a specific, actionable error when dir has
// less than the configured minimum free space, rather than letting a
// backup or restore proceed and have its history bookkeeping write fail
// confusingly, or corrupt the control plane's own WAL-mode SQLite
// database, partway through. dir == "" skips the check entirely: the same
// "absence is not an error" shape internal/api's WithDataDir already
// establishes for an unconfigured signal. A statfs failure also degrades
// to "allow the operation to proceed" rather than blocking it, matching
// internal/build's own checkDiskSpace.
func checkDiskSpace(dir string) error {
	if dir == "" {
		return nil
	}
	free, err := diskFreeFunc(dir)
	if err != nil {
		return nil
	}
	minBytes := minDiskSpaceBytes()
	if free < minBytes {
		return fmt.Errorf("backup: insufficient disk space at %q: %d bytes free, need at least %d bytes (set %s to override)", dir, free, minBytes, envMinDiskSpaceMB)
	}
	return nil
}

// resolveWorkDir returns explicit if set, else APP_DATA_DIR (the control
// plane's own data directory, the same variable cmd/levelrail already
// resolves at startup and internal/api's WithDataDir accepts), else "":
// no directory to check, so checkDiskSpace is a no-op.
func resolveWorkDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return os.Getenv("APP_DATA_DIR")
}
