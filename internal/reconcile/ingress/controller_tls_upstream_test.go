package ingress

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestController_Reconcile_TLSTerminatedUpstreamSkipsACME(t *testing.T) {
	tests := []struct {
		name     string
		settings store.IngressSettings
		wantACME bool
	}{
		{"acme on", store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"}, true},
		{"acme on behind upstream TLS", store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com", TLSTerminatedUpstream: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Domains: []string{"web.example.com"}}
			rt := newFakeRuntime()
			rt.seedRunning(application.ContainerName(desired.Name, desired.Image, ""), 34567)
			st := &fakeStore{services: []store.DesiredService{desired}, settings: tt.settings}
			applier := &fakeApplier{}
			c := New(st, rt, applier, WithLogger(discardLogger()))

			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			raw, err := json.Marshal(applier.lastCfg.Apps.TLS)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := strings.Contains(string(raw), `"module":"acme"`); got != tt.wantACME {
				t.Errorf("acme issuer present = %v, want %v (tls app: %s)", got, tt.wantACME, raw)
			}
		})
	}
}
