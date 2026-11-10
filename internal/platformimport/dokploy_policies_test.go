package platformimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
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

func TestDokployDiscoverBackups(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/project.all":     "dokploy/project_backups.json",
		"/api/destination.all": "dokploy/destinations.json",
	}, "", "x-api-key", "key-fixture")
	src, err := NewDokploy(srv.URL, "key-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Destinations) != 2 || !d.Destinations[0].HasCredentials || d.Destinations[1].HasCredentials {
		t.Fatalf("destinations: %+v", d.Destinations)
	}
	if len(d.Databases) != 2 || len(d.Databases[0].Backups) != 2 || d.Databases[0].Backups[1].Schedule != "0 * * * *" || d.Databases[0].Backups[0].KeepLatest != 7 {
		t.Fatalf("databases: %+v", d.Databases)
	}
	if strings.Contains(fmt.Sprintf("%+v", d), "fixture-secret") {
		t.Fatal("discovery carries a destination key")
	}
	ak, sk, err := src.DestinationCredentials(context.Background(), "dst-r2")
	if err != nil || ak != "fixture-access-r2" || sk != "fixture-secret-r2" {
		t.Fatalf("credentials: %v", err)
	}
	if _, _, err := src.DestinationCredentials(context.Background(), "dst-minio"); !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("masked credentials: %v", err)
	}
}

func TestValidateDestinationEndpoint(t *testing.T) {
	ctx := context.Background()
	for _, bad := range []string{"http://s3.example.com", "https://user:pw@s3.example.com", "https://127.0.0.1", "https://10.0.0.5", "https://169.254.169.254", "s3.example.com"} {
		if ValidateDestinationEndpoint(ctx, bad, NetworkPolicy{}) == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if err := ValidateDestinationEndpoint(ctx, "https://1.1.1.1", NetworkPolicy{}); err != nil {
		t.Errorf("public endpoint refused: %v", err)
	}
}

func TestPolicyBlocksEmbeddedRanges(t *testing.T) {
	open := NetworkPolicy{AllowPrivate: true, AllowLoopback: true}
	for _, a := range []string{"64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::1", "::a9fe:a9fe", "0.1.2.3"} {
		if open.Check(netip.MustParseAddr(a)) == nil {
			t.Errorf("%s allowed", a)
		}
	}
	if open.Check(netip.MustParseAddr("::1")) != nil {
		t.Error("loopback opt-in broke")
	}
}
