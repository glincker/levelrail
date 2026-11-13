package cloudiac

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestCloudPlannerAdapterNotReadyWithoutEngine(t *testing.T) {
	experimental.Set(experimental.Cloud)
	t.Cleanup(func() {
		experimental.Reset()
		iac.SetCloudPlanner(nil)
		iac.SetCloudPolicy(nil)
	})
	if (CloudPlannerAdapter{}).CloudReady() {
		t.Fatal("adapter without engine reports ready")
	}
	if !InstallInto(true, CloudPlannerAdapter{}, PolicyAdapter{}) {
		t.Fatal("install refused")
	}
	doc := "version: 1\nkind: CloudAccount\nmetadata: {name: oci-test}\nspec:\n  provider: oracle\n  region: us-phoenix-1\n  auth: {method: secret_ref, secretRef: oci-key}\n  limits: {monthlyCost: 0, instances: 1}\n" +
		"---\nversion: 1\nkind: CloudBucket\nmetadata: {name: backups}\nspec: {account: oci-test}\n"
	docs, issues := iac.ParseDocuments([]iac.Source{{Name: "c.yaml", Data: []byte(doc)}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	res, issues := iac.Build(docs, iac.Options{})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	st, err := iac.Load(t.Context(), nopDoer{}, res, iac.Options{})
	if err != nil {
		t.Fatal(err)
	}
	plan := iac.PlanFor(st, res, iac.Options{})
	if len(plan.Changes) != 2 {
		t.Fatalf("changes = %+v", plan.Changes)
	}
	for _, c := range plan.Changes {
		if c.Action != iac.ActionError || c.Code != iac.CodeUnavailable {
			t.Fatalf("item %+v", c)
		}
	}
}

type nopDoer struct{}

func (nopDoer) Do(context.Context, string, string, any, any) error { return nil }
