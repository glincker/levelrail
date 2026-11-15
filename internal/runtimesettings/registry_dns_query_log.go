package runtimesettings

import "time"

// Mesh DNS query visibility (beta, off by default).
const (
	KeyDNSQueryLogAppRefresh = "dns.query_log_app_refresh"
	KeyDNSQueryLogEnabled    = "dns.query_log_enabled"
	KeyDNSQueryLogSize       = "dns.query_log_size"
	KeyDNSQueryLogPrivacy    = "dns.query_log_privacy"
	// DNSQueryLogSizeMin and Max clamp how many records the ring may hold.
	DNSQueryLogSizeMin = 10
	DNSQueryLogSizeMax = 100000
)

func init() {
	Register(Definition{
		Key: KeyDNSQueryLogEnabled, Category: CategoryObservability, Kind: KindBool, Default: "false",
		Env: "LEVELRAIL_DNS_QUERY_LOG_ENABLED", Apply: ApplyLive,
		Title:       "Mesh DNS query log (beta)",
		Description: "Record queries to the mesh DNS server in memory, with per-app counters. Off by default.",
	})
	Register(Definition{
		Key: KeyDNSQueryLogSize, Category: CategoryObservability, Kind: KindInt, Default: "1000",
		Env: "LEVELRAIL_DNS_QUERY_LOG_SIZE", Apply: ApplyLive, CheckValue: intRangeCheck(DNSQueryLogSizeMin, DNSQueryLogSizeMax),
		Title:       "Mesh DNS query log size",
		Description: "Most query records kept in memory. The oldest are dropped first.",
	})
	Register(Definition{
		Key: KeyDNSQueryLogAppRefresh, Category: CategoryObservability, Kind: KindDuration, Default: "10s",
		Env: "LEVELRAIL_DNS_QUERY_LOG_APP_REFRESH", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, time.Hour),
		Title:       "Mesh DNS app lookup refresh",
		Description: "Longest wait before the source IP to app map is rebuilt from running containers.",
	})
	Register(Definition{
		Key: KeyDNSQueryLogPrivacy, Category: CategoryObservability, Kind: KindString, Default: "names_only",
		Env: "LEVELRAIL_DNS_QUERY_LOG_PRIVACY", Apply: ApplyLive,
		AllowedStrings: []string{"full", "names_only", "counters_only"},
		Title:          "Mesh DNS query log privacy",
		Description:    "full keeps the source IP, names_only drops it, counters_only stores no records and keeps only per-app counters.",
	})
}
