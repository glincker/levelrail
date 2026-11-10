package runtimesettings

// Registered keys for the database space report and host process table.
const (
	KeyDBSpaceRows         = "db.space_report_rows"
	KeyDBCompactMinFree    = "db.compact_min_free_percent"
	KeySystemProcessesRows = "system.processes_max_rows"
)

func init() {
	registerLimit(Definition{
		Key: KeyDBSpaceRows, Category: CategoryObservability, Default: "20", Env: "APP_DB_SPACE_REPORT_ROWS", Apply: ApplyLive,
		Title:       "Database space report rows",
		Description: "Most tables the database space report lists, largest first. From 1 to 200. The next request uses a changed value.",
	}, 1, 200)
	registerLimit(Definition{
		Key: KeyDBCompactMinFree, Category: CategoryObservability, Default: "10", Env: "APP_DB_COMPACT_MIN_FREE_PERCENT", Apply: ApplyLive,
		Title:       "Database compaction minimum free space",
		Description: "Percent of free pages below which a compaction request is declined unless forced. From 0 to 100. Compaction only ever runs on request.",
	}, 0, 100)
	registerLimit(Definition{
		Key: KeySystemProcessesRows, Category: CategoryObservability, Default: "25", Env: "APP_SYSTEM_PROCESSES_MAX_ROWS", Apply: ApplyLive,
		Title:       "Host process table rows",
		Description: "Most processes the control plane host process table lists, largest memory first. From 1 to 500. The next request uses a changed value.",
	}, 1, 500)
}
