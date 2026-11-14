package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // builds a SHA-1 signature only to prove it is refused
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const syncHookSHA = "0123456789abcdef0123456789abcdef01234567"

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

type syncHookEnv struct {
	rt      *Router
	db      *store.DB
	root    *http.Cookie
	logs    *lockedBuf
	runs    *atomic.Int32
	secret  string
	stackID string
}

func newSyncHookEnv(t *testing.T) *syncHookEnv {
	t.Helper()
	logs := &lockedBuf{}
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(logs, nil)), testBrand(), db, WithGitSourceSecrets(newFakeGitSourceSecrets()))
	env := &syncHookEnv{rt: rt, db: db, logs: logs, runs: &atomic.Int32{}}
	for _, s := range []struct{ id, name string }{{"stk-hook", "infra"}, {"stk-off", "dormant"}} {
		made, err := db.CreateStack(t.Context(), store.StackRecord{ID: s.id, Name: s.name, Scope: store.StackScopeInstall, DriftPolicy: "warn", CreatedBy: "t"}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if s.id == "stk-hook" {
			env.stackID = made.ID
		}
		cfg := store.StackSyncConfig{RepoURL: "https://git.example.invalid/org/infra.git", Branch: "main", Path: "stacks", Enabled: s.id == "stk-hook", WebhookOnly: true}
		if err := db.PutStackSyncConfig(t.Context(), made.ID, cfg, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	rt.stackSyncState().syncer.Fetch = func(context.Context, store.StackSyncRecord) (string, string, func(), error) {
		env.runs.Add(1)
		return "", "", nil, errors.New("offline in tests")
	}
	admin := storeUserWithAbilitiesForTest(t, db, "root@example.test", []string{AbilityRoot})
	env.root = sessionCookieForTest(t, rt, admin.ID)
	env.secret = env.rotate(t, "infra")
	env.rotate(t, "dormant")
	return env
}

func (e *syncHookEnv) rotate(t *testing.T, stack string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, authedRequest(t, e.root, http.MethodPost, "/api/v1/stacks/"+stack+"/sync/webhook-secret/rotate", ""))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("rotate = %d cache %q %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
	}
	var out stackSyncWebhookSecretOutput
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Secret) != 64 {
		t.Fatalf("secret length = %d, want 64 hex chars", len(out.Secret))
	}
	return out.Secret
}

func stackPushBody(ref, after string) string {
	return `{"ref":"` + ref + `","after":"` + after + `","before":"x","commits":[{"message":"CANARY-payload-msg"}]}`
}

