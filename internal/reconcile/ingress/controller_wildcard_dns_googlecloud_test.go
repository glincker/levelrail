package ingress

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeGCDKeyResolver struct{ key string }

func (f fakeGCDKeyResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if serviceName != store.GoogleCloudDNSSecretsKey() || envKey != store.GoogleCloudDNSKeyEnvKey {
		return "", nil
	}
	return f.key, nil
}

func TestController_Reconcile_GoogleCloudDNS(t *testing.T) {
	tests := []struct {
		name         string
		settings     store.GoogleCloudDNSSettings
		key          string
		wantDNS01    bool
		wantPolicies int
	}{
		{"disabled", store.GoogleCloudDNSSettings{}, "key-json", false, 1},
		{"enabled without key", store.GoogleCloudDNSSettings{Enabled: true}, "", false, 1},
		{"enabled with key", store.GoogleCloudDNSSettings{Enabled: true}, "key-json", true, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wildcard := store.DesiredService{Name: "wild", Image: "img:v1", Port: 80, Domains: []string{"*.example.com"}}
			plain := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
			rt := newFakeRuntime()
			rt.seedRunning(application.ContainerName(wildcard.Name, wildcard.Image, ""), 34567)
			rt.seedRunning(application.ContainerName(plain.Name, plain.Image, ""), 34568)
			st := &fakeStore{
				services:    []store.DesiredService{wildcard, plain},
				settings:    store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
				gcdSettings: tc.settings,
			}
			applier := &fakeApplier{}
			c := New(st, rt, applier, WithLogger(discardLogger()), WithGoogleCloudDNSKey(fakeGCDKeyResolver{key: tc.key}))
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			policies := applier.lastCfg.Apps.TLS.Automation.Policies
			if len(policies) != tc.wantPolicies {
				t.Fatalf("policies = %d, want %d", len(policies), tc.wantPolicies)
			}
			raw, err := json.Marshal(policies)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(raw), `"googleclouddns"`); got != tc.wantDNS01 {
				t.Fatalf("googleclouddns in policies = %v, want %v: %s", got, tc.wantDNS01, raw)
			}
		})
	}
}
