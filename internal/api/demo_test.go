package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type stubVerifier struct{ err error }

func (s stubVerifier) Verify(context.Context, string, string) error { return s.err }

func demoRouter(t *testing.T, cfg DemoConfig) *Router {
	t.Helper()
	db := openTestDB(t)
	cfg.VisitorUserID = demoTestVisitorID
	if err := db.CreateUser(t.Context(), store.User{ID: demoTestVisitorID, Email: "demo", DisplayName: "Demo", Abilities: []string{AbilityRead}}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return NewRouter(discardLogger(), testBrand(), db, WithDemoMode(cfg))
}

func postDemo(rt *Router, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestDemoInfoOffOnRealInstance(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Errorf("status = %d body = %s, want enabled:false", rec.Code, rec.Body.String())
	}
	for _, target := range []string{"/api/v1/demo/session", "/api/v1/demo/cta"} {
		if got := postDemo(rt, target, `{}`).Code; got == http.StatusOK || got == http.StatusNoContent {
			t.Errorf("POST %s on a real instance status = %d, want it closed", target, got)
		}
	}
	if rt.Handler() == nil || rt.demo != nil {
		t.Error("a Router built without WithDemoMode must have no demo state")
	}
}

func TestDemoSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		verifier   HumanVerifier
		wantStatus int
		wantCookie bool
	}{
		{"no human check", nil, http.StatusOK, true},
		{"human check passes", stubVerifier{}, http.StatusOK, true},
		{"human check fails", stubVerifier{err: errors.New("bad token")}, http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := demoRouter(t, DemoConfig{Verifier: tt.verifier})
			rec := postDemo(rt, "/api/v1/demo/session", `{"humanToken":"t"}`)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", rec.Code, rec.Body.String(), tt.wantStatus)
			}
			var cookie bool
			for _, c := range rec.Result().Cookies() {
				if c.Name == sessionCookieName && c.Value != "" {
					cookie = true
				}
			}
			if cookie != tt.wantCookie {
				t.Errorf("session cookie set = %v, want %v", cookie, tt.wantCookie)
			}
		})
	}
}

func TestDemoSessionRateLimit(t *testing.T) {
	t.Parallel()
	rt := demoRouter(t, DemoConfig{SessionsPerMinute: 2})
	var codes []int
	for i := 0; i < 3; i++ {
		codes = append(codes, postDemo(rt, "/api/v1/demo/session", `{}`).Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes = %v, want [200 200 429]", codes)
	}
}

func TestDemoSessionIsReadOnlyVisitor(t *testing.T) {
	t.Parallel()
	rt := demoRouter(t, DemoConfig{})
	rec := postDemo(rt, "/api/v1/demo/session", `{}`)
	var cookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	for _, target := range []string{"/api/v1/apps", "/api/v1/nodes"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.AddCookie(cookie)
		out := httptest.NewRecorder()
		rt.Handler().ServeHTTP(out, req)
		if out.Code != http.StatusOK {
			t.Errorf("GET %s with demo session = %d, want 200", target, out.Code)
		}
	}
}

func TestDemoCTAAndStats(t *testing.T) {
	t.Parallel()
	rt := demoRouter(t, DemoConfig{})
	tests := []struct {
		body   string
		status int
	}{
		{`{"kind":"install"}`, http.StatusNoContent},
		{`{"kind":"github"}`, http.StatusNoContent},
		{`{"kind":"unknown"}`, http.StatusBadRequest},
		{`not json`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		if got := postDemo(rt, "/api/v1/demo/cta", tt.body).Code; got != tt.status {
			t.Errorf("cta %q = %d, want %d", tt.body, got, tt.status)
		}
	}
	if s := rt.DemoStats(); s.CTAClicks["install"] != 1 || s.CTAClicks["github"] != 1 {
		t.Errorf("stats = %+v", s)
	}
}

func TestDemoInfoEnabled(t *testing.T) {
	t.Parallel()
	reset := time.Date(2026, 10, 15, 4, 0, 0, 0, time.UTC)
	rt := demoRouter(t, DemoConfig{Username: "demo", Password: "demo-readonly", InstallCommand: "curl install | sh", ResetsAt: func() time.Time { return reset }})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/demo", nil))
	body := rec.Body.String()
	for _, want := range []string{`"enabled":true`, `"username":"demo"`, `"installCommand":"curl install | sh"`, `2026-10-15T04:00:00Z`} {
		if !strings.Contains(body, want) {
			t.Errorf("body %s missing %s", body, want)
		}
	}
}
