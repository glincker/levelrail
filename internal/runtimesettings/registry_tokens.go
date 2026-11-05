package runtimesettings

import (
	"context"
	"time"
)

// Token hygiene settings. Both default to 0 so an existing install behaves as before.
const (
	KeyTokensDefaultTTLDays  = "tokens.default_ttl_days"  //nolint:gosec // setting key, not a credential
	KeyTokensUnusedAlertDays = "tokens.unused_alert_days" //nolint:gosec // setting key, not a credential

	KeyTokensExpiryWarningDays      = "tokens.expiry_warning_days"          //nolint:gosec // setting key, not a credential
	KeyTokensSystemRotationLeadMins = "tokens.system_rotation_lead_minutes" //nolint:gosec // setting key, not a credential
)

const (
	maxTokenWarningDays   = 90
	maxSystemRotationMins = 1440
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
	Register(Definition{
		Key: KeyTokensExpiryWarningDays, Category: CategorySecurity, Kind: KindInt, Default: "7",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, maxTokenWarningDays),
		Title:       "API token expiry warning",
		Description: "Days before a token you created expires that it appears as one grouped row on Status, from 1 to 90. It also bounds how long an expired token stays listed and what counts as still in use. Platform-owned tokens never show.",
	})
	Register(Definition{
		Key: KeyTokensSystemRotationLeadMins, Category: CategorySecurity, Kind: KindInt, Default: "30",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, maxSystemRotationMins),
		Title:       "Platform token rotation lead",
		Description: "Minutes before a platform-owned token (such as the stack drift read token) expires that the platform replaces it and revokes the old one itself, from 1 to 1440. Tokens you created are never rotated automatically.",
	})
}

// SystemTokenRotationLead reads the rotation lead, forced into its safe range.
func (s *Service) SystemTokenRotationLead(ctx context.Context) time.Duration {
	mins := min(max(s.Int(ctx, KeyTokensSystemRotationLeadMins), 1), maxSystemRotationMins)
	return time.Duration(mins) * time.Minute
}
