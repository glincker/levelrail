package runtimesettings

import "testing"

func TestScheduledTasksHistoryKeepBounds(t *testing.T) {
	var def Definition
	for _, d := range Definitions() {
		if d.Key == KeyScheduledTasksHistoryKeep {
			def = d
		}
	}
	if def.CheckValue == nil {
		t.Fatal("setting not registered")
	}
	if err := def.CheckValue("0"); err == nil {
		t.Fatal("0 accepted")
	}
	if err := def.CheckValue("50"); err != nil {
		t.Fatalf("50 rejected: %v", err)
	}
}
