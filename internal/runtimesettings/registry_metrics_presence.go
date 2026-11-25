package runtimesettings

import "time"

// Keys for how the metrics collector slows down while nobody watches.
const (
	KeyMetricsIdleInterval   = "metrics.idle_interval"
	KeyMetricsPresenceWindow = "metrics.presence_window"
	KeyStoreTouchFlush       = "store.touch_flush_interval"
	KeyMetricsSampleInterval = "metrics.sample_interval"
	KeySlowRequestThreshold  = "api.slow_request_threshold"
	KeyMetricsCapTarget      = "metrics.cap_target_percent"
)

func init() {
	Register(Definition{
		Key: KeyMetricsSampleInterval, Category: CategoryObservability, Kind: KindDuration, Default: "15s", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(5*time.Second, 5*time.Minute),
		Title:       "Container stats interval",
		Description: "How often every running container's stats are sampled, from 5 seconds to 5 minutes. A small host can pick 30s or 1m to cut idle CPU, at the cost of coarser charts. The collector uses a saved change on its next tick.",
	})
	Register(Definition{
		Key: KeyMetricsCapTarget, Category: CategoryObservability, Kind: KindInt, Default: "85", Apply: ApplyLive,
		CheckValue:  intRangeCheck(50, 99),
		Title:       "Metrics size cap headroom",
		Description: "Percent of the metrics size cap that pruning aims for once the cap is hit, from 50 to 99. A lower value frees more room per prune. A saved change applies to the next prune.",
	})
	Register(Definition{
		Key: KeySlowRequestThreshold, Category: CategoryObservability, Kind: KindDuration, Default: "500ms", Env: "APP_SLOW_REQUEST_THRESHOLD", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(10*time.Millisecond, time.Minute),
		Title:       "Slow request threshold",
		Description: "API requests slower than this are logged at Warn and listed on the control plane health page, from 10ms to 1 minute. A saved change applies to the next request.",
	})
	Register(Definition{
		Key: KeyMetricsIdleInterval, Category: CategoryObservability, Kind: KindDuration, Default: "1m", Env: "APP_METRICS_IDLE_INTERVAL", Apply: ApplyRestart,
		CheckValue:  durationRangeCheck(15*time.Second, 24*time.Hour),
		Title:       "Idle metrics interval",
		Description: "How often metrics are collected while nobody has a dashboard open, from 15 seconds to 24 hours. Takes effect after a restart.",
	})
	Register(Definition{
		Key: KeyMetricsPresenceWindow, Category: CategoryObservability, Kind: KindDuration, Default: "2m", Env: "APP_METRICS_PRESENCE_WINDOW", Apply: ApplyRestart,
		CheckValue:  durationRangeCheck(15*time.Second, 24*time.Hour),
		Title:       "Viewer presence window",
		Description: "How long after the last metrics read a viewer still counts as watching, from 15 seconds to 24 hours. Takes effect after a restart.",
	})
	Register(Definition{
		Key: KeyStoreTouchFlush, Category: CategoryObservability, Kind: KindDuration, Default: "5s", Env: "LEVELRAIL_TOUCH_FLUSH_INTERVAL", Apply: ApplyRestart,
		CheckValue:  durationRangeCheck(0, time.Hour),
		Title:       "Last-seen flush interval",
		Description: "How often buffered last-seen and last-used timestamps are written, up to 1 hour. 0 writes through on every request. Takes effect after a restart.",
	})
}
