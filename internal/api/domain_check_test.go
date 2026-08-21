package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithLookupHost builds a Router with lookupHost overridden
// directly on the unexported field and publicHost pinned to a fixed
// value, the same "no public Option, nothing outside this package needs
// to fake DNS I/O" pattern newTestRouterWithListBranches already
// establishes for listBranches.
func newTestRouterWithLookupHost(t *testing.T, publicHost string, fn lookupHostFunc) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.publicHost = publicHost
	rt.lookupHost = fn
	return rt, db
}

func seedAppWithDomains(t *testing.T, db *store.DB, domains ...string) {
	t.Helper()
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "levelrail/web:1", Port: 3000, Domains: domains,
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
}

func TestHandleCheckDomain_Connected(t *testing.T) {
	calls := 0
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, host string) ([]string, error) {
		calls++
		if host != "app.example.com" {
			t.Errorf("lookupHost called with %q, want %q", host, "app.example.com")
		}
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/check", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != domainCheckStatusConnected {
		t.Errorf("status = %q, want %q", got.Status, domainCheckStatusConnected)
	}
	if !got.Resolved {
		t.Error("resolved = false, want true")
	}
	if got.ExpectedHost != "203.0.113.10" {
		t.Errorf("expected_host = %q, want %q", got.ExpectedHost, "203.0.113.10")
	}
	if got.HostInferred {
		t.Error("host_inferred = true, want false: publicHost was explicitly configured")
	}
	if calls != 1 {
		t.Errorf("lookupHost called %d times, want 1", calls)
	}
}

func TestHandleCheckDomain_NotResolving(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		return nil, errors.New("no such host")
	})
	seedAppWithDomains(t, db, "unresolved.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/unresolved.example.com/check", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != domainCheckStatusNotResolving {
		t.Errorf("status = %q, want %q", got.Status, domainCheckStatusNotResolving)
	}
	if got.Resolved {
		t.Error("resolved = true, want false")
	}
	if len(got.ResolvedHosts) != 0 {
		t.Errorf("resolved_hosts = %v, want empty", got.ResolvedHosts)
	}
}

func TestHandleCheckDomain_ResolvesElsewhere(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		return []string{"198.51.100.99"}, nil
	})
	seedAppWithDomains(t, db, "elsewhere.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/elsewhere.example.com/check", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != domainCheckStatusResolvesElsewhere {
		t.Errorf("status = %q, want %q", got.Status, domainCheckStatusResolvesElsewhere)
	}
	if !got.Resolved {
		t.Error("resolved = false, want true: the domain does resolve, just not to us")
	}
}

// TestHandleCheckDomain_CachesLookupsWithinTTL proves the rate-limiting
// restraint domainCheckCache exists for: a second request for the same
// domain shortly after the first must not perform a second real DNS
// lookup, so a frontend polling this endpoint (web/src/queries/
// domainCheck.ts) can't turn one open tab into a steady stream of
// outbound DNS queries.
func TestHandleCheckDomain_CachesLookupsWithinTTL(t *testing.T) {
	calls := 0
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		calls++
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "cached.example.com")
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/cached.example.com/check", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, rec.Code, http.StatusOK)
		}
	}
	// One lookup for the domain itself; expectedIPs never calls lookupHost
	// at all here since "203.0.113.10" is already an IP literal (see
	// expectedIPs' own doc comment), so exactly 1 call total across all 3
	// requests proves the cache, not a fluke of expectedIPs skipping it.
	if calls != 1 {
		t.Errorf("lookupHost called %d times across 3 requests within the cache TTL, want 1", calls)
	}
}

