package models

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type keyedStore struct {
	models []store.Model
	keys   []store.ModelKey
}

func (k keyedStore) ListModels(context.Context) ([]store.Model, error) { return k.models, nil }
func (k keyedStore) ListActiveModelKeys(context.Context) ([]store.ModelKey, error) {
	return k.keys, nil
}

func mkKey(t *testing.T, id string, mod func(*store.ModelKey)) (string, store.ModelKey) {
	t.Helper()
	plain, hash, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	k := store.ModelKey{ID: id, ModelName: "chat", Name: id, KeyHash: hash, KeyPrefix: prefix, CreatedAt: time.Now()}
	if mod != nil {
		mod(&k)
	}
	return plain, k
}

func gatewayFor(t *testing.T, upstream http.Handler, keys ...store.ModelKey) *Gateway {
	t.Helper()
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)
	m := store.Model{Name: "chat", Domain: "chat.example.com", EndpointDial: strings.TrimPrefix(srv.URL, "http://")}
	return NewGateway(keyedStore{models: []store.Model{m}, keys: keys}, NewHostResolver("", nil), nil)
}

func call(gw *Gateway, key, path, body string) *httptest.ResponseRecorder {
	method := http.MethodPost
	if path == "/v1/models" {
		method = http.MethodGet
	}
	req := httptest.NewRequest(method, "http://chat.example.com"+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.Handle(rec, req)
	return rec
}

func okUpstream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[],"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10}}`)
	})
}

func TestGateway_MultipleKeysAndStates(t *testing.T) {
	past, future := time.Now().Add(-time.Minute), time.Now().Add(time.Hour)
	kA, a := mkKey(t, "a", nil)
	kB, b := mkKey(t, "b", nil)
	kRev, rev := mkKey(t, "rev", func(k *store.ModelKey) { k.RevokedAt = &past })
	kExp, exp := mkKey(t, "exp", func(k *store.ModelKey) { k.ExpiresAt = &past })
	kGrace, grace := mkKey(t, "grace", func(k *store.ModelKey) { k.ExpiresAt = &future })
	gw := gatewayFor(t, okUpstream(), a, b, rev, exp, grace)
	for name, tc := range map[string]struct {
		key  string
		want int
	}{"a": {kA, 200}, "b": {kB, 200}, "revoked": {kRev, 401}, "expired": {kExp, 401}, "in grace": {kGrace, 200}, "unknown": {"lr-unknown", 401}, "empty": {"", 401}} {
		if got := call(gw, tc.key, "/v1/chat/completions", `{"model":"m"}`).Code; got != tc.want {
			t.Errorf("%s: status = %d, want %d", name, got, tc.want)
		}
	}
}

