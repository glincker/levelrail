package runtimesettings

// Automatic newest stable version for databases created without a version.
const (
	KeyDatabasesAutoLatest                = "databases.auto_latest"
	KeyDatabasesLatestVersionCacheSeconds = "databases.latest_version_cache_seconds"
	KeyDatabasesLatestLookupSeconds       = "databases.latest_lookup_timeout_seconds"
)

func init() {
	Register(Definition{
		Key: KeyDatabasesAutoLatest, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "LEVELRAIL_DATABASES_AUTO_LATEST", Apply: ApplyLive,
		Title:       "Pick the latest database version automatically",
		Description: "When a database is created without a version, resolve the newest stable supported version from the engine's registry and store it. Off requires an explicit version.",
	})
	Register(Definition{
		Key: KeyDatabasesLatestVersionCacheSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "3600",
		Env: "LEVELRAIL_DATABASES_LATEST_VERSION_CACHE_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 604800),
		Title:       "Latest version cache (seconds)",
		Description: "How long a resolved latest version is reused before the registry is asked again. 0 asks every time.",
	})
	Register(Definition{
		Key: KeyDatabasesLatestLookupSeconds, Category: CategoryDeploys, Kind: KindInt, Default: "8",
		Env: "LEVELRAIL_DATABASES_LATEST_LOOKUP_TIMEOUT_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 60),
		Title:       "Latest version lookup timeout (seconds)",
		Description: "Longest wait on the registry before a create falls back to the engine default version.",
	})
}
