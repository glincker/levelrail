package runtimesettings

import "time"

// App metrics scrape setting keys.
const (
	KeyAppMetricsScrapeInterval = "app_metrics.scrape_interval"
	KeyAppMetricsScrapeTimeout  = "app_metrics.scrape_timeout"
	KeyAppMetricsMaxBodyBytes   = "app_metrics.max_body_bytes"
	KeyAppMetricsMaxSeries      = "app_metrics.max_series"
	KeyAppMetricsMaxLabels      = "app_metrics.max_labels"
	KeyAppMetricsRetentionHours = "app_metrics.retention_hours"
	KeyAppMetricsQueryMaxPoints = "app_metrics.query_max_points"
	KeyAppMetricsMaxLabelBytes  = "app_metrics.max_label_bytes"
	KeyAppMetricsMaxStoredRows  = "app_metrics.max_stored_rows"
	KeyAppMetricsScrapeWorkers  = "app_metrics.scrape_workers"
)

func init() {
	Register(Definition{
		Key: KeyAppMetricsScrapeInterval, Category: CategoryObservability, Kind: KindDuration, Default: "60s",
		Env: "APP_APP_METRICS_SCRAPE_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(15*time.Second, time.Hour),
		Title:       "App metrics scrape interval",
		Description: "How often the control plane reads each app's own metrics endpoint, from 15s to 1h. The next tick uses a changed value.",
	})
	Register(Definition{
		Key: KeyAppMetricsScrapeTimeout, Category: CategoryObservability, Kind: KindDuration, Default: "5s",
		Env: "APP_APP_METRICS_SCRAPE_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Second, 30*time.Second),
		Title:       "App metrics scrape timeout",
		Description: "How long one scrape may take, from 1s to 30s, including connect and body read.",
	})
	Register(Definition{
		Key: KeyAppMetricsMaxBodyBytes, Category: CategoryObservability, Kind: KindInt, Default: "1048576",
		Env: "APP_APP_METRICS_MAX_BODY_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(4096, 16777216),
		Title:       "App metrics body limit",
		Description: "Largest metrics response read, in bytes, from 4096 to 16777216; a larger body fails the scrape and nothing is stored.",
	})
	Register(Definition{
		Key: KeyAppMetricsMaxSeries, Category: CategoryObservability, Kind: KindInt, Default: "500",
		Env: "APP_APP_METRICS_MAX_SERIES", Apply: ApplyLive, CheckValue: intRangeCheck(10, 10000),
		Title:       "App metrics series limit",
		Description: "Most distinct series stored per app per scrape, from 10 to 10000; the rest are dropped and counted.",
	})
	Register(Definition{
		Key: KeyAppMetricsMaxLabels, Category: CategoryObservability, Kind: KindInt, Default: "10",
		Env: "APP_APP_METRICS_MAX_LABELS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 30),
		Title:       "App metrics label limit",
		Description: "Most labels one series may carry, from 1 to 30; a series with more is dropped to keep cardinality bounded.",
	})
	Register(Definition{
		Key: KeyAppMetricsRetentionHours, Category: CategoryObservability, Kind: KindInt, Default: "72",
		Env: "APP_APP_METRICS_RETENTION_HOURS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 720),
		Title:       "App metrics retention",
		Description: "Hours of scraped app samples kept, from 1 to 720; older samples are deleted on the next scrape tick.",
	})
	Register(Definition{
		Key: KeyAppMetricsQueryMaxPoints, Category: CategoryObservability, Kind: KindInt, Default: "10000",
		Env: "APP_APP_METRICS_QUERY_MAX_POINTS", Apply: ApplyLive, CheckValue: intRangeCheck(100, 100000),
		Title:       "App metrics query point limit",
		Description: "Most stored samples one scraped metrics query returns, from 100 to 100000, so a wide window cannot load the whole table.",
	})
	Register(Definition{
		Key: KeyAppMetricsMaxLabelBytes, Category: CategoryObservability, Kind: KindInt, Default: "65536",
		Env: "APP_APP_METRICS_MAX_LABEL_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1024, 1048576),
		Title:       "App metrics label bytes limit",
		Description: "Most label text kept from one scrape, in bytes, from 1024 to 1048576; series past it are dropped and counted.",
	})
	Register(Definition{
		Key: KeyAppMetricsMaxStoredRows, Category: CategoryObservability, Kind: KindInt, Default: "500000",
		Env: "APP_APP_METRICS_MAX_STORED_ROWS", Apply: ApplyLive, CheckValue: intRangeCheck(1000, 10000000),
		Title:       "App metrics stored row limit",
		Description: "Most samples stored per app, from 1000 to 10000000; at the limit new scrapes are dropped until retention frees room.",
	})
	Register(Definition{
		Key: KeyAppMetricsScrapeWorkers, Category: CategoryObservability, Kind: KindInt, Default: "4",
		Env: "APP_APP_METRICS_SCRAPE_WORKERS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 16),
		Title:       "App metrics scrape workers",
		Description: "How many apps are scraped at once, from 1 to 16, so one slow endpoint cannot delay the others.",
	})
}
