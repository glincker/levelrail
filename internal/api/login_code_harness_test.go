package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	codeTestEmail    = "dev@example.com"
	codeTestPassword = "a-long-enough-password"
	codeTestIP       = "192.0.2.10:4000"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type capturingMail struct {
	mu     sync.Mutex
	bodies []string
}

func (m *capturingMail) Send(_ context.Context, _, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bodies = append(m.bodies, subject+"\n"+body)
	return nil
}

func (m *capturingMail) all() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.bodies, "\n")
}

type codeHarness struct {
	t    *testing.T
	rt   *Router
	db   *store.DB
	logs *lockedBuffer
	mail *capturingMail
	user store.User
}

func newCodeHarness(t *testing.T, approval bool) *codeHarness {
	t.Helper()
	t.Setenv(envLoginCodeMinResponse, "1ms")
	db := openTestDB(t)
	h := &codeHarness{t: t, db: db, logs: &lockedBuffer{}, mail: &capturingMail{}}
	logger := slog.New(slog.NewTextHandler(h.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h.rt = NewRouter(logger, testBrand(), db, WithEmailSender(h.mail), WithNewDeviceApproval(approval))
	bootstrapTestAdmin(t, db)
	hash, err := bcrypt.GenerateFromPassword([]byte(codeTestPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	hs := string(hash)
	h.user = store.User{ID: "user_dev", Email: codeTestEmail, DisplayName: "Dev", PasswordHash: &hs,
		Abilities: []string{AbilityRead, AbilityWrite}, CreatedAt: time.Now()}
	if err := db.CreateUser(context.Background(), h.user); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *codeHarness) send(method, path, body, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if remote != "" {
		req.RemoteAddr = remote
	}
	for _, c := range cookies {
		if c != nil {
			req.AddCookie(c)
		}
	}
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func responseCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}

// requestCode asks for a code for username and returns the binding cookie.
func (h *codeHarness) requestCode(username string) (*httptest.ResponseRecorder, *http.Cookie) {
	h.t.Helper()
	rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+username+`"}`, codeTestIP)
	return rec, responseCookie(rec, loginCodeCookie)
}

// revealFirst reads the user's newest code through their own session.
func (h *codeHarness) revealFirst(session *http.Cookie) string {
	h.t.Helper()
	rec := h.send(http.MethodGet, "/api/v1/auth/sign-in-requests", "", "", session)
	var list signInRequestsResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Codes) == 0 {
		h.t.Fatalf("list sign-in requests: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.send(http.MethodPost, "/api/v1/auth/sign-in-requests/codes/"+list.Codes[0].ID+"/reveal", "", "", session)
	var out revealLoginCodeResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		h.t.Fatalf("reveal: %d %s", rec.Code, rec.Body.String())
	}
	return out.Code
}

func (h *codeHarness) redeem(code string, binding *http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.send(http.MethodPost, "/api/v1/auth/login-code/redeem", `{"code":"`+code+`"}`, codeTestIP, binding)
}

func (h *codeHarness) userSession() *http.Cookie {
	h.t.Helper()
	return sessionCookieForTest(h.t, h.rt, h.user.ID)
}

func (h *codeHarness) passwordLogin(cookies ...*http.Cookie) *httptest.ResponseRecorder {
	h.t.Helper()
	return h.send(http.MethodPost, "/api/v1/auth/login",
		`{"username":"`+codeTestEmail+`","password":"`+codeTestPassword+`"}`, codeTestIP, cookies...)
}

func (h *codeHarness) auditActions() []string {
	h.t.Helper()
	rows, err := h.db.QueryContext(context.Background(), `SELECT action FROM audit_log WHERE action != '' ORDER BY created_at`)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			h.t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func httptestRequestWithToken(method, path string, rec *store.APIToken) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return req.WithContext(withTokenIdentity(req.Context(), rec))
}

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }

func containsAction(actions []string, want string) bool {
	for _, a := range actions {
		if a == want {
			return true
		}
	}
	return false
}
