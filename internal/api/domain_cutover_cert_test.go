package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func cutoverCertRouter(t *testing.T) (*Router, func(method string) (int, string)) {
	t.Helper()
	var calls atomic.Int32
	rt, cookie := cutoverRouter(t, false, &calls)
	rt.SetDNS01ProviderCheck(func(context.Context) bool { return true })
	rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: &fakeDNSManager{}, Provider: "cloudflare", Zone: "example.com."}, nil
	}
	do := func(method string) (int, string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/domains/far.example.com/cutover/certificate", ""))
		return rec.Code, rec.Body.String()
	}
	return rt, do
}

func TestCutoverCertificateIsPerHost(t *testing.T) {
	rt, do := cutoverCertRouter(t)
	if rt.runtimeSettings == nil {
		t.Skip("no runtime settings in this router")
	}
	s, err := rt.ingressSettings.GetIngressSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.ACMEEmail = "me@example.com"
	if err := rt.ingressSettings.UpdateIngressSettings(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	code, body := do(http.MethodPost)
	if code != http.StatusAccepted {
		t.Fatalf("post = %d %s", code, body)
	}
	if on, _, _ := rt.domainACME.DomainDNS01(context.Background(), "far.example.com"); !on {
		t.Fatal("host was not switched to DNS-01")
	}
	if rt.runtimeSettings.Bool(context.Background(), runtimesettings.KeyIngressDNS01AllHosts) {
		t.Fatal("the all-hosts DNS-01 setting must stay off")
	}
	if code, body := do(http.MethodDelete); code != http.StatusOK {
		t.Fatalf("revert = %d %s", code, body)
	}
	if on, _, _ := rt.domainACME.DomainDNS01(context.Background(), "far.example.com"); on {
		t.Fatal("revert did not clear the flag")
	}
}

func TestCutoverCertificateRefusals(t *testing.T) {
	t.Run("no provider", func(t *testing.T) {
		rt, do := cutoverCertRouter(t)
		rt.SetDNS01ProviderCheck(func(context.Context) bool { return false })
		if rt.runtimeSettings == nil {
			t.Skip("no runtime settings")
		}
		if code, body := do(http.MethodPost); code != http.StatusConflict || !strings.Contains(body, "no_dns_provider") {
			t.Fatalf("got %d %s", code, body)
		}
		if on, _, _ := rt.domainACME.DomainDNS01(context.Background(), "far.example.com"); on {
			t.Fatal("refusal must not store the flag")
		}
	})
	t.Run("unmanaged zone", func(t *testing.T) {
		rt, do := cutoverCertRouter(t)
		rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) { return nil, nil }
		if rt.runtimeSettings == nil {
			t.Skip("no runtime settings")
		}
		if code, body := do(http.MethodPost); code != http.StatusConflict || !strings.Contains(body, "zone_not_managed") {
			t.Fatalf("got %d %s", code, body)
		}
	})
	t.Run("proxy owns tls", func(t *testing.T) {
		rt, do := cutoverCertRouter(t)
		if rt.runtimeSettings == nil {
			t.Skip("no runtime settings")
		}
		s, _ := rt.ingressSettings.GetIngressSettings(context.Background())
		s.TLSTerminatedUpstream = true
		if err := rt.ingressSettings.UpdateIngressSettings(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		if code, body := do(http.MethodPost); code != http.StatusConflict || !strings.Contains(body, "upstream_proxy") {
			t.Fatalf("got %d %s", code, body)
		}
	})
}
