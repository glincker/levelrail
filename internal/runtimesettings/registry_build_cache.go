package runtimesettings

import "time"

const (
	// KeyBuildCacheAppQuotaGB caps one app's cache mounts; 0 turns the quota off.
	KeyBuildCacheAppQuotaGB = "build.cache_app_quota_gb"
	// KeyBuildCacheReclaimMaxRecords bounds how many records one reclaim run removes.
	KeyBuildCacheReclaimMaxRecords = "build.cache_reclaim_max_records"
	// KeyBuildCacheReclaimTimeout bounds one reclaim so a wedged builder cannot hold it.
	KeyBuildCacheReclaimTimeout = "build.cache_reclaim_timeout"
)

const (
	buildCacheMaxGB               = 10240
	buildCacheReclaimTimeoutFloor = 10 * time.Second
	buildCacheReclaimTimeoutCeil  = 30 * time.Minute
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
}
