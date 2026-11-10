package runtimesettings

// Keys for SQLite connection tuning of the control plane database.
const (
	KeyDBCacheSizeMB = "store.cache_size_mb"
	KeyDBMmapSizeMB  = "store.mmap_size_mb"
	KeyDBTempStore   = "store.temp_store"
	KeyDBSynchronous = "store.synchronous"
)

// Tuning value names; "default" leaves the SQLite built-in untouched.
const (
	DBTuningDefault = "default"
	DBTempFile      = "file"
	DBTempMemory    = "memory"
	DBSyncNormal    = "normal"
	DBSyncFull      = "full"
)

const (
	maxDBCacheSizeMB = 4096
	maxDBMmapSizeMB  = 16384
)

func init() {
	Register(Definition{
		Key: KeyDBCacheSizeMB, Category: CategoryObservability, Kind: KindInt, Default: "0", Env: "APP_DB_CACHE_SIZE_MB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, maxDBCacheSizeMB),
		Title:       "Database page cache (MB)",
		Description: "Page cache per control plane database connection in megabytes, 0 to 4096. 0 keeps the SQLite default. Applies to the writer and read connections within a minute.",
	})
	Register(Definition{
		Key: KeyDBMmapSizeMB, Category: CategoryObservability, Kind: KindInt, Default: "0", Env: "APP_DB_MMAP_SIZE_MB", Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, maxDBMmapSizeMB),
		Title:       "Database memory map size (MB)",
		Description: "How much of the control plane database file SQLite may memory map, 0 to 16384. 0 turns memory mapping off. Applies within a minute.",
	})
	Register(Definition{
		Key: KeyDBTempStore, Category: CategoryObservability, Kind: KindString, Default: DBTuningDefault, Env: "APP_DB_TEMP_STORE", Apply: ApplyLive,
		AllowedStrings: []string{DBTuningDefault, DBTempFile, DBTempMemory},
		Title:          "Database temp store",
		Description:    "Where SQLite keeps temporary tables and indexes: default, file or memory. Memory is faster but uses RAM on large sorts. Applies within a minute.",
	})
	Register(Definition{
		Key: KeyDBSynchronous, Category: CategoryObservability, Kind: KindString, Default: DBTuningDefault, Env: "APP_DB_SYNCHRONOUS", Apply: ApplyLive,
		AllowedStrings: []string{DBTuningDefault, DBSyncNormal, DBSyncFull},
		StrengthOrder:  []string{DBSyncNormal, DBTuningDefault, DBSyncFull},
		Risk:           RiskWeakensSecurity,
		Title:          "Database synchronous mode",
		Description:    "How hard SQLite syncs the WAL to disk: default, normal or full. Normal is faster but a power loss can drop the last commits. OFF is not offered. Applies within a minute.",
	})
}
