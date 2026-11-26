package runtimesettings

// Registered keys for GET /api/v1/databases/{name}/branches/{branch}/schema-diff.
const (
	KeyDatabasesSchemaDiffMaxObjects     = "databases.schema_diff_max_objects"
	KeyDatabasesSchemaDiffTimeoutSeconds = "databases.schema_diff_timeout_seconds"
)

func init() {
	registerLimit(Definition{
		Key: KeyDatabasesSchemaDiffMaxObjects, Category: CategoryDeploys, Default: "2000", Env: "APP_DATABASES_SCHEMA_DIFF_MAX_OBJECTS", Apply: ApplyLive,
		Title:       "Schema diff object cap",
		Description: "Most tables and views one schema diff reads per database; a bigger schema is cut and the result says so. From 10 to 20000. The next diff uses a changed value.",
	}, 10, 20000)
	registerLimit(Definition{
		Key: KeyDatabasesSchemaDiffTimeoutSeconds, Category: CategoryDeploys, Default: "20", Env: "APP_DATABASES_SCHEMA_DIFF_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Schema diff timeout (seconds)",
		Description: "Longest one schema diff may read a database before it is stopped. From 1 to 300. The next diff uses a changed value.",
	}, 1, 300)
}
