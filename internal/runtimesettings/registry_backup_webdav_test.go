package runtimesettings

import "testing"

func TestBackupWebDAVSettingsRegistered(t *testing.T) {
	var found bool
	for _, d := range Definitions() {
		if d.Key == KeyBackupWebDAVAllowPrivate {
			found = d.Risk == RiskWeakensSecurity && d.Apply == ApplyLive && d.Default == "false"
		}
	}
	if !found {
		t.Fatal("private network setting must be live and flagged as weakening security")
	}
	if got := ClampLimit(KeyBackupWebDAVMaxDepth, 0); got < 1 {
		t.Fatalf("depth clamp = %d, want at least 1", got)
	}
}
