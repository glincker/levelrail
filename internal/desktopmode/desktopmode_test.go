package desktopmode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tokenhash"
)

type fakeSessions struct {
	mu      sync.Mutex
	n       int
	live    map[string]bool
	revoked []string
	allFor  string
}

func (f *fakeSessions) IssueSession(_ context.Context, _ authengine.LegacyUser, _, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if f.live == nil {
		f.live = map[string]bool{}
	}
	tok := "sess-" + strconv.Itoa(f.n)
	f.live[tok] = true
	return tok, nil
}

func (f *fakeSessions) Revoke(_ context.Context, tok string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.live, tok)
	f.revoked = append(f.revoked, tok)
}

func (f *fakeSessions) RevokeAll(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.live = map[string]bool{}
	f.allFor = id
	return nil
}

type rig struct {
	svc   *Service
	h     http.Handler
	tok   *Tokens
	sess  *fakeSessions
	dir   string
	now   *time.Time
	audit []store.AuditEntry
	ttl   time.Duration
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{dir: t.TempDir(), sess: &fakeSessions{}, ttl: time.Hour}
	start := time.Unix(1_800_000_000, 0)
	r.now = &start
	tok, err := NewTokens(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	r.tok = tok
	r.svc = New(Config{
		Tokens: tok, Sessions: r.sess, Ports: []string{"8080"},
		Admin: func(context.Context) (authengine.LegacyUser, error) {
			return authengine.LegacyUser{ID: "u1", Email: "a@b.c"}, nil
		},
		TTL:        func(context.Context) time.Duration { return r.ttl },
		Audit:      func(_ context.Context, e store.AuditEntry) { r.audit = append(r.audit, e) },
		RatePerMin: 100, MaxFailures: 3, Lockout: time.Minute,
		Now: func() time.Time { return *r.now }, AllowRemotePeer: true,
	})
	r.h = r.svc.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	return r
}

func (r *rig) plain(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(TokenPath(r.dir))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

func (r *rig) do(method, path, token, host, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	return w
}

func TestTokenCreatedHashedAndFileMode(t *testing.T) {
	r := newRig(t)
	plain := r.plain(t)
	if !strings.HasPrefix(plain, tokenPrefix) || len(plain) < 40 {
		t.Fatalf("weak token %q", plain)
	}
	hash, _ := os.ReadFile(hashPath(r.dir))
	if string(hash) != tokenhash.Sum(plain) || strings.Contains(string(hash), plain) {
		t.Fatal("stored value is not the hash")
	}
	if runtime.GOOS != "windows" {
		for _, p := range []string{TokenPath(r.dir), hashPath(r.dir)} {
			st, err := os.Stat(p)
			if err != nil || st.Mode().Perm() != 0o600 {
				t.Fatalf("%s mode = %v, %v", p, st.Mode().Perm(), err)
			}
		}
	}
	again, err := NewTokens(r.dir)
	if err != nil || !again.Verify(plain) {
		t.Fatal("token not stable across restarts")
	}
}

func TestVerifyUsesConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("token.go")
	if err != nil || !strings.Contains(string(src), "tokenhash.Matches(") {
		t.Fatal("Verify must compare through tokenhash.Matches")
	}
	r := newRig(t)
	p := r.plain(t)
	if r.tok.Verify(p[:len(p)-1]) || r.tok.Verify("") || !r.tok.Verify(p) {
		t.Fatal("verify wrong")
	}
}

func TestExchangeSuccessSetsStrictCookieAndAudits(t *testing.T) {
	r := newRig(t)
	w := r.do("POST", ExchangePath, r.plain(t), "127.0.0.1:8080", "http://127.0.0.1:8080")
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	c := w.Result().Cookies()
	if len(c) != 1 || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode || c[0].Name != "session_token" {
		t.Fatalf("cookie %+v", c)
	}
	if !c[0].Expires.Equal(r.now.Add(time.Hour).Truncate(time.Second)) {
		t.Fatalf("expires %v", c[0].Expires)
	}
	if len(r.audit) != 1 || r.audit[0].Action != "desktop.exchange" {
		t.Fatalf("audit %+v", r.audit)
	}
	if strings.Contains(r.audit[0].Detail+r.audit[0].Path, c[0].Value) || strings.Contains(r.audit[0].Detail, r.plain(t)) {
		t.Fatal("audit leaks a secret")
	}
}

func TestExchangeFailures(t *testing.T) {
	r := newRig(t)
	for name, tok := range map[string]string{"missing": "", "wrong": "dsk_nope"} {
		if w := r.do("POST", ExchangePath, tok, "localhost:8080", ""); w.Code != 401 {
			t.Fatalf("%s: %d", name, w.Code)
		}
	}
	if w := r.do("GET", ExchangePath, r.plain(t), "localhost:8080", ""); w.Code != 405 {
		t.Fatalf("GET: %d", w.Code)
	}
	if len(r.sess.live) != 0 {
		t.Fatal("session issued on failure")
	}
}

func TestHostAndOriginRejected(t *testing.T) {
	r := newRig(t)
	p := r.plain(t)
	for _, host := range []string{"evil.example:8080", "127.0.0.1:9999", "127.0.0.1", "localhost.evil.com:8080"} {
		if w := r.do("GET", "/api/x", "", host, ""); w.Code != 403 {
			t.Fatalf("host %q: %d", host, w.Code)
		}
	}
	for _, origin := range []string{"http://evil.example", "null", "https://127.0.0.1:8080", "http://127.0.0.1:9999"} {
		if w := r.do("POST", ExchangePath, p, "127.0.0.1:8080", origin); w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
		if w := r.do("DELETE", "/api/apps/x", "", "127.0.0.1:8080", origin); w.Code != 403 {
			t.Fatalf("state change origin %q: %d", origin, w.Code)
		}
	}
	if w := r.do("GET", "/api/x", "", "127.0.0.1:8080", "http://evil.example"); w.Code != http.StatusTeapot {
		t.Fatalf("safe GET with foreign origin should pass through: %d", w.Code)
	}
	if w := r.do("POST", "/api/x", "", "[::1]:8080", "http://[::1]:8080"); w.Code != http.StatusTeapot {
		t.Fatalf("good origin: %d", w.Code)
	}
}

func TestNonLoopbackPeerRefused(t *testing.T) {
	r := newRig(t)
	r.svc.cfg.AllowRemotePeer = false
	req := httptest.NewRequest("POST", ExchangePath, nil)
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "10.0.0.5:4000"
	req.Header.Set("Authorization", "Bearer "+r.plain(t))
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestLockoutAndRateLimit(t *testing.T) {
	r := newRig(t)
	good := r.plain(t)
	for range 3 {
		r.do("POST", ExchangePath, "bad", "localhost:8080", "")
	}
	if w := r.do("POST", ExchangePath, good, "localhost:8080", ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("locked out should refuse the right token: %d", w.Code)
	}
	*r.now = r.now.Add(2 * time.Minute)
	if w := r.do("POST", ExchangePath, good, "localhost:8080", ""); w.Code != 200 {
		t.Fatalf("after lockout: %d", w.Code)
	}
	r.svc.lim.rpm = 2
	r.svc.lim.window = nil
	r.do("POST", ExchangePath, good, "localhost:8080", "")
	r.do("POST", ExchangePath, good, "localhost:8080", "")
	if w := r.do("POST", ExchangePath, good, "localhost:8080", ""); w.Code != 429 {
		t.Fatalf("rate limit: %d", w.Code)
	}
}

func TestTTLHonoredAndExpiredSessionRevoked(t *testing.T) {
	r := newRig(t)
	r.ttl = 90 * time.Minute
	w := r.do("POST", ExchangePath, r.plain(t), "localhost:8080", "")
	sess := w.Result().Cookies()[0]
	if !sess.Expires.Equal(r.now.Add(90 * time.Minute).Truncate(time.Second)) {
		t.Fatalf("expires %v", sess.Expires)
	}
	req := func() {
		q := httptest.NewRequest("GET", "/api/x", nil)
		q.Host = "localhost:8080"
		q.AddCookie(sess)
		r.h.ServeHTTP(httptest.NewRecorder(), q)
	}
	req()
	if !r.sess.live[sess.Value] {
		t.Fatal("session revoked before TTL")
	}
	*r.now = r.now.Add(91 * time.Minute)
	req()
	if r.sess.live[sess.Value] {
		t.Fatal("session survived past the setting's TTL")
	}
}

func TestRotateAndRevokeInvalidate(t *testing.T) {
	r := newRig(t)
	old := r.plain(t)
	r.do("POST", ExchangePath, old, "localhost:8080", "")
	if w := r.do("POST", RotatePath, old, "localhost:8080", ""); w.Code != 204 {
		t.Fatalf("rotate %d", w.Code)
	}
	if len(r.sess.live) != 0 || r.sess.allFor != "u1" {
		t.Fatal("rotate left sessions alive")
	}
	if r.tok.Verify(old) || !r.tok.Verify(r.plain(t)) {
		t.Fatal("old token still valid or new one not")
	}
	cur := r.plain(t)
	r.do("POST", ExchangePath, cur, "localhost:8080", "")
	if w := r.do("POST", RevokePath, cur, "localhost:8080", ""); w.Code != 204 {
		t.Fatalf("revoke %d", w.Code)
	}
	if len(r.sess.live) != 0 || r.tok.Verify(cur) {
		t.Fatal("revoke left sessions or token alive")
	}
	if _, err := os.Stat(TokenPath(r.dir)); !os.IsNotExist(err) {
		t.Fatal("token file survived revoke")
	}
	var actions []string
	for _, e := range r.audit {
		actions = append(actions, e.Action)
	}
	got := strings.Join(actions, ",")
	if !strings.Contains(got, "desktop.rotate") || !strings.Contains(got, "desktop.revoke") {
		t.Fatalf("audit %s", got)
	}
}

func TestOffByDefaultRouteIsAbsent(t *testing.T) {
	if Enabled(func(string) string { return "" }) {
		t.Fatal("enabled by default")
	}
	mux := http.NewServeMux()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", ExchangePath, nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("code %d", w.Code)
	}
}
