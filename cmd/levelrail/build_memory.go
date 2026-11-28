package main

import (
	"context"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/telemetry"
)

// buildMemorySettings is bound once the API router exists; until then the env seed applies.
var buildMemorySettings atomic.Pointer[runtimesettings.Service]

func bindBuildMemorySetting(router *api.Router) {
	if rs := router.RuntimeSettings(); rs != nil {
		buildMemorySettings.Store(rs)
	}
}

func hostMemoryMB() int {
	total, _, err := telemetry.HostMemoryBytes()
	if err != nil {
		return 0
	}
	return int(total >> 20)
}

// buildMemoryCap resolves builds.memory_limit_mb (stored wins, env seeds) to a cap in MiB, auto when 0.
func buildMemoryCap(g build.MemGuard) func(context.Context) int {
	return func(ctx context.Context) int {
		set, pct := build.LimitsFromEnv(os.LookupEnv).MemoryMB, g.Percent
		if rs := buildMemorySettings.Load(); rs != nil {
			set = rs.Int(ctx, runtimesettings.KeyBuildsMemoryLimitMB)
			pct = rs.Int(ctx, runtimesettings.KeyBuildMemoryPercent)
		}
		if set > 0 {
			return set
		}
		return build.EffectiveMemoryPercentMB(pct, hostMemoryMB(), g)
	}
}

// buildCPUCap resolves the per build CPU cap in millicores: a fixed env value wins, else build.cpu_percent of the cores.
func buildCPUCap(seedPercent int) func(context.Context) int {
	return func(ctx context.Context) int {
		fixed, pct := build.LimitsFromEnv(os.LookupEnv).CPUMillis, seedPercent
		if rs := buildMemorySettings.Load(); rs != nil {
			pct = rs.Int(ctx, runtimesettings.KeyBuildCPUPercent)
		}
		return build.EffectiveCPUMillis(fixed, pct, runtime.NumCPU())
	}
}

// buildCPUPercentSeed is the percent used before the settings service is bound: the env seed, else the registry default.
func buildCPUPercentSeed() int {
	if n, err := strconv.Atoi(os.Getenv("APP_BUILD_CPU_PERCENT")); err == nil && n >= 0 {
		return n
	}
	for _, d := range runtimesettings.Definitions() {
		if d.Key == runtimesettings.KeyBuildCPUPercent {
			n, _ := strconv.Atoi(d.Default)
			return n
		}
	}
	return 0
}
