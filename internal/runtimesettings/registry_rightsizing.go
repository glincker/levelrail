package runtimesettings

import "time"

// Right-sizing hint setting keys.
const (
	KeySizingLookback     = "sizing.lookback"
	KeySizingRaiseAtPct   = "sizing.raise_at_pct"
	KeySizingLowerAtPct   = "sizing.lower_at_pct"
	KeySizingMemoryMargin = "sizing.memory_margin_pct"
	KeySizingCPUMargin    = "sizing.cpu_margin_pct"
	KeySizingMinSamples   = "sizing.min_samples"
	KeySizingMinCoverage  = "sizing.min_coverage"
)

func init() {
	Register(Definition{
		Key: KeySizingLookback, Category: CategoryObservability, Kind: KindDuration, Default: "168h",
		Env: "APP_RESOURCE_RECOMMENDATION_LOOKBACK", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Hour, 30*24*time.Hour),
		Title:       "Right-sizing history window",
		Description: "How far back usage history is read for app and database resource recommendations, from 1h to 720h. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeySizingRaiseAtPct, Category: CategoryObservability, Kind: KindInt, Default: "85",
		Env: "APP_SIZING_RAISE_AT_PCT", Apply: ApplyLive, CheckValue: intRangeCheck(50, 100),
		Title:       "Right-sizing raise threshold",
		Description: "p95 usage as a percent of the current limit at or above which a raise is suggested, from 50 to 100.",
	})
	Register(Definition{
		Key: KeySizingLowerAtPct, Category: CategoryObservability, Kind: KindInt, Default: "40",
		Env: "APP_SIZING_LOWER_AT_PCT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 90),
		Title:       "Right-sizing lower threshold",
		Description: "p95 usage as a percent of the current limit below which a lower limit is suggested, from 1 to 90. It must stay under the raise threshold or the default is used.",
	})
	Register(Definition{
		Key: KeySizingMemoryMargin, Category: CategoryObservability, Kind: KindInt, Default: "130",
		Env: "APP_SIZING_MEMORY_MARGIN_PCT", Apply: ApplyLive, CheckValue: intRangeCheck(101, 500),
		Title:       "Right-sizing memory headroom",
		Description: "Suggested memory limit as a percent of p95 usage, from 101 to 500.",
	})
	Register(Definition{
		Key: KeySizingCPUMargin, Category: CategoryObservability, Kind: KindInt, Default: "130",
		Env: "APP_SIZING_CPU_MARGIN_PCT", Apply: ApplyLive, CheckValue: intRangeCheck(101, 500),
		Title:       "Right-sizing CPU headroom",
		Description: "Suggested CPU limit as a percent of p95 usage, from 101 to 500.",
	})
	Register(Definition{
		Key: KeySizingMinSamples, Category: CategoryObservability, Kind: KindInt, Default: "20",
		Env: "APP_SIZING_MIN_SAMPLES", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100000),
		Title:       "Right-sizing minimum samples",
		Description: "Usage samples needed before a recommendation is trusted; fewer gives low confidence and no change is suggested.",
	})
	Register(Definition{
		Key: KeySizingMinCoverage, Category: CategoryObservability, Kind: KindDuration, Default: "6h",
		Env: "APP_SIZING_MIN_COVERAGE", Apply: ApplyLive, CheckValue: durationRangeCheck(time.Minute, 30*24*time.Hour),
		Title:       "Right-sizing minimum history",
		Description: "How much time the samples must span before a recommendation is trusted, from 1m to 720h.",
	})
}
