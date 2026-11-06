package runtimesettings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	minSystemRotationMins = 10
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
		Env: "APP_ATTENTION_TOKEN_WINDOW", EnvConvert: tokenWindowDaysFromEnv,
		Apply: ApplyLive, CheckValue: intRangeCheck(1, maxTokenWarningDays),
		Title:       "API token expiry warning",
		Description: "Days before a token you created expires that it appears as one grouped row on Status, from 1 to 90. It also bounds how long an expired token stays listed and what counts as still in use. Platform-owned tokens never show.",
	})
	Register(Definition{
		Key: KeyTokensSystemRotationLeadMins, Category: CategorySecurity, Kind: KindInt, Default: "30",
		Apply: ApplyLive, CheckValue: intRangeCheck(minSystemRotationMins, maxSystemRotationMins),
		Title:       "Platform token rotation lead",
		Description: "Minutes before a platform-owned token (such as the stack drift read token) expires that the platform replaces it and revokes the old one itself, from 10 to 1440, and never less than an eighth of the token lifetime. Tokens you created are never rotated automatically.",
	})
}

// SystemTokenRotationLead reads the rotation lead, forced into its safe range.
func (s *Service) SystemTokenRotationLead(ctx context.Context) time.Duration {
	mins := min(max(s.Int(ctx, KeyTokensSystemRotationLeadMins), minSystemRotationMins), maxSystemRotationMins)
	return time.Duration(mins) * time.Minute
}

// tokenWindowDaysFromEnv turns the legacy duration form of the window into whole days within the setting's range.
func tokenWindowDaysFromEnv(raw string) (string, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return "", fmt.Errorf("%q is not a positive duration such as 168h", raw)
	}
	days := min(max(int((d+24*time.Hour-1)/(24*time.Hour)), 1), maxTokenWarningDays)
	return strconv.Itoa(days), nil
}
