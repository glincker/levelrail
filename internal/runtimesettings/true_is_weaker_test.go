package runtimesettings

import "testing"

func TestSettingTrueIsWeaker(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	for _, tc := range []struct {
		key, val string
		want     bool
	}{
		{KeyDoctorReachabilityDialbackAllowPrivate, `true`, true},
		{KeyDoctorReachabilityDialbackAllowPrivate, `false`, false},
		{KeyVolumesImportOverwrite, `true`, true},
		{KeyVolumesImportOverwrite, `false`, false},
		{KeyTemplatesCapNetAdmin, `true`, true},
		{KeyTemplatesCapNetAdmin, `false`, false},
		{KeyAuditSensitiveReads, `false`, true},
	} {
		if got := svc.WeakensTo(t.Context(), tc.key, raw(tc.val)); got != tc.want {
			t.Errorf("WeakensTo(%s,%s)=%v want %v", tc.key, tc.val, got, tc.want)
		}
	}
}

// Writing a weakens_security bool back to its default is never a weakening, whatever the flags say.
func TestWeakeningBoolDefaultNeverNeedsConfirm(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	for _, d := range Definitions() {
		if d.Kind != KindBool || d.Risk != RiskWeakensSecurity {
			continue
		}
		if svc.WeakensTo(t.Context(), d.Key, raw(d.Default)) {
			t.Errorf("%s: writing the default %s asks for confirmation", d.Key, d.Default)
		}
	}
	if svc.WeakensTo(t.Context(), KeyK8sAllowClusterAdmin, raw(`false`)) {
		t.Error("k8s.allow_cluster_admin false must not weaken")
	}
	if !svc.WeakensTo(t.Context(), KeyK8sAllowClusterAdmin, raw(`true`)) {
		t.Error("k8s.allow_cluster_admin true must weaken")
	}
}
