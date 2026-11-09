package runtimesettings

import "testing"

func TestSpoolSettingsRangeChecks(t *testing.T) {
	for _, key := range []string{KeySpoolMaxAgeSeconds, KeySpoolQuotaBytes, KeyAPIExpensiveReadTTL} {
		var d Definition
		for _, c := range Definitions() {
			if c.Key == key {
				d = c
			}
		}
		if d.CheckValue == nil {
			t.Fatalf("%s not registered", key)
		}
		if err := d.CheckValue("0"); err != nil {
			t.Errorf("%s: 0 should keep the env seed: %v", key, err)
		}
		if err := d.CheckValue("-1"); err == nil {
			t.Errorf("%s: negative accepted", key)
		}
	}
}
