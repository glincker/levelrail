package runtimesettings

// KeyAccessLogSize is the per-route ring size of the in-memory recent-requests log.
const (
	KeyAccessLogSize = "telemetry.access_log_size"
	maxAccessLogSize = 100000
)

func init() {
	registerLimit(Definition{
		Key: KeyAccessLogSize, Category: CategoryObservability, Default: "1000", Env: "APP_ACCESS_LOG_SIZE", Apply: ApplyLive,
		Title:       "Access log size",
		Description: "How many recent requests the edge keeps in memory per route, from 0 to 100000; 0 turns recording off. Memory is bounded by routes times this size. A saved change resizes the rings within a second, keeping the newest entries, and 0 frees them.",
	}, 0, maxAccessLogSize)
}
