package main

import "testing"

// self-upgrade reads free disk in the data dir, so it must adopt the unit APP_DATA_DIR like other host commands.
func TestSelfUpgradeIsHostCommand(t *testing.T) {
	for _, c := range []string{"self-upgrade", "rollback", "restore-point"} {
		if !hostCommands[c] {
			t.Errorf("%s must adopt the installed data dir", c)
		}
	}
}
