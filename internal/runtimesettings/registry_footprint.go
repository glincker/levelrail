package runtimesettings

import (
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

// Keys for the live Go runtime footprint knobs.
const (
	KeyFootprintGCPercent       = "footprint.gc_percent"
	KeyFootprintMemLimitPercent = "footprint.memlimit_percent"
	KeyFootprintTrimInterval    = "footprint.trim_interval"
	KeyFootprintTrimMinIdleMB   = "footprint.trim_min_idle_mb"
)

func init() {
	registerLimit(Definition{
		Key: KeyFootprintGCPercent, Category: CategoryObservability, Default: "50", Env: footprint.EnvGCPercent, Apply: ApplyLive,
		Title:       "Garbage collection target",
		Description: "How far the heap may grow past live data before a collection, from 10 to 400 percent. Lower uses less memory and a little more CPU. Re-read every 30 seconds. Ignored while the GOGC environment variable is set.",
	}, 10, 400)
	registerLimit(Definition{
		Key: KeyFootprintMemLimitPercent, Category: CategoryObservability, Default: "80", Env: footprint.EnvMemLimitPercent, Apply: ApplyLive,
		Title:       "Soft memory limit",
		Description: "Percent of container or host memory the Go runtime tries to stay under, from 0 to 95; 0 lifts the limit. Re-read every 30 seconds. Ignored while the GOMEMLIMIT environment variable is set.",
	}, 0, 95)
	registerAbuseDuration(Definition{
		Key: KeyFootprintTrimInterval, Category: CategoryObservability, Default: footprint.DefaultTrimEvery.String(), Env: footprint.EnvTrimInterval, Apply: ApplyLive,
		Title:       "Memory trim interval",
		Description: "How often idle heap is handed back to the operating system, from 30 seconds to 24 hours. Re-read every 30 seconds.",
	}, 30*time.Second, 24*time.Hour)
	registerLimit(Definition{
		Key: KeyFootprintTrimMinIdleMB, Category: CategoryObservability, Default: "24", Env: footprint.EnvTrimMinIdleMB, Apply: ApplyLive,
		Title:       "Memory trim threshold",
		Description: "Idle heap in MiB that must build up before a trim runs, from 0 to 4096. Re-read every 30 seconds.",
	}, 0, 4096)
}
