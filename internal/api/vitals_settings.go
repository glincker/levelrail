package api

import (
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Web vitals setting keys; read per request so changes apply live.
const (
	KeyVitalsEnabled      = "vitals.enabled"
	KeyVitalsRatePerMin   = "vitals.rate_limit_per_minute"
	KeyVitalsRetentionDay = "vitals.retention_days"
	KeyVitalsMaxBody      = "vitals.max_body_bytes"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsEnabled, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindBool,
		Default: "false", Env: "APP_VITALS_ENABLED", Apply: runtimesettings.ApplyLive,
		Title:       "Web vitals collection default",
		Description: "Whether apps without their own switch accept web vitals beacons. An app's Web vitals card overrides it per app. Read on every beacon.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsRatePerMin, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "600", Env: "APP_VITALS_RATE_PER_MIN", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 1000000),
		Title:       "Web vitals beacons per minute",
		Description: "Beacons accepted per app per minute, from 1 to 1000000. Extra beacons answer 429 and are dropped. Read on every beacon.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsRetentionDay, Category: runtimesettings.CategoryRetention, Kind: runtimesettings.KindInt,
		Default: "30", Env: "APP_VITALS_RETENTION_DAYS", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 365),
		Title:       "Web vitals retention",
		Description: "Days of web vitals rollups kept, from 1 to 365. Older rows are deleted as new beacons arrive.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsMaxBody, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "4096", Env: "APP_VITALS_MAX_BODY_BYTES", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(256, 65536),
		Title:       "Web vitals beacon size cap",
		Description: "Largest accepted beacon body in bytes, from 256 to 65536. Bigger bodies answer 413.",
	})
}

func vitalsIntRange(lo, hi int) func(string) error {
	return func(canonical string) error {
		if n, err := strconv.Atoi(canonical); err != nil || n < lo || n > hi {
			return fmt.Errorf("value must be between %d and %d", lo, hi)
		}
		return nil
	}
}
