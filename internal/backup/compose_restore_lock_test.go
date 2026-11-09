package backup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComposeRestoreLockBlocksSecondAndExpires(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	release, err := AcquireComposeRestoreLock("app", "db", "r1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireComposeRestoreLock("app", "db", "r2", time.Hour); !errors.Is(err, ErrComposeRestoreBusy) {
		t.Fatalf("second lock err = %v", err)
	}
	// A zero ttl treats the existing file as dead, as after a crashed run.
	r2, err := AcquireComposeRestoreLock("app", "db", "r3", 0)
	if err != nil {
		t.Fatalf("stale lock not replaced: %v", err)
	}
	r2()
	release()
	matches, _ := filepath.Glob(filepath.Join(os.Getenv("APP_DATA_DIR"), "compose-restore-*.lock"))
	if len(matches) != 0 {
		t.Fatalf("lock left behind: %v", matches)
	}
}

func TestComposeRestoreTimeoutEnvSeedsDefault(t *testing.T) {
	t.Setenv("APP_COMPOSE_RESTORE_TIMEOUT", "5m")
	if got := ComposeRestoreTimeout(); got != 5*time.Minute {
		t.Fatalf("timeout = %v", got)
	}
	t.Setenv("APP_COMPOSE_RESTORE_TIMEOUT", "junk")
	if got := ComposeRestoreTimeout(); got != defaultComposeRestoreTimeout {
		t.Fatalf("fallback = %v", got)
	}
}
