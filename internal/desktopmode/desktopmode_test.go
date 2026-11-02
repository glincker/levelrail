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
	ttl     time.Duration
}

func (f *fakeSessions) IssueSessionTTL(_ context.Context, _ authengine.LegacyUser, _, _ string, ttl time.Duration) (string, error) {
	f.ttl = ttl
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
		Tokens: tok, Sessions: r.sess, Ports: []string{"8080"}, FailuresPerMin: 3, Block: time.Minute,
		Admin: func(context.Context) (authengine.LegacyUser, error) {
			return authengine.LegacyUser{ID: "u1", Email: "a@b.c"}, nil
		},
		TTL:   func(context.Context) time.Duration { return r.ttl },
		Audit: func(_ context.Context, e store.AuditEntry) { r.audit = append(r.audit, e) },
		Now:   func() time.Time { return *r.now }, AllowRemotePeer: true,
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

func TestVerifyExactMatchOnly(t *testing.T) {
	r := newRig(t)
	p := r.plain(t)
	if r.tok.Verify(p[:len(p)-1]) || r.tok.Verify(p+"x") || r.tok.Verify("") || !r.tok.Verify(p) {
		t.Fatal("verify wrong")
	}
}

func TestExchangeSuccessSetsStrictCookieAndAudits(t *testing.T) {
	r := newRig(t)
	w := r.do("POST", ExchangePath, r.plain(t), "levelrail.localhost:8080", "http://levelrail.localhost:8080")
	if w.Code != 200 {
		t.Fatalf("code %d %s", w.Code, w.Body)
	}
	c := w.Result().Cookies()
	if len(c) != 1 || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode || c[0].Name != "session_token" || c[0].Domain != "" {
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
		if w := r.do("POST", ExchangePath, tok, "levelrail.localhost:8080", ""); w.Code != 401 {
			t.Fatalf("%s: %d", name, w.Code)
		}
	}
	if w := r.do("GET", ExchangePath, r.plain(t), "levelrail.localhost:8080", ""); w.Code != 405 {
		t.Fatalf("GET: %d", w.Code)
	}
	if len(r.sess.live) != 0 {
		t.Fatal("session issued on failure")
	}
}

func TestHostAndOriginRejected(t *testing.T) {
	r := newRig(t)
	p := r.plain(t)
	for _, host := range []string{"localhost:3000", "localhost:8080", "127.0.0.1:8080", "[::1]:8080", "levelrail.localhost", "levelrail.localhost:9999", "evil.example:8080", "levelrail.localhost.evil.com:8080"} {
		if w := r.do("GET", "/api/x", "", host, ""); w.Code != 403 {
			t.Fatalf("host %q: %d", host, w.Code)
		}
	}
	if w := r.do("GET", "/api/x", "", "LevelRail.Localhost:8080", ""); w.Code != http.StatusTeapot {
		t.Fatalf("allowed host: %d", w.Code)
	}
	for _, origin := range []string{"http://evil.example", "null", "https://levelrail.localhost:8080", "http://localhost:8080", "http://levelrail.localhost:9999"} {
		if w := r.do("POST", ExchangePath, p, "levelrail.localhost:8080", origin); w.Code != 403 {
			t.Fatalf("origin %q: %d", origin, w.Code)
		}
		if w := r.do("DELETE", "/api/apps/x", "", "levelrail.localhost:8080", origin); w.Code != 403 {
			t.Fatalf("state change origin %q: %d", origin, w.Code)
		}
	}
	if w := r.do("GET", "/api/x", "", "levelrail.localhost:8080", "http://evil.example"); w.Code != http.StatusTeapot {
		t.Fatalf("safe GET with foreign origin should pass through: %d", w.Code)
	}
	if w := r.do("POST", "/api/x", "", "levelrail.localhost:8080", "http://levelrail.localhost:8080"); w.Code != http.StatusTeapot {
		t.Fatalf("good origin: %d", w.Code)
	}
}

func TestSecFetchSiteCrossSiteRefusedWithoutOrigin(t *testing.T) {
	r := newRig(t)
	req := httptest.NewRequest("POST", "/api/x", nil)
	req.Host = "levelrail.localhost:8080"
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestWebsocketUpgradeChecksOrigin(t *testing.T) {
	r := newRig(t)
	req := httptest.NewRequest("GET", "/api/ws", nil)
	req.Host = "levelrail.localhost:8080"
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestNonLoopbackPeerRefused(t *testing.T) {
	r := newRig(t)
	r.svc.cfg.AllowRemotePeer = false
	req := httptest.NewRequest("POST", ExchangePath, nil)
	req.Host = "levelrail.localhost:8080"
	req.RemoteAddr = "10.0.0.5:4000"
	req.Header.Set("Authorization", "Bearer "+r.plain(t))
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("code %d", w.Code)
	}
}

func (r *rig) doFrom(peer, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", ExchangePath, nil)
	req.Host = "levelrail.localhost:8080"
	req.RemoteAddr = peer + ":4000"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	return w
}

func TestPerPeerLimiterNeverBlocksValidToken(t *testing.T) {
	r := newRig(t)
	good := r.plain(t)
	var last int
	for range 50 {
		last = r.doFrom("10.0.0.9", "bad").Code
	}
	if last != 429 {
		t.Fatalf("bad peer should be blocked, got %d", last)
	}
	if w := r.doFrom("10.0.0.9", "bad"); w.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	if w := r.doFrom("127.0.0.1", good); w.Code != 200 {
		t.Fatalf("valid token after bad attempts from another peer: %d", w.Code)
	}
	if w := r.doFrom("10.0.0.9", good); w.Code != 200 {
		t.Fatalf("valid token from a blocked peer: %d", w.Code)
	}
	for range 100 {
		if w := r.doFrom("127.0.0.1", good); w.Code != 200 {
			t.Fatalf("successes must not count toward a limit: %d", w.Code)
		}
	}
	*r.now = r.now.Add(2 * time.Minute)
	if w := r.doFrom("10.0.0.9", "bad"); w.Code != 401 {
		t.Fatalf("block should have expired: %d", w.Code)
	}
}

func TestTTLComesFromSettingAndRidesTheSession(t *testing.T) {
	r := newRig(t)
	r.ttl = 90 * time.Minute
	w := r.do("POST", ExchangePath, r.plain(t), "levelrail.localhost:8080", "")
	sess := w.Result().Cookies()[0]
	if !sess.Expires.Equal(r.now.Add(90 * time.Minute).Truncate(time.Second)) {
		t.Fatalf("expires %v", sess.Expires)
	}
	if r.sess.ttl != 90*time.Minute {
		t.Fatalf("library got ttl %v, so a restart would lose it", r.sess.ttl)
	}
	r.ttl = time.Hour
	r.do("POST", ExchangePath, r.plain(t), "levelrail.localhost:8080", "")
	if r.sess.ttl != time.Hour {
		t.Fatal("setting change not applied live")
	}
}

func TestNotSingleUserRefused(t *testing.T) {
	r := newRig(t)
	r.svc.cfg.Admin = func(context.Context) (authengine.LegacyUser, error) { return authengine.LegacyUser{}, ErrNotSingleUser }
	if w := r.do("POST", ExchangePath, r.plain(t), "levelrail.localhost:8080", ""); w.Code != 403 {
		t.Fatalf("code %d", w.Code)
	}
}

func TestBootRotatesBrokenHandoff(t *testing.T) {
	r := newRig(t)
	old := r.plain(t)
	again, err := NewTokens(r.dir)
	if err != nil || !again.Verify(old) {
		t.Fatal("intact handoff should be kept")
	}
	// A crash between the two writes: token file and hash disagree.
	if err := os.WriteFile(TokenPath(r.dir), []byte("dsk_stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := NewTokens(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	cur := r.plain(t)
	if cur == "dsk_stale" || !fresh.Verify(cur) || fresh.Verify(old) {
		t.Fatal("mismatched handoff not rotated")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(TokenPath(r.dir), 0o644); err != nil { //nolint:gosec // deliberately loose to test rotation
		t.Fatal(err)
	}
	loose, err := NewTokens(r.dir)
	if err != nil || loose.Verify(cur) {
		t.Fatal("loose mode not rotated")
	}
	target := TokenPath(r.dir) + ".real"
	if err := os.Rename(TokenPath(r.dir), target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, TokenPath(r.dir)); err != nil {
		t.Fatal(err)
	}
	prev := r.plain(t)
	link, err := NewTokens(r.dir)
	if err != nil || link.Verify(prev) {
		t.Fatal("symlink not rotated")
	}
	if st, _ := os.Lstat(TokenPath(r.dir)); !st.Mode().IsRegular() {
		t.Fatal("handoff is still not a regular file")
	}
}

func TestModeParsing(t *testing.T) {
	env := func(v string) func(string) string { return func(string) string { return v } }
	for v, want := range map[string]bool{"": false, "0": false, "off": false, "1": true, "true": true, "yes": true, "ON": true} {
		got, err := Mode(env(v))
		if err != nil || got != want {
			t.Fatalf("%q: %v %v", v, got, err)
		}
	}
	if _, err := Mode(env("maybe")); err == nil {
		t.Fatal("garbage value must be an error")
	}
}

func TestRotateAndRevokeInvalidate(t *testing.T) {
	r := newRig(t)
	old := r.plain(t)
	r.do("POST", ExchangePath, old, "levelrail.localhost:8080", "")
	if w := r.do("POST", RotatePath, old, "levelrail.localhost:8080", ""); w.Code != 204 {
		t.Fatalf("rotate %d", w.Code)
	}
	if len(r.sess.live) != 0 || r.sess.allFor != "u1" {
		t.Fatal("rotate left sessions alive")
	}
	if r.tok.Verify(old) || !r.tok.Verify(r.plain(t)) {
		t.Fatal("old token still valid or new one not")
	}
	cur := r.plain(t)
	r.do("POST", ExchangePath, cur, "levelrail.localhost:8080", "")
	if w := r.do("POST", RevokePath, cur, "levelrail.localhost:8080", ""); w.Code != 204 {
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
