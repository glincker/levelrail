package runtimesettings

// Limits that keep the token list manageable at scale.
const (
	KeyTokensEmergencyMaxLive = "tokens.emergency_max_live" //nolint:gosec // setting key, not a credential
	KeyTokensBulkMax          = "tokens.bulk_max"           //nolint:gosec // setting key, not a credential
)

const (
	maxEmergencyLiveCeiling = 1000
	maxBulkMaxCeiling       = 10000
)

func init() {
	Register(Definition{
		Key: KeyTokensEmergencyMaxLive, Category: CategorySecurity, Kind: KindInt, Default: "3",
		Env: "APP_TOKENS_EMERGENCY_MAX_LIVE", Apply: ApplyLive, CheckValue: intRangeCheck(0, maxEmergencyLiveCeiling),
		Title:       "Live emergency tokens kept",
		Description: "Most emergency tokens that may be valid at once, from 0 to 1000. Minting another through the host command revokes the oldest ones beyond this number. 0 means no limit.",
	})
	Register(Definition{
		Key: KeyTokensBulkMax, Category: CategorySecurity, Kind: KindInt, Default: "500",
		Env: "APP_TOKENS_BULK_MAX", Apply: ApplyLive, CheckValue: intRangeCheck(1, maxBulkMaxCeiling),
		Title:       "Most tokens revoked per call",
		Description: "Largest number of API tokens one bulk revoke may change, from 1 to 10000. A larger match is refused so nobody revokes more than they reviewed.",
	})
}
