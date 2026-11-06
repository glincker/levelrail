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
	KeyVitalsRatePerIP    = "vitals.rate_limit_per_ip_per_minute"
	KeyVitalsMaxRoutes    = "vitals.max_routes_per_app"
	KeyVitalsTopRoutes    = "vitals.report_top_routes"
	KeyVitalsFlushSeconds = "vitals.flush_interval_seconds"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsEnabled, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindBool,
		Default: "false", Env: "APP_VITALS_ENABLED", Apply: runtimesettings.ApplyLive,
		Title:       "Web vitals new key default",
		Description: "Whether a beacon key created by Rotate starts enabled. An app only accepts beacons once it has a saved config with collection on; the Web vitals card switch owns that per app. Read when a key is first created.",
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
		Description: "Days of web vitals rollups kept, from 1 to 365. Older rows are deleted by a background job every ten minutes.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsMaxBody, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "4096", Env: "APP_VITALS_MAX_BODY_BYTES", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(256, 65536),
		Title:       "Web vitals beacon size cap",
		Description: "Largest accepted beacon body in bytes, from 256 to 65536. Bigger bodies answer 413.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsRatePerIP, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "120", Env: "APP_VITALS_RATE_PER_IP", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 1000000),
		Title:       "Web vitals beacons per client per minute",
		Description: "Beacons accepted per client address per minute, from 1 to 1000000, checked before the body is read. Extra beacons answer 429.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsMaxRoutes, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "200", Env: "APP_VITALS_MAX_ROUTES", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 10000),
		Title:       "Web vitals routes per app per day",
		Description: "Distinct routes one app may record per day, from 1 to 10000. Further routes are counted under /other.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsTopRoutes, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "50", Env: "APP_VITALS_REPORT_TOP_ROUTES", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 500),
		Title:       "Web vitals routes in the report",
		Description: "Busiest routes returned by the Web vitals report and shown on the card, from 1 to 500.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyVitalsFlushSeconds, Category: runtimesettings.CategoryObservability, Kind: runtimesettings.KindInt,
		Default: "10", Env: "APP_VITALS_FLUSH_SECONDS", Apply: runtimesettings.ApplyLive, CheckValue: vitalsIntRange(1, 3600),
		Title:       "Web vitals flush interval",
		Description: "Seconds between writes of buffered web vitals counters, from 1 to 3600. A crash loses at most one interval.",
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
