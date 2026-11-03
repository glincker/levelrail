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
