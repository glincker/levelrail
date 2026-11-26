package runtimesettings

// Registered keys for the database crash loop detector.
const (
	KeyDBCrashLoopRestarts      = "databases.crash_loop_restarts"
	KeyDBCrashLoopWindowSeconds = "databases.crash_loop_window_seconds"
)

func init() {
	registerLimit(Definition{
		Key: KeyDBCrashLoopRestarts, Category: CategoryDeploys, Default: "3", Env: "APP_DB_CRASH_LOOP_RESTARTS", Apply: ApplyLive,
		Title:       "Database crash loop restarts",
		Description: "How many quick exits of a database container count as a crash loop, after which it shows as crash looping and restarts back off. From 1 to 50. The next reconcile pass uses a changed value.",
	}, 1, 50)
	registerLimit(Definition{
		Key: KeyDBCrashLoopWindowSeconds, Category: CategoryDeploys, Default: "60", Env: "APP_DB_CRASH_LOOP_WINDOW_SECONDS", Apply: ApplyLive,
		Title:       "Database crash loop window",
		Description: "Seconds over which quick database container exits are counted toward a crash loop. From 5 to 3600. The next reconcile pass uses a changed value.",
	}, 5, 3600)
}
