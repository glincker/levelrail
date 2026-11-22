package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func dnsCheckCall(t *testing.T, rt *Router, cookie *http.Cookie, method, domain string) (int, domainCheckResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, "/api/v1/domains/"+domain+"/dns-check", ""))
	var got domainCheckResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rec.Code, got
}

func TestDomainDNSCheck_ByDomainStatuses(t *testing.T) {
	cases := []struct {
		name  string
		hosts []string
		want  string
	}{
		{"connected", []string{"203.0.113.10"}, domainCheckStatusConnected},
		{"elsewhere", []string{"198.51.100.99"}, domainCheckStatusResolvesElsewhere},
		{"not resolving", nil, domainCheckStatusNotResolving},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
				if tc.hosts == nil {
					return nil, context.DeadlineExceeded
				}
				return tc.hosts, nil
			})
			rt.detectPublicIPs = func(context.Context) []string { return nil }
			rt.publicLookup = func(context.Context, string) []resolverResult { return nil }
			seedAppWithDomains(t, db, "pre.example.com")
			cookie := loginTestSession(t, rt, db)
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				code, got := dnsCheckCall(t, rt, cookie, method, "pre.example.com")
				if code != http.StatusOK {
					t.Fatalf("%s status = %d", method, code)
				}
				if got.Status != tc.want {
					t.Errorf("%s status = %q, want %q", method, got.Status, tc.want)
				}
			}
		})
	}
}

func TestDomainDNSCheck_UnknownDomain(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(context.Context, string) ([]string, error) { return nil, nil })
	cookie := loginTestSession(t, rt, db)
	if code, _ := dnsCheckCall(t, rt, cookie, http.MethodGet, "nobody.example.com"); code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}
