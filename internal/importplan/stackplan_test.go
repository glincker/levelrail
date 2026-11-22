package importplan

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStackPlanFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/stack.yml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Plan(context.Background(), Input{Text: string(data)}, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, r := range p.Resources {
		kinds[r.Kind]++
	}
	if kinds[ResourceApp] != 1 || kinds[ResourceDatabase] != 1 || kinds[ResourceVolume] != 2 || kinds[ResourceDomain] != 1 {
		t.Fatalf("resources: %+v", p.Resources)
	}
	foundPriv := false
	for _, u := range p.Unsupported {
		if u.Feature == "privileged" && u.Service == "web" && u.Reason != "" && u.Alternative != "" {
			foundPriv = true
		}
	}
	if !foundPriv {
		t.Fatalf("unsupported: %+v", p.Unsupported)
	}
	want := map[string]bool{"web/DATABASE_PASSWORD": true, "db/POSTGRES_PASSWORD": true}
	for _, r := range p.SecretRefs {
		delete(want, r)
	}
	if len(want) != 0 {
		t.Fatalf("secret refs: %v", p.SecretRefs)
	}
	out, _ := json.Marshal(p.Services)
	for _, leak := range []string{"hunter2hunter2", "s3cretvalue"} {
		if strings.Contains(string(out), leak) || strings.Contains(strings.Join(p.SecretRefs, ","), leak) {
			t.Fatalf("secret value leaked: %s", leak)
		}
	}
}

func TestMarkConflicts(t *testing.T) {
	p := &DeploymentPlan{Resources: []PlannedResource{{Kind: ResourceApp, Name: "stack"}, {Kind: ResourceVolume, Name: "stack"}}}
	p.MarkConflicts(map[string]bool{"stack": true})
	if len(p.Conflicts) != 1 || p.Conflicts[0].Kind != ResourceApp {
		t.Fatalf("conflicts: %+v", p.Conflicts)
	}
	p = &DeploymentPlan{Resources: p.Resources}
	p.MarkConflicts(nil)
	if len(p.Conflicts) != 0 {
		t.Fatal("unexpected conflict")
	}
}
