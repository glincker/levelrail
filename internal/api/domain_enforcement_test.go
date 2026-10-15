package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// controlOf reads one control flag by its wire name.
func controlOf(e domainEnforcement, control string) bool {
	switch control {
	case "maintenance":
		return e.Maintenance
	case "basic_auth":
		return e.BasicAuth
	case "auth_gate":
		return e.AuthGate
	case "waf":
		return e.WAF
	case "redirect":
		return e.Redirect
	case "error_pages":
		return e.ErrorPages
	}
	panic("unknown control " + control)
}

var enforcementControls = []string{"maintenance", "basic_auth", "auth_gate", "waf", "redirect", "error_pages"}

func TestResolveEnforcement_PerControl(t *testing.T) {
	written := &store.ProxyRouteStatus{Domain: "a.example.com", Written: true, Reachable: true, CertTrusted: true}
	cases := []struct {
		name     string
		in       enforcementInput
		mode     string
		enforced bool
		reason   string
	}{
		{"embedded ingress", enforcementInput{Routed: true}, enforcementLevelrail, true, ""},
		{"unrouted", enforcementInput{Routed: false}, enforcementNone, false, enforcementReasonNotRouted},
		{"unrouted beats proxy", enforcementInput{Routed: false, Upstream: true, Route: written}, enforcementNone, false, enforcementReasonNotRouted},
		{"managed route not proven", enforcementInput{Routed: true, Upstream: true, Route: written}, enforcementProxyTraefik, false, enforcementReasonRouteUnverified},
		{"unmanaged proxy", enforcementInput{Routed: true, Upstream: true}, enforcementProxyExternal, false, enforcementReasonUnmanagedProxy},
		{"route row not written", enforcementInput{Routed: true, Upstream: true, Route: &store.ProxyRouteStatus{}}, enforcementProxyExternal, false, enforcementReasonUnmanagedProxy},
	}
	for _, control := range enforcementControls {
		for _, tc := range cases {
			t.Run(control+"/"+tc.name, func(t *testing.T) {
				got := resolveEnforcement(tc.in)
				if got.Mode != tc.mode || got.Reason != tc.reason {
					t.Fatalf("mode, reason = %q, %q; want %q, %q", got.Mode, got.Reason, tc.mode, tc.reason)
				}
				if controlOf(got, control) != tc.enforced {
					t.Errorf("%s enforced = %v, want %v", control, controlOf(got, control), tc.enforced)
				}
			})
		}
	}
}

// The live bug: a proxied domain with maintenance saved must not report it
// as enforced, and the traffic summary must not count it as in maintenance.
func TestListDomains_ProxiedMaintenanceIsNotEnforced(t *testing.T) {
	f := newTLSOwnerFixture(t, true, true)
	db, ok := f.rt.domainMaintenance.(*store.DB)
	if !ok {
		t.Fatal("domain maintenance store is not the test DB")
	}
	if err := db.SetDomainMaintenance(context.Background(), f.proxyHost); err != nil {
		t.Fatal(err)
	}
	var domains []domainResource
	if code := trafficCall(t, f.rt, f.cookie, http.MethodGet, "/api/v1/domains", &domains); code != http.StatusOK {
		t.Fatalf("domains status = %d", code)
	}
	var row *domainResource
	for i := range domains {
		if domains[i].Domain == f.proxyHost {
			row = &domains[i]
		}
	}
	if row == nil || row.Enforcement == nil {
		t.Fatalf("row = %+v, want an enforcement block", row)
	}
	if !row.MaintenanceEnabled {
		t.Error("the saved flag must still be reported as saved")
	}
	if row.Enforcement.Mode != enforcementProxyTraefik || row.Enforcement.Maintenance {
		t.Errorf("enforcement = %+v, want proxy:traefik with maintenance not enforced", *row.Enforcement)
	}
}

func TestListDomains_EmbeddedIngressEnforcesEverything(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 8080, Domains: []string{"own.example.com"}}); err != nil {
		t.Fatal(err)
	}
	cookie := loginTestSession(t, rt, db)
	var domains []domainResource
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains", &domains); code != http.StatusOK {
		t.Fatalf("domains status = %d", code)
	}
	if len(domains) == 0 || domains[0].Enforcement == nil {
		t.Fatalf("domains = %+v", domains)
	}
	e := *domains[0].Enforcement
	for _, control := range enforcementControls {
		if e.Mode != enforcementLevelrail || !controlOf(e, control) {
			t.Errorf("%s: enforcement = %+v, want levelrail and enforced", control, e)
		}
	}
}

// stripLevelrailEnforcement checks an embedded-ingress row reports every
// control enforced, then drops the block so older struct comparisons hold.
func stripLevelrailEnforcement(t *testing.T, row domainResource) domainResource {
	t.Helper()
	if row.Enforcement == nil || row.Enforcement.Mode != enforcementLevelrail || !row.Enforcement.Maintenance {
		t.Errorf("%s enforcement = %+v, want levelrail", row.Domain, row.Enforcement)
	}
	row.Enforcement = nil
	return row
}
