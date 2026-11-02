package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func sessPolicyReq(t *testing.T, cookie *http.Cookie, method, path, body, remote string) *http.Request {
	t.Helper()
	req := authedRequest(t, cookie, method, path, body)
	req.RemoteAddr = remote
	return req
}

func servePolicy(rt *Router, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func policyCode(rec *httptest.ResponseRecorder) string {
	var body struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Code
}

func TestSessionPolicyIPAllowlistAndLockoutGuard(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	const inside, outside = "203.0.113.10:5000", "198.51.100.7:5000"

	rec := servePolicy(rt, sessPolicyReq(t, admin, http.MethodPut, "/api/v1/security/session-policies/"+store.DefaultTeamID,
		`{"ip_allowlist":["198.51.100.0/24"]}`, inside))
	if rec.Code != http.StatusConflict {
		t.Fatalf("self-lockout accepted: %d %s", rec.Code, rec.Body)
	}
	rec = servePolicy(rt, sessPolicyReq(t, admin, http.MethodPut, "/api/v1/security/session-policies/"+store.DefaultTeamID,
		`{"ip_allowlist":["203.0.113.0/24"],"idle_timeout":"1m"}`, inside))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("idle below minimum accepted: %d", rec.Code)
	}
	rec = servePolicy(rt, sessPolicyReq(t, admin, http.MethodPut, "/api/v1/security/session-policies/"+store.DefaultTeamID,
		`{"ip_allowlist":["203.0.113.0/24"]}`, inside))
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if rec := servePolicy(rt, sessPolicyReq(t, admin, http.MethodGet, "/api/v1/auth/session", "", inside)); rec.Code != http.StatusOK {
		t.Fatalf("allowed address refused: %d", rec.Code)
	}
	rec = servePolicy(rt, sessPolicyReq(t, admin, http.MethodGet, "/api/v1/auth/session", "", outside))
	if rec.Code != http.StatusForbidden || policyCode(rec) != "ip_not_allowed" {
		t.Fatalf("outside address: %d %s", rec.Code, rec.Body)
	}
	spoof := sessPolicyReq(t, admin, http.MethodGet, "/api/v1/auth/session", "", outside)
	spoof.Header.Set("X-Forwarded-For", "203.0.113.10")
	if rec := servePolicy(rt, spoof); rec.Code != http.StatusForbidden {
		t.Fatalf("untrusted X-Forwarded-For honored: %d", rec.Code)
	}
	proxied := sessPolicyReq(t, admin, http.MethodGet, "/api/v1/auth/session", "", "127.0.0.1:443")
	proxied.Header.Set("X-Forwarded-For", "203.0.113.10")
	if rec := servePolicy(rt, proxied); rec.Code != http.StatusOK {
		t.Fatalf("trusted proxy hop ignored: %d", rec.Code)
	}
	if n, err := db.ClearSessionPolicies(context.Background()); err != nil || n != 1 {
		t.Fatalf("host reset: %d %v", n, err)
	}
	if rec := servePolicy(rt, sessPolicyReq(t, admin, http.MethodGet, "/api/v1/auth/session", "", outside)); rec.Code != http.StatusOK {
		t.Fatalf("after reset: %d", rec.Code)
	}
}

