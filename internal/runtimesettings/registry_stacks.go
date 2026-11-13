package runtimesettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Registered keys for stack and IaC apply. Env names stay as seeds; call sites
// still read env until each slice moves them to the registry.
const (
	KeyIaCApplyEnabled           = "iac.apply_enabled"
	KeyIaCV1HashAccepted         = "iac.v1_hash_accepted"
	KeyStacksSoloSelfApprove     = "stacks.solo_self_approve"
	KeyStackLockTTL              = "stacks.lock_ttl"
	KeyStackLockRenewFraction    = "stacks.lock_renew_fraction"
	KeyStackApprovalTTL          = "stacks.approval_ttl"
	KeyStackOIDCTTL              = "stacks.oidc_ttl"
	KeyStackDriftIntervalSecs    = "stacks.drift_interval_seconds"
	KeyStackDriftSummaryMax      = "stacks.drift_summary_max"
	KeyStackDriftRotationMinutes = "stacks.drift_rotation_minutes"
)

// stackMinutesFromEnv converts a Go duration env value into whole minutes.
func stackMinutesFromEnv(raw string) (string, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return "", fmt.Errorf("%q is not a duration such as 2h", raw)
	}
	return strconv.FormatInt(int64(d/time.Minute), 10), nil
}

// stackSecondsFromEnv keeps the Go duration env form, where 0 turns the drift loop off.
func stackSecondsFromEnv(raw string) (string, error) {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d < 0 {
		return "", fmt.Errorf("%q is not a duration such as 15m, or 0 to turn drift checks off", raw)
	}
	return strconv.FormatInt(int64(d/time.Second), 10), nil
}

func init() {
	Register(Definition{
		Key: KeyIaCApplyEnabled, Category: CategoryDeploys, Kind: KindBool, Default: "true", Env: "APP_IAC_APPLY_ENABLED", Apply: ApplyLive,
		Title:       "Stack apply kill switch",
		Description: "Turn off to refuse every stack apply while plans and reads keep working. The next apply request uses a changed value.",
	})
	Register(Definition{
		Key: KeyIaCV1HashAccepted, Category: CategorySecurity, Kind: KindBool, Default: "true", Env: "APP_IAC_V1_HASH_ACCEPTED", Apply: ApplyLive,
		Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Accept legacy plan hashes",
		Description: "Accept the older unkeyed plan hash on apply during the transition. Turn off so only per-install keyed hashes approve a plan. The next apply uses a changed value.",
	})
	Register(Definition{
		Key: KeyStacksSoloSelfApprove, Category: CategorySecurity, Kind: KindBool, Default: "false", Env: "APP_STACK_SOLO_SELF_APPROVE", Apply: ApplyLive,
		Risk: RiskWeakensSecurity, TrueIsWeaker: true,
		Title:       "Solo admin self approval",
		Description: "Let the only admin of an install approve their own stack plan. Off requires a second person. The next approval uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackLockTTL, Category: CategoryDeploys, Kind: KindDuration, Default: "10m", Env: "APP_STACK_LOCK_TTL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(5*time.Second, time.Hour),
		Title:       "Stack apply lease",
		Description: "How long a stack apply lease lasts before another apply may take it, from 5s to 1h. The running apply renews it. The next apply uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackLockRenewFraction, Category: CategoryDeploys, Kind: KindInt, Default: "3", Env: "APP_STACK_LOCK_RENEW_FRACTION", Apply: ApplyLive,
		CheckValue:  intRangeCheck(2, 100),
		Title:       "Stack lease renewal divisor",
		Description: "The apply renews its lease every lease length divided by this value, from 2 to 100. Higher values renew more often. The next apply uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackApprovalTTL, Category: CategoryDeploys, Kind: KindDuration, Default: "24h", Env: "APP_STACK_APPROVAL_TTL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Minute, 168*time.Hour),
		Title:       "Stack plan approval window",
		Description: "How long a pending stack plan waits for approval before it expires, from 1m to 168h. The next plan uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackOIDCTTL, Category: CategorySecurity, Kind: KindDuration, Default: "15m", Env: "APP_STACK_OIDC_TTL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Minute, time.Hour),
		Title:       "Stack CI token lifetime",
		Description: "How long a CI token minted for a stack apply stays valid, from 1m to 1h. The next token uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackDriftIntervalSecs, Category: CategoryDeploys, Kind: KindInt, Default: "900",
		Env: "APP_STACK_DRIFT_INTERVAL", EnvConvert: stackSecondsFromEnv, Apply: ApplyLive,
		CheckValue:  intRangeCheck(0, 86400),
		Title:       "Stack drift check interval",
		Description: "Seconds between drift checks of stacks, from 0 to 86400. 0 turns drift checks off. The next pass uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackDriftSummaryMax, Category: CategoryObservability, Kind: KindInt, Default: "50", Env: "APP_STACK_DRIFT_SUMMARY_MAX", Apply: ApplyLive,
		CheckValue:  intRangeCheck(1, 1000),
		Title:       "Stack drift summary items",
		Description: "Most drifted items one drift summary lists, from 1 to 1000. The next drift check uses a changed value.",
	})
	Register(Definition{
		Key: KeyStackDriftRotationMinutes, Category: CategorySecurity, Kind: KindInt, Default: "120", Env: "APP_STACK_DRIFT_TOKEN_TTL",
		EnvConvert: stackMinutesFromEnv, Apply: ApplyLive, CheckValue: intRangeCheck(10, 1440),
		Title:       "Stack drift token lifetime",
		Description: "How long the shared stack drift token lives before it rotates, in minutes from 10 to 1440. The next rotation uses a changed value.",
	})
}
