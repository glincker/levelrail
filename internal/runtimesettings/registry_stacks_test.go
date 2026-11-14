package runtimesettings

import (
	"testing"
	"time"
)

func TestStackSettingsRegistry(t *testing.T) {
	keys := []string{
		KeyIaCApplyEnabled, KeyIaCV1HashAccepted, KeyStacksSoloSelfApprove,
		KeyStackLockTTL, KeyStackLockRenewFraction, KeyStackApprovalTTL, KeyStackOIDCTTL,
		KeyStackDriftIntervalSecs, KeyStackDriftSummaryMax, KeyStackDriftRotationMinutes,
	}
	known := map[string]bool{}
	for _, d := range Definitions() {
		known[d.Key] = true
	}
	for _, key := range keys {
		if !known[key] {
			t.Errorf("%s is not registered", key)
		}
	}

	svc := New(newFakeStore(), envOf(nil))
	ctx := t.Context()
	if !svc.Bool(ctx, KeyIaCApplyEnabled) || !svc.Bool(ctx, KeyIaCV1HashAccepted) {
		t.Error("iac apply and v1 hash acceptance must default to on")
	}
	if svc.Bool(ctx, KeyStacksSoloSelfApprove) {
		t.Error("solo self approval must default to off")
	}
	if got := svc.Duration(ctx, KeyStackLockTTL); got != 10*time.Minute {
		t.Errorf("lock ttl default = %s, want 10m", got)
	}
	if got := svc.Int(ctx, KeyStackLockRenewFraction); got != 3 {
		t.Errorf("lock renew fraction default = %d, want 3", got)
	}
	if got := svc.Duration(ctx, KeyStackApprovalTTL); got != 24*time.Hour {
		t.Errorf("approval ttl default = %s, want 24h", got)
	}
	if got := svc.Duration(ctx, KeyStackOIDCTTL); got != 15*time.Minute {
		t.Errorf("oidc ttl default = %s, want 15m", got)
	}
	if got := svc.Int(ctx, KeyStackDriftIntervalSecs); got != 900 {
		t.Errorf("drift interval default = %d seconds, want 900", got)
	}
	if got := svc.Int(ctx, KeyStackDriftSummaryMax); got != 50 {
		t.Errorf("drift summary default = %d, want 50", got)
	}
	if got := svc.Int(ctx, KeyStackDriftRotationMinutes); got != 120 {
		t.Errorf("drift token minutes default = %d, want 120", got)
	}

	seeded := New(newFakeStore(), envOf(map[string]string{
		"APP_IAC_APPLY_ENABLED":         "false",
		"APP_STACK_SOLO_SELF_APPROVE":   "true",
		"APP_STACK_LOCK_TTL":            "2m",
		"APP_STACK_LOCK_RENEW_FRACTION": "4",
		"APP_STACK_APPROVAL_TTL":        "3h",
		"APP_STACK_OIDC_TTL":            "5m",
		"APP_STACK_DRIFT_INTERVAL":      "0",
		"APP_STACK_DRIFT_SUMMARY_MAX":   "7",
		"APP_STACK_DRIFT_TOKEN_TTL":     "30m",
	}))
	if seeded.Bool(ctx, KeyIaCApplyEnabled) || !seeded.Bool(ctx, KeyStacksSoloSelfApprove) {
		t.Error("bool env seeds not applied")
	}
	if got := seeded.Duration(ctx, KeyStackLockTTL); got != 2*time.Minute {
		t.Errorf("lock ttl seed = %s, want 2m", got)
	}
	if got := seeded.Int(ctx, KeyStackLockRenewFraction); got != 4 {
		t.Errorf("lock renew seed = %d, want 4", got)
	}
	if got := seeded.Duration(ctx, KeyStackApprovalTTL); got != 3*time.Hour {
		t.Errorf("approval ttl seed = %s, want 3h", got)
	}
	if got := seeded.Duration(ctx, KeyStackOIDCTTL); got != 5*time.Minute {
		t.Errorf("oidc ttl seed = %s, want 5m", got)
	}
	if got := seeded.Int(ctx, KeyStackDriftIntervalSecs); got != 0 {
		t.Errorf("drift interval seed = %d seconds, want 0 (off)", got)
	}
	if got := seeded.Int(ctx, KeyStackDriftSummaryMax); got != 7 {
		t.Errorf("drift summary seed = %d, want 7", got)
	}
	if got := seeded.Int(ctx, KeyStackDriftRotationMinutes); got != 30 {
		t.Errorf("drift token minutes seed = %d, want 30", got)
	}

	if !svc.WeakensTo(ctx, KeyStacksSoloSelfApprove, raw("true")) {
		t.Error("turning solo self approval on must count as weakening")
	}
}
