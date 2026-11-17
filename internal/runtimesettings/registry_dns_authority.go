package runtimesettings

// KeyDNSAuthorityWarnTTL is the answer TTL above which who-answers reports slow failover.
const KeyDNSAuthorityWarnTTL = "dns.authority_warn_ttl_seconds"

func init() {
	Register(Definition{
		Key: KeyDNSAuthorityWarnTTL, Category: CategoryNetworking, Kind: KindInt, Default: "3600",
		Env: "APP_DNS_AUTHORITY_WARN_TTL_SECONDS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 604800),
		Title:       "DNS failover TTL warning",
		Description: "Seconds above which the who-answers check warns that a long DNS TTL slows failover, from 0 to 604800. 0 turns the TTL warning off. The next check uses a changed value.",
	})
}
