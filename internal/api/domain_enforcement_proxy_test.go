package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestResolveEnforcement_ProvenDelivery(t *testing.T) {
	written := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		route store.ProxyRouteStatus
		want  bool
	}{
		{"proved after the write", store.ProxyRouteStatus{Written: true, WrittenAt: written, VerifiedAt: written.Add(time.Minute), Delivered: true}, true},
		{"marker never came back", store.ProxyRouteStatus{Written: true, WrittenAt: written, VerifiedAt: written.Add(time.Minute)}, false},
		{"proof predates the current file", store.ProxyRouteStatus{Written: true, WrittenAt: written, VerifiedAt: written.Add(-time.Minute), Delivered: true}, false},
		{"never verified", store.ProxyRouteStatus{Written: true, WrittenAt: written, Delivered: true}, false},
	}
	for _, control := range enforcementControls {
		for _, tc := range cases {
			t.Run(control+"/"+tc.name, func(t *testing.T) {
				r := tc.route
				got := resolveEnforcement(enforcementInput{Routed: true, Upstream: true, Route: &r})
				if got.Mode != enforcementProxyTraefik || controlOf(got, control) != tc.want {
					t.Errorf("enforcement = %+v, want %s enforced = %v", got, control, tc.want)
				}
			})
		}
	}
}

// Verification proves delivery with the ingress marker; a competing router
// that answers instead turns every control back to not enforced.
func TestProxyVerify_MarkerDecidesEnforcement(t *testing.T) {
	env := newProxyTestEnv(t)
	rc, err := ingress.NewRouteCheck()
	if err != nil {
		t.Fatal(err)
	}
	env.rt.SetProxyRouteCheck(rc)
	delivered := true
	base := env.rt.proxyIntegration.probe
	env.rt.proxyIntegration.probe = func(ctx context.Context, pt proxyroutes.ProbeTarget) proxyroutes.ProbeResult {
		res := base(ctx, pt)
		if pt.Marker != nil {
			if pt.Marker.Value != rc.Token || pt.Marker.Reply != rc.Reply {
				t.Errorf("marker = %+v", pt.Marker)
			}
			res.Delivered = delivered
		}
		return res
	}
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, body)
	}
	if e := domainEnforcementOf(t, env, "app.example.com"); e.Mode != enforcementProxyTraefik || !e.Maintenance || !e.BasicAuth {
		t.Fatalf("after proven delivery enforcement = %+v", e)
	}

	delivered = false
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/verify?domain=app.example.com", ""); code != http.StatusOK {
		t.Fatalf("verify: %d %s", code, body)
	}
	e := domainEnforcementOf(t, env, "app.example.com")
	if e.Maintenance || e.Reason != enforcementReasonRouteUnverified {
		t.Errorf("after a competing router enforcement = %+v", e)
	}
	rows, err := env.db.ListProxyRouteStatus(context.Background())
	if err != nil || len(rows) == 0 || rows[0].LastError != proxyNotDeliveredMessage {
		t.Errorf("route status = %+v, %v", rows, err)
	}
}

// Without a route check marker nothing can be proven, so nothing is enforced.
func TestProxyVerify_NoMarkerNeverEnforces(t *testing.T) {
	env := newProxyTestEnv(t)
	if code, body := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/setup", `{"confirm":true}`); code != http.StatusOK {
		t.Fatalf("setup: %d %s", code, body)
	}
	if e := domainEnforcementOf(t, env, "app.example.com"); e.Maintenance || e.Mode != enforcementProxyTraefik {
		t.Errorf("enforcement = %+v", e)
	}
}

func domainEnforcementOf(t *testing.T, env *proxyTestEnv, domain string) domainEnforcement {
	t.Helper()
	code, body := env.do(t, http.MethodGet, "/api/v1/domains", "")
	var rows []domainResource
	if err := json.Unmarshal(body, &rows); err != nil || code != http.StatusOK {
		t.Fatalf("domains: %d %s", code, body)
	}
	for _, r := range rows {
		if r.Domain == domain && r.Enforcement != nil {
			return *r.Enforcement
		}
	}
	t.Fatalf("no enforcement for %s in %s", domain, body)
	return domainEnforcement{}
}
