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
		{KeyAuditSensitiveReads, `false`, true},
	} {
		if got := svc.WeakensTo(t.Context(), tc.key, raw(tc.val)); got != tc.want {
			t.Errorf("WeakensTo(%s,%s)=%v want %v", tc.key, tc.val, got, tc.want)
		}
	}
}
