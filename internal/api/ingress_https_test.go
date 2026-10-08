package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassifyACMEError(t *testing.T) {
	tests := []struct{ in, want string }{
		{"urn:ietf:params:acme:error:rateLimited: too many failed authorizations", httpsHintRateLimited},
		{"Timeout during connect (likely firewall problem)", httpsHintUnreachable},
		{"DNS problem: NXDOMAIN looking up A for x", httpsHintDNS},
		{"CAA record for x prevents issuance", httpsHintCAA},
		{"something odd", httpsHintUnknown},
	}
	for _, tc := range tests {
		if got := classifyACMEError(tc.in); got != tc.want {
			t.Errorf("classifyACMEError(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestComputeHTTPSStatus(t *testing.T) {
	now := time.Now()
	on := store.IngressSettings{PrimaryDomain: "1-2-3-4.sslip.io", ACMEEnabled: true}
	prodCert := alerting.CertInfo{Domain: "1-2-3-4.sslip.io", Issuer: "R11", NotAfter: now.Add(80 * 24 * time.Hour)}
	stagingCert := alerting.CertInfo{Domain: "1-2-3-4.sslip.io", Issuer: "(STAGING) Artificial Amaranth YE1", NotAfter: now.Add(80 * 24 * time.Hour)}
	stagingOn := on
	stagingOn.ACMEDirectoryURL = ACMEStagingDirectoryURL
	internal := alerting.CertInfo{Domain: "1-2-3-4.sslip.io", Issuer: "Caddy Local Authority - ECC Intermediate"}
	fail := &ingress.ACMEFailure{Error: "Timeout during connect (likely firewall problem)"}

	tests := []struct {
		name    string
		s       store.IngressSettings
		certs   []alerting.CertInfo
		failure *ingress.ACMEFailure
		started time.Time
		want    string
		hint    string
	}{
		{"acme off", store.IngressSettings{PrimaryDomain: "x"}, nil, nil, now, httpsStateOff, ""},
		{"no domain", store.IngressSettings{ACMEEnabled: true}, nil, nil, now, httpsStateOff, ""},
		{"issued", on, []alerting.CertInfo{prodCert}, nil, now, httpsStateIssued, ""},
		{"issued beats stale failure", on, []alerting.CertInfo{prodCert}, fail, now, httpsStateIssued, ""},
		{"stale staging cert is not production success", on, []alerting.CertInfo{stagingCert}, nil, now, httpsStatePending, ""},
		{"staging cert counts when staging", stagingOn, []alerting.CertInfo{stagingCert}, nil, now, httpsStateIssued, ""},
		{"internal cert only is pending", on, []alerting.CertInfo{internal}, nil, now, httpsStatePending, ""},
		{"ca error", on, nil, fail, now, httpsStateFailed, httpsHintUnreachable},
		{"pending too long", on, nil, nil, now.Add(-time.Hour), httpsStateFailed, httpsHintUnreachable},
		{"pending fresh", on, nil, nil, now.Add(-time.Second), httpsStatePending, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeHTTPSStatus(tc.s, tc.certs, tc.failure, tc.started, now, time.Minute)
			if got.State != tc.want || got.Hint != tc.hint {
				t.Fatalf("state/hint = %q/%q, want %q/%q", got.State, got.Hint, tc.want, tc.hint)
			}
		})
	}
}

func TestAllowHTTPSAttempt(t *testing.T) {
	rt := &Router{httpsMaxAttemptsPerHour: 2}
	now := time.Now()
	for i := 0; i < 2; i++ {
		if !rt.allowHTTPSAttempt(now, true) {
			t.Fatalf("production attempt %d must pass", i+1)
		}
	}
	if rt.allowHTTPSAttempt(now, true) {
		t.Fatal("third production attempt within an hour must be refused")
	}
	if !rt.allowHTTPSAttempt(now, false) {
		t.Fatal("staging attempts are never limited")
	}
	if !rt.allowHTTPSAttempt(now.Add(61*time.Minute), true) {
		t.Fatal("limit must reset after an hour")
	}
}

func TestHandleEnableHTTPS(t *testing.T) {
	post := func(rt *Router, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/ingress/https", body))
		return rec
	}

	t.Run("no public ip is a clear conflict", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		rec := post(rt, cookie, `{"email":"a@example.com"}`)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "APP_PUBLIC_HOST") {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("sets primary domain and acme with staging directory", func(t *testing.T) {
		db := openTestDB(t)
		rt := NewRouter(discardLogger(), testBrand(), db, WithPublicHost("203.0.113.5"), WithPublicHostSource("detected"))
		rt.doctorDialContext = func(_ context.Context, _, addr string) (net.Conn, error) {
			if strings.HasSuffix(addr, ":80") {
				c, _ := net.Pipe()
				return c, nil
			}
			return nil, errors.New("closed")
		}
		cookie := loginTestSession(t, rt, db)
		rec := post(rt, cookie, `{"email":"a@example.com","staging":true}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		var got httpsStatusResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Domain != "203-0-113-5.sslip.io" || !got.Staging || got.State != httpsStatePending {
			t.Fatalf("unexpected status %+v", got)
		}
		wantPre := []httpsPreflightPort{{Port: 80, Reachable: true}, {Port: 443}}
		if !slices.Equal(got.Preflight, wantPre) {
			t.Fatalf("preflight = %+v, want %+v", got.Preflight, wantPre)
		}
		s, err := db.GetIngressSettings(t.Context())
		if err != nil || !s.ACMEEnabled || s.ACMEDirectoryURL != ACMEStagingDirectoryURL || s.ACMEEmail != "a@example.com" {
			t.Fatalf("settings = %+v, %v", s, err)
		}
	})

	t.Run("rejects bad email and rate limits production", func(t *testing.T) {
		db := openTestDB(t)
		rt := NewRouter(discardLogger(), testBrand(), db, WithPublicHost("203.0.113.5"), WithHTTPSEnableLimits(1, 0))
		rt.doctorDialContext = fakeDoctorOfflineDialContext
		cookie := loginTestSession(t, rt, db)
		if rec := post(rt, cookie, `{"email":"nope"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad email: %d", rec.Code)
		}
		if rec := post(rt, cookie, `{"email":"a@example.com"}`); rec.Code != http.StatusAccepted {
			t.Fatalf("first: %d %s", rec.Code, rec.Body.String())
		}
		if rec := post(rt, cookie, `{"email":"a@example.com"}`); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("second: %d", rec.Code)
		}
	})
}

func TestIngressSettingsFallbackToggle(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPublicHost("203.0.113.5"), WithPublicHostSource("detected"))
	cookie := loginTestSession(t, rt, db)
	put := func(body string) {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", body))
		if rec.Code != http.StatusOK {
			t.Fatalf("put %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	put(`{"fallback_domains_enabled":false}`)
	if s, _ := db.GetIngressSettings(t.Context()); !s.FallbackDomainsDisabled {
		t.Fatal("toggle off did not persist")
	}
	put(`{"hsts_enabled":true}`)
	if s, _ := db.GetIngressSettings(t.Context()); !s.FallbackDomainsDisabled {
		t.Fatal("a PUT that omits the toggle must keep it")
	}
	put(`{"fallback_domains_enabled":true}`)
	if s, _ := db.GetIngressSettings(t.Context()); s.FallbackDomainsDisabled {
		t.Fatal("toggle on did not persist")
	}
}

func TestWithHSTS(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	h := rt.WithHSTS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	get := func(forwardedHTTPS bool) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		if forwardedHTTPS {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Header().Get("Strict-Transport-Security")
	}
	if get(true) != "" {
		t.Fatal("HSTS must stay off until enabled")
	}
	if err := db.UpdateIngressSettings(t.Context(), store.IngressSettings{HSTSEnabled: true}); err != nil {
		t.Fatal(err)
	}
	rt.hstsDBEnabled.Store(true)
	if !strings.Contains(get(true), "max-age=") {
		t.Fatal("HSTS missing on an https request once enabled")
	}
	if get(false) != "" {
		t.Fatal("HSTS must not be sent over plain http")
	}
}
