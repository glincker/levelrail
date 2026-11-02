package runtimesettings

import (
	"fmt"
	"strconv"
)

// KeyBuildsMemoryLimitMB caps the memory one build may use; 0 means auto.
const KeyBuildsMemoryLimitMB = "builds.memory_limit_mb"

const (
	buildMemoryMinMB = 256
	buildMemoryMaxMB = 1048576
)

// buildMemoryCheck allows 0 (auto) or an explicit cap inside the safe range.
func buildMemoryCheck(canonical string) error {
	n, err := strconv.Atoi(canonical)
	if err != nil || (n != 0 && (n < buildMemoryMinMB || n > buildMemoryMaxMB)) {
		return fmt.Errorf("value must be 0 (auto) or between %d and %d MiB", buildMemoryMinMB, buildMemoryMaxMB)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyBuildsMemoryLimitMB, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_BUILD_MEMORY_MB", Apply: ApplyLive, CheckValue: buildMemoryCheck,
		Title:       "Build memory limit",
		Description: "Memory in MiB one build may use (256 to 1048576). 0 means auto: 60 percent of the host memory, at least 512 and always leaving a reserve for the control plane (APP_BUILD_MEMORY_PERCENT and APP_BUILD_MEMORY_RESERVE_MB tune the formula). The cap also limits swap, and Node builds get a matching --max-old-space-size. A change applies to the next build; running builds keep their limit. A saved per-app or global value from build limits still wins.",
	})
}
