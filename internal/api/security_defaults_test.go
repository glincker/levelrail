package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func postJSON(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestCreateToken_DefaultsToNinetyDays(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := postJSON(t, rt, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"ci","abilities":["read"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.ExpiresAt == nil {
		t.Fatal("a token created without an expiry must get the default lifetime")
	}
	if got := time.Until(*res.ExpiresAt); got < 89*24*time.Hour || got > 91*24*time.Hour {
		t.Fatalf("default lifetime = %s, want about 90 days", got)
	}
}

func TestCreateToken_ExplicitExpiryStillHonoured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := postJSON(t, rt, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"ci","abilities":["read"],"expires_in_days":7}`)
	var res createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || res.ExpiresAt == nil {
		t.Fatalf("decode: %v body = %s", err, rec.Body.String())
	}
	if got := time.Until(*res.ExpiresAt); got > 8*24*time.Hour {
		t.Fatalf("expiry = %s, want about 7 days", got)
	}
}

func TestCreateToken_NoExpiryRequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	rootCookie := loginTestSession(t, rt, db)
	rec := postJSON(t, rt, rootCookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"forever","abilities":["read"],"no_expiry":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("root no_expiry status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.ExpiresAt != nil {
		t.Fatalf("no_expiry token has expiry %v", res.ExpiresAt)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "reader-noexp@example.com", []string{AbilityRead})
	readCookie := sessionCookieForTest(t, rt, reader.ID)
	rec = postJSON(t, rt, readCookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"forever","abilities":["read"],"no_expiry":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-root no_expiry status = %d, want 403, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreateToken_NoExpiryConflictsWithDays(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := postJSON(t, rt, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"x","abilities":["read"],"no_expiry":true,"expires_in_days":5}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChangePassword_WrongPasswordsLockOut(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	body := `{"current_password":"definitely-wrong","new_password":"a-long-enough-new-password"}`
	locked := false
	for i := 0; i < 40 && !locked; i++ {
		rec := postJSON(t, rt, cookie, http.MethodPut, "/api/v1/auth/password", body)
		switch rec.Code {
		case http.StatusUnauthorized:
		case http.StatusTooManyRequests:
			locked = true
			if !strings.Contains(rec.Body.String(), "step_up_locked") {
				t.Fatalf("lockout body = %s", rec.Body.String())
			}
		default:
			t.Fatalf("attempt %d status = %d, body = %s", i, rec.Code, rec.Body.String())
		}
	}
	if !locked {
		t.Fatal("repeated wrong current passwords never locked the change-password route")
	}
	good := `{"current_password":"` + testAdminPassword + `","new_password":"a-long-enough-new-password"}`
	if rec := postJSON(t, rt, cookie, http.MethodPut, "/api/v1/auth/password", good); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the correct password must not bypass an active lockout, status = %d", rec.Code)
	}
	actions := map[string]bool{}
	for _, e := range listAuditLog(t, rt, cookie) {
		actions[e.Action] = true
	}
	if !actions[auditActionStepUpFailed] || !actions[auditActionStepUpLocked] {
		t.Fatalf("failed and locked attempts must be audited, got %v", actions)
	}
}

func TestChangePassword_CorrectPasswordStillWorks(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	good := `{"current_password":"` + testAdminPassword + `","new_password":"a-long-enough-new-password"}`
	if rec := postJSON(t, rt, cookie, http.MethodPut, "/api/v1/auth/password", good); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityHeaders_PermissionsAndOpenerPolicy(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if got := rec.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
		t.Errorf("COOP = %q", got)
	}
	pp := rec.Header().Get("Permissions-Policy")
	for _, feature := range []string{"camera=()", "microphone=()", "geolocation=()"} {
		if !strings.Contains(pp, feature) {
			t.Errorf("Permissions-Policy %q lacks %s", pp, feature)
		}
	}
}

func TestDeviceStartRate_NegativeEnvKeepsLimiter(t *testing.T) {
	t.Setenv(envDeviceStartRate, "-5")
	if got := deviceStartRatePerMinute(); got != defaultDeviceStartRate {
		t.Fatalf("negative env gave %d, want the default %d", got, defaultDeviceStartRate)
	}
	t.Setenv(envDeviceStartRate, "0")
	if got := deviceStartRatePerMinute(); got != 0 {
		t.Fatalf("explicit 0 gave %d, want 0 (documented opt-out)", got)
	}
}

func TestDeviceTokenTTL_HasCeiling(t *testing.T) {
	t.Setenv(envDeviceTokenTTLDays, "3650")
	if got := deviceTokenTTL(); got != time.Duration(maxDeviceTokenDays)*24*time.Hour {
		t.Fatalf("ttl = %s, want the %d day ceiling", got, maxDeviceTokenDays)
	}
	t.Setenv(envDeviceTokenTTLDays, "10")
	if got := deviceTokenTTL(); got != 10*24*time.Hour {
		t.Fatalf("ttl = %s, want 10 days", got)
	}
}

func TestTrustedDeviceTTL_DefaultAndCeiling(t *testing.T) {
	if defaultTrustedDeviceTTL != 30*24*time.Hour {
		t.Fatalf("default trusted device ttl = %s, want 30 days", defaultTrustedDeviceTTL)
	}
	t.Setenv(envTrustedDeviceTTL, "99999h")
	if got := newCodeLoginState().trustTTL; got != maxTrustedDeviceTTL {
		t.Fatalf("ttl = %s, want ceiling %s", got, maxTrustedDeviceTTL)
	}
}

func TestSensitiveRead_IsAudited(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	postJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/env/bound", "")
	postJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps", "")
	var sensitive, plain int
	for _, e := range listAuditLog(t, rt, cookie) {
		if e.Action == AuditActionSensitiveRead && e.Path == "/api/v1/apps/ghost/env/bound" {
			sensitive++
		}
		if e.Method == http.MethodGet && e.Path == "/api/v1/apps" {
			plain++
		}
	}
	if sensitive != 1 {
		t.Fatalf("sensitive env read rows = %d, want 1", sensitive)
	}
	if plain != 0 {
		t.Fatalf("plain list reads must stay unaudited, got %d rows", plain)
	}
}

func TestSensitiveReadRoute_Matrix(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/v1/apps/web/logs", true},
		{"GET", "/api/v1/apps/web/logs/stream", true},
		{"GET", "/api/v1/apps/web/logs/download", true},
		{"GET", "/api/v1/apps/web/deploys/d1/logs", true},
		{"GET", "/api/v1/apps/web/deploys/d1/logs/download", true},
		{"GET", "/api/v1/databases/db/logs", true},
		{"GET", "/api/v1/databases/db/logs/stream", true},
		{"GET", "/api/v1/loki/query_range", true},
		{"POST", "/api/v1/loki/query", true},
		{"GET", "/api/v1/loki/loki/api/v1/query_range", true},
		{"GET", "/api/v1/export", true},
		{"GET", "/api/v1/apps/web/env/export", true},
		{"GET", "/api/v1/dashboards/d1/export", true},
		{"GET", "/api/v1/dns/zones/example.com/records/export", true},
		{"GET", "/api/v1/apps", false},
		{"GET", "/api/v1/apps/web", false},
		{"GET", "/api/v1/loki/labels", false},
		{"POST", "/api/v1/apps/web/logs", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := sensitiveReadRoute(r); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}

func TestSensitiveContentReads_WriteAuditRow(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	paths := []string{
		"/api/v1/apps/ghost/logs",
		"/api/v1/apps/ghost/logs/download",
		"/api/v1/databases/ghost/logs",
		"/api/v1/export",
		"/api/v1/apps/ghost/env/export",
		"/api/v1/dashboards/ghost/export",
		"/api/v1/dns/zones/ghost.test/records/export",
		"/api/v1/loki/query_range",
	}
	for _, p := range paths {
		postJSON(t, rt, cookie, http.MethodGet, p, "")
	}
	postJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps", "")
	counts := map[string]int{}
	for _, e := range listAuditLog(t, rt, cookie) {
		if e.Action == AuditActionSensitiveRead {
			counts[e.Path]++
		}
		if e.Path == "/api/v1/apps" {
			t.Errorf("plain app list was audited")
		}
	}
	for _, p := range paths {
		if counts[p] != 1 {
			t.Errorf("%s audit rows = %d, want 1", p, counts[p])
		}
	}
}

func TestCredentialRoutes_ChargeWriteBudget(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.apiRateLimit = newAPIRateLimit(1000, 2)
	var limited bool
	for i := 0; i < 6; i++ {
		rec := postJSON(t, rt, cookie, http.MethodDelete, "/api/v1/auth/trusted-devices/td_none", "")
		if rec.Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("credential-touching routes must draw on the write budget even though they are gated at read")
	}
}

func TestAuditVerifyRoute_ReportsIntactAndTampered(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	postJSON(t, rt, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"a","abilities":["read"]}`)

	rec := postJSON(t, rt, cookie, http.MethodGet, "/api/v1/audit-log/verify", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rep struct {
		OK      bool `json:"ok"`
		Checked int  `json:"checked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil || !rep.OK || rep.Checked == 0 {
		t.Fatalf("intact report = %s", rec.Body.String())
	}

	if _, err := db.Exec(`UPDATE audit_log SET status_code = 299 WHERE chain_seq = 1`); err != nil {
		t.Fatal(err)
	}
	rec = postJSON(t, rt, cookie, http.MethodGet, "/api/v1/audit-log/verify", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &rep); err != nil || rep.OK {
		t.Fatalf("tampered report = %s", rec.Body.String())
	}
}

func TestAuditVerifyRoute_RequiresRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	loginTestSession(t, rt, db)
	reader := storeUserWithAbilitiesForTest(t, db, "verify-reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	if rec := postJSON(t, rt, cookie, http.MethodGet, "/api/v1/audit-log/verify", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
