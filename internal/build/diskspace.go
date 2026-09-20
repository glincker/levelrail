package build

import (
	"fmt"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/diskspace"
)

// envMinDiskSpaceMB overrides defaultMinDiskSpaceMB, the free-space floor
// a build's context (and local cache, when configured) must clear before
// Build/BuildRailpack/SolveRemote starts.
const envMinDiskSpaceMB = "APP_MIN_BUILD_DISK_MB"

// defaultMinDiskSpaceMB is deliberately the same order of magnitude as
// doctorCheckDiskSpace's own default warning floor (internal/api/doctor.go,
// 1GiB): a build that starts with less room than that is very likely to
// hit a raw ENOSPC partway through rather than complete.
const defaultMinDiskSpaceMB int64 = 1024

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

// checkDiskSpace fails fast with a specific, actionable error when any of
// dirs has less than the configured minimum free space, rather than
// letting a build proceed and fail confusingly mid-solve with a raw disk
// I/O error. Empty entries in dirs are skipped (an unset cache dir, for
// instance). A statfs failure degrades to "allow the build to proceed"
// rather than blocking it, the same "unknown is not itself a reason to
// fail" choice doctorCheckDiskSpace already makes for an unreadable path.
func checkDiskSpace(dirs ...string) error {
	minBytes := minDiskSpaceBytes()
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		free, err := diskFreeFunc(dir)
		if err != nil {
			continue
		}
		if free < minBytes {
			return fmt.Errorf("build: insufficient disk space at %q: %d bytes free, need at least %d bytes (set %s to override)", dir, free, minBytes, envMinDiskSpaceMB)
		}
	}
	return nil
}
