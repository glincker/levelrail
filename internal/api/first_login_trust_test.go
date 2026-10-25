package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newFirstUserHarness is a fresh install: the only account is the setup admin.
func newFirstUserHarness(t *testing.T) *codeHarness {
	t.Helper()
	return newFirstUserHarnessAged(t, 0)
}

// newFirstUserHarnessAged backdates the account by age.
func newFirstUserHarnessAged(t *testing.T, age time.Duration) *codeHarness {
	t.Helper()
	t.Setenv(envLoginCodeMinResponse, "1ms")
	db := openTestDB(t)
	h := &codeHarness{t: t, db: db, logs: &lockedBuffer{}, mail: &capturingMail{}}
	logger := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.rt = NewRouter(logger, testBrand(), db, WithEmailSender(h.mail), WithNewDeviceApproval(true))
	hash, err := bcrypt.GenerateFromPassword([]byte(codeTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hs := string(hash)
	h.user = store.User{ID: "user_setup", Email: codeTestEmail, DisplayName: "Setup", PasswordHash: &hs,
		Abilities: []string{AbilityRoot}, IsFirstUser: true, CreatedAt: time.Now().Add(-age)}
	if err := db.CreateUser(context.Background(), h.user); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *codeHarness) wantThrough(rec *httptest.ResponseRecorder) {
	h.t.Helper()
	if rec.Code != http.StatusOK || responseCookie(rec, sessionCookieName) == nil || strings.Contains(rec.Body.String(), "approval_required") {
		h.t.Fatalf("sign-in must go through: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFirstLoginTrust(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		setupSess   int
		age         time.Duration
		wantThrough bool
	}{
		{name: "setup session only, inside the window", setupSess: 1, wantThrough: true},
		{name: "no other session at all", wantThrough: true},
		{name: "two other sessions", setupSess: 2},
		{name: "window turned off", env: map[string]string{envFirstLoginTrustWindow: "0"}, setupSess: 1},
		{name: "window elapsed", age: 31 * time.Minute, setupSess: 1},
		{name: "window elapsed, tighter env", env: map[string]string{envFirstLoginTrustWindow: "5m"}, age: 10 * time.Minute, setupSess: 1},
		{name: "extra session opened long after setup", age: 10 * time.Minute, setupSess: 1},
		{name: "custom longer window", env: map[string]string{envFirstLoginTrustWindow: "2h", envFirstLoginSetupGrace: "30m"}, age: 10 * time.Minute, setupSess: 1, wantThrough: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			h := newFirstUserHarnessAged(t, tt.age)
			for range tt.setupSess {
				sessionCookieForTest(t, h.rt, h.user.ID)
			}
			rec := h.passwordLogin()
			if tt.wantThrough {
				h.wantThrough(rec)
				if !containsAction(h.auditActions(), store.AuditActionFirstLoginTrust) {
					t.Fatalf("audit actions %v lack %s", h.auditActions(), store.AuditActionFirstLoginTrust)
				}
				return
			}
			h.pendingApproval(rec)
			if containsAction(h.auditActions(), store.AuditActionFirstLoginTrust) {
				t.Fatal("a refused trust must not be audited as granted")
			}
		})
	}
}

func TestFirstLoginTrustIsOneShot(t *testing.T) {
	h := newFirstUserHarness(t)
	setup := sessionCookieForTest(t, h.rt, h.user.ID)
	first := h.passwordLogin()
	h.wantThrough(first)
	if responseCookie(first, trustedCookie) == nil {
		t.Fatal("the trusted browser cookie must be issued with the first login")
	}
	// A different browser (no trusted cookie) now needs approval: a session and a trusted device exist.
	h.pendingApproval(h.passwordLogin())
	h.pendingApproval(h.passwordLogin(setup))
	// The trusted browser keeps signing in without approval.
	h.wantThrough(h.passwordLogin(responseCookie(first, trustedCookie)))
}

func TestFirstLoginTrustNeverForLaterUsers(t *testing.T) {
	h := newCodeHarness(t, true)
	h.userSession()
	h.pendingApproval(h.passwordLogin())
	if containsAction(h.auditActions(), store.AuditActionFirstLoginTrust) {
		t.Fatal("a later user must never get first-login trust")
	}
}

func TestFirstLoginTrustNeedsPasswordMethod(t *testing.T) {
	h := newFirstUserHarness(t)
	if h.rt.firstLoginTrusted(context.Background(), h.user, "", signInMethodPasskey) {
		t.Fatal("only a password sign-in may be trusted on first use")
	}
	if h.rt.firstLoginTrusted(context.Background(), h.user, "", signInMethodOAuth) {
		t.Fatal("only a password sign-in may be trusted on first use")
	}
}

func TestRootTokenDecidesOwnApprovals(t *testing.T) {
	approveBody := func(h *codeHarness, id string) string {
		a, err := h.db.GetLoginApproval(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return `{"match":` + strconv.Itoa(approvalMatch(a.BrowserHash)) + `}`
	}
	tests := []struct {
		name   string
		token  func(h *codeHarness) *store.APIToken
		body   func(h *codeHarness, id string) string
		deny   bool
		want   int
		wantOK bool
	}{
		{"root token approves", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_root", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}}
		}, approveBody, false, http.StatusNoContent, true},
		{"root token denies", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_root", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}}
		}, nil, true, http.StatusNoContent, true},
		{"root token cannot use the password shortcut", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_root", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}}
		}, func(*codeHarness, string) string {
			return `{"step_up":{"password":"` + codeTestPassword + `"}}`
		}, false, http.StatusForbidden, false},
		{"root token of another user", func(*codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_other", OwnerUserID: "someone_else", Abilities: []string{AbilityRoot}}
		}, approveBody, false, http.StatusNotFound, false},
		{"root token without an owner", func(*codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_sys", Abilities: []string{AbilityRoot}}
		}, approveBody, false, http.StatusForbidden, false},
		{"root agent token", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_agent", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}, AgentName: "bot"}
		}, approveBody, false, http.StatusForbidden, false},
		{"write:sensitive token", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_ws", OwnerUserID: h.user.ID, Abilities: []string{AbilityWriteSensitive}}
		}, approveBody, false, http.StatusForbidden, false},
		{"read token", func(h *codeHarness) *store.APIToken {
			return &store.APIToken{ID: "tok_read", OwnerUserID: h.user.ID, Abilities: []string{AbilityRead}}
		}, approveBody, false, http.StatusForbidden, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFirstUserHarness(t)
			sessionCookieForTest(t, h.rt, h.user.ID)
			sessionCookieForTest(t, h.rt, h.user.ID)
			id, binding := h.pendingApproval(h.passwordLogin())
			body := `{}`
			if tt.body != nil {
				body = tt.body(h, id)
			}
			verb := "approve"
			if tt.deny {
				verb = "deny"
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login-approvals/x/"+verb, strings.NewReader(body))
			req = req.WithContext(withTokenIdentity(req.Context(), tt.token(h)))
			req.SetPathValue("id", id)
			rec := httptest.NewRecorder()
			if tt.deny {
				h.rt.handleDenyLoginApproval(rec, req)
			} else {
				h.rt.handleApproveLoginApproval(rec, req)
			}
			if rec.Code != tt.want {
				t.Fatalf("%s = %d (%s), want %d", verb, rec.Code, rec.Body.String(), tt.want)
			}
			out, _ := h.poll(binding)
			switch {
			case tt.wantOK && !tt.deny && out.Status != approvalPollApproved:
				t.Fatalf("poll = %q, want approved", out.Status)
			case tt.wantOK && tt.deny && out.Status != approvalPollDenied:
				t.Fatalf("poll = %q, want denied", out.Status)
			case !tt.wantOK && out.Status != approvalPollPending:
				t.Fatalf("a refused decision must leave the sign-in pending, poll = %q", out.Status)
			}
		})
	}
}

