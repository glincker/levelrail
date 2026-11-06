package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Registered here so internal/build does not depend on the settings package.
func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: build.KeyBuildCachePerApp, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindBool,
		Default: "true", Env: build.EnvBuildCachePerApp, Apply: runtimesettings.ApplyLive,
		Title:       "Build cache per app",
		Description: "Give each app its own build cache for package stores and framework caches, so one app cannot evict another's. Read on every build; the environment variable only seeds the default.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: build.KeyBuildNextMinFreeMemoryMiB, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "2048", Env: build.EnvBuildNextMinFreeMemoryMiB, Apply: runtimesettings.ApplyLive, EnvLoose: true,
		CheckValue:  checkBuildMemoryMiB,
		Title:       "Next.js build memory warning",
		Description: "Host memory in MiB below which a Next.js build logs a low memory warning. 0 turns the warning off. The build is never blocked. Read on every build.",
	})
}

// wireBuildCacheSettings feeds the saved values to the build package so changes apply live.
func wireBuildCacheSettings(svc *runtimesettings.Service) {
	build.SetCachePerAppResolver(func(ctx context.Context) (bool, error) { return svc.Bool(ctx, build.KeyBuildCachePerApp), nil })
	build.SetNextMinFreeMemoryResolver(func(ctx context.Context) int { return svc.Int(ctx, build.KeyBuildNextMinFreeMemoryMiB) })
}

func checkBuildMemoryMiB(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 0 || n > 1048576 {
		return fmt.Errorf("value must be between 0 and 1048576")
	}
	return nil
}
