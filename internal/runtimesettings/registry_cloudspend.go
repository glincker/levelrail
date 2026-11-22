package runtimesettings

// Registered keys for cloud account spend and the instance list cache.
const (
	KeyCloudSpendEnabled              = "cloud.spend.enabled"
	KeyCloudSpendCacheTTLMinutes      = "cloud.spend.cache_ttl_minutes"
	KeyCloudSpendRefreshMinInterval   = "cloud.spend.refresh_min_interval_minutes"
	KeyCloudSpendRefreshPerUserHour   = "cloud.spend.refresh_per_user_per_hour"
	KeyCloudSpendStaleAfterHours      = "cloud.spend.stale_after_hours"
	KeyCloudSpendAWSCostExplorer      = "cloud.spend.aws_cost_explorer"
	KeyCloudSpendMaxPages             = "cloud.spend.max_pages"
	KeyCloudInstancesCacheTTLSecs     = "cloud.instances.cache_ttl_seconds"
	KeyCloudInstancesEventsWindowDays = "cloud.instances.events_window_days"
)

func init() {
	cat := CategoryObservability
	Register(Definition{
		Key: KeyCloudSpendEnabled, Category: cat, Kind: KindBool, Default: "true", Env: "APP_CLOUD_SPEND_ENABLED", Apply: ApplyLive,
		Title: "Cloud spend reads", Description: "Lets the control plane read spend from connected cloud accounts. Off answers every spend request with a disabled report.",
	})
	registerLimit(Definition{
		Key: KeyCloudSpendCacheTTLMinutes, Category: cat, Default: "15", Env: "APP_CLOUD_SPEND_CACHE_TTL_MINUTES", Apply: ApplyLive,
		Title: "Cloud spend cache", Description: "Minutes a spend report is served from memory before one background refetch. From 1 to 1440.",
	}, 1, 1440)
	registerLimit(Definition{
		Key: KeyCloudSpendRefreshMinInterval, Category: cat, Default: "5", Env: "APP_CLOUD_SPEND_REFRESH_MIN_INTERVAL_MINUTES", Apply: ApplyLive,
		Title: "Cloud spend refresh spacing", Description: "Minutes that must pass between forced refreshes of one account. From 1 to 1440.",
	}, 1, 1440)
	registerLimit(Definition{
		Key: KeyCloudSpendRefreshPerUserHour, Category: cat, Default: "6", Env: "APP_CLOUD_SPEND_REFRESH_PER_USER_PER_HOUR", Apply: ApplyLive,
		Title: "Cloud spend refreshes per user", Description: "Most forced refreshes one user may trigger per hour across accounts. From 1 to 1000.",
	}, 1, 1000)
	registerLimit(Definition{
		Key: KeyCloudSpendStaleAfterHours, Category: cat, Default: "72", Env: "APP_CLOUD_SPEND_STALE_AFTER_HOURS", Apply: ApplyLive,
		Title: "Cloud spend stale after", Description: "Hours after which provider data counts as stale, because providers publish lagged numbers. From 1 to 720.",
	}, 1, 720)
	Register(Definition{
		Key: KeyCloudSpendAWSCostExplorer, Category: cat, Kind: KindBool, Default: "false", Env: "APP_CLOUD_SPEND_AWS_COST_EXPLORER", Apply: ApplyLive,
		Title: "AWS Cost Explorer calls", Description: "Allows paid Cost Explorer API calls for AWS spend. Each call is billed by AWS, so it stays off until enabled.",
	})
	registerLimit(Definition{
		Key: KeyCloudSpendMaxPages, Category: cat, Default: "5", Env: "APP_CLOUD_SPEND_MAX_PAGES", Apply: ApplyLive,
		Title: "Cloud spend page cap", Description: "Most provider pages one spend read follows. From 1 to 100.",
	}, 1, 100)
	registerLimit(Definition{
		Key: KeyCloudInstancesCacheTTLSecs, Category: cat, Default: "30", Env: "APP_CLOUD_INSTANCES_CACHE_TTL_SECONDS", Apply: ApplyLive,
		Title: "Cloud instance list cache", Description: "Seconds a provider instance list is reused before the next provider call. From 1 to 3600.",
	}, 1, 3600)
	registerLimit(Definition{
		Key: KeyCloudInstancesEventsWindowDays, Category: cat, Default: "7", Env: "APP_CLOUD_INSTANCES_EVENTS_WINDOW_DAYS", Apply: ApplyLive,
		Title: "Cloud instance events window", Description: "Days of provider events shown on an instance. From 1 to 90.",
	}, 1, 90)
}
