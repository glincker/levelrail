package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrComposeRestoreBusy means a restore lock for the service is still live.
var ErrComposeRestoreBusy = errors.New("a restore is already running for this service")

const defaultComposeRestoreTimeout = 2 * time.Hour

// ComposeRestoreTimeout is the deadline for one in-place restore; the
// environment value only seeds the default.
func ComposeRestoreTimeout() time.Duration {
	if v, err := time.ParseDuration(os.Getenv("APP_COMPOSE_RESTORE_TIMEOUT")); err == nil && v > 0 {
		return v
	}
	return defaultComposeRestoreTimeout
}

// AcquireComposeRestoreLock takes a lock file in the data directory so a
// restart cannot start a second restore while a hung exec may still run.
// A lock older than ttl is treated as dead and replaced.
func AcquireComposeRestoreLock(app, service, runID string, ttl time.Duration) (release func(), err error) {
	dir := resolveWorkDir("")
	if dir == "" {
		dir = os.TempDir()
	}
	sum := sha256.Sum256([]byte(app + "/" + service))
	path := filepath.Join(dir, "compose-restore-"+hex.EncodeToString(sum[:8])+".lock")
	for attempt := 0; attempt < 2; attempt++ {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // hashed file name inside the data dir
		if openErr == nil {
			_, _ = fmt.Fprintf(f, "%s %d\n", runID, os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(openErr, os.ErrExist) {
			return nil, fmt.Errorf("take restore lock: %w", openErr)
		}
		st, statErr := os.Stat(path)
		if statErr == nil && time.Since(st.ModTime()) < ttl {
			return nil, ErrComposeRestoreBusy
		}
		_ = os.Remove(path)
	}
	return nil, ErrComposeRestoreBusy
}
