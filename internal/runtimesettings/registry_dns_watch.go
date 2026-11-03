package runtimesettings

// Keys for the background DNS watcher that follows a new domain until it is live.
const (
	KeyDomainsDNSWatchInterval = "domains.dns_watch_interval_seconds"
	KeyDomainsDNSWatchMaxMins  = "domains.dns_watch_max_minutes"
)

func init() {
	Register(Definition{
		Key: KeyDomainsDNSWatchInterval, Category: CategoryNetworking, Kind: KindInt, Default: "15",
		Env: "APP_DOMAINS_DNS_WATCH_INTERVAL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(5, 300),
		Title:       "Domains: DNS watch interval",
		Description: "Seconds between background DNS checks of a domain that is not live yet, from 5 to 300. Checks back off while the domain keeps waiting. The next check uses a changed value.",
	})
	Register(Definition{
		Key: KeyDomainsDNSWatchMaxMins, Category: CategoryNetworking, Kind: KindInt, Default: "120",
		Env: "APP_DOMAINS_DNS_WATCH_MAX_MINUTES", Apply: ApplyLive, CheckValue: intRangeCheck(5, 1440),
		Title:       "Domains: DNS watch time limit",
		Description: "Minutes the watcher keeps checking a domain at the normal pace, from 5 to 1440. After that the domain shows as stalled and is checked hourly for a day, then no more.",
	})
}
