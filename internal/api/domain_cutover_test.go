package api

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/domaincutover"
)

func cutoverRouter(t *testing.T, trusted bool, calls *atomic.Int32) (*Router, *http.Cookie) {
	t.Helper()
	t.Setenv(envCutoverMinInterval, "1h")
	cutoverCache.Lock()
	clear(cutoverCache.m)
	cutoverCache.Unlock()
	rt, db := newTestRouter(t)
	rt.publicHost = "203.0.113.7"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.lookupHost = func(_ context.Context, host string) ([]string, error) {
		if host == "far.example.com" {
			return []string{"198.51.100.4"}, nil
		}
		return []string{"203.0.113.7"}, nil
	}
	rt.publicLookup = func(context.Context, string) []resolverResult { return nil }
	rt.domainAuto.tlsProbe = func(_ context.Context, dial, sni string) (tlsProbeResult, error) {
		calls.Add(1)
		if dial != "127.0.0.1:443" || sni != "far.example.com" {
			t.Errorf("probe dialed %q sni %q, want this server's own ingress", dial, sni)
		}
		return tlsProbeResult{Issuer: "R3", Trusted: trusted}, nil
	}
	rt.domainAuto.httpProbe = func(_ context.Context, _, dial string) (int, error) {
		if dial != "127.0.0.1:443" {
			t.Errorf("http probe dialed %q", dial)
		}
		return 200, nil
	}
	rt.domainAuto.authoritative = func(context.Context, string, []string) (bool, int, int) { return true, 0, 2 }
	seedRunningApp(t, db, "far.example.com")
	return rt, loginTestSession(t, rt, db)
}

func TestDomainCutoverReadyWithoutDNS(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, true, &calls)
	var res domaincutover.Result
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/cutover", &res); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if res.Risk != domaincutover.RiskNone || !res.ReadyToSwitch {
		t.Fatalf("risk = %s ready = %v: %s", res.Risk, res.ReadyToSwitch, res.Summary)
	}
	if res.DNS.Points != domaincutover.PointsElsewhere || len(res.DNS.Records) == 0 || res.DNS.Records[0].Value != "203.0.113.7" {
		t.Fatalf("dns = %+v", res.DNS)
	}
	before := calls.Load()
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/cutover", &res)
	if calls.Load() != before {
		t.Errorf("second call probed again, the rate limit cache did not hold")
	}
}

func TestDomainCutoverUntrustedCertIsRisky(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	var res domaincutover.Result
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/cutover", &res)
	if res.Risk != domaincutover.RiskLikely || !res.ReadyToSwitch {
		t.Fatalf("risk = %s ready = %v", res.Risk, res.ReadyToSwitch)
	}
	if code := trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/nobody.example.com/cutover", nil); code != http.StatusNotFound {
		t.Errorf("unknown domain status = %d, want 404", code)
	}
}

func TestCutoverCertificateNeedsProvider(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	var out cutoverCertBlocked
	if code := trafficCall(t, rt, cookie, http.MethodPost, "/api/v1/domains/far.example.com/cutover/certificate", &out); code != http.StatusConflict && code != http.StatusNotImplemented {
		t.Fatalf("status = %d", code)
	}
	if out.Code != "" && out.Code != "no_dns_provider" {
		t.Errorf("code = %q", out.Code)
	}
}

func TestDomainCutoverReadsTTLFromProvider(t *testing.T) {
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, true, &calls)
	mgr := &fakeDNSManager{recs: []libdns.Record{
		libdns.RR{Name: "far", Type: "A", Data: "198.51.100.4", TTL: 3600 * time.Second},
		libdns.RR{Name: "other", Type: "A", Data: "198.51.100.9", TTL: 5 * time.Second},
	}}
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: mgr, Provider: "cloudflare", Zone: "example.com."}, nil
	}
	var res domaincutover.Result
	trafficCall(t, rt, cookie, http.MethodGet, "/api/v1/domains/far.example.com/cutover", &res)
	if res.DNS.TTLSeconds != 3600 {
		t.Fatalf("ttl = %d, want 3600", res.DNS.TTLSeconds)
	}
	if res.Steps[2].State != domaincutover.StateWait {
		t.Errorf("current dns step = %s, want wait (advise lowering the ttl)", res.Steps[2].State)
	}
}
