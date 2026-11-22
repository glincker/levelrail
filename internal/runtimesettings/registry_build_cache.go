package runtimesettings

import "time"

const (
	// KeyBuildCacheAppQuotaGB caps one app's cache mounts; 0 turns the quota off.
	KeyBuildCacheAppQuotaGB = "build.cache_app_quota_gb"
	// KeyBuildCacheNodeDepCapGB caps all dependency cache mounts on one build node; 0 turns it off.
	KeyBuildCacheNodeDepCapGB = "build.cache_node_dep_cap_gb"
	// KeyBuildCacheReclaimMaxRecords bounds how many records one reclaim run removes.
	KeyBuildCacheReclaimMaxRecords = "build.cache_reclaim_max_records"
	// KeyBuildCacheReclaimTimeout bounds one reclaim so a wedged builder cannot hold it.
	KeyBuildCacheReclaimTimeout = "build.cache_reclaim_timeout"
	// KeyBuildCachePruneAgeHours is the idle age the dashboard "older than" prune offers.
	KeyBuildCachePruneAgeHours = "build.cache_prune_age_hours"
	// KeyBuildWarmBaseImages lists base images build nodes keep pulled; empty turns warming off.
	KeyBuildWarmBaseImages = "build.warm_base_images"
	// KeyBuildWarmBaseInterval is how often the warm list is pulled again.
	KeyBuildWarmBaseInterval = "build.warm_base_interval"
	// KeyBuildWarmBaseTimeout bounds one image pull on one node.
	KeyBuildWarmBaseTimeout = "build.warm_base_timeout"
)

const (
	buildCacheMaxGB               = 10240
	buildCacheReclaimTimeoutFloor = 10 * time.Second
	buildCacheReclaimTimeoutCeil  = 30 * time.Minute
	buildWarmIntervalFloor        = 5 * time.Minute
	buildWarmIntervalCeil         = 7 * 24 * time.Hour
	buildWarmTimeoutFloor         = 30 * time.Second
	buildWarmTimeoutCeil          = time.Hour
)

// ClampBuildCacheReclaimTimeout forces an env seeded timeout into its safe range.
func ClampBuildCacheReclaimTimeout(d time.Duration) time.Duration {
	return min(max(d, buildCacheReclaimTimeoutFloor), buildCacheReclaimTimeoutCeil)
}

func init() {
	registerLimit(Definition{
		Key: KeyBuildCacheAppQuotaGB, Category: CategoryDeploys, Default: "0", Env: "APP_BUILD_CACHE_APP_QUOTA_GB", Apply: ApplyLive,
		Title:       "Build cache: per app quota",
		Description: "GiB of builder cache mounts one app may keep, from 0 to 10240. After each build, an app over its quota loses its least recently used cache mounts until it fits. Cache held by a running build is never removed. 0 turns the quota off.",
	}, 0, buildCacheMaxGB)
	registerLimit(Definition{
		Key: KeyBuildCacheNodeDepCapGB, Category: CategoryDeploys, Default: "0", Apply: ApplyLive,
		Title:       "Build cache: dependency cap per build node",
		Description: "GiB of dependency cache mounts (npm, pip, Go modules and similar) all apps together may keep on one build node, from 0 to 10240. After each build, a node over the cap loses its least recently used mounts until it fits, on top of the per app quota. Cache held by a running build is never removed. 0 turns the cap off.",
	}, 0, buildCacheMaxGB)
	registerLimit(Definition{
		Key: KeyBuildCacheReclaimMaxRecords, Category: CategoryDeploys, Default: "50", Env: "APP_BUILD_CACHE_RECLAIM_MAX_RECORDS", Apply: ApplyLive,
		Title:       "Build cache: records per reclaim",
		Description: "The most cache records one quota reclaim removes, from 1 to 1000, so a single run stays short. The rest is reclaimed after the next build.",
	}, 1, 1000)
	Register(Definition{
		Key: KeyBuildCacheReclaimTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "2m", Env: "APP_BUILD_CACHE_RECLAIM_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildCacheReclaimTimeoutFloor, buildCacheReclaimTimeoutCeil),
		Title:       "Build cache: reclaim timeout",
		Description: "How long one quota reclaim may take, from 10s to 30m. A reclaim that runs out of time stops, and what it already freed is still reported and audited.",
	})
	registerLimit(Definition{
		Key: KeyBuildCachePruneAgeHours, Category: CategoryDeploys, Default: "168", Apply: ApplyLive,
		Title:       "Build cache: prune age",
		Description: "Hours a build cache record must sit idle before the build cache card offers to prune it, from 1 to 8760. The default of 168 is 7 days. Records a running build holds are never pruned.",
	}, 1, 8760)
	Register(Definition{
		Key: KeyBuildWarmBaseImages, Category: CategoryDeploys, Kind: KindStringList, Default: "", Apply: ApplyLive,
		Title:       "Build nodes: warm base images",
		Description: "Base images such as node:22, python:3.13 or golang:1.24 that every online build node pulls in the background, so a first build skips the base pull. Empty turns warming off.",
	})
	Register(Definition{
		Key: KeyBuildWarmBaseInterval, Category: CategoryDeploys, Kind: KindDuration, Default: "6h", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildWarmIntervalFloor, buildWarmIntervalCeil),
		Title:       "Build nodes: warm interval",
		Description: "How often the warm base image list is pulled again, from 5m to 7d, so tags that moved are refreshed.",
	})
	Register(Definition{
		Key: KeyBuildWarmBaseTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10m", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(buildWarmTimeoutFloor, buildWarmTimeoutCeil),
		Title:       "Build nodes: warm pull timeout",
		Description: "How long one warm base image pull on one node may take, from 30s to 1h. A pull that runs out of time is skipped until the next round.",
	})
}
