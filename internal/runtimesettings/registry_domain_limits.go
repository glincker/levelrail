package runtimesettings

// Registered keys for the ceilings on per-domain request limits.
const (
	KeyDomainLimitsMaxBodyBytes      = "domains.limits_max_body_bytes"
	KeyDomainLimitsMaxTimeoutSeconds = "domains.limits_max_timeout_seconds"

	domainLimitsBodyCeiling    = 1 << 40
	domainLimitsTimeoutCeiling = 86400
)

func init() {
	registerLimit(Definition{
		Key: KeyDomainLimitsMaxBodyBytes, Category: CategoryNetworking, Default: "10737418240", Env: "APP_DOMAIN_LIMITS_MAX_BODY_BYTES", Apply: ApplyLive, Risk: RiskWeakensSecurity, HigherIsWeaker: true,
		Title:       "Domain limits: largest request body",
		Description: "Highest request body cap in bytes that a domain may set for itself, from 1 MiB to 1 TiB. A domain's own cap (domains limits set --max-body) must be at least 1 MiB and no higher than this. Caps already stored are not rewritten. The next save uses a changed value.",
	}, bodyLimitFloor, domainLimitsBodyCeiling)
	registerLimit(Definition{
		Key: KeyDomainLimitsMaxTimeoutSeconds, Category: CategoryNetworking, Default: "3600", Env: "APP_DOMAIN_LIMITS_MAX_TIMEOUT_SECONDS", Apply: ApplyLive,
		Title:       "Domain limits: longest read or write timeout",
		Description: "Longest upstream read or write timeout in seconds that a domain may set for itself, from 1 to 86400. A domain's timeout of 0 keeps the default behavior. The next save uses a changed value.",
	}, 1, domainLimitsTimeoutCeiling)
}
