package runtimesettings

import "time"

// Gauge scrape limits for custom autoscale signals. The gauge path stays env-only until a store migration lands.
const (
	KeyAutoscaleGaugeTimeout  = "autoscale.gauge_timeout"
	KeyAutoscaleGaugeMaxBytes = "autoscale.gauge_max_bytes"
)

func init() {
	Register(Definition{
		Key: KeyAutoscaleGaugeTimeout, Category: CategoryObservability, Kind: KindDuration, Default: "3s",
		Env: "APP_AUTOSCALE_GAUGE_TIMEOUT", Apply: ApplyLive, CheckValue: durationRangeCheck(500*time.Millisecond, time.Minute),
		Title:       "Autoscale gauge timeout",
		Description: "How long one scrape of an app's gauge endpoint may take, from 500ms to 1m; the next autoscale evaluation uses a changed value.",
	})
	registerLimit(Definition{
		Key: KeyAutoscaleGaugeMaxBytes, Category: CategoryObservability, Default: "1048576",
		Env: "APP_AUTOSCALE_GAUGE_MAX_BYTES", Apply: ApplyLive,
		Title:       "Autoscale gauge response cap",
		Description: "The most bytes read from an app's gauge endpoint per scrape, from 1024 to 67108864; a larger response is cut off and the reading counts as unknown.",
	}, 1024, 64<<20)
}