// TestHandleCheckDomain_CachesExpectedHostLookupWithinTTL is a
// regression test: TestHandleCheckDomain_CachesLookupsWithinTTL only
// ever exercises an IP-literal APP_PUBLIC_HOST, which expectedIPs
// never calls lookupHost for at all (see that function's own doc
// comment), so it never actually proved the expected-host lookup gets
// cached. When APP_PUBLIC_HOST is a hostname instead (a load balancer
// DNS name, plausible in a fronted deployment), that second lookup
// used to run uncached on every single request.
func TestHandleCheckDomain_CachesExpectedHostLookupWithinTTL(t *testing.T) {
	domainCalls, expectedCalls := 0, 0
	rt, db := newTestRouterWithLookupHost(t, "lb.example.net", func(_ context.Context, host string) ([]string, error) {
		if host == "lb.example.net" {
			expectedCalls++
		} else {
			domainCalls++
		}
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "cached2.example.com")
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/cached2.example.com/check", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, rec.Code, http.StatusOK)
		}
	}
	if domainCalls != 1 {
		t.Errorf("domain lookupHost called %d times across 3 requests within the cache TTL, want 1", domainCalls)
	}
	if expectedCalls != 1 {
		t.Errorf("expected-host lookupHost called %d times across 3 requests within the cache TTL, want 1", expectedCalls)
	}
}

func TestHandleCheckDomain_HostInferredFromRequest(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "", func(_ context.Context, _ string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "inferred.example.com")
	cookie := loginTestSession(t, rt, db)

	req := authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/inferred.example.com/check", "")
	req.Host = "203.0.113.10:8443"
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.HostInferred {
		t.Error("host_inferred = false, want true: no publicHost was configured")
	}
	if got.ExpectedHost != "203.0.113.10" {
		t.Errorf("expected_host = %q, want %q (port stripped)", got.ExpectedHost, "203.0.113.10")
	}
}

func TestHandleCheckDomain_AppNotFound(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		t.Fatal("lookupHost must not be called when the app doesn't exist")
		return nil, nil
	})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/domains/app.example.com/check", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCheckDomain_RequiresAuth(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "app.example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/domains/app.example.com/check", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHostsOverlap(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want bool
	}{
		{"overlap", []string{"1.1.1.1", "2.2.2.2"}, []string{"3.3.3.3", "2.2.2.2"}, true},
		{"no overlap", []string{"1.1.1.1"}, []string{"2.2.2.2"}, false},
		{"empty a", nil, []string{"2.2.2.2"}, false},
		{"empty b", []string{"1.1.1.1"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hostsOverlap(tt.a, tt.b); got != tt.want {
				t.Errorf("hostsOverlap(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSplitByFamily(t *testing.T) {
	tests := []struct {
		name   string
		addrs  []string
		wantV4 []string
		wantV6 []string
	}{
		{"mixed", []string{"203.0.113.10", "2001:db8::1"}, []string{"203.0.113.10"}, []string{"2001:db8::1"}},
		{"v4 only", []string{"203.0.113.10"}, []string{"203.0.113.10"}, nil},
		{"v6 only", []string{"2001:db8::1"}, nil, []string{"2001:db8::1"}},
		{"empty", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotV4, gotV6 := splitByFamily(tt.addrs)
			if !slices.Equal(gotV4, tt.wantV4) || !slices.Equal(gotV6, tt.wantV6) {
				t.Errorf("splitByFamily(%v) = (%v, %v), want (%v, %v)", tt.addrs, gotV4, gotV6, tt.wantV4, tt.wantV6)
			}
		})
	}
}

func TestHandleCheckDomain_ExpectedHostResolvesToBothFamilies(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "app.internal", func(_ context.Context, host string) ([]string, error) {
		if host == "app.internal" {
			return []string{"203.0.113.10", "2001:db8::1"}, nil
		}
		return []string{"203.0.113.10"}, nil
	})
	seedAppWithDomains(t, db, "app.example.com")
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/check", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got domainCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !slices.Equal(got.ExpectedIPv4, []string{"203.0.113.10"}) {
		t.Errorf("expected_ipv4 = %v, want [203.0.113.10]", got.ExpectedIPv4)
	}
	if !slices.Equal(got.ExpectedIPv6, []string{"2001:db8::1"}) {
		t.Errorf("expected_ipv6 = %v, want [2001:db8::1]", got.ExpectedIPv6)
	}
}
