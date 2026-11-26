package runtimesettings

// Keys for database statistics sampling.
const (
	KeyDatabaseStatsInterval     = "databases.stats_interval_seconds"
	KeyDatabaseStatsMaxDatabases = "databases.stats_max_databases"
	KeyDatabaseStatsTimeout      = "databases.stats_timeout_seconds"
)

func init() {
	registerLimit(Definition{
		Key: KeyDatabaseStatsInterval, Category: CategoryObservability, Default: "30", Env: "APP_DB_STATS_INTERVAL_SECONDS", Apply: ApplyLive,
		Title:       "Database statistics interval",
		Description: "Seconds between statistics samples (connections, size, cache hits, row changes) for running databases, from 10 to 3600. A saved change applies on the next sample.",
	}, 10, 3600)
	registerLimit(Definition{
		Key: KeyDatabaseStatsMaxDatabases, Category: CategoryObservability, Default: "20", Env: "APP_DB_STATS_MAX_DATABASES", Apply: ApplyLive,
		Title:       "Database statistics: databases per server",
		Description: "How many logical databases per Postgres server get a size series and counter row, from 1 to 200. A saved change applies on the next sample.",
	}, 1, 200)
	registerLimit(Definition{
		Key: KeyDatabaseStatsTimeout, Category: CategoryObservability, Default: "10", Env: "APP_DB_STATS_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Database statistics timeout",
		Description: "Seconds one statistics query may run inside a database container before it is cancelled, from 1 to 120. A saved change applies on the next sample.",
	}, 1, 120)
}
