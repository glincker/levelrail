package runtimesettings

// Egress dry run: caps how many distinct names one report lists.
const (
	KeyDNSEgressReportMaxNames = "dns.egress_report_max_names"
	DNSEgressReportMaxNamesMin = 10
	DNSEgressReportMaxNamesMax = 10000
)

func init() {
	Register(Definition{
		Key: KeyDNSEgressReportMaxNames, Category: CategoryObservability, Kind: KindInt, Default: "500",
		Env: "LEVELRAIL_DNS_EGRESS_REPORT_MAX_NAMES", Apply: ApplyLive,
		CheckValue:  intRangeCheck(DNSEgressReportMaxNamesMin, DNSEgressReportMaxNamesMax),
		Title:       "Egress report name limit",
		Description: "Most distinct names one app egress dry run report lists. Names beyond it are counted as truncated.",
	})
}
