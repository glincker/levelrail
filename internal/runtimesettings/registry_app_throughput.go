package runtimesettings

// Operator ceilings for the per-app throughput fields; 0 means no ceiling.
const (
	KeyAppsMaxIOBps   = "apps.resources_max_io_bps"
	KeyAppsMaxNetMbit = "apps.resources_max_net_mbit"
)

// throughputCeil keeps a typo from becoming an effectively unlimited cap.
const throughputCeil = 1 << 40

func init() {
	Register(Definition{
		Key: KeyAppsMaxIOBps, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_RESOURCES_MAX_IO_BPS", Apply: ApplyLive, CheckValue: intRangeCheck(0, throughputCeil),
		Title:       "App disk throughput ceiling",
		Description: "Largest io_read_bps or io_write_bps, in bytes per second, an app may request. 0 means no ceiling. A change applies to the next app save.",
	})
	Register(Definition{
		Key: KeyAppsMaxNetMbit, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_RESOURCES_MAX_NET_MBIT", Apply: ApplyLive, CheckValue: intRangeCheck(0, throughputCeil),
		Title:       "App bandwidth ceiling",
		Description: "Largest net_mbit, in megabits per second, an app may request. 0 means no ceiling. A change applies to the next app save.",
	})
}
