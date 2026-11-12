package runtimesettings

import (
	"fmt"
	"strings"
)

// Secret reference policy: environments where literal secret values are not accepted.
const (
	KeySecretsRequireRefEnvironments = "secrets.require_ref_environments" //nolint:gosec // setting key, not a credential
	KeySecretsRequireRefMode         = "secrets.require_ref_mode"         //nolint:gosec // setting key, not a credential

	SecretRefModeOff     = "off"
	SecretRefModeWarn    = "warn"
	SecretRefModeEnforce = "enforce"

	secretRefEnvNameMax = 128
)

func requireRefEnvCheck(item string) error {
	item = strings.TrimSpace(item)
	if item == "" || len(item) > secretRefEnvNameMax || strings.ContainsAny(item, "\x00\r\n\t") {
		return fmt.Errorf("%q must be an environment name of 1 to %d characters without control characters", item, secretRefEnvNameMax)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeySecretsRequireRefEnvironments, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_SECRETS_REQUIRE_REF_ENVIRONMENTS", Apply: ApplyLive, Risk: RiskWeakensSecurity, RemovalIsWeaker: true, CheckItem: requireRefEnvCheck,
		Title:       "Environments that require secret references",
		Description: "Environment names (matched without regard to case) where secret values must come from a secret:// reference instead of a literal. What happens to a literal is set by secrets.require_ref_mode. Empty by default, so no environment is affected.",
	})
	Register(Definition{
		Key: KeySecretsRequireRefMode, Category: CategorySecurity, Kind: KindString, Default: SecretRefModeOff,
		Env: "APP_SECRETS_REQUIRE_REF_MODE", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		StrengthOrder: []string{SecretRefModeOff, SecretRefModeWarn, SecretRefModeEnforce},
		CheckValue:    oneOfCheck(SecretRefModeOff, SecretRefModeWarn, SecretRefModeEnforce),
		Title:         "Literal secret handling in required environments",
		Description:   "off ignores the rule, warn accepts literal secrets and lists them in app details and deploy results, enforce refuses new or changed literal secrets with a 422 that names the variable. Existing values are never deleted. Applies only to environments listed in secrets.require_ref_environments.",
	})
}
