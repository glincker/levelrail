package runtimesettings

import "time"

// Change stream caps, so one caller cannot pin unbounded connections or memory.
const (
	KeyChangesStreamMaxPerCaller = "events.changes_max_streams_per_caller"
	KeyChangesStreamBuffer       = "events.changes_buffer"
	KeyChangesStreamHeartbeat    = "events.changes_heartbeat"
)

func init() {
	registerLimit(Definition{
		Key: KeyChangesStreamMaxPerCaller, Category: CategoryObservability, Default: "4", Env: "APP_CHANGES_STREAM_MAX_PER_CALLER", Apply: ApplyLive,
		Title:       "Change stream: streams per caller",
		Description: "Most live change streams one user or token may hold open at once. From 1 to 32. Extra connections get 429.",
	}, 1, 32)
	registerLimit(Definition{
		Key: KeyChangesStreamBuffer, Category: CategoryObservability, Default: "64", Env: "APP_CHANGES_STREAM_BUFFER", Apply: ApplyLive,
		Title:       "Change stream: buffer per stream",
		Description: "Queued change nudges per open stream before new ones are coalesced. From 8 to 1024. Applies to streams opened after the change.",
	}, 8, 1024)
	Register(Definition{
		Key: KeyChangesStreamHeartbeat, Category: CategoryObservability, Kind: KindDuration, Default: "20s", Env: "APP_CHANGES_STREAM_HEARTBEAT", Apply: ApplyLive,
		CheckValue: durationRangeCheck(5*time.Second, 5*time.Minute),
		Title:      "Change stream: heartbeat", Description: "How often an idle change stream sends a keepalive and re-reads the caller's access, from 5s to 5m.",
	})
}