func stackHookSign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (e *syncHookEnv) deliver(t *testing.T, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (e *syncHookEnv) github(t *testing.T, stack, secret, body string) *httptest.ResponseRecorder {
	return e.deliver(t, "/api/v1/hooks/stacks/"+stack+"/sync", body, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": stackHookSign(secret, body)})
}

func (e *syncHookEnv) settle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for e.rt.stackSyncState().syncer.Busy(e.stackID) {
		if time.Now().After(deadline) {
			t.Fatal("sync never finished")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStackSyncWebhookGitHubTriggersOneRun(t *testing.T) {
	e := newSyncHookEnv(t)
	rec := e.github(t, "infra", e.secret, stackPushBody("refs/heads/main", syncHookSHA))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"status":"queued"`) {
		t.Fatalf("valid delivery = %d %s", rec.Code, rec.Body.String())
	}
	e.settle(t)
	if n := e.runs.Load(); n != 1 {
		t.Fatalf("runs = %d, want 1", n)
	}
	get := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(get, authedRequest(t, e.root, http.MethodGet, "/api/v1/stacks/infra/sync", ""))
	sig := stackHookSign(e.secret, stackPushBody("refs/heads/main", syncHookSHA))
	for where, text := range map[string]string{"status": get.Body.String(), "logs": e.logs.String()} {
		for _, canary := range []string{e.secret, sig, "CANARY-payload-msg"} {
			if strings.Contains(text, canary) {
				t.Fatalf("%s leaks %q", where, canary)
			}
		}
	}
}

func TestStackSyncWebhookRefusals(t *testing.T) {
	e := newSyncHookEnv(t)
	main := stackPushBody("refs/heads/main", syncHookSHA)
	cases := []struct {
		name string
		rec  *httptest.ResponseRecorder
		code int
		want string
	}{
		{"bad signature", e.github(t, "infra", "wrong-secret", main), http.StatusNotFound, "not found"},
		{"sha1 only", e.deliver(t, "/api/v1/hooks/stacks/infra/sync", main, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature": stackHookSHA1(e.secret, main)}), http.StatusNotFound, "not found"},
		{"wrong branch", e.github(t, "infra", e.secret, stackPushBody("refs/heads/feature", syncHookSHA)), http.StatusAccepted, `"ignored":"branch"`},
		{"tag push", e.github(t, "infra", e.secret, stackPushBody("refs/tags/v1.0.0", syncHookSHA)), http.StatusAccepted, `"ignored":"tag"`},
		{"deleted branch", e.github(t, "infra", e.secret, stackPushBody("refs/heads/main", strings.Repeat("0", 40))), http.StatusAccepted, `"ignored":"deleted"`},
		{"form encoded", e.deliver(t, "/api/v1/hooks/stacks/infra/sync", main, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}), http.StatusUnsupportedMediaType, "application/json"},
		{"gitlab wrong token", e.deliver(t, "/api/v1/hooks/stacks/infra/sync", main, map[string]string{"X-Gitlab-Event": "Push Hook", "X-Gitlab-Token": "nope"}), http.StatusNotFound, "not found"},
	}
	for _, c := range cases {
		if c.rec.Code != c.code || !strings.Contains(c.rec.Body.String(), c.want) {
			t.Errorf("%s = %d %s, want %d %s", c.name, c.rec.Code, c.rec.Body.String(), c.code, c.want)
		}
	}
	e.settle(t)
	if n := e.runs.Load(); n != 0 {
		t.Fatalf("refused deliveries ran %d syncs", n)
	}
}

func TestStackSyncWebhookGitLabToken(t *testing.T) {
	e := newSyncHookEnv(t)
	rec := e.deliver(t, "/api/v1/hooks/stacks/infra/refresh", stackPushBody("refs/heads/main", syncHookSHA), map[string]string{"X-Gitlab-Event": "Push Hook", "X-Gitlab-Token": e.secret})
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"verb":"refresh"`) {
		t.Fatalf("gitlab delivery = %d %s", rec.Code, rec.Body.String())
	}
	e.settle(t)
	if e.runs.Load() != 1 {
		t.Fatalf("runs = %d, want 1", e.runs.Load())
	}
}

// countingBody fails the test if the handler reads a body it should refuse first.
type countingBody struct{ reads atomic.Int32 }

func (c *countingBody) Read([]byte) (int, error) { c.reads.Add(1); return 0, io.EOF }
func (c *countingBody) Close() error             { return nil }

func TestStackSyncWebhookOversizeRefusedBeforeRead(t *testing.T) {
	e := newSyncHookEnv(t)
	body := &countingBody{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/stacks/infra/sync", nil)
	req.Body, req.ContentLength = body, 65<<10
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || body.reads.Load() != 0 {
		t.Fatalf("oversize = %d after %d reads, want 413 and none", rec.Code, body.reads.Load())
	}
	big := `{"ref":"refs/heads/main","pad":"` + strings.Repeat("a", 65<<10) + `"}`
	chunked := httptest.NewRequest(http.MethodPost, "/api/v1/hooks/stacks/infra/sync", strings.NewReader(big))
	chunked.ContentLength = -1
	chunked.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, chunked)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("chunked oversize = %d, want 413", rec.Code)
	}
}

func TestStackSyncWebhookUniformNotFound(t *testing.T) {
	e := newSyncHookEnv(t)
	main := stackPushBody("refs/heads/main", syncHookSHA)
	unknown := e.github(t, "nosuchstack", e.secret, main)
	off := e.github(t, "dormant", e.secret, main)
	bad := e.github(t, "infra", "wrong-secret", main)
	norm := func(r *httptest.ResponseRecorder) string {
		h := r.Header().Clone()
		h.Del("Date")
		h.Del(requestIDHeader)
		var body map[string]any
		_ = json.Unmarshal(r.Body.Bytes(), &body)
		delete(body, "request_id")
		raw, _ := json.Marshal(map[string]any{"code": r.Code, "h": h, "b": body})
		return string(raw)
	}
	if norm(unknown) != norm(off) || norm(off) != norm(bad) || unknown.Code != http.StatusNotFound {
		t.Fatalf("responses differ:\nunknown %s\noff     %s\nbad     %s", norm(unknown), norm(off), norm(bad))
	}
}

func TestStackSyncWebhookSecretNeverReadBack(t *testing.T) {
	e := newSyncHookEnv(t)
	second := e.rotate(t, "infra")
	if second == e.secret {
		t.Fatal("rotation returned the same secret")
	}
	get := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(get, authedRequest(t, e.root, http.MethodGet, "/api/v1/stacks/infra/sync", ""))
	if strings.Contains(get.Body.String(), second) || strings.Contains(get.Body.String(), e.secret) {
		t.Fatal("GET sync returns the webhook secret")
	}
	if rec := e.github(t, "infra", e.secret, stackPushBody("refs/heads/main", syncHookSHA)); rec.Code != http.StatusNotFound {
		t.Fatalf("old secret after rotation = %d, want 404", rec.Code)
	}
}

func stackHookSHA1(secret, body string) string {
	m := hmac.New(sha1.New, []byte(secret))
	m.Write([]byte(body))
	return "sha1=" + hex.EncodeToString(m.Sum(nil))
}
