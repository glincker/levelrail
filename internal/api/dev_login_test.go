package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const devLoginTestToken = "0123456789abcdef0123456789abcdef-test"

func devLoginRouter(t *testing.T, token, publicHost string) *Router {
	t.Helper()
	t.Setenv("APP_DEV_MODE", "1")
	t.Setenv("APP_DEV_LOGIN_TOKEN", token)
	t.Setenv("APP_PUBLIC_HOST", publicHost)
	rt, db := newTestRouter(t)
	if err := BootstrapAdmin(context.Background(), db, devModeUsername, devModePassword); err != nil {
		t.Fatalf("bootstrap dev admin: %v", err)
	}
	return rt
}

func devLoginDo(rt *Router, target, host, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = host
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDevLogin_Success(t *testing.T) {
	rt := devLoginRouter(t, devLoginTestToken, "localhost")
	rec := devLoginDo(rt, "/api/v1/auth/dev-login?token="+devLoginTestToken+"&next=/apps", "127.0.0.1:5173", "127.0.0.1:50000")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/apps" {
		t.Fatalf("got %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing privacy headers: %v", rec.Header())
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" && c.HttpOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("no session cookie set")
	}
}

func TestDevLogin_Refusals(t *testing.T) {
	good := "/api/v1/auth/dev-login?token=" + devLoginTestToken
	cases := []struct {
		name       string
		token      string
		publicHost string
		target     string
		host       string
		remote     string
		want       int
	}{
		{"wrong token", devLoginTestToken, "localhost", "/api/v1/auth/dev-login?token=nope", "127.0.0.1:1", "127.0.0.1:2", http.StatusNotFound},
		{"missing token", devLoginTestToken, "localhost", "/api/v1/auth/dev-login", "127.0.0.1:1", "127.0.0.1:2", http.StatusNotFound},
		{"short configured token", "short", "localhost", "/api/v1/auth/dev-login?token=short", "127.0.0.1:1", "127.0.0.1:2", http.StatusNotFound},
		{"unset token", "", "localhost", good, "127.0.0.1:1", "127.0.0.1:2", http.StatusNotFound},
		{"public host configured", devLoginTestToken, "example.com", good, "127.0.0.1:1", "127.0.0.1:2", http.StatusNotFound},
		{"non-loopback host header", devLoginTestToken, "localhost", good, "example.com", "127.0.0.1:2", http.StatusNotFound},
		{"non-loopback remote", devLoginTestToken, "localhost", good, "127.0.0.1:1", "203.0.113.9:2", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := devLoginRouter(t, tc.token, tc.publicHost)
			rec := devLoginDo(rt, tc.target, tc.host, tc.remote)
			if rec.Code != tc.want {
				t.Fatalf("got %d want %d", rec.Code, tc.want)
			}
			for _, c := range rec.Result().Cookies() {
				if c.Name == sessionCookieName && c.Value != "" {
					t.Fatal("session cookie set on refusal")
				}
			}
		})
	}
}

func TestDevLogin_OpenRedirectRejected(t *testing.T) {
	rt := devLoginRouter(t, devLoginTestToken, "localhost")
	for _, next := range []string{"https://evil.example", "//evil.example", "/\\evil.example", "evil.example", "javascript:alert(1)"} {
		target := "/api/v1/auth/dev-login?token=" + devLoginTestToken + "&next=" + url.QueryEscape(next)
		rec := devLoginDo(rt, target, "127.0.0.1:1", "127.0.0.1:2")
		if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/") || strings.HasPrefix(loc, "//") || loc != "/" {
			t.Fatalf("next %q gave %d %q", next, rec.Code, loc)
		}
	}
}

func TestDevLogin_ForwardedHeadersRefused(t *testing.T) {
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "Forwarded"} {
		t.Run(h, func(t *testing.T) {
			rt := devLoginRouter(t, devLoginTestToken, "localhost")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/dev-login?token="+devLoginTestToken, nil)
			req.Host = "127.0.0.1:1"
			req.RemoteAddr = "127.0.0.1:2"
			req.Header.Set(h, "203.0.113.9")
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("got %d want 404", rec.Code)
			}
		})
	}
}

func TestDevLogin_RateLimitIgnoresForwardedBuckets(t *testing.T) {
	rt := devLoginRouter(t, devLoginTestToken, "localhost")
	rt.tokenRedeemRateLimit = newAPIRateLimiter(1)
	target := "/api/v1/auth/dev-login?token=wrong"
	if rec := devLoginDo(rt, target, "127.0.0.1:1", "127.0.0.1:2"); rec.Code != http.StatusNotFound {
		t.Fatalf("first got %d", rec.Code)
	}
	// A different remote port and source must land in the same bucket.
	if rec := devLoginDo(rt, target, "127.0.0.1:1", "[::1]:3"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second got %d want 429", rec.Code)
	}
}

func TestDevLogin_DisabledOrMFAUserRefused(t *testing.T) {
	for _, mode := range []string{"disabled", "mfa"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("APP_DEV_MODE", "1")
			t.Setenv("APP_DEV_LOGIN_TOKEN", devLoginTestToken)
			t.Setenv("APP_PUBLIC_HOST", "localhost")
			h := newMFAHarness(t)
			ctx := context.Background()
			if err := h.db.CreateUser(ctx, store.User{
				ID: "user_devlogin", Email: devModeUsername, DisplayName: devModeUsername,
				Abilities: []string{AbilityRoot}, CreatedAt: time.Now(),
			}); err != nil {
				t.Fatalf("create dev user: %v", err)
			}
			user, err := h.db.GetUserByEmail(ctx, devModeUsername)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "disabled" {
				if err := h.rt.authEngine.DisableUser(ctx, user.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := authengine.NewDirectory(h.db.DB).EnsureEngineUser(ctx, user.ID); err != nil {
					t.Fatal(err)
				}
				enroll, err := h.rt.mfaLib.mfa.TOTPBegin(ctx, user.ID, user.Email)
				if err != nil {
					t.Fatal(err)
				}
				h.secret = enroll.Secret
				if _, err := h.rt.mfaLib.mfa.TOTPFinish(ctx, user.ID, h.code(0)); err != nil {
					t.Fatal(err)
				}
			}
			rec := devLoginDo(h.rt, "/api/v1/auth/dev-login?token="+devLoginTestToken, "127.0.0.1:1", "127.0.0.1:2")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("got %d want 404", rec.Code)
			}
		})
	}
}
