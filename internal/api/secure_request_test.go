package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIsHTTPS(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		tls        bool
		proto      string
		want       bool
	}{
		{name: "direct plain http from remote", remoteAddr: "203.0.113.7:5000", want: false},
		{name: "direct tls", remoteAddr: "203.0.113.7:5000", tls: true, want: true},
		{name: "remote peer spoofing forwarded proto", remoteAddr: "203.0.113.7:5000", proto: "https", want: false},
		{name: "loopback v4 proxy with https", remoteAddr: "127.0.0.1:40000", proto: "https", want: true},
		{name: "loopback v6 proxy with https", remoteAddr: "[::1]:40000", proto: "HTTPS", want: true},
		{name: "loopback proxy with http", remoteAddr: "127.0.0.1:40000", proto: "http", want: false},
		{name: "loopback without header", remoteAddr: "127.0.0.1:40000", want: false},
		{name: "loopback with list, first wins", remoteAddr: "127.0.0.1:40000", proto: "https, http", want: true},
		{name: "loopback with list, first is http", remoteAddr: "127.0.0.1:40000", proto: "http, https", want: false},
		{name: "private lan peer is not trusted", remoteAddr: "10.0.0.5:40000", proto: "https", want: false},
		{name: "unparseable remote addr", remoteAddr: "garbage", proto: "https", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			r.RemoteAddr = tt.remoteAddr
			if !tt.tls {
				r.TLS = nil
			} else {
				r.TLS = &tls.ConnectionState{}
			}
			if tt.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			if got := requestIsHTTPS(r); got != tt.want {
				t.Errorf("requestIsHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func sessionCookieFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in response", sessionCookieName)
	return nil
}

func TestSessionCookieSecureFollowsRequest(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		proto      string
		wantSecure bool
	}{
		{name: "plain http", remoteAddr: "203.0.113.7:5000", wantSecure: false},
		{name: "via embedded caddy over https", remoteAddr: "127.0.0.1:40000", proto: "https", wantSecure: true},
		{name: "spoofed header from remote", remoteAddr: "203.0.113.7:5000", proto: "https", wantSecure: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)

			login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
				strings.NewReader(`{"username":"`+testAdminUsername+`","password":"`+testAdminPassword+`"}`))
			login.RemoteAddr = tt.remoteAddr
			if tt.proto != "" {
				login.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, login)
			if rec.Code != http.StatusOK {
				t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
			}
			c := sessionCookieFrom(t, rec)
			if c.Secure != tt.wantSecure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Errorf("login cookie Secure=%v HttpOnly=%v SameSite=%v, want Secure=%v HttpOnly SameSite=Lax", c.Secure, c.HttpOnly, c.SameSite, tt.wantSecure)
			}

			logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
			logout.RemoteAddr = tt.remoteAddr
			if tt.proto != "" {
				logout.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			logout.AddCookie(&http.Cookie{Name: sessionCookieName, Value: c.Value}) //nolint:gosec // request cookie, not a response Set-Cookie
			rec = httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, logout)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("logout status = %d", rec.Code)
			}
			cleared := sessionCookieFrom(t, rec)
			if cleared.Secure != tt.wantSecure || cleared.MaxAge >= 0 || cleared.Value != "" {
				t.Errorf("logout cookie Secure=%v MaxAge=%d Value=%q, want Secure=%v and cleared", cleared.Secure, cleared.MaxAge, cleared.Value, tt.wantSecure)
			}
		})
	}
}

func TestInsecureLoginRefusal(t *testing.T) {
	tests := []struct {
		name         string
		dashboardURL string
		allow        bool
		remoteAddr   string
		proto        string
		wantStatus   int
	}{
		{name: "no dashboard url", remoteAddr: "203.0.113.7:5000", wantStatus: http.StatusOK},
		{name: "http dashboard url", dashboardURL: "http://203.0.113.7:8080", remoteAddr: "203.0.113.7:5000", wantStatus: http.StatusOK},
		{name: "https dashboard url, plain http request", dashboardURL: "https://dash.example.com", remoteAddr: "203.0.113.7:5000", wantStatus: http.StatusForbidden},
		{name: "https dashboard url, spoofed header", dashboardURL: "https://dash.example.com", remoteAddr: "203.0.113.7:5000", proto: "https", wantStatus: http.StatusForbidden},
		{name: "https dashboard url, via caddy", dashboardURL: "https://dash.example.com", remoteAddr: "127.0.0.1:40000", proto: "https", wantStatus: http.StatusOK},
		{name: "https dashboard url, escape hatch", dashboardURL: "https://dash.example.com", allow: true, remoteAddr: "203.0.113.7:5000", wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			bootstrapTestAdmin(t, db)
			if err := db.SetDashboardURL(context.Background(), tt.dashboardURL); err != nil {
				t.Fatal(err)
			}
			logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
			rt := NewRouter(logger, testBrand(), db, WithAllowInsecureLogin(tt.allow))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
				strings.NewReader(`{"username":"`+testAdminUsername+`","password":"`+testAdminPassword+`"}`))
			req.RemoteAddr = tt.remoteAddr
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusForbidden {
				return
			}
			var body insecureLoginResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.SecureURL != tt.dashboardURL || !strings.Contains(body.Error, tt.dashboardURL) {
				t.Errorf("refusal body = %+v, want it to point at %s", body, tt.dashboardURL)
			}
			for _, c := range rec.Result().Cookies() {
				if c.Name == sessionCookieName {
					t.Error("a refused login must not set a session cookie")
				}
			}
		})
	}
}

func TestInsecureLoginRefusal_CoversTwoFactorAndInvites(t *testing.T) {
	db := openTestDB(t)
	if err := db.SetDashboardURL(context.Background(), "https://dash.example.com"); err != nil {
		t.Fatal(err)
	}
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	for _, path := range []string{"/api/v1/auth/2fa/verify", "/api/v1/invites/accept"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", path, rec.Code)
		}
	}
}
