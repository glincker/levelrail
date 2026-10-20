//go:build chaos

package chaos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/failure"
)

// Every injected failure must classify into a named condition that carries a cause and a suggested fix.
func TestChaos_EveryFailureHasAConditionAndFix(t *testing.T) {
	tests := []struct {
		name string
		in   failure.Input
		code string
	}{
		{"docker daemon dies mid deploy", failure.Input{Status: "failed", Error: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock"}, failure.CodeDockerUnreachable},
		{"ENOSPC on a data path", failure.Input{Status: "failed", Error: "write /var/lib/docker/tmp: no space left on device"}, failure.CodeDiskFull},
		{"refused by the disk guard", failure.Input{Status: "failed", Error: (&diskguard.LowDiskError{Op: diskguard.OpPull, Volume: diskguard.VolumeDocker, Path: "/var/lib/docker", FreeBytes: 10 << 20, NeedBytes: 512 << 20}).Error()}, failure.CodeDiskFull},
		{"OOM kill during readiness", failure.Input{Status: "failed", Error: "OOMKilledDuringReadiness: container exited with code 137"}, failure.CodeOOMKilled},
		{"corrupt image layer", failure.Input{Status: "failed", Error: "failed to register layer: archive/tar: invalid tar header"}, failure.CodeImageCorrupt},
		{"missing image", failure.Input{Status: "failed", Error: "manifest unknown: manifest unknown"}, failure.CodeImagePullFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := failure.Classify(tc.in, failure.Options{})
			if !ok || got.Code != tc.code {
				t.Fatalf("Classify() = %q, %v, want %q", got.Code, ok, tc.code)
			}
			if got.Cause == "" || got.SuggestedFix == "" {
				t.Fatalf("condition %q has no cause or fix: %+v", got.Code, got)
			}
		})
	}
}

// The disk guard refuses before a disk fills, releases its reserve so a commit can still land, and restores it once space returns.
func TestChaos_DiskFillRefusesThenRecovers(t *testing.T) {
	dir := t.TempDir()
	free := int64(4096) << 20
	stat := func(string) (int64, int64, error) { return free, int64(8192) << 20, nil }
	th := diskguard.Thresholds{LowFree: 1024 << 20, CriticalFree: 512 << 20, Reserve: 1 << 20}
	g := diskguard.New(dir, "", th, stat, nil)
	ctx := context.Background()

	if err := g.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if g.Status().ReserveBytes != th.Reserve {
		t.Fatalf("reserve = %d, want %d", g.Status().ReserveBytes, th.Reserve)
	}

	free = 100 << 20
	_ = g.Tick(ctx)
	var low *diskguard.LowDiskError
	for _, op := range []diskguard.Op{diskguard.OpBuild, diskguard.OpBackup} {
		if err := g.Refuse(op); !errors.As(err, &low) {
			t.Fatalf("Refuse(%s) = %v, want a LowDiskError", op, err)
		}
	}
	if g.Status().ReserveBytes != 0 {
		t.Fatal("reserve still held at the critical floor")
	}

	free = 4096 << 20
	_ = g.Tick(ctx)
	if err := g.Refuse(diskguard.OpBuild); err != nil {
		t.Fatalf("still refusing after recovery: %v", err)
	}
	if g.Status().ReserveBytes != th.Reserve {
		t.Fatal("reserve not restored after recovery")
	}
}

// A write that dies with ENOSPC mid transaction rolls back and leaves the database intact and writable.
func TestChaos_SQLiteSurvivesNoSpaceMidTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO t (v) VALUES ('before')`); err != nil {
		t.Fatal(err)
	}

	write := func(inject error) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO t (v) VALUES ('during')`); err != nil {
			_ = tx.Rollback()
			return err
		}
		if inject != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit: %w", inject)
		}
		return tx.Commit()
	}

	err = write(&os.PathError{Op: "write", Path: path + "-wal", Err: syscall.ENOSPC})
	if !diskguard.IsNoSpace(err) {
		t.Fatalf("injected error not recognised as ENOSPC: %v", err)
	}
	var check string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		t.Fatalf("integrity_check = %q, %v", check, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM t`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows after rollback = %d, %v, want 1 (no half-applied write)", n, err)
	}
	if err := write(nil); err != nil {
		t.Fatalf("database not writable after the failure: %v", err)
	}
}
