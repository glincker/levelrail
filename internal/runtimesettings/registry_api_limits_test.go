package runtimesettings

import "testing"

func TestAPILimitsDefaultsAndEnvSeed(t *testing.T) {
	want := map[string]int{
		KeyAppsMetricsMax: 200, KeyLogExportMaxLines: 5000, KeyStackHistoryLimit: 50, KeyMaxTagsPerApp: 20,
		KeyFixitMaxPerHour: 0, KeyDeployApprovalClaimTimeoutSec: 900, KeyApproveDigestCheckTimeoutSec: 0,
	}
	svc := New(newFakeStore(), envOf(nil))
	for key, n := range want {
		if got := svc.Limit(t.Context(), key); got != n {
			t.Errorf("%s default = %d, want %d", key, got, n)
		}
	}
	d, _ := svc.definition(KeyMaxTagsPerApp)
	seeded := New(newFakeStore(), envOf(map[string]string{d.Env: "3"}))
	if got := seeded.Limit(t.Context(), KeyMaxTagsPerApp); got != 3 {
		t.Errorf("env seed = %d, want 3", got)
	}
}
