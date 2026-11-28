package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

func (h *codeHarness) setApprovalMode(mode string) {
	h.t.Helper()
	if err := h.db.SaveSecurityPolicy(context.Background(), store.SecurityPolicy{ApprovalWithoutSession: &mode, UpdatedAt: time.Now()}); err != nil {
		h.t.Fatal(err)
	}
}

func TestApprovalWithoutSession_Gate(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		accountHold bool
		wantHold    bool
	}{
		{"allow lets a sign-in with no other session through", approvalWithoutSessionAllow, false, false},
		{"hold holds a sign-in with no other session", approvalWithoutSessionHold, false, true},
		{"the account switch overrides platform allow", approvalWithoutSessionAllow, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			h.setApprovalMode(tt.mode)
			if tt.accountHold {
				if err := h.db.SetRequireNewDeviceApproval(context.Background(), h.user.ID, true, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			rec := h.passwordLogin()
			if tt.wantHold {
				h.pendingApproval(rec)
				if !containsAction(h.auditActions(), store.AuditActionNewDeviceRequest) {
					t.Fatalf("audit actions %v lack the request", h.auditActions())
				}
				return
			}
			if responseCookie(rec, sessionCookieName) == nil {
				t.Fatalf("allow must sign in: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestApprovalWithoutSession_TrustedBrowserStillSkips(t *testing.T) {
	h := newCodeHarness(t, true)
	existing := responseCookie(h.passwordLogin(), sessionCookieName)
	id, binding := h.pendingApproval(h.passwordLogin())
	if rec := h.approve(id, existing); rec.Code != http.StatusNoContent {
		t.Fatalf("approve = %d", rec.Code)
	}
	_, rec := h.poll(binding)
	trusted := responseCookie(rec, trustedCookie)
	h.setApprovalMode(approvalWithoutSessionHold)
	if login := h.passwordLogin(trusted); responseCookie(login, sessionCookieName) == nil {
		t.Fatalf("a trusted browser must skip the hold: %d %s", login.Code, login.Body.String())
	}
}

func TestApprovalWithoutSession_HostApproveDoesNotTrust(t *testing.T) {
	h := newCodeHarness(t, true)
	h.setApprovalMode(approvalWithoutSessionHold)
	id, binding := h.pendingApproval(h.passwordLogin())
	if ok, err := h.db.DecideLoginApproval(context.Background(), id, h.user.ID, true, "host:approve-signin", time.Now()); err != nil || !ok {
		t.Fatalf("host approve = %v %v", ok, err)
	}
	out, rec := h.poll(binding)
	if out.Status != approvalPollApproved || responseCookie(rec, sessionCookieName) == nil {
		t.Fatalf("poll = %q %s", out.Status, rec.Body.String())
	}
	if responseCookie(rec, trustedCookie) != nil {
		t.Fatal("a host approval must not trust the device")
	}
}

type unreadablePolicyStore struct{ SecurityStore }

func (unreadablePolicyStore) GetSecurityPolicy(context.Context) (store.SecurityPolicy, bool, error) {
	return store.SecurityPolicy{}, false, errors.New("store down")
}

func TestApprovalWithoutSession_ReadErrorUsesLastGood(t *testing.T) {
	tests := []struct {
		name        string
		last        string
		accountHold bool
		want        bool
	}{
		{"last good hold stays hold", approvalWithoutSessionHold, false, true},
		{"last good allow stays allow", approvalWithoutSessionAllow, false, false},
		{"no last good value with the account switch fails closed", "", true, true},
		{"no last good value without the switch uses the default", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			if tt.accountHold {
				if err := h.db.SetRequireNewDeviceApproval(context.Background(), h.user.ID, true, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			if tt.last != "" {
				h.rt.approvalModeMemo().remember(tt.last)
			}
			h.rt.security = unreadablePolicyStore{h.rt.security}
			if got := h.rt.holdWithoutLiveSession(context.Background(), h.user.ID); got != tt.want {
				t.Fatalf("hold = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApprovalWithoutSession_SetSafeguards(t *testing.T) {
	stepUp := `"step_up":{"method":"password","password":"` + testAdminPassword + `"}`
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"unknown value is refused", `{"approval_without_session":"maybe"}`, http.StatusBadRequest, ""},
		{"hold needs a fresh step-up", `{"approval_without_session":"hold","confirm_host_access":true}`, http.StatusForbidden, errCodeStepUpRequired},
		{"hold with no approval path is refused", `{"approval_without_session":"hold",` + stepUp + `}`, http.StatusConflict, errCodeApprovalHoldNoChannel},
		{"hold with host access confirmed is saved", `{"approval_without_session":"hold","confirm_host_access":true,` + stepUp + `}`, http.StatusOK, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			admin := loginTestSession(t, h.rt, h.db)
			rec := h.send(http.MethodPut, "/api/v1/security/policy", tt.body, "", admin)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantErr != "" && !strings.Contains(rec.Body.String(), tt.wantErr) {
				t.Fatalf("body = %s, want code %s", rec.Body.String(), tt.wantErr)
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var out securityPolicyResource
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.ApprovalWithoutSession != approvalWithoutSessionHold {
				t.Fatalf("policy = %s", rec.Body.String())
			}
			if responseCookie(rec, trustedCookie) == nil {
				t.Fatal("the device that sets hold must stay trusted")
			}
		})
	}
}

func TestApprovalWithoutSession_PostureSuggestsWithoutScore(t *testing.T) {
	in := healthyPosture()
	in.Policy.ApprovalWithoutSession = approvalWithoutSessionAllow
	items := evaluatePosture(in)
	got := postureByID(items)["approval_without_session_allow"]
	if got.Status != postureStatusFail || !got.Recommended || got.Fix == nil {
		t.Fatalf("allow = %+v, want a recommended suggestion with a fix", got)
	}
	if postureScore(items) != 100 {
		t.Fatalf("score = %d, allow must not cost score", postureScore(items))
	}
}

// tokenApprove decides id as a signin:approve token owned by owner, sending the shown number.
func (h *codeHarness) tokenApprove(id, owner string) *httptest.ResponseRecorder {
	h.t.Helper()
	a, err := h.db.GetLoginApproval(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	tok := &store.APIToken{ID: "tok_" + owner, OwnerUserID: owner, Abilities: []string{AbilitySignInApprove}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login-approvals/x/approve", strings.NewReader(`{"match":`+strconv.Itoa(approvalMatch(a.BrowserHash))+`}`))
	req = req.WithContext(withTokenIdentity(req.Context(), tok))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.rt.handleApproveLoginApproval(rec, req)
	return rec
}

func TestApprovalWithoutSession_OnlyTheOwnersTokenApproves(t *testing.T) {
	h := newCodeHarness(t, true)
	h.setApprovalMode(approvalWithoutSessionHold)
	id, binding := h.pendingApproval(h.passwordLogin())
	if rec := h.tokenApprove(id, "user_other"); rec.Code != http.StatusNotFound {
		t.Fatalf("another user's token = %d, want 404", rec.Code)
	}
	if out, _ := h.poll(binding); out.Status != approvalPollPending {
		t.Fatalf("poll = %q, a foreign token must leave it pending", out.Status)
	}
	if rec := h.tokenApprove(id, h.user.ID); rec.Code != http.StatusNoContent {
		t.Fatalf("own token = %d (%s), want 204", rec.Code, rec.Body.String())
	}
	if out, _ := h.poll(binding); out.Status != approvalPollApproved {
		t.Fatalf("poll = %q, want approved", out.Status)
	}
}

func TestApprovalWithoutSession_FirstLoginTrustStillWins(t *testing.T) {
	h := newFirstUserHarness(t)
	h.setApprovalMode(approvalWithoutSessionHold)
	h.setupSession()
	h.wantThrough(h.passwordLogin())
	if !containsAction(h.auditActions(), store.AuditActionFirstLoginTrust) {
		t.Fatalf("audit actions %v lack %s", h.auditActions(), store.AuditActionFirstLoginTrust)
	}
	// The trust is one-shot: the next browser is held.
	h.pendingApproval(h.passwordLogin())
}

func TestApprovalWithoutSession_ConcurrentPairMakesOnePendingApproval(t *testing.T) {
	h := newCodeHarness(t, true)
	h.setApprovalMode(approvalWithoutSessionHold)
	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, 2)
	for i := range recs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recs[i] = h.passwordLogin()
		}()
	}
	wg.Wait()
	for _, rec := range recs {
		if responseCookie(rec, sessionCookieName) != nil {
			t.Fatalf("a held sign-in must not get a session: %s", rec.Body.String())
		}
	}
	pending, err := h.db.ListPendingLoginApprovalsForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending approvals = %d (%v), want 1", len(pending), err)
	}
}

func TestApprovalWithoutSession_HoldNoteAndChannels(t *testing.T) {
	h := newCodeHarness(t, true)
	admin := loginTestSession(t, h.rt, h.db)
	if got := h.rt.approvalChannels(context.Background(), ""); len(got) != 1 || got[0] != approvalChannelHostCommand {
		t.Fatalf("channels = %v, want only the host command", got)
	}
	h.setApprovalMode(approvalWithoutSessionHold)
	rec := h.send(http.MethodGet, "/api/v1/security/policy", "", "", admin)
	var out securityPolicyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.ApprovalHoldNote, "trusted") {
		t.Fatalf("hold note = %q", out.ApprovalHoldNote)
	}
	if note := h.rt.approvalHoldNote(securityPolicy{ApprovalScope: approvalScopePasswordOnly}); !strings.Contains(note, "SSO and passkey") {
		t.Fatalf("scope note missing: %q", note)
	}
	h.setApprovalMode(approvalWithoutSessionAllow)
	rec = h.send(http.MethodGet, "/api/v1/security/policy", "", "", admin)
	if strings.Contains(rec.Body.String(), "approval_hold_note") {
		t.Fatalf("allow mode must carry no hold note: %s", rec.Body.String())
	}
}
