package main

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dirFreeBytes is the space an unprivileged writer can still use in dir.
func dirFreeBytes(dir string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil //nolint:gosec // statfs fields are non-negative
}

// hostDiskReport measures the data dir against what an upgrade keeps on disk.
func hostDiskReport(dataDir, live, exe string) selfupgrade.DiskReport {
	var st syscall.Statfs_t
	err := syscall.Statfs(dataDir, &st)
	free := int64(st.Bavail) * int64(st.Bsize) //nolint:gosec // statfs fields are non-negative
	return selfupgrade.PlanDisk(free, err, fileSize(live), fileSize(exe), os.Getenv("APP_UPGRADE_MIN_FREE_BYTES"))
}

func fileSize(path string) int64 {
	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// restoreCommandFor is the host command that puts a pre-upgrade backup back;
// NAME stands for the backup file the upgrade reports.
func restoreCommandFor(program, dataDir string) string {
	return "sudo systemctl stop " + program + " && sudo " + program + " restore-db " + filepath.Join(dataDir, cpbackup.DirName, "NAME")
}

func onOff(b bool) string {
	if b {
		return "on, restored to a scratch copy and checked before the swap"
	}
	return "off"
}

// verifyBackupEnabled reads the live setting; an unreadable database leaves the safe default on.
func verifyBackupEnabled(ctx context.Context, live string) bool {
	db, err := store.Open(ctx, live)
	if err != nil {
		return true
	}
	defer func() { _ = db.Close() }()
	return runtimesettings.New(db, nil).Bool(ctx, runtimesettings.KeyUpgradeVerifyBackup)
}