func TestRootTokenStillCannotRevealCodes(t *testing.T) {
	h := newFirstUserHarness(t)
	tok := &store.APIToken{ID: "tok_root", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}}
	h.requestCode(codeTestEmail)
	req := httptestRequestWithToken(http.MethodPost, "/api/v1/auth/sign-in-requests/codes/x/reveal", tok)
	req.SetPathValue("id", "any")
	rec := newRecorder()
	h.rt.handleRevealLoginCode(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reveal with a root token = %d, want 403", rec.Code)
	}
}

func TestPendingApprovalDedupe(t *testing.T) {
	h := newCodeHarness(t, true)
	h.userSession()
	id1, binding := h.pendingApproval(h.passwordLogin())
	if binding == nil {
		t.Fatal("no binding cookie")
	}
	before := h.auditActions()

	id2, again := h.pendingApproval(h.passwordLogin(binding))
	if id2 != id1 {
		t.Fatalf("a retry from the same browser must return the open request: %s != %s", id2, id1)
	}
	if again != nil {
		t.Fatal("a reused request must not rotate the binding cookie")
	}
	if got := len(h.auditActions()); got != len(before) {
		t.Fatalf("a reused request must not write new audit rows: %v", h.auditActions())
	}
	list, err := h.db.ListPendingLoginApprovalsForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(list) != 1 {
		t.Fatalf("pending approvals = %d, %v; want exactly 1", len(list), err)
	}

	// A different browser has no binding, so it gets its own request and the old one is superseded.
	id3, _ := h.pendingApproval(h.passwordLogin())
	if id3 == id1 {
		t.Fatal("a browser without the binding must not share another's request")
	}
}

func TestPendingApprovalDedupeIgnoresOtherUsersAndSettledRequests(t *testing.T) {
	h := newCodeHarness(t, true)
	session := h.userSession()
	id, binding := h.pendingApproval(h.passwordLogin())
	if rec := h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/deny", `{}`, "", session); rec.Code != http.StatusNoContent {
		t.Fatalf("deny = %d %s", rec.Code, rec.Body.String())
	}
	next, _ := h.pendingApproval(h.passwordLogin(binding))
	if next == id {
		t.Fatal("a denied request must not be handed out again")
	}
}
