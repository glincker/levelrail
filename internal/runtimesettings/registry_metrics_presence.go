package runtimesettings

import "time"

// Keys for how the metrics collector slows down while nobody watches.
const (
	KeyMetricsIdleInterval   = "metrics.idle_interval"
	KeyMetricsPresenceWindow = "metrics.presence_window"
	KeyStoreTouchFlush       = "store.touch_flush_interval"
)

func init() {
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
