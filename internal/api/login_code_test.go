package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLoginCodeRequest_NoEnumeration(t *testing.T) {
	h := newCodeHarness(t, false)
	tests := []struct {
		name     string
		username string
	}{
		{"eligible account", codeTestEmail},
		{"unknown account", "nobody@example.com"},
		{"admin account with codes off by default", testAdminUsername},
	}
	var firstBody string
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, binding := h.requestCode(tt.username)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", rec.Code)
			}
			if binding == nil || !binding.HttpOnly || binding.SameSite != http.SameSiteStrictMode {
				t.Fatalf("binding cookie = %+v, want httpOnly SameSite=Strict", binding)
			}
			if i == 0 {
				firstBody = rec.Body.String()
			} else if rec.Body.String() != firstBody {
				t.Fatalf("body = %q, want identical to %q", rec.Body.String(), firstBody)
			}
		})
	}
}

func TestLoginCodeRedeem(t *testing.T) {
	tests := []struct {
		name       string
		run        func(h *codeHarness, code string, binding *http.Cookie) int
		wantStatus int
		wantAudit  string
	}{
		{"right code in the right browser signs in", func(h *codeHarness, code string, b *http.Cookie) int {
			rec := h.redeem(code, b)
			if rec.Code == http.StatusOK && responseCookie(rec, sessionCookieName) == nil {
				t.Error("no session cookie after redeem")
			}
			return rec.Code
		}, http.StatusOK, store.AuditActionLoginCodeRedeem},
		{"lower case and no dash still match", func(h *codeHarness, code string, b *http.Cookie) int {
			return h.redeem(strings.ToLower(strings.ReplaceAll(code, "-", "")), b).Code
		}, http.StatusOK, store.AuditActionLoginCodeRedeem},
		{"wrong browser cookie", func(h *codeHarness, code string, _ *http.Cookie) int {
			_, other := h.requestCode("nobody@example.com")
			return h.redeem(code, other).Code
		}, http.StatusUnauthorized, ""},
		{"no browser cookie", func(h *codeHarness, code string, _ *http.Cookie) int {
			return h.redeem(code, nil).Code
		}, http.StatusUnauthorized, ""},
		{"replay after a successful redeem", func(h *codeHarness, code string, b *http.Cookie) int {
			if first := h.redeem(code, b); first.Code != http.StatusOK {
				t.Fatalf("first redeem = %d", first.Code)
			}
			return h.redeem(code, b).Code
		}, http.StatusUnauthorized, ""},
		{"expired code", func(h *codeHarness, code string, b *http.Cookie) int {
			past := store.FormatAuditTime(time.Now().Add(-time.Minute))
			if _, err := h.db.ExecContext(context.Background(), `UPDATE login_code_challenges SET expires_at = ?`, past); err != nil {
				t.Fatal(err)
			}
			if err := h.rt.SweepSignInExpiry(context.Background(), time.Now()); err != nil {
				t.Fatal(err)
			}
			return h.redeem(code, b).Code
		}, http.StatusUnauthorized, store.AuditActionLoginCodeExpire},
		{"five wrong attempts lock the challenge", func(h *codeHarness, code string, b *http.Cookie) int {
			for range defaultLoginCodeAttempts {
				if rec := h.redeem("ZZZZ-ZZZZ", b); rec.Code != http.StatusUnauthorized {
					t.Fatalf("wrong code status = %d", rec.Code)
				}
			}
			return h.redeem(code, b).Code
		}, http.StatusUnauthorized, store.AuditActionLoginCodeLockout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, false)
			_, binding := h.requestCode(codeTestEmail)
			code := h.revealFirst(h.userSession())
			if got := tt.run(h, code, binding); got != tt.wantStatus {
				t.Fatalf("status = %d, want %d", got, tt.wantStatus)
			}
			if tt.wantAudit != "" && !containsAction(h.auditActions(), tt.wantAudit) {
				t.Fatalf("audit actions %v lack %s", h.auditActions(), tt.wantAudit)
			}
		})
	}
}

