package runtimesettings

// Registered keys for POST /api/v1/databases/bulk.
const (
	KeyDatabaseBulkConcurrency = "databases.bulk_concurrency"
	KeyDatabaseBulkMaxItems    = "databases.bulk_max_items"
)

func init() {
	registerLimit(Definition{
		Key: KeyDatabaseBulkConcurrency, Category: CategoryDeploys, Default: "4", Env: "APP_DATABASES_BULK_CONCURRENCY", Apply: ApplyLive,
		Title:       "Database bulk concurrency",
		Description: "How many databases one bulk start, stop or backup (databases bulk, POST /api/v1/databases/bulk) acts on at once. From 1 to 32. The next bulk call uses a changed value.",
	}, 1, 32)
	registerLimit(Definition{
		Key: KeyDatabaseBulkMaxItems, Category: CategoryDeploys, Default: "200", Env: "APP_DATABASES_BULK_MAX_ITEMS", Apply: ApplyLive,
		Title:       "Database bulk size limit",
		Description: "Most databases one bulk call may target; a bigger selection is refused with HTTP 400 and nothing runs. From 1 to 2000. The next bulk call uses a changed value.",
	}, 1, 2000)
}
