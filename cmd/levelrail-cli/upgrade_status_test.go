package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintUpgradeStatusAhead(t *testing.T) {
	latest, schema := "v0.7.0", 2690
	var out bytes.Buffer
	printUpgradeStatus(&out, updatePreflight{CurrentVersion: "dev", LatestVersion: &latest, State: "ahead_of_releases", StateReason: "newer data", DBSchema: &schema})
	for _, want := range []string{"state:           ahead_of_releases", "reason:          newer data", "2690"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}
