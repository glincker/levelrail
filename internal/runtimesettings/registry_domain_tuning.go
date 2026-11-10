package runtimesettings

// Registered keys for the bounds on per-domain proxy tuning.
const (
	KeyDomainTuningMinTimeoutSeconds = "domains.tuning_min_timeout_seconds"
	KeyDomainTuningMaxTimeoutSeconds = "domains.tuning_max_timeout_seconds"
	KeyDomainTuningMaxStreamSeconds  = "domains.tuning_max_stream_seconds"

	// The min setting tops out where the max settings start, so min <= max always holds.
	domainTuningMinCeiling     = 60
	domainTuningTimeoutCeiling = 86400
	domainTuningStreamCeiling  = 604800
)

func init() {
	registerLimit(Definition{
		Key: KeyDomainTuningMinTimeoutSeconds, Category: CategoryNetworking, Default: "1", Env: "APP_DOMAIN_TUNING_MIN_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Domain tuning: shortest timeout",
		Description: "Shortest dial, response header, stream lifetime or stream close delay in seconds that a domain may set with domains tuning set, from 1 to 60, so it can never pass the longest timeout. A value of 0 on the domain keeps the ingress default.",
	}, 1, domainTuningMinCeiling)
	registerLimit(Definition{
		Key: KeyDomainTuningMaxTimeoutSeconds, Category: CategoryNetworking, Default: "600", Env: "APP_DOMAIN_TUNING_MAX_TIMEOUT_SECONDS", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Domain tuning: longest upstream timeout",
		Description: "Longest dial or response header timeout, and longest websocket close delay, in seconds that a domain may set, from 60 to 86400. A lowered value also tightens tuning already stored on the next ingress pass.",
	}, domainTuningMinCeiling, domainTuningTimeoutCeiling)
	registerLimit(Definition{
		Key: KeyDomainTuningMaxStreamSeconds, Category: CategoryNetworking, Default: "86400", Env: "APP_DOMAIN_TUNING_MAX_STREAM_SECONDS", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Domain tuning: longest stream lifetime",
		Description: "Longest websocket or streaming connection lifetime in seconds that a domain may set, from 60 to 604800, so no tenant can pin a connection forever. A lowered value also tightens tuning already stored on the next ingress pass.",
	}, 60, domainTuningStreamCeiling)
}
