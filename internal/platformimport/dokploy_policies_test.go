package platformimport

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDokployPoliciesFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/dokploy/app_policies.json")
	if err != nil {
		t.Fatal(err)
	}
	var a dokployApp
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	app, unsup := mapDokployApp("p", a)
	if unsup != nil {
		t.Fatalf("unsupported: %+v", unsup)
	}
	if app.Ingress == nil || !app.Ingress.BasicAuth || len(app.Ingress.Forwarders) != 1 {
		t.Fatalf("ingress = %+v, want basic auth and one forwarder", app.Ingress)
	}
	if app.Build.PreDeploy != "npm run migrate" || app.Build.PostDeploy != "npm run warm" {
		t.Fatalf("build hooks = %q / %q", app.Build.PreDeploy, app.Build.PostDeploy)
	}
	dump, _ := json.Marshal(app)
	if strings.Contains(string(dump), "must-not-leak-pw") {
		t.Fatal("basic-auth password reached the model")
	}
	var ports bool
	for _, n := range app.Notes {
		ports = ports || strings.Contains(n.Reason, "9000:9000/tcp")
	}
	if !ports {
		t.Fatal("extra port was not reported")
	}
}
