package spec

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func TestBudgetCap(t *testing.T) {
	tests := []struct {
		name    string
		in      Budget
		wantErr bool
		check   func(spendcap.Cap) bool
	}{
		{"egress and pause", Budget{Egress: "500GiB", Action: "pause"}, false, func(c spendcap.Cap) bool {
			return c.MonthlyEgressBytes == 500<<30 && c.Action == spendcap.ActionPause && len(c.Thresholds) == 3
		}},
		{"cost only throttle", Budget{Cost: 40, Action: "throttle"}, false, func(c spendcap.Cap) bool {
			return c.MonthlyCostCeiling == 40 && c.ThrottleRPS == spendcap.DefaultThrottleRPS
		}},
		{"empty block", Budget{}, true, nil},
		{"bad size", Budget{Egress: "lots"}, true, nil},
		{"bad action", Budget{Egress: "1GB", Action: "reboot"}, true, nil},
		{"bad threshold", Budget{Egress: "1GB", Thresholds: []int{150}}, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.in.Cap("web")
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v", err)
			}
			if tc.check != nil && !tc.check(c) {
				t.Errorf("cap = %+v", c)
			}
		})
	}
	var none *Budget
	if err := none.Validate("web"); err != nil {
		t.Errorf("nil block must be valid: %v", err)
	}
}
