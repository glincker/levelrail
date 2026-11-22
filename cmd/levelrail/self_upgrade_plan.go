package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/diskfree"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// dirFreeBytes is the space an unprivileged writer can still use in dir.
func dirFreeBytes(dir string) (int64, error) {
	info, err := diskfree.Stat(dir)
	return info.Free, err
}

// hostDiskReport measures the data dir against what an upgrade keeps on disk.
func hostDiskReport(dataDir, live, exe string) selfupgrade.DiskReport {
	info, err := diskfree.Stat(dataDir)
	return selfupgrade.PlanDisk(info.Free, err, fileSize(live), fileSize(exe), os.Getenv("APP_UPGRADE_MIN_FREE_BYTES"))
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

// fallbackReleaseURL reads the live fallback download setting; an unreadable database means no fallback.
func fallbackReleaseURL(ctx context.Context, live string) string {
	db, err := store.Open(ctx, live)
	if err != nil {
		return ""
	}
	defer func() { _ = db.Close() }()
	return strings.TrimSpace(runtimesettings.New(db, nil).String(ctx, runtimesettings.KeyUpdatesFallbackBaseURL))
}
