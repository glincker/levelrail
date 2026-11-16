package runtimesettings

const (
	// KeyBuildCacheBudget turns the post build cache budget prune on, off or auto.
	KeyBuildCacheBudget = "build.cache_budget"
	// KeyBuildCacheBudgetMaxMB is the most builder cache kept after a build; 0 sizes it from the disk.
	KeyBuildCacheBudgetMaxMB = "build.cache_budget_max_mb"
	// KeyBuildCacheBudgetReservedMB is the cache a prune never goes below; 0 picks a default.
	KeyBuildCacheBudgetReservedMB = "build.cache_budget_reserved_mb"
)

// Cache budget modes.
const (
	BuildCacheBudgetAuto = "auto"
	BuildCacheBudgetOn   = "on"
	BuildCacheBudgetOff  = "off"
)

const buildCacheBudgetMaxMB = 10 * 1024 * 1024

func init() {
	Register(Definition{
		Key: KeyBuildCacheBudget, Category: CategoryDeploys, Kind: KindString, Default: BuildCacheBudgetAuto,
		Env: "APP_BUILD_CACHE_BUDGET", Apply: ApplyLive,
		AllowedStrings: []string{BuildCacheBudgetAuto, BuildCacheBudgetOn, BuildCacheBudgetOff},
		Title:          "Build cache: budget",
		Description:    "After each build, prune the builder cache down to the cache budget, least recently used records first. Cache held by a running build is never removed. auto turns it on for a Linux engine and off on other systems and on Docker Desktop; on and off force it. Off applies to the next build.",
	})
	registerLimit(Definition{
		Key: KeyBuildCacheBudgetMaxMB, Category: CategoryDeploys, Default: "0", Env: "APP_BUILDKIT_GC_KEEP_MB", Apply: ApplyLive,
		Title:       "Build cache: budget size",
		Description: "MiB of builder cache kept after a build, from 0 to 10485760. 0 sizes it from the engine disk: 20 percent, at most 10240 MiB, or 5120 MiB when the disk cannot be read.",
	}, 0, buildCacheBudgetMaxMB)
	registerLimit(Definition{
		Key: KeyBuildCacheBudgetReservedMB, Category: CategoryDeploys, Default: "0", Env: "APP_BUILDKIT_GC_RESERVED_MB", Apply: ApplyLive,
		Title:       "Build cache: always kept",
		Description: "MiB of builder cache a budget prune never goes below, from 0 to 10485760, capped at the budget size. 0 keeps 2048 MiB, or half the budget when that is smaller.",
	}, 0, buildCacheBudgetMaxMB)
}
