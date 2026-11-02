package runtimesettings

// KeyDomainsParkedRetentionDays is how long a parked domain may sit before the hourly job forgets it.
const KeyDomainsParkedRetentionDays = "domains.parked_retention_days"

func init() {
	Register(Definition{
		Key: KeyDomainsParkedRetentionDays, Category: CategoryNetworking, Kind: KindInt, Default: "0",
		Env: "APP_DOMAINS_PARKED_RETENTION_DAYS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 3650),
		Title:       "Domains: parked domain retention",
		Description: "Days a domain may stay parked before its parked registration is deleted automatically, from 0 to 3650. 0 never deletes. DNS records and certificates are never touched. Review the dry run on the Domains page before turning this on.",
	})
}
