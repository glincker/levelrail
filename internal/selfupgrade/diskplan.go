package selfupgrade

import (
	"fmt"
	"strconv"
)

// Copies of the database one upgrade keeps at once: the backup, the restore
// point copy and the scratch copy the migration dry run works on.
const dbCopiesPerUpgrade = 3

// DiskReport says whether the data directory has room for an upgrade.
type DiskReport struct {
	// Known is false when the free space could not be read.
	Known         bool  `json:"known"`
	FreeBytes     int64 `json:"free_bytes"`
	RequiredBytes int64 `json:"required_bytes"`
	DBBytes       int64 `json:"db_bytes"`
	OK            bool  `json:"ok"`
}

// PlanDisk estimates the space an upgrade needs: database copies, the new
// binary and the kept previous one. floorEnv is the optional operator floor
// (APP_UPGRADE_MIN_FREE_BYTES) so the estimate never undercuts it.
func PlanDisk(free int64, freeErr error, dbBytes, binBytes int64, floorEnv string) DiskReport {
	need := dbBytes*dbCopiesPerUpgrade + binBytes*2
	if f, err := strconv.ParseInt(floorEnv, 10, 64); err == nil && f > need {
		need = f
	}
	r := DiskReport{DBBytes: dbBytes, RequiredBytes: need}
	if freeErr != nil {
		return r
	}
	r.Known, r.FreeBytes, r.OK = true, free, free >= need
	return r
}

// Summary is the one-line human form.
func (r DiskReport) Summary() string {
	if !r.Known {
		return "free space could not be read"
	}
	verdict := "enough"
	if !r.OK {
		verdict = "NOT ENOUGH"
	}
	return fmt.Sprintf("%d MiB free, about %d MiB needed for the backup, restore point and new binary (%s)", r.FreeBytes>>20, r.RequiredBytes>>20, verdict)
}
