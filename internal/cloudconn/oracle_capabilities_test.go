package cloudconn

import (
	"strings"
	"testing"
)

func TestBuildOraclePolicyKitScopes(t *testing.T) {
	kit, err := BuildOraclePolicyKit(OraclePolicyKitInput{Group: "g1", Compartment: "lr-test", Manage: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kit.PolicyText, "manage instance-family in compartment lr-test") || !strings.Contains(kit.PolicyText, "read usage-budgets in tenancy") {
		t.Fatalf("policy text = %q", kit.PolicyText)
	}
	for _, bad := range []string{"tenancy", "root", "bad name"} {
		if _, err := BuildOraclePolicyKit(OraclePolicyKitInput{Group: "g1", Compartment: bad}); err == nil {
			t.Fatalf("compartment %q must be refused", bad)
		}
	}
}

func TestOracleCredentialFromStoredRejectsGarbage(t *testing.T) {
	if _, err := OracleCredentialFromStored("not json"); err == nil {
		t.Fatal("garbage must fail")
	}
}
