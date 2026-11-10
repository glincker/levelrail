package runtimesettings

// Limits for one-off command runs (apps run, the dashboard Run card, POST /api/v1/jobs/runs).
const (
	KeyAppsRunMaxConcurrentPerApp = "apps_run.max_concurrent_per_app"
	KeyAppsRunMaxTimeoutSeconds   = "apps_run.max_timeout_seconds"
	KeyAppsRunMemoryLimitMB       = "apps_run.memory_limit_mb"
	KeyAppsRunCPULimitMillis      = "apps_run.cpu_limit_millis"
)

func init() {
	Register(Definition{
		Key: KeyAppsRunMaxConcurrentPerApp, Category: CategoryDeploys, Kind: KindInt, Default: "3",
		Env: "APP_RUN_MAX_CONCURRENT_PER_APP", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "One-off runs per app at once",
		Description: "How many one-off command runs (apps run, jobs run, the dashboard Run card) may be active on one app at the same time. A new run past the cap is refused with 429.",
	})
	Register(Definition{
		Key: KeyAppsRunMaxTimeoutSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "3600",
		Env: "APP_RUN_MAX_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 86400),
		Title:       "One-off run timeout ceiling",
		Description: "The longest a one-off command run may last, in seconds. A longer requested timeout is lowered to this value, and the container is killed and removed when it is reached.",
	})
	Register(Definition{
		Key: KeyAppsRunMemoryLimitMB, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_RUN_MEMORY_LIMIT_MB", Apply: ApplyLive, CheckValue: intRangeCheck(0, 1<<20),
		Title:       "One-off run memory limit",
		Description: "Memory limit in MB for the container of a one-off command run. 0 keeps the app's own limit.",
	})
	Register(Definition{
		Key: KeyAppsRunCPULimitMillis, Category: CategoryDeploys, Kind: KindInt, Default: "0",
		Env: "APP_RUN_CPU_LIMIT_MILLIS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 1<<20),
		Title:       "One-off run CPU limit",
		Description: "CPU limit in thousandths of a core (1000 is one core) for the container of a one-off command run. 0 keeps the app's own limit.",
	})
}
