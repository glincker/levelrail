package buildkitd

import (
	"fmt"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/build/cachebudget"
)

const (
	mib = int64(1024 * 1024)

	defaultGCSourceKeep  = 48 * time.Hour
	defaultGCSourceMaxMB = 512
	// defaultCPUShares is a quarter of the engine weight so builds yield to the control plane.
	defaultCPUShares = 256
)

// gcSizing is the resolved byte budget written into the three GC rules.
type gcSizing struct {
	ReservedBytes int64
	MaxUsedBytes  int64
	MinFreePct    int64
	SourceMaxByte int64
}

// sizeGC resolves the byte budget for the GC rules from the disk holding
// BuildKit state; the shared sizing rule lives in cachebudget.
func (c Config) sizeGC(total int64) gcSizing {
	s := cachebudget.Size(total, c.GCKeepMB, c.GCReservedMB, c.GCMinFreePct)
	return gcSizing{ReservedBytes: s.ReservedBytes, MaxUsedBytes: s.MaxUsedBytes, MinFreePct: s.MinFreePct, SourceMaxByte: c.GCSourceMaxMB * mib}
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
