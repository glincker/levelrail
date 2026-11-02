package runtimesettings

import (
	"context"
	"testing"
	"time"
)

func TestFirewallConfirmWindowDefaultAndFloor(t *testing.T) {
	var nilSvc *Service
	if d := nilSvc.FirewallConfirmWindow(context.Background()); d != time.Minute {
		t.Fatalf("default = %v, want 1m", d)
	}
	var def Definition
	for _, d := range Definitions() {
		if d.Key == KeyFirewallConfirmWindow {
			def = d
		}
	}
	if def.CheckValue == nil {
		t.Fatal("firewall.confirm_window is not registered with a range check")
	}
	for _, tc := range []struct {
		val string
		ok  bool
	}{{"9s", false}, {"10s", true}, {"10m", true}, {"11m", false}} {
		err := def.CheckValue(tc.val)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.val, err, tc.ok)
		}
	}
}
