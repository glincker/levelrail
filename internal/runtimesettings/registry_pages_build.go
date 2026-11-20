package runtimesettings

import (
	"context"
	"time"
)

// Keys for Pages build logs and the dependency cache.
const (
	KeyPagesBuildLogMaxBytes   = "pages.build_log_max_bytes"
	KeyPagesBuildLogRetention  = "pages.build_log_retention"
	KeyPagesBuildCacheMaxBytes = "pages.build_cache_max_bytes"
)

func init() {
	Register(Definition{
		Key: KeyPagesBuildLogMaxBytes, Category: CategoryRetention, Kind: KindInt, Default: "5242880",
		Env: "APP_PAGES_BUILD_LOG_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(65536, 1<<30),
		Title:       "Pages: build log size cap",
		Description: "Largest build log in bytes served per Pages deploy. When a log is bigger, the oldest output is dropped and a notice is shown.",
	})
	Register(Definition{
		Key: KeyPagesBuildLogRetention, Category: CategoryRetention, Kind: KindDuration, Default: "720h",
		Env: "APP_PAGES_BUILD_LOG_RETENTION", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Hour, 24*365*time.Hour),
		Title:       "Pages: build log retention",
		Description: "How long a finished Pages deploy's build log stays readable before the logs endpoint answers 410.",
	})
	Register(Definition{
		Key: KeyPagesBuildCacheMaxBytes, Category: CategoryRetention, Kind: KindInt, Default: "1073741824",
		Env: "APP_PAGES_BUILD_CACHE_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1<<20, 1<<40),
		Title:       "Pages: dependency cache cap per site",
		Description: "Largest dependency cache in bytes kept for one Pages site. The oldest lockfile cache is evicted first; caches are never shared between sites.",
	})
}

// PagesBuildLogLimits returns the saved log size cap in bytes and the retention.
func (s *Service) PagesBuildLogLimits(ctx context.Context) (int64, time.Duration) {
	return int64(s.Int(ctx, KeyPagesBuildLogMaxBytes)), s.Duration(ctx, KeyPagesBuildLogRetention)
}
