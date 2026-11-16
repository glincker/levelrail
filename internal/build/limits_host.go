package build

import (
	"log/slog"
	"sync"
)

// minCPUMillis keeps a tiny host share from rounding to a cgroup quota that cannot run a build.
const minCPUMillis = 100

var hostDetectWarn sync.Once

// WarnHostDetectionFailed logs once that host sizing is unknown, so percent limits are skipped.
func WarnHostDetectionFailed(what string) {
	hostDetectWarn.Do(func() {
		slog.Warn("build: host size unknown, percent based build limits are not applied", "detected", what)
	})
}

// EffectiveMemoryPercentMB is the memory cap for a percent setting: 0 percent or an
// unknown host means unlimited. The guard's reserve and floors still apply.
func EffectiveMemoryPercentMB(percent, hostMB int, g MemGuard) int {
	if percent <= 0 {
		return 0
	}
	if hostMB <= 0 {
		WarnHostDetectionFailed("memory")
		return 0
	}
	g.Percent = percent
	return AutoMemoryCapMB(hostMB, g)
}

// EffectiveCPUMillis resolves the CPU cap: a fixed millicore value wins, else
// percent of the host cores, 0 meaning unlimited or unknown host.
func EffectiveCPUMillis(fixedMillis, percent, hostCPUs int) int {
	if fixedMillis > 0 {
		return fixedMillis
	}
	if percent <= 0 {
		return 0
	}
	if hostCPUs <= 0 {
		WarnHostDetectionFailed("cpu")
		return 0
	}
	return max(hostCPUs*1000*percent/100, minCPUMillis)
}