func TestGateway_AllowLists(t *testing.T) {
	key, k := mkKey(t, "a", func(k *store.ModelKey) {
		k.AllowPaths = []string{"/v1/chat/completions", "/v1/models/"}
		k.AllowModels = []string{"llama"}
	})
	gw := gatewayFor(t, okUpstream(), k)
	tests := []struct {
		name, path, body string
		want             int
	}{
		{"allowed", "/v1/chat/completions", `{"model":"llama"}`, 200},
		{"other model", "/v1/chat/completions", `{"model":"other"}`, 403},
		{"no model in body", "/v1/chat/completions", `{"messages":[]}`, 403},
		{"other path", "/v1/embeddings", `{"model":"llama"}`, 403},
		{"listing not allowed by path list", "/v1/models", "", 403},
	}
	for _, tt := range tests {
		if got := call(gw, key, tt.path, tt.body).Code; got != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGateway_RPMLimit(t *testing.T) {
	key, k := mkKey(t, "a", func(k *store.ModelKey) { k.RPM = 2 })
	other, o := mkKey(t, "b", nil)
	gw := gatewayFor(t, okUpstream(), k, o)
	for i := range 2 {
		if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != 200 {
			t.Fatalf("request %d = %d", i, got)
		}
	}
	rec := call(gw, key, "/v1/chat/completions", `{}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("third request = %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if strings.Contains(rec.Body.String(), "rpm") || strings.Contains(rec.Body.String(), "tokens") {
		t.Errorf("error leaks which limit tripped: %s", rec.Body.String())
	}
	if got := call(gw, other, "/v1/chat/completions", `{}`).Code; got != 200 {
		t.Errorf("another key was limited: %d", got)
	}
}

func TestGateway_ParallelLimit(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	up := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = io.WriteString(w, `{}`)
	})
	key, k := mkKey(t, "a", func(k *store.ModelKey) { k.MaxParallel = 1 })
	gw := gatewayFor(t, up, k)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		call(gw, key, "/v1/chat/completions", `{}`)
	}()
	<-entered
	if got := gw.InFlight()["a"]; got != 1 {
		t.Errorf("in flight = %d, want 1", got)
	}
	if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != http.StatusTooManyRequests {
		t.Errorf("second parallel request = %d, want 429", got)
	}
	close(release)
	wg.Wait()
	if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != 200 {
		t.Errorf("after release = %d, want 200", got)
	}
}

func TestGateway_TPMSoftLimit(t *testing.T) {
	key, k := mkKey(t, "a", func(k *store.ModelKey) { k.TPM = 10 })
	gw := gatewayFor(t, okUpstream(), k)
	if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != 200 {
		t.Fatalf("first = %d", got)
	}
	if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != http.StatusTooManyRequests {
		t.Errorf("after 10 metered tokens = %d, want 429", got)
	}
}

func TestKeyLimiter_DailyTokenBudget(t *testing.T) {
	tests := []struct {
		name     string
		tpd      int
		used     int64
		elapsed  time.Duration
		wantOK   bool
		wantWait time.Duration
	}{
		{"under budget", 100, 99, time.Hour, true, 0},
		{"at budget blocks", 100, 100, time.Hour, false, 23 * time.Hour},
		{"window resets after a day", 100, 100, 25 * time.Hour, true, 0},
		{"unlimited", 0, 1 << 40, time.Hour, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var l keyLimiter
			start := time.Now()
			k := store.ModelKey{ID: "k", TPD: tc.tpd}
			rel, _, ok := l.acquire(k, start)
			if !ok {
				t.Fatal("first acquire failed")
			}
			rel()
			l.addTokens("k", tc.used, start)
			rel, wait, ok := l.acquire(k, start.Add(tc.elapsed))
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				rel()
				return
			}
			if wait != tc.wantWait {
				t.Errorf("wait = %v, want %v", wait, tc.wantWait)
			}
		})
	}
}

func TestGateway_TPDLimit(t *testing.T) {
	key, k := mkKey(t, "a", func(k *store.ModelKey) { k.TPD = 10 })
	gw := gatewayFor(t, okUpstream(), k)
	if got := call(gw, key, "/v1/chat/completions", `{}`).Code; got != 200 {
		t.Fatalf("first = %d", got)
	}
	rec := call(gw, key, "/v1/chat/completions", `{}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Errorf("after the daily budget = %d, retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestGateway_InvalidateDropsRevokedKey(t *testing.T) {
	key, k := mkKey(t, "a", nil)
	st := &mutableKeys{keyedStore: keyedStore{models: []store.Model{{Name: "chat", Domain: "chat.example.com", EndpointDial: "127.0.0.1:1"}}, keys: []store.ModelKey{k}}}
	gw := NewGateway(st, NewHostResolver("", nil), nil)
	if got := call(gw, key, "/v1/models", "").Code; got == http.StatusUnauthorized {
		t.Fatal("key rejected before revoke")
	}
	st.keys = nil
	gw.Invalidate()
	if got := call(gw, key, "/v1/models", "").Code; got != http.StatusUnauthorized {
		t.Errorf("after revoke and invalidate = %d, want 401", got)
	}
}

type mutableKeys struct{ keyedStore }

type fakeUsageStore struct {
	mu      sync.Mutex
	rows    []store.ModelUsage
	batches int
	fail    bool
	touched map[string]time.Time
	pruned  int
}

func (f *fakeUsageStore) AddModelUsage(_ context.Context, rows []store.ModelUsage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("db down")
	}
	f.batches++
	f.rows = append(f.rows, rows...)
	return nil
}

func (f *fakeUsageStore) TouchModelKeys(_ context.Context, used map[string]time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touched = used
	return nil
}

func (f *fakeUsageStore) PruneModelUsage(context.Context, time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned++
	return 0, nil
}

func TestGateway_MeteringJSONAndFlush(t *testing.T) {
	key, k := mkKey(t, "a", nil)
	gw := gatewayFor(t, okUpstream(), k)
	call(gw, key, "/v1/chat/completions", `{}`)
	call(gw, key, "/v1/chat/completions", `{}`)
	call(gw, "lr-bad", "/v1/chat/completions", `{}`)
	fs := &fakeUsageStore{}
	gw.meter.flush(context.Background(), fs)
	if len(fs.rows) != 1 {
		t.Fatalf("rows = %+v", fs.rows)
	}
	u := fs.rows[0]
	if u.KeyID != "a" || u.Requests != 2 || u.Status2xx != 2 || u.InputTokens != 14 || u.OutputTokens != 6 || u.UsageRequests != 2 || u.BytesOut == 0 {
		t.Errorf("usage = %+v", u)
	}
	if fs.touched["a"].IsZero() {
		t.Error("last used not recorded")
	}
}

func TestGateway_MeteringSSEUsageAndNoUsage(t *testing.T) {
	sse := func(final string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}],\"usage\":null}\n\n")
			_, _ = io.WriteString(w, final)
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		})
	}
	key, k := mkKey(t, "a", nil)
	withUsage := gatewayFor(t, sse("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":9}}\n\n"), k)
	call(withUsage, key, "/v1/chat/completions", `{"stream":true}`)
	fs := &fakeUsageStore{}
	withUsage.meter.flush(context.Background(), fs)
	if u := fs.rows[0]; u.InputTokens != 5 || u.OutputTokens != 9 || u.UsageRequests != 1 || u.TTFTCount != 1 {
		t.Errorf("stream with usage = %+v", u)
	}

	without := gatewayFor(t, sse(""), k)
	call(without, key, "/v1/chat/completions", `{"stream":true}`)
	fs = &fakeUsageStore{}
	without.meter.flush(context.Background(), fs)
	if u := fs.rows[0]; u.Requests != 1 || u.UsageRequests != 0 || u.InputTokens != 0 || u.OutputTokens != 0 {
		t.Errorf("stream without usage must count the request only: %+v", u)
	}
}

