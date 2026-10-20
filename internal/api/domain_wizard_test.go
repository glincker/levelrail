package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAcmeActionForError(t *testing.T) {
	tests := []struct {
		name, msg, wantReason, wantAction string
	}{
		{"timeout is port 80", "Timeout during connect (likely firewall problem)", httpsHintUnreachable, acmeActionOpenPort80},
		{"nxdomain is dns", "DNS problem: NXDOMAIN looking up A for x.example.com", httpsHintDNS, acmeActionFixDNS},
		{"rate limit", "urn:ietf:params:acme:error:rateLimited: too many", httpsHintRateLimited, acmeActionWaitRetry},
		{"caa", "CAA record for example.com prevents issuance", httpsHintCAA, acmeActionFixCAA},
		{"unknown falls to logs", "something odd", httpsHintUnknown, acmeActionCheckLogs},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newACMEFailureResource(ingress.ACMEFailure{Error: tt.msg, At: time.Now()})
			if got.Reason != tt.wantReason || got.Action != tt.wantAction {
				t.Fatalf("got reason=%q action=%q, want %q %q", got.Reason, got.Action, tt.wantReason, tt.wantAction)
			}
		})
	}
}

func TestChallengeFor(t *testing.T) {
	tests := []struct {
		name, domain string
		private      bool
		want         string
	}{
		{"public host", "app.example.com", false, challengeHTTP01},
		{"private host needs dns-01", "app.example.com", true, challengeDNS01Required},
		{"wildcard needs dns-01", "*.example.com", false, challengeDNS01Required},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := challengeFor(tt.domain, tt.private); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestAllPrivate(t *testing.T) {
	tests := []struct {
		name  string
		addrs []string
		want  bool
	}{
		{"rfc1918", []string{"192.168.1.20"}, true},
		{"cgnat", []string{"100.64.1.2"}, true},
		{"public", []string{"203.0.113.10"}, false},
		{"mixed is not private", []string{"10.0.0.2", "8.8.8.8"}, false},
		{"empty", nil, false},
		{"ula v6", []string{"fd00::1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allPrivate(tt.addrs); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestConnectivityGuidance(t *testing.T) {
	open := []connectivityPort{{Port: 80, Reachable: true}, {Port: 443, Reachable: true}}
	half := []connectivityPort{{Port: 80, Reachable: false}, {Port: 443, Reachable: true}}
	tests := []struct {
		name    string
		host    string
		private bool
		ports   []connectivityPort
		want    string
	}{
		{"no host", "", false, nil, guidanceNoHost},
		{"private wins over open ports", "192.168.1.5", true, open, guidancePrivateAddress},
		{"port closed", "203.0.113.10", false, half, guidancePortsUnreachable},
		{"all open", "203.0.113.10", false, open, guidanceOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := connectivityGuidance(tt.host, tt.private, tt.ports); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestHandleIngressConnectivity_PrivateHost(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "192.168.1.50", nil)
	rt.doctorDialContext = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("refused")
	}
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ingress/connectivity", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got ingressConnectivityResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Private || got.HTTP01Possible || got.Guidance != guidancePrivateAddress || got.DNSProvider != dnsProviderNone {
		t.Fatalf("got %+v", got)
	}
	if len(got.Ports) != 2 || got.Ports[0].Reachable {
		t.Fatalf("ports = %+v", got.Ports)
	}
}

func TestHandleCheckDomain_PrivateHostRequiresDNS01WithFailure(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "10.0.0.5", func(context.Context, string) ([]string, error) {
		return []string{"10.0.0.5"}, nil
	})
	seedAppWithDomains(t, db, "nas.example.com")
	ingress.DefaultACMEFailures().Record(ingress.ACMEFailure{Identifier: "nas.example.com", Error: "Timeout during connect", At: time.Now()})
	t.Cleanup(func() { ingress.DefaultACMEFailures().Clear("nas.example.com") })
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/nas.example.com/check", ""))
	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.ExpectedPrivate || got.Challenge != challengeDNS01Required {
		t.Fatalf("got %+v", got)
	}
	if got.ACMEFailure == nil || got.ACMEFailure.Action != acmeActionOpenPort80 {
		t.Fatalf("acme failure = %+v", got.ACMEFailure)
	}
}

func TestEnvironmentDomainsEndpoints(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Domains: []string{"example.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_production"); err != nil {
		t.Fatal(err)
	}

	var res editDomainsResponse
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"environment":"dev","add":["dev.example.com"]}`, &res); code != http.StatusOK {
		t.Fatalf("env add = %d", code)
	}
	if res.EnvironmentID != "env_dev" || !slices.Equal(res.Domains, []string{"dev.example.com"}) {
		t.Fatalf("env add result = %+v", res)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"environment":"nope","add":["x.example.com"]}`, nil); code != http.StatusNotFound {
		t.Fatalf("unknown environment = %d, want 404", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPatch, "/api/v1/apps/web/domains", `{"environment":"uat","add":["example.com"]}`, nil); code != http.StatusOK {
		t.Fatalf("same-app duplicate across sets = %d, want 200", code)
	}

	var got appEnvironmentDomainsResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/environment-domains", "", &got); code != http.StatusOK {
		t.Fatalf("get = %d", code)
	}
	if !slices.Equal(got.RoutedDomains, []string{"example.com"}) || !slices.Equal(got.DefaultDomains, []string{"example.com"}) {
		t.Fatalf("routed = %v default = %v", got.RoutedDomains, got.DefaultDomains)
	}
	var dev *environmentDomainSetResource
	for i := range got.Environments {
		if got.Environments[i].EnvironmentID == "env_dev" {
			dev = &got.Environments[i]
		}
	}
	if dev == nil || !slices.Equal(dev.Domains, []string{"dev.example.com"}) || dev.Active {
		t.Fatalf("dev = %+v", dev)
	}

	if err := db.SetServiceEnvironment(ctx, "web", "env_dev"); err != nil {
		t.Fatal(err)
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/environment-domains", "", &got); code != http.StatusOK {
		t.Fatalf("get after move = %d", code)
	}
	if !slices.Equal(got.RoutedDomains, []string{"dev.example.com"}) {
		t.Fatalf("routed after move = %v", got.RoutedDomains)
	}
}
