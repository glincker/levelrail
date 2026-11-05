package api

import "testing"

func TestRuleResourceToRule_DomainLifecycleKinds(t *testing.T) {
	cases := []struct {
		name string
		res  ruleResource
		ok   bool
	}{
		{"drop defaults", ruleResource{Name: "n", Kind: "domain_traffic_drop", ResourceID: "service:web"}, true},
		{"drop custom", ruleResource{Name: "n", Kind: "domain_traffic_drop", Threshold: 30, ForDuration: "2h", RestartCountThreshold: 50}, true},
		{"drop percent over 100", ruleResource{Name: "n", Kind: "domain_traffic_drop", Threshold: 120}, false},
		{"drop window over a day", ruleResource{Name: "n", Kind: "domain_traffic_drop", ForDuration: "25h"}, false},
		{"parked defaults", ruleResource{Name: "n", Kind: "parked_domain_cert_expiry"}, true},
		{"parked negative days", ruleResource{Name: "n", Kind: "parked_domain_cert_expiry", Threshold: -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.res.toRule("id"); (err == nil) != tc.ok {
				t.Errorf("err=%v, want ok=%v", err, tc.ok)
			}
		})
	}
}
