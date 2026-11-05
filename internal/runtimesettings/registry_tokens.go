package runtimesettings

// Token hygiene settings. Both default to 0 so an existing install behaves as before.
const (
	KeyTokensDefaultTTLDays  = "tokens.default_ttl_days"  //nolint:gosec // setting key, not a credential
	KeyTokensUnusedAlertDays = "tokens.unused_alert_days" //nolint:gosec // setting key, not a credential
)

const tokensDaysCeiling = 3650

func init() {
	// No env seed: these are runtime settings only.
	Register(Definition{
		Key: KeyTokensDefaultTTLDays, Category: CategorySecurity, Kind: KindInt, Default: "0",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, tokensDaysCeiling),
		Title:       "Default API token lifetime",
		Description: "Days a new API token lasts when the request names no expiry, from 1 to 3650. 0 keeps the built-in default of 90 days. An explicit expiry, or no expiry from a root caller, always wins, and the security policy ceiling still applies. Existing tokens are untouched.",
	})
	Register(Definition{
		Key: KeyTokensUnusedAlertDays, Category: CategorySecurity, Kind: KindInt, Default: "0",
		Apply: ApplyLive, CheckValue: intRangeCheck(0, tokensDaysCeiling),
		Title:       "Unused API token alert after",
		Description: "Days without use after which a token is listed as unused in the token list, the doctor and the Security Center, from 1 to 3650. 0 turns the alert off. It only warns, no token is revoked.",
	})
}