func TestLoginCodeRequest_RateLimits(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		username  func(i int) string
		remote    func(i int) string
		limitedAt int
	}{
		{"per IP", nil, func(i int) string { return "user" + strconv.Itoa(i) + "@example.com" },
			func(int) string { return codeTestIP }, defaultLoginCodeIPRate},
		{"per account", nil, func(int) string { return codeTestEmail },
			func(i int) string { return "198.51.100." + strconv.Itoa(i+1) + ":1" }, defaultLoginCodeAcctRate},
		{"global", map[string]string{envLoginCodeGlobalRate: "2"}, func(i int) string { return "g" + strconv.Itoa(i) + "@example.com" },
			func(i int) string { return "203.0.113." + strconv.Itoa(i+1) + ":1" }, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			h := newCodeHarness(t, false)
			for i := 0; i <= tt.limitedAt; i++ {
				rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+tt.username(i)+`"}`, tt.remote(i))
				want := http.StatusAccepted
				if i == tt.limitedAt {
					want = http.StatusTooManyRequests
				}
				if rec.Code != want {
					t.Fatalf("request %d status = %d, want %d", i, rec.Code, want)
				}
			}
		})
	}
}

func TestLoginCodeRedeem_TOTPStillRequired(t *testing.T) {
	h := newMFAHarness(t)
	h.enrollTOTP()
	ctx := context.Background()
	if err := h.db.SaveCodeLoginSettings(ctx, store.CodeLoginSettings{Admins: true, Others: true, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ch := &codeHarness{t: t, rt: h.rt, db: h.db}
	_, binding := ch.requestCode(testAdminUsername)
	code := ch.revealFirst(h.cookie)
	rec := ch.redeem(code, binding)
	if rec.Code != http.StatusOK || responseCookie(rec, sessionCookieName) != nil {
		t.Fatalf("redeem = %d %s, want 200 with no session yet", rec.Code, rec.Body.String())
	}
	token := h.mfaToken(rec)
	if v := h.verifyLogin(token, h.code(0)); v.Code != http.StatusOK || responseCookie(v, sessionCookieName) == nil {
		t.Fatalf("verify = %d %s", v.Code, v.Body.String())
	}
}

func TestLoginCode_NeverLoggedOrListed(t *testing.T) {
	h := newCodeHarness(t, false)
	_, binding := h.requestCode(codeTestEmail)
	var mailed string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if mailed = h.mail.all(); mailed != "" {
			break
		}
	}
	code := h.revealFirst(h.userSession())
	if !strings.Contains(mailed, code) {
		t.Fatalf("email %q does not carry the code", mailed)
	}
	admin := loginTestSession(t, h.rt, h.db)
	surfaces := []struct {
		name   string
		path   string
		cookie *http.Cookie
	}{
		{"sign-in requests list", "/api/v1/auth/sign-in-requests", h.userSession()},
		{"attention feed", "/api/v1/attention/feed", h.userSession()},
		{"audit log", "/api/v1/audit-log", admin},
		{"device pending summary", "/api/v1/auth/device/pending-summary", admin},
	}
	bare := strings.ReplaceAll(code, "-", "")
	for _, s := range surfaces {
		t.Run(s.name, func(t *testing.T) {
			rec := h.send(http.MethodGet, s.path, "", "", s.cookie)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), code) || strings.Contains(rec.Body.String(), bare) {
				t.Fatalf("%s leaks the code: %s", s.path, rec.Body.String())
			}
		})
	}
	if rec := h.redeem("ZZZZ-ZZZZ", binding); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong code = %d", rec.Code)
	}
	if logs := h.logs.String(); strings.Contains(logs, code) || strings.Contains(logs, bare) {
		t.Fatalf("logs leak the code: %s", logs)
	}
	var feed attentionFeedResponse
	rec := h.send(http.MethodGet, "/api/v1/attention/feed", "", "", h.userSession())
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range feed.Items {
		found = found || it.Kind == "login_code"
	}
	if !found {
		t.Fatalf("attention feed lacks the login_code item: %s", rec.Body.String())
	}
}

func TestSignInRequests_TokenNeedsOwnerAndWriteSensitive(t *testing.T) {
	h := newCodeHarness(t, false)
	tests := []struct {
		name  string
		token *store.APIToken
		want  int
	}{
		{"unowned token", &store.APIToken{ID: "t1", Abilities: []string{AbilityRoot}}, http.StatusForbidden},
		{"read-only owner token", &store.APIToken{ID: "t2", OwnerUserID: h.user.ID, Abilities: []string{AbilityRead}}, http.StatusForbidden},
		{"write:sensitive owner token", &store.APIToken{ID: "t3", OwnerUserID: h.user.ID, Abilities: []string{AbilityWriteSensitive}}, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptestRequestWithToken(http.MethodGet, "/api/v1/auth/sign-in-requests", tt.token)
			rec := newRecorder()
			h.rt.handleListSignInRequests(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
