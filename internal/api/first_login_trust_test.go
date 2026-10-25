package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

// setupSession opens the session the setup admin registered with and records
// it, from setupTestIP, the way libRegister does.
func (h *codeHarness) setupSession() *http.Cookie {
	h.t.Helper()
	c := sessionCookieForTest(h.t, h.rt, h.user.ID)
	sid, ok := h.rt.libSessions.SessionIDFor(context.Background(), c.Value)
	if !ok {
		h.t.Fatal("setup session is not live")
	}
	if err := h.db.SetUserSetupSession(context.Background(), h.user.ID, sid, setupTestIP); err != nil {
		h.t.Fatal(err)
	}
	return c
}

const setupTestIP = "192.0.2.10"

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
			for i := range tt.setupSess {
				if i == 0 {
					h.setupSession()
					continue
				}
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
	setup := h.setupSession()
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
	if h.rt.firstLoginTrusted(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), h.user, "", signInMethodPasskey) {
		t.Fatal("only a password sign-in may be trusted on first use")
	}
	if h.rt.firstLoginTrusted(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), h.user, "", signInMethodOAuth) {
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
		}, approveBody, false, http.StatusForbidden, false},
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
			h.setupSession()
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
	if got := h.auditActions(); len(got) != len(before)+1 || got[len(got)-1] != store.AuditActionNewDeviceRequest {
		t.Fatalf("a reused request must write exactly one retry audit row: %v", got)
	}
	if n := h.auditPathCount(approvalAuditPrefix + id1 + "#retry"); n != 1 {
		t.Fatalf("retry audit rows for %s = %d, want 1", id1, n)
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

func (h *codeHarness) auditPathCount(path string) int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audit_log WHERE path = ?`, path).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// pendingSetupApproval is a fresh install with a setup session and one waiting browser.
func pendingSetupApproval(t *testing.T, age time.Duration) (*codeHarness, string, *http.Cookie) {
	t.Helper()
	h := newFirstUserHarnessAged(t, age)
	h.setupSession()
	sessionCookieForTest(t, h.rt, h.user.ID)
	id, binding := h.pendingApproval(h.passwordLogin())
	return h, id, binding
}

func (h *codeHarness) rootApprove(id string) *httptest.ResponseRecorder {
	h.t.Helper()
	a, err := h.db.GetLoginApproval(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	tok := &store.APIToken{ID: "tok_root", OwnerUserID: h.user.ID, Abilities: []string{AbilityRoot}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login-approvals/x/approve", strings.NewReader(`{"match":`+strconv.Itoa(approvalMatch(a.BrowserHash))+`}`))
	req = req.WithContext(withTokenIdentity(req.Context(), tok))
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()
	h.rt.handleApproveLoginApproval(rec, req)
	return rec
}

func TestRootTokenApprovalClosesWithTheFirstLoginWindow(t *testing.T) {
	h, id, binding := pendingSetupApproval(t, 40*time.Minute)
	if rec := h.rootApprove(id); rec.Code != http.StatusForbidden {
		t.Fatalf("root approve after the window = %d (%s), want 403", rec.Code, rec.Body.String())
	}
	if out, _ := h.poll(binding); out.Status != approvalPollPending {
		t.Fatalf("poll = %q, want pending", out.Status)
	}
}

func TestRootTokenApprovalRefusedWhenWindowOff(t *testing.T) {
	t.Setenv(envFirstLoginTrustWindow, "off")
	h, id, _ := pendingSetupApproval(t, 0)
	if rec := h.rootApprove(id); rec.Code != http.StatusForbidden {
		t.Fatalf("root approve with the window off = %d, want 403", rec.Code)
	}
}

func TestRootApprovedSignInEarnsNoTrustedDevice(t *testing.T) {
	h, id, binding := pendingSetupApproval(t, 0)
	if rec := h.rootApprove(id); rec.Code != http.StatusNoContent {
		t.Fatalf("root approve inside the window = %d (%s)", rec.Code, rec.Body.String())
	}
	out, rec := h.poll(binding)
	if out.Status != approvalPollApproved || responseCookie(rec, sessionCookieName) == nil {
		t.Fatalf("poll = %q, want approved with a session", out.Status)
	}
	if responseCookie(rec, trustedCookie) != nil {
		t.Fatal("a root-approved sign-in must not be handed the 30-day trusted device")
	}
	devices, err := h.db.ListTrustedDevices(context.Background(), h.user.ID, time.Now())
	if err != nil || len(devices) != 0 {
		t.Fatalf("trusted devices = %d, %v; want none", len(devices), err)
	}
}

func TestResetRetiresRootApprovedSignIn(t *testing.T) {
	h, id, binding := pendingSetupApproval(t, 0)
	if rec := h.rootApprove(id); rec.Code != http.StatusNoContent {
		t.Fatalf("root approve = %d", rec.Code)
	}
	h.rt.resetSignInTrust(context.Background(), nil, h.user.ID, "password_changed")
	if out, _ := h.poll(binding); out.Status == approvalPollApproved {
		t.Fatal("an approval a root token gave must not survive a credential reset")
	}
}

func TestFirstLoginTrustNeedsTheSetupDevice(t *testing.T) {
	tests := []struct {
		name        string
		remote      string
		useSetup    bool
		wantThrough bool
	}{
		{name: "same address as the registering browser", remote: "192.0.2.10:4000", wantThrough: true},
		{name: "other address, no setup cookie", remote: "198.51.100.77:4000"},
		{name: "other address carrying the setup session", remote: "198.51.100.77:4000", useSetup: true, wantThrough: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newFirstUserHarness(t)
			setup := h.setupSession()
			var cookies []*http.Cookie
			if tt.useSetup {
				cookies = append(cookies, setup)
			}
			rec := h.send(http.MethodPost, "/api/v1/auth/login", `{"username":"`+codeTestEmail+`","password":"`+codeTestPassword+`"}`, tt.remote, cookies...)
			if tt.wantThrough {
				h.wantThrough(rec)
				return
			}
			h.pendingApproval(rec)
		})
	}
}

func TestFirstLoginTrustRefusesUnrecordedSession(t *testing.T) {
	h := newFirstUserHarness(t)
	sessionCookieForTest(t, h.rt, h.user.ID)
	h.pendingApproval(h.passwordLogin())
}

func TestConcurrentFirstLoginsTrustOnlyOne(t *testing.T) {
	h := newFirstUserHarness(t)
	const n = 6
	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recs[i] = h.passwordLogin()
		}()
	}
	wg.Wait()
	through := 0
	for _, rec := range recs {
		if rec.Code == http.StatusOK && responseCookie(rec, sessionCookieName) != nil && !strings.Contains(rec.Body.String(), "approval_required") {
			through++
		}
	}
	trusted := 0
	for _, a := range h.auditActions() {
		if a == store.AuditActionFirstLoginTrust {
			trusted++
		}
	}
	if trusted != 1 {
		t.Fatalf("first-login trust granted %d times, want exactly 1 (through=%d)", trusted, through)
	}
}

func TestFirstLoginTrustCannotReopenAfterRevoke(t *testing.T) {
	h := newFirstUserHarness(t)
	h.setupSession()
	h.wantThrough(h.passwordLogin())
	if _, err := h.db.RevokeAllTrustedDevices(context.Background(), h.user.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Back to the exact precondition of the first grant: no trusted device, only the setup session.
	if rec, err := h.db.ClaimFirstLoginTrust(context.Background(), h.user.ID); err != nil || rec {
		t.Fatalf("the flag must already be spent: %v, %v", rec, err)
	}
	h.pendingApproval(h.passwordLogin())
}

func TestEnvDurationOrOffTreatsGarbageAsOff(t *testing.T) {
	for _, raw := range []string{"off", "false", "disabled", "0", "-5m", "30 minutes"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(envFirstLoginTrustWindow, raw)
			if got := envDurationOrOff(envFirstLoginTrustWindow, defaultFirstLoginTrustWindow); got != 0 {
				t.Fatalf("%q = %v, want off", raw, got)
			}
		})
	}
	t.Setenv(envFirstLoginTrustWindow, "")
	if got := envDurationOrOff(envFirstLoginTrustWindow, defaultFirstLoginTrustWindow); got != defaultFirstLoginTrustWindow {
		t.Fatalf("unset = %v, want the default", got)
	}
	t.Setenv(envFirstLoginTrustWindow, "off")
	h := newFirstUserHarness(t)
	h.setupSession()
	h.pendingApproval(h.passwordLogin())
}
