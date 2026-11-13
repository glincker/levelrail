package buildkitd

import (
	"fmt"
	"strings"
	"time"
)

const (
	mib = int64(1024 * 1024)

	defaultGCReservedMB    = 2048
	defaultGCMaxUsedCapMB  = 10240
	defaultGCMaxUsedPct    = 20
	defaultGCUnknownDiskMB = 5120
	defaultGCMinFreePct    = 15
	defaultGCSourceKeep    = 48 * time.Hour
	defaultGCSourceMaxMB   = 512
)

// gcSizing is the resolved byte budget written into the three GC rules.
type gcSizing struct {
	ReservedBytes int64
	MaxUsedBytes  int64
	MinFreePct    int64
	SourceMaxByte int64
}

// sizeGC derives the cache budget from the disk holding BuildKit state.
// An unknown disk (total <= 0) gets a small fixed budget so a failed statfs
// never leaves the cache unbounded. Explicit overrides win, but reserved is
// clamped to max used because BuildKit rejects a floor above the cap.
func (c Config) sizeGC(total int64) gcSizing {
	maxUsed := c.GCKeepMB * mib
	if maxUsed <= 0 {
		if total > 0 {
			maxUsed = total * defaultGCMaxUsedPct / 100
			if capBytes := int64(defaultGCMaxUsedCapMB) * mib; maxUsed > capBytes {
				maxUsed = capBytes
			}
		} else {
			maxUsed = int64(defaultGCUnknownDiskMB) * mib
		}
	}
	reserved := c.GCReservedMB * mib
	if reserved <= 0 {
		reserved = int64(defaultGCReservedMB) * mib
		if reserved > maxUsed/2 {
			reserved = maxUsed / 2
		}
	}
	if reserved > maxUsed {
		reserved = maxUsed
	}
	minFree := c.GCMinFreePct
	if minFree <= 0 {
		minFree = defaultGCMinFreePct
	}
	return gcSizing{ReservedBytes: reserved, MaxUsedBytes: maxUsed, MinFreePct: minFree, SourceMaxByte: c.GCSourceMaxMB * mib}
}

// gcPolicyTOML renders the three rules: short lived source data, anything
// idle past the keep duration, then a capped catch-all. keepBytes is
// deprecated in BuildKit v0.34 and acts as a never-reclaimed floor.
func (c Config) gcPolicyTOML(total int64) string {
	s := c.sizeGC(total)
	var b strings.Builder
	fmt.Fprintf(&b, "  [[worker.oci.gcpolicy]]\n    filters = [\"type==source.local\", \"type==source.git.checkout\"]\n    keepDuration = %d\n    maxUsedSpace = \"%dMB\"\n",
		int64(c.GCSourceKeep.Seconds()), s.SourceMaxByte/mib)
	fmt.Fprintf(&b, "  [[worker.oci.gcpolicy]]\n    keepDuration = %d\n", int64(c.GCKeepFor.Seconds()))
	fmt.Fprintf(&b, "  [[worker.oci.gcpolicy]]\n    all = true\n    reservedSpace = \"%dMB\"\n    maxUsedSpace = \"%dMB\"\n    minFreeSpace = \"%d%%\"\n",
		s.ReservedBytes/mib, s.MaxUsedBytes/mib, s.MinFreePct)
	return b.String()
}
