package main

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

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
