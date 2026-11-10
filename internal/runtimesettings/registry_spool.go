package runtimesettings

// Upload spool and expensive-read cache keys. 0 keeps the value the env seed gives.
const (
	KeySpoolMaxAgeSeconds     = "spool.max_age_seconds"
	KeySpoolQuotaBytes        = "spool.quota_bytes"
	KeyAPIExpensiveReadTTL    = "api.expensive_read_ttl"
	spoolMaxAgeCeilingSeconds = 7 * 24 * 3600
	spoolQuotaCeilingBytes    = 1 << 40
	expensiveReadTTLCeiling   = 3600
)

func init() {
	Register(Definition{
		Key: KeySpoolMaxAgeSeconds, Category: CategoryRetention, Kind: KindInt, Default: "0",
		Env: "LEVELRAIL_SPOOL_MAX_AGE_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, spoolMaxAgeCeilingSeconds),
		Title:       "Upload spool max age",
		Description: "Seconds an unfinished upload file may sit in the spool before the sweeper removes it. 0 uses the built-in default.",
	})
	Register(Definition{
		Key: KeySpoolQuotaBytes, Category: CategoryRetention, Kind: KindInt, Default: "0",
		Env: "LEVELRAIL_SPOOL_QUOTA_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(0, spoolQuotaCeilingBytes),
		Title:       "Upload spool size cap",
		Description: "Most bytes the upload spool may hold. A source zip upload past the cap is refused with 507. 0 uses the built-in default.",
	})
	Register(Definition{
		Key: KeyAPIExpensiveReadTTL, Category: CategoryObservability, Kind: KindInt, Default: "0",
		Env: "LEVELRAIL_EXPENSIVE_READ_TTL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, expensiveReadTTLCeiling),
		Title:       "Expensive read cache time",
		Description: "Seconds the API reuses the answer of costly read endpoints. 0 uses the built-in default.",
	})
}
