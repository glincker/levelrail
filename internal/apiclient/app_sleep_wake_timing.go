package apiclient

// WakeTiming mirrors internal/api's wakeTimingResource. Ready is start to the
// first ready signal, Released is start to the moment held requests were let
// go. Every figure is nil, and Samples 0, until the app has a timed wake.
type WakeTiming struct {
	Samples        int    `json:"samples"`
	ReadyLastMs    *int64 `json:"ready_last_ms,omitempty"`
	ReadyP50Ms     *int64 `json:"ready_p50_ms,omitempty"`
	ReadyP95Ms     *int64 `json:"ready_p95_ms,omitempty"`
	ReleasedLastMs *int64 `json:"released_last_ms,omitempty"`
	ReleasedP50Ms  *int64 `json:"released_p50_ms,omitempty"`
	ReleasedP95Ms  *int64 `json:"released_p95_ms,omitempty"`
}

// SleepIdleLimits mirrors internal/api's sleepIdleLimits.
type SleepIdleLimits struct {
	MinMinutes     int `json:"min_minutes"`
	MaxMinutes     int `json:"max_minutes"`
	DefaultMinutes int `json:"default_minutes"`
}
