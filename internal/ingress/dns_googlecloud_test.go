package ingress

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
)

func TestGoogleCloudDNSProviderWireShape(t *testing.T) {
	iss := NewDNSACMEIssuer("ops@example.com", "", GoogleCloudDNSProvider{Name: "googleclouddns", ServiceAccountJSON: "{}"})
	b, err := json.Marshal(iss)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"name":"googleclouddns"`) || !strings.Contains(string(b), `"service_account_json"`) {
		t.Fatalf("issuer json = %s", b)
	}
}

func TestGoogleCloudDNSModuleRegisteredAndRejectsBadKey(t *testing.T) {
	info, err := caddy.GetModule("dns.providers.googleclouddns")
	if err != nil {
		t.Fatalf("module not registered: %v", err)
	}
	m, ok := info.New().(*GoogleCloudDNSModule)
	if !ok {
		t.Fatalf("unexpected module type %T", info.New())
	}
	m.ServiceAccountJSON = `{"not":"a key"}`
	if err := m.Provision(caddy.Context{}); err == nil {
		t.Fatal("Provision should fail without a project_id")
	}
}
