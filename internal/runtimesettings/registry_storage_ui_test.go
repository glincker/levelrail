package runtimesettings

import (
	"strings"
	"testing"
)

func TestMibRangeErrorNamesBothUnits(t *testing.T) {
	check := mibRangeCheck(64, 1048576)
	err := check("10")
	if err == nil {
		t.Fatal("10 MiB is below the floor and must be rejected")
	}
	for _, want := range []string{"64", "1048576 MiB", "GB"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if err := check("2048"); err != nil {
		t.Errorf("2048 MiB must be accepted: %v", err)
	}
}

func TestStoragePercentSettingsRanges(t *testing.T) {
	for key, c := range map[string]struct{ ok, bad string }{
		KeyStorageLowFreePercent:      {"15", "51"},
		KeyStorageCriticalFreePercent: {"5", "26"},
	} {
		var def Definition
		for _, d := range Definitions() {
			if d.Key == key {
				def = d
			}
		}
		if def.CheckValue == nil {
			t.Fatalf("%s is not registered", key)
		}
		if err := def.CheckValue(c.ok); err != nil {
			t.Errorf("%s rejects %s: %v", key, c.ok, err)
		}
		if def.CheckValue(c.bad) == nil {
			t.Errorf("%s accepts %s", key, c.bad)
		}
	}
}
