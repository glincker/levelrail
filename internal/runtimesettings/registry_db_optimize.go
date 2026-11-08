package runtimesettings

import (
	"fmt"
	"time"
)

// KeyDBOptimizeInterval is how often the control plane database refreshes planner statistics.
const KeyDBOptimizeInterval = "store.optimize_interval"

const maxDBOptimizeInterval = 30 * 24 * time.Hour

// Zero disables the loop, so the check allows it besides the bounded range.
func dbOptimizeCheck(canonical string) error {
	d, err := time.ParseDuration(canonical)
	if err != nil || d < 0 || d > maxDBOptimizeInterval {
		return fmt.Errorf("value must be 0 or a duration up to %s", maxDBOptimizeInterval)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyDBOptimizeInterval, Category: CategoryObservability, Kind: KindDuration, Default: "6h", Env: "APP_DB_OPTIMIZE_INTERVAL", Apply: ApplyLive,
		CheckValue:  dbOptimizeCheck,
		Title:       "Database optimize interval",
		Description: "How often the control plane database refreshes its query planner statistics (PRAGMA optimize). 0 turns the periodic run off; one run still happens at shutdown. A saved change applies within a minute.",
	})
}
