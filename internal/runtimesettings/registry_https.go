package runtimesettings

// KeyDomainsHTTPSAttemptsPerHour caps production HTTPS enable attempts per hour.
const KeyDomainsHTTPSAttemptsPerHour = "domains.https_attempts_per_hour"

func init() {
	Register(Definition{
		Key: KeyDomainsHTTPSAttemptsPerHour, Category: CategoryNetworking, Kind: KindInt, Default: "5",
		Env: "APP_ACME_MAX_ATTEMPTS_PER_HOUR", Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "Domains: HTTPS enable attempts per hour",
		Description: "Most production certificate enable attempts allowed per hour, from 1 to 100. Guards the CA's failed-validation limit; staging is never capped. The next attempt uses a changed value.",
	})
}

// KeyDomainsRollbackWindowMinutes bounds how long a domain change can be rolled back.
const KeyDomainsRollbackWindowMinutes = "domains.rollback_window_minutes"

func init() {
	Register(Definition{
		Key: KeyDomainsRollbackWindowMinutes, Category: CategoryNetworking, Kind: KindInt, Default: "60",
		Env: "APP_DOMAINS_ROLLBACK_WINDOW_MINUTES", Apply: ApplyLive, CheckValue: intRangeCheck(1, 10080),
		Title:       "Domains: rollback window (minutes)",
		Description: "How long after a domain change (go-live or apex and www apply) it can still be rolled back, from 1 to 10080 minutes. A shorter window makes a later rollback less likely to undo someone's newer work. The next request uses a changed value.",
	})
}