func TestSessionPolicyLifetimeAndIdle(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveSessionPolicy(ctx, store.SessionPolicy{TeamID: store.DefaultTeamID, MaxLifetime: time.Hour, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/session", "")); rec.Code != http.StatusOK {
		t.Fatalf("fresh session refused: %d", rec.Code)
	}
	if _, err := db.ExecContext(ctx, `UPDATE theauth_sessions SET created_at = ?`, time.Now().Add(-2*time.Hour).UnixMicro()); err != nil {
		t.Fatal(err)
	}
	rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/session", ""))
	if rec.Code != http.StatusUnauthorized || policyCode(rec) != "session_max_lifetime" {
		t.Fatalf("old session: %d %s", rec.Code, rec.Body)
	}
	if rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/session", "")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session was not revoked: %d", rec.Code)
	}

	admin = loginTestSession(t, rt, db)
	if err := db.SaveSessionPolicy(ctx, store.SessionPolicy{TeamID: store.DefaultTeamID, IdleTimeout: 10 * time.Minute, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/session", "")); rec.Code != http.StatusOK {
		t.Fatalf("active session refused: %d", rec.Code)
	}
	if _, err := db.ExecContext(ctx, `UPDATE session_activity SET last_seen_at = ?`, time.Now().Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	rec = servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/session", ""))
	if rec.Code != http.StatusUnauthorized || policyCode(rec) != "session_idle_timeout" {
		t.Fatalf("idle session: %d %s", rec.Code, rec.Body)
	}
}

func TestSessionPolicyRequiresMFA(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	rec := servePolicy(rt, authedRequest(t, admin, http.MethodPut, "/api/v1/security/session-policies/"+store.DefaultTeamID, `{"require_mfa":true}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("admin without a factor could require MFA: %d %s", rec.Code, rec.Body)
	}
	if err := db.SaveSessionPolicy(context.Background(), store.SessionPolicy{TeamID: store.DefaultTeamID, RequireMFA: true, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rec = servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/apps", ""))
	if rec.Code != http.StatusForbidden || policyCode(rec) != "mfa_enrollment_required" {
		t.Fatalf("no factor: %d %s", rec.Code, rec.Body)
	}
	if rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/2fa", "")); rec.Code == http.StatusForbidden {
		t.Fatal("enrollment route blocked")
	}
	if rec := servePolicy(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/security/session-policy/effective", "")); rec.Code != http.StatusOK {
		t.Fatalf("effective policy: %d", rec.Code)
	}
}

func TestSessionPolicyConcurrentSessionCap(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("evict_oldest", func(t *testing.T) {
		rt, db := newTestRouter(t)
		first := loginTestSession(t, rt, db)
		if err := db.SaveSessionPolicy(ctx, store.SessionPolicy{TeamID: store.DefaultTeamID, MaxSessions: 1, UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		second := loginTestSession(t, rt, db)
		if rec := servePolicy(rt, authedRequest(t, second, http.MethodGet, "/api/v1/auth/session", "")); rec.Code != http.StatusOK {
			t.Fatalf("newest session refused: %d %s", rec.Code, rec.Body)
		}
		rec := servePolicy(rt, authedRequest(t, first, http.MethodGet, "/api/v1/auth/session", ""))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("oldest session survived the cap: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("reject", func(t *testing.T) {
		rt, db := newTestRouter(t)
		first := loginTestSession(t, rt, db)
		if err := db.SaveSessionPolicy(ctx, store.SessionPolicy{TeamID: store.DefaultTeamID, MaxSessions: 1, OnLimit: store.SessionOnLimitReject, UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		second := loginTestSession(t, rt, db)
		rec := servePolicy(rt, authedRequest(t, second, http.MethodGet, "/api/v1/auth/session", ""))
		if rec.Code != http.StatusConflict || policyCode(rec) != "session_limit" {
			t.Fatalf("new session over the cap: %d %s", rec.Code, rec.Body)
		}
		if rec := servePolicy(rt, authedRequest(t, first, http.MethodGet, "/api/v1/auth/session", "")); rec.Code != http.StatusOK {
			t.Fatalf("existing session lost under reject: %d %s", rec.Code, rec.Body)
		}
	})
}

func TestSessionPolicyCapValidation(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	path := "/api/v1/security/session-policies/" + store.DefaultTeamID
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"negative", `{"max_concurrent_sessions":-1}`, http.StatusBadRequest},
		{"too large", `{"max_concurrent_sessions":5000}`, http.StatusBadRequest},
		{"bad mode", `{"max_concurrent_sessions":2,"on_limit":"drop"}`, http.StatusBadRequest},
		{"reject", `{"max_concurrent_sessions":2,"on_limit":"reject"}`, http.StatusOK},
		{"default mode", `{"max_concurrent_sessions":3}`, http.StatusOK},
	} {
		if rec := servePolicy(rt, authedRequest(t, admin, http.MethodPut, path, tc.body)); rec.Code != tc.want {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body)
		}
	}
	p, err := db.GetSessionPolicy(context.Background(), store.DefaultTeamID)
	if err != nil || p.MaxSessions != 3 || p.OnLimit != store.SessionOnLimitEvictOldest {
		t.Fatalf("stored policy = %+v, %v", p, err)
	}
}
