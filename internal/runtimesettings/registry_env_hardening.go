package runtimesettings

import (
	"fmt"
	"strings"
)

// Keys for env key and value validation, read by internal/envrules callers.
const (
	KeyEnvKeyPolicy         = "env.key_policy"
	KeyEnvKeyMaxLength      = "env.key_max_length"
	KeyEnvValueMaxBytes     = "env.value_max_bytes"
	KeyEnvReservedPrefixes  = "env.reserved_prefixes"
	KeyEnvDangerousKeysMode = "env.dangerous_keys_mode"
	KeyEnvPathOverrideMode  = "env.path_override_mode"
)

func envPrefixCheck(item string) error {
	item = strings.TrimSpace(item)
	if item == "" || len(item) > 64 || strings.ContainsAny(item, "\x00\r\n\t =") {
		return fmt.Errorf("%q must be a prefix of 1 to 64 characters without spaces or =", item)
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyEnvKeyPolicy, Category: CategorySecurity, Kind: KindString, Default: "legacy",
		Env: "APP_ENV_KEY_POLICY", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		StrengthOrder: []string{"legacy", "strict"},
		CheckValue:    oneOfCheck("legacy", "strict"),
		Title:         "Env key naming policy",
		Description:   "strict accepts only letters, digits and underscore in new and changed keys. legacy also lets a key that already exists keep a dot or dash. Only the keys being written are checked, so saving other keys never fails because of an old name.",
	})
	Register(Definition{
		Key: KeyEnvKeyMaxLength, Category: CategorySecurity, Kind: KindInt, Default: "255",
		Env: "APP_ENV_KEY_MAX_LENGTH", Apply: ApplyLive, CheckValue: intRangeCheck(16, 1024),
		Title:       "Longest env key",
		Description: "Longest accepted env or secret key in characters, from 16 to 1024. Checked when a key is written.",
	})
	Register(Definition{
		Key: KeyEnvValueMaxBytes, Category: CategorySecurity, Kind: KindInt, Default: "65536",
		Env: "APP_ENV_VALUE_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1024, 1048576),
		Title:       "Largest env value",
		Description: "Largest accepted env or secret value in bytes, from 1024 to 1048576. A value already stored and unchanged is never refused.",
	})
	Register(Definition{
		Key: KeyEnvReservedPrefixes, Category: CategorySecurity, Kind: KindStringList, Default: "LEVELRAIL_",
		Env: "APP_ENV_RESERVED_PREFIXES", Apply: ApplyLive, Risk: RiskWeakensSecurity, RemovalIsWeaker: true, CheckItem: envPrefixCheck,
		Title:       "Reserved env prefixes",
		Description: "New env and secret keys starting with one of these prefixes (any letter case) are refused because the platform sets such keys itself. A key that already exists is not affected.",
	})
	Register(Definition{
		Key: KeyEnvDangerousKeysMode, Category: CategorySecurity, Kind: KindString, Default: "refuse",
		Env: "APP_ENV_DANGEROUS_KEYS_MODE", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		StrengthOrder: []string{"warn", "refuse"},
		CheckValue:    oneOfCheck("warn", "refuse"),
		Title:         "Code-loading env keys",
		Description:   "What happens to LD_PRELOAD, LD_LIBRARY_PATH, LD_AUDIT, DYLD_*, BASH_ENV and similar keys that change how processes load code. refuse blocks the write, warn accepts it and reports a warning.",
	})
	Register(Definition{
		Key: KeyEnvPathOverrideMode, Category: CategorySecurity, Kind: KindString, Default: "warn",
		Env: "APP_ENV_PATH_OVERRIDE_MODE", Apply: ApplyLive, Risk: RiskWeakensSecurity,
		StrengthOrder: []string{"off", "warn", "refuse"},
		CheckValue:    oneOfCheck("off", "warn", "refuse"),
		Title:         "PATH override in env",
		Description:   "What happens when an app sets PATH. off accepts silently, warn accepts and reports a warning, refuse blocks the write.",
	})
}
