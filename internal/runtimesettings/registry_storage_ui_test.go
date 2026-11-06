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