func TestMeter_BoundedBatchesRequeueAndCap(t *testing.T) {
	m := newMeter(MeterConfig{BatchSize: 2, MaxBuffered: 5, Retention: time.Hour}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Now()
	for i := range 5 {
		m.record(observation{model: "m", keyID: string(rune('a' + i)), at: now, status: 200})
	}
	m.record(observation{model: "m", keyID: "overflow", at: now, status: 200})
	if m.dropped != 1 {
		t.Errorf("dropped = %d, want 1", m.dropped)
	}
	fs := &fakeUsageStore{fail: true}
	m.flush(context.Background(), fs)
	if len(m.agg) != 5 {
		t.Fatalf("failed batches must be requeued, buffered = %d", len(m.agg))
	}
	fs.fail = false
	m.flush(context.Background(), fs)
	if len(fs.rows) != 5 || fs.batches != 3 {
		t.Errorf("rows = %d batches = %d, want 5 rows in 3 batches", len(fs.rows), fs.batches)
	}
	if fs.pruned != 1 {
		t.Errorf("pruned = %d, want 1", fs.pruned)
	}
}

func TestExtractUsage(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   tokenUsage
		wantOK bool
	}{
		{"chat", `{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2}}`, tokenUsage{1, 2}, true},
		{"responses api", `{"response":{"usage":{"input_tokens":3,"output_tokens":4}}}`, tokenUsage{3, 4}, true},
		{"embeddings", `{"data":[],"usage":{"prompt_tokens":8,"total_tokens":8}}`, tokenUsage{8, 0}, true},
		{"null usage", `data: {"usage":null}`, tokenUsage{}, false},
		{"earlier object then null", `{"usage":{"prompt_tokens":1,"completion_tokens":1}} {"usage":null}`, tokenUsage{1, 1}, true},
		{"escaped in content", `{"content":"say \"usage\":{\"prompt_tokens\":99}"}`, tokenUsage{}, false},
		{"none", `{"ok":true}`, tokenUsage{}, false},
		{"spaced", `{"usage" : { "prompt_tokens" : 4 , "completion_tokens" : 5 }}`, tokenUsage{4, 5}, true},
	}
	for _, tt := range tests {
		got, ok := extractUsage([]byte(tt.in))
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("%s: got %+v %v, want %+v %v", tt.name, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestKeyLimiterRefill(t *testing.T) {
	var l keyLimiter
	k := store.ModelKey{ID: "a", RPM: 60}
	now := time.Now()
	for range 60 {
		rel, _, ok := l.acquire(k, now)
		if !ok {
			t.Fatal("burst denied")
		}
		rel()
	}
	if _, wait, ok := l.acquire(k, now); ok || wait <= 0 {
		t.Fatalf("expected denial with wait, got ok=%v wait=%v", ok, wait)
	}
	if _, _, ok := l.acquire(k, now.Add(1100*time.Millisecond)); !ok {
		t.Error("token not refilled after 1.1s at 60 rpm")
	}
}

func TestValidateAllowPaths(t *testing.T) {
	if bad, ok := ValidateAllowPaths(EngineOllama, []string{"/v1/chat/completions", "/v1/models/"}); !ok {
		t.Errorf("valid paths rejected: %q", bad)
	}
	if bad, ok := ValidateAllowPaths(EngineOllama, []string{"/api/pull"}); ok || bad != "/api/pull" {
		t.Errorf("engine admin path accepted: %q %v", bad, ok)
	}
	if _, ok := ValidateAllowPaths(EngineOllama, []string{"/v1/audio/transcriptions"}); ok {
		t.Error("vllm-only path accepted for ollama")
	}
}
