package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestApprovalScopeMatrix(t *testing.T) {
	methods := []string{signInMethodPassword, signInMethodPasskey, signInMethodOAuth}
	tests := []struct {
		name         string
		gate         bool
		scope        string
		accountOptIn bool
		otherSession bool
		wantHeld     map[string]bool
	}{
		{"default scope gates password only", true, approvalScopePasswordOnly, false, true,
			map[string]bool{signInMethodPassword: true}},
		{"all methods gates every method", true, approvalScopeAllMethods, false, true,
			map[string]bool{signInMethodPassword: true, signInMethodPasskey: true, signInMethodOAuth: true}},
		{"account opt-in gates every method", true, approvalScopePasswordOnly, true, true,
			map[string]bool{signInMethodPassword: true, signInMethodPasskey: true, signInMethodOAuth: true}},
		{"no other session never gates", true, approvalScopeAllMethods, true, false, map[string]bool{}},
		{"break glass switch off never gates", false, approvalScopeAllMethods, true, true, map[string]bool{}},
	}
	for _, tt := range tests {
		for _, method := range methods {
			t.Run(tt.name+"/"+method, func(t *testing.T) {
				h := newCodeHarness(t, tt.gate)
				ctx := context.Background()
				scope := tt.scope
				if err := h.db.SaveSecurityPolicy(ctx, store.SecurityPolicy{ApprovalScope: &scope, UpdatedAt: time.Now()}); err != nil {
					t.Fatal(err)
				}
				if err := h.db.SetRequireNewDeviceApproval(ctx, h.user.ID, tt.accountOptIn, time.Now()); err != nil {
					t.Fatal(err)
				}
				if tt.otherSession {
					h.userSession()
				}
				req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
				req.RemoteAddr = codeTestIP
				hold, err := h.rt.holdForApproval(httptest.NewRecorder(), req, h.user, "", method)
				if err != nil {
					t.Fatal(err)
				}
				if got := hold != nil; got != tt.wantHeld[method] {
					t.Fatalf("held = %v, want %v", got, tt.wantHeld[method])
				}
			})
		}
	}
}

func TestApprovalScope_UnreadablePolicyFailsClosed(t *testing.T) {
	h := newCodeHarness(t, true)
	if _, err := h.db.ExecContext(context.Background(), `DROP TABLE security_policy_settings`); err != nil {
		t.Fatal(err)
	}
	if !h.rt.approvalRequiredFor(context.Background(), h.user.ID, signInMethodPasskey) {
		t.Fatal("an unreadable policy must gate the sign-in")
	}
}

func waitForMail(t *testing.T, h *codeHarness, marker string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if body := h.mail.all(); strings.Contains(body, marker) {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no mail containing %q", marker)
	return ""
}

func alertTokenFromMail(t *testing.T, body string) string {
	t.Helper()
	_, after, ok := strings.Cut(body, signInAlertPagePath+"?token=")
	if !ok {
		t.Fatalf("no alert link in %q", body)
	}
	return strings.Fields(after)[0]
}

func TestSignInAlert_NewBrowserAndDisownLink(t *testing.T) {
	h := newCodeHarness(t, false)
	if rec := h.passwordLogin(); rec.Code != http.StatusOK {
		t.Fatalf("first login = %d", rec.Code)
	}
	if strings.Contains(h.mail.all(), "New sign-in") {
		t.Fatal("an account's first browser must not raise an alert")
	}
	rec := h.send(http.MethodPost, "/api/v1/auth/login", `{"username":"`+codeTestEmail+`","password":"`+codeTestPassword+`"}`, "198.51.100.7:5000")
	if rec.Code != http.StatusOK {
		t.Fatalf("second login = %d %s", rec.Code, rec.Body.String())
	}
	token := alertTokenFromMail(t, waitForMail(t, h, "New sign-in"))
	session := responseCookie(rec, sessionCookieName)

	id, _, _ := strings.Cut(token, ".")
	expiredID := "sa_expired"
	if err := h.db.CreateSignInAlertToken(context.Background(), store.SignInAlertToken{ID: expiredID, UserID: h.user.ID,
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	disown := func(tok string) *httptest.ResponseRecorder {
		return h.send(http.MethodPost, "/api/v1/auth/sign-in-alert/disown", `{"token":"`+tok+`"}`, "203.0.113.50:1")
	}
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"tampered signature", id + ".AAAA", http.StatusBadRequest},
		{"id without signature", id, http.StatusBadRequest},
		{"another id with this signature", "sa_other." + strings.SplitN(token, ".", 2)[1], http.StatusBadRequest},
		{"expired", h.rt.signAlertID(expiredID), http.StatusBadRequest},
		{"valid link", token, http.StatusOK},
		{"replay", token, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := disown(tt.token); rec.Code != tt.want {
				t.Fatalf("got %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
	if _, ok := h.rt.libSessions.Lookup(context.Background(), session.Value); ok {
		t.Fatal("the disowned session still validates")
	}
	s, err := h.db.GetUserSecuritySettings(context.Background(), h.user.ID)
	if err != nil || s.ResetFlaggedAt.IsZero() {
		t.Fatalf("account not flagged: %+v %v", s, err)
	}
	if !containsAction(h.auditActions(), store.AuditActionSignInDisowned) {
		t.Fatal("disown not audited")
	}
}

func TestSecurityPolicy_PutValidatesAndEnforcesTokenLifetime(t *testing.T) {
	h := newCodeHarness(t, false)
	admin := adminSessionForTest(t, h)
	tests := []struct {
		name, body string
		want       int
	}{
		{"bad scope", `{"approval_scope":"everything"}`, http.StatusBadRequest},
		{"negative days", `{"max_token_lifetime_days":-1}`, http.StatusBadRequest},
		{"valid", `{"approval_scope":"all_methods","max_token_lifetime_days":30}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/v1/security/policy", strings.NewReader(tt.body))
			req.AddCookie(admin)
			rec := httptest.NewRecorder()
			h.rt.Handler().ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d %s", rec.Code, rec.Body.String())
			}
		})
	}
	nonAdmin := httptest.NewRequest(http.MethodPut, "/api/v1/security/policy", strings.NewReader(`{"approval_scope":"password_only"}`))
	nonAdmin.AddCookie(h.userSession())
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, nonAdmin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin policy change = %d, want 403", rec.Code)
	}
	var got securityPolicyResource
	rec = h.send(http.MethodGet, "/api/v1/security/policy", "", "", admin)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ApprovalScope != approvalScopeAllMethods || got.Sources["approval_scope"] != policySourceSaved {
		t.Fatalf("policy = %+v %v", got, err)
	}
	lifetime := []struct {
		days int
		ok   bool
	}{{0, false}, {31, false}, {30, true}, {1, true}}
	for _, l := range lifetime {
		msg, err := h.rt.tokenLifetimeProblem(context.Background(), l.days)
		if err != nil || (msg == "") != l.ok {
			t.Fatalf("days %d: msg %q err %v, want ok=%v", l.days, msg, err, l.ok)
		}
	}
}
