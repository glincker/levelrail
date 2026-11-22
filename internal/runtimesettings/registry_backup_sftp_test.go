package runtimesettings

import "testing"

func TestBackupSFTPSettingsRegistered(t *testing.T) {
	var found bool
	for _, d := range Definitions() {
		if d.Key == KeyBackupSFTPAllowPrivate {
			found = d.Risk == RiskWeakensSecurity && d.Apply == ApplyLive && d.Default == "false"
		}
	}
	if !found {
		t.Fatal("private network setting must be live, off by default and flagged as weakening security")
	}
	if got := ClampLimit(KeyBackupSFTPMaxConns, 0); got < 1 {
		t.Fatalf("connection clamp = %d, want at least 1", got)
	}
}
