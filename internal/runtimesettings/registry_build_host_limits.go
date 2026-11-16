package runtimesettings

import (
	"fmt"
	"strconv"
)

const (
	// KeyBuildMemoryPercent caps one build's memory as a share of host RAM; 0 means unlimited.
	KeyBuildMemoryPercent = "build.memory_percent"
	// KeyBuildCPUPercent caps one build's CPU as a share of host cores; 0 means unlimited.
	KeyBuildCPUPercent = "build.cpu_percent"
	// KeyBuildkitdPidsLimit caps the processes inside the BuildKit daemon container; 0 means unlimited.
	KeyBuildkitdPidsLimit = "build.buildkitd_pids_limit"
)

const (
	buildPercentMin  = 5
	buildPercentMax  = 100
	buildkitdPidsMin = 256
	buildkitdPidsMax = 4194304
)

// zeroOrRangeCheck allows 0 (unlimited) or a value inside the safe range.
func zeroOrRangeCheck(floor, ceil int) func(string) error {
	return func(canonical string) error {
		n, err := strconv.Atoi(canonical)
		if err != nil || (n != 0 && (n < floor || n > ceil)) {
			return fmt.Errorf("value must be 0 (unlimited) or between %d and %d", floor, ceil)
		}
		return nil
	}
}

func init() {
	Register(Definition{
		Key: KeyBuildMemoryPercent, Category: CategoryDeploys, Kind: KindInt, Default: "60",
		Env: "APP_BUILD_MEMORY_PERCENT", Apply: ApplyLive, CheckValue: zeroOrRangeCheck(buildPercentMin, buildPercentMax),
		Title:       "Build memory limit (percent of host)",
		Description: "Share of the host memory one build may use, from 5 to 100 percent, so a runaway build cannot starve the apps on the same host. 0 turns the limit off. Used when the build memory limit in MiB is 0, and a fixed MiB value wins over it. A change applies to the next build.",
	})
	Register(Definition{
		Key: KeyBuildCPUPercent, Category: CategoryDeploys, Kind: KindInt, Default: "75",
		Env: "APP_BUILD_CPU_PERCENT", Apply: ApplyLive, CheckValue: zeroOrRangeCheck(buildPercentMin, buildPercentMax),
		Title:       "Build CPU limit (percent of host)",
		Description: "Share of the host CPU cores one build may use, from 5 to 100 percent. 0 turns the limit off. A fixed APP_BUILD_CPU_MILLIS or a saved build limit wins over it. A change applies to the next build.",
	})
	Register(Definition{
		Key: KeyBuildkitdPidsLimit, Category: CategoryDeploys, Kind: KindInt, Default: "4096",
		Env: "APP_BUILDKIT_PIDS_LIMIT", Apply: ApplyLive, CheckValue: zeroOrRangeCheck(buildkitdPidsMin, buildkitdPidsMax),
		Title:       "BuildKit process limit",
		Description: "Most processes the BuildKit daemon container may run, from 256 to 4194304, which stops a fork bomb in a build from exhausting the host. 0 turns the limit off. It applies when the daemon container is next created.",
	})
}
