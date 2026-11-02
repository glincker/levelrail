package main

import (
	"context"
	"os"
	"sync/atomic"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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
		set := build.LimitsFromEnv(os.LookupEnv).MemoryMB
		if rs := buildMemorySettings.Load(); rs != nil {
			set = rs.Int(ctx, runtimesettings.KeyBuildsMemoryLimitMB)
		}
		return build.EffectiveMemoryMB(set, hostMemoryMB(), g)
	}
}
