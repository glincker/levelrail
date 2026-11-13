package cloudiac

import "testing"

var testPrices = StaticPrices{Date: "2026-11-01", Prices: map[string]int64{
	"oci_core_instance":        10_000_000,
	"oci_objectstorage_bucket": 2_000_000,
	"oci_core_vcn":             0,
}}

func TestCostEstimate(t *testing.T) {
	cases := []struct {
		name     string
		policy   CostPolicy
		changes  []ResourceChange
		existing int64
		total    int64
		want     []string
	}{
		{"free network under zero limit", CostPolicy{}, []ResourceChange{{"n", "oci_core_vcn", ActionCreate}}, 0, 0, nil},
		{"instance over zero limit", CostPolicy{}, []ResourceChange{{"i", "oci_core_instance", ActionCreate}}, 0, 10_000_000, []string{CodeCostLimit}},
		{"within limit", CostPolicy{MonthlyLimitMicros: 15_000_000}, []ResourceChange{{"i", "oci_core_instance", ActionCreate}, {"b", "oci_objectstorage_bucket", ActionCreate}}, 0, 12_000_000, nil},
		{"existing counts", CostPolicy{MonthlyLimitMicros: 15_000_000}, []ResourceChange{{"i", "oci_core_instance", ActionCreate}}, 8_000_000, 18_000_000, []string{CodeCostLimit}},
		{"delete reduces", CostPolicy{MonthlyLimitMicros: 5_000_000}, []ResourceChange{{"i", "oci_core_instance", ActionDelete}}, 10_000_000, 0, nil},
		{"unpriced blocks", CostPolicy{MonthlyLimitMicros: 99}, []ResourceChange{{"d", "oci_database_db_system", ActionCreate}}, 0, 0, []string{CodeCostUnpriced}},
		{"unpriced allowed", CostPolicy{MonthlyLimitMicros: 99, AllowUnpriced: true}, []ResourceChange{{"d", "oci_database_db_system", ActionCreate}}, 0, 0, nil},
		{"update ignored", CostPolicy{}, []ResourceChange{{"i", "oci_core_instance", ActionUpdate}}, 0, 0, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			est := tc.policy.Estimate(testPrices, tc.changes, tc.existing)
			if est.MonthlyMicros != tc.total {
				t.Fatalf("total %d want %d", est.MonthlyMicros, tc.total)
			}
			got := codes(est.Refusals)
			if len(got) != len(tc.want) || (len(got) > 0 && got[0] != tc.want[0]) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if est.Allowed() != (len(tc.want) == 0) || est.AsOf != "2026-11-01" {
				t.Fatalf("verdict or date wrong: %+v", est)
			}
		})
	}
}

func TestCostPolicyFromEnv(t *testing.T) {
	if CostPolicyFromEnv(func(string) string { return "" }, 5).AllowUnpriced {
		t.Fatal("default must block unpriced")
	}
	if !CostPolicyFromEnv(func(string) string { return "off" }, 5).AllowUnpriced {
		t.Fatal("off must allow")
	}
}
