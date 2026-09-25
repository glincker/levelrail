package models

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestMatchRoute(t *testing.T) {
	tests := []struct {
		name, engine, path, method string
		want                       routeMatch
	}{
		{"chat", EngineOllama, "/v1/chat/completions", "POST", routeAllowed},
		{"completions", EngineLlamaCpp, "/v1/completions", "POST", routeAllowed},
		{"embeddings", EngineVLLM, "/v1/embeddings", "POST", routeAllowed},
		{"models list", EngineOllama, "/v1/models", "GET", routeAllowed},
		{"model detail", EngineOllama, "/v1/models/llama3.1:8b", "GET", routeAllowed},
		{"model detail with slash id", EngineVLLM, "/v1/models/meta-llama/Llama-3.1-8B", "GET", routeAllowed},
		{"responses ollama", EngineOllama, "/v1/responses", "POST", routeAllowed},
		{"responses llamacpp", EngineLlamaCpp, "/v1/responses", "POST", routeAllowed},
		{"transcriptions vllm", EngineVLLM, "/v1/audio/transcriptions", "POST", routeAllowed},
		{"transcriptions ollama not served", EngineOllama, "/v1/audio/transcriptions", "POST", routeNotFound},
		{"transcriptions llamacpp not served", EngineLlamaCpp, "/v1/audio/transcriptions", "POST", routeNotFound},
		{"vllm lora load", EngineVLLM, "/v1/load_lora_adapter", "POST", routeNotFound},
		{"vllm lora unload", EngineVLLM, "/v1/unload_lora_adapter", "POST", routeNotFound},
		{"vllm chat batch", EngineVLLM, "/v1/chat/completions/batch", "POST", routeNotFound},
		{"vllm render", EngineVLLM, "/v1/chat/completions/render", "POST", routeNotFound},
		{"llamacpp completion control", EngineLlamaCpp, "/v1/chat/completions/control", "POST", routeNotFound},
		{"llamacpp anthropic messages", EngineLlamaCpp, "/v1/messages", "POST", routeNotFound},
		{"vllm sleep", EngineVLLM, "/sleep", "POST", routeNotFound},
		{"ollama pull", EngineOllama, "/api/pull", "POST", routeNotFound},
		{"llamacpp props", EngineLlamaCpp, "/props", "POST", routeNotFound},
		{"trailing slash", EngineOllama, "/v1/chat/completions/", "POST", routeNotFound},
		{"models trailing slash is empty id", EngineOllama, "/v1/models/", "GET", routeNotFound},
		{"upper case v1", EngineOllama, "/V1/chat/completions", "POST", routeNotFound},
		{"mixed case route", EngineOllama, "/v1/Chat/Completions", "POST", routeNotFound},
		{"double slash", EngineOllama, "/v1//chat/completions", "POST", routeNotFound},
		{"backslash", EngineOllama, `/v1/models/a\..\b`, "GET", routeNotFound},
		{"dot dot", EngineVLLM, "/v1/models/../load_lora_adapter", "GET", routeNotFound},
		{"GET on chat", EngineOllama, "/v1/chat/completions", "GET", routeWrongMethod},
		{"POST on models", EngineOllama, "/v1/models", "POST", routeWrongMethod},
		{"DELETE on model detail", EngineOllama, "/v1/models/x", "DELETE", routeWrongMethod},
		{"HEAD on models", EngineOllama, "/v1/models", "HEAD", routeWrongMethod},
		{"unknown engine gets the common set", "mystery", "/v1/responses", "POST", routeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := matchRoute(tt.engine, tt.path, tt.method); got != tt.want {
				t.Errorf("matchRoute(%s, %q, %s) = %d, want %d", tt.engine, tt.path, tt.method, got, tt.want)
			}
		})
	}
}

type gwFixture struct {
	gw  *Gateway
	key string
}

func newFixture(t *testing.T, engine, dial string, lim GatewayLimits) gwFixture {
	t.Helper()
	key, hash, _, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	m := store.Model{Name: "chat", Engine: engine, Domain: "chat.example.com", APIKeyHash: hash, EndpointDial: dial}
	gw := NewGateway(listStore{[]store.Model{m}}, NewHostResolver("", nil), nil)
	gw.SetLimits(lim)
	return gwFixture{gw: gw, key: key}
}

func (f gwFixture) do(ctx context.Context, method, target string, body io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://chat.example.com"+target, body).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+f.key)
	rec := httptest.NewRecorder()
	f.gw.Handle(rec, req)
	return rec
}

func countingUpstream(t *testing.T, hits *atomic.Int32, lastBody *atomic.Value) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		if lastBody != nil {
			lastBody.Store(string(b))
		}
		_, _ = io.WriteString(w, "{}")
	}))
	t.Cleanup(srv.Close)
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestGateway_PathAllowlistNeverReachesEngine(t *testing.T) {
	var hits atomic.Int32
	f := newFixture(t, EngineVLLM, countingUpstream(t, &hits, nil), DefaultGatewayLimits())
	tests := []struct {
		name, method, target string
		want                 int
	}{
		{"lora load", "POST", "/v1/load_lora_adapter", 404},
		{"lora unload", "POST", "/v1/unload_lora_adapter", 404},
		{"encoded slash", "GET", "/v1/models/a%2Fb", 404},
		{"encoded slash lower", "GET", "/v1/models/a%2fb", 404},
		{"encoded backslash", "GET", "/v1/models/a%5Cb", 404},
		{"encoded dot dot", "GET", "/v1/models/%2e%2e/x", 404},
		{"trailing slash", "POST", "/v1/chat/completions/", 404},
		{"case variation", "POST", "/v1/Chat/completions", 404},
		{"wrong method", "GET", "/v1/chat/completions", 405},
		{"query string on allowed path", "POST", "/v1/chat/completions?x=/../api/pull", 200},
		{"query string does not unlock", "POST", "/v1/load_lora_adapter?x=/v1/chat/completions", 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := hits.Load()
			rec := f.do(context.Background(), tt.method, tt.target, strings.NewReader("{}"))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want != 200 && hits.Load() != before {
				t.Error("blocked request reached the engine")
			}
			if tt.want == 405 && rec.Header().Get("Allow") != "POST" {
				t.Errorf("Allow = %q, want POST", rec.Header().Get("Allow"))
			}
		})
	}
}

func TestGateway_BlockedPathsAnswerBeforeAuth(t *testing.T) {
	f := newFixture(t, EngineOllama, "127.0.0.1:1", DefaultGatewayLimits())
	req := httptest.NewRequest("POST", "http://chat.example.com/api/pull", nil)
	rec := httptest.NewRecorder()
	f.gw.Handle(rec, req)
	if rec.Code != 404 || rec.Header().Get("WWW-Authenticate") != "" {
		t.Errorf("status %d, WWW-Authenticate %q: an unknown path must not reveal auth details", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
}

func TestGateway_BodyLimits(t *testing.T) {
	lim := DefaultGatewayLimits()
	lim.MaxBodyBytes = 1024
	lim.MaxN = 4
	lim.MaxTokens = 100
	var hits atomic.Int32
	var last atomic.Value
	f := newFixture(t, EngineVLLM, countingUpstream(t, &hits, &last), lim)

	tests := []struct {
		name, path, body string
		chunked          bool
		hdr              map[string]string
		want             int
	}{
		{"ok", "/v1/chat/completions", `{"n":4,"max_tokens":100}`, false, nil, 200},
		{"empty body forwarded", "/v1/chat/completions", ``, false, nil, 200},
		{"n too high", "/v1/chat/completions", `{"n":5}`, false, nil, 400},
		{"best_of too high", "/v1/completions", `{"best_of":500}`, false, nil, 400},
		{"max_tokens too high", "/v1/chat/completions", `{"max_tokens":101}`, false, nil, 400},
		{"max_completion_tokens too high", "/v1/chat/completions", `{"max_completion_tokens":1000000}`, false, nil, 400},
		{"max_output_tokens too high", "/v1/responses", `{"max_output_tokens":101}`, false, nil, 400},
		{"huge number", "/v1/chat/completions", `{"max_tokens":1e999}`, false, nil, 400},
		{"unlimited sentinel", "/v1/chat/completions", `{"max_tokens":-1}`, false, nil, 400},
		{"null and strings left to engine", "/v1/chat/completions", `{"max_tokens":null,"n":"2"}`, false, nil, 200},
		{"not an object", "/v1/chat/completions", `[1]`, false, nil, 400},
		{"invalid json", "/v1/chat/completions", `{`, false, nil, 400},
		{"content type does not matter", "/v1/chat/completions", `{"n":99}`, false, map[string]string{"Content-Type": "text/plain"}, 400},
		{"declared length too large", "/v1/chat/completions", `{"x":"` + strings.Repeat("a", 2000) + `"}`, false, nil, 413},
		{"chunked too large", "/v1/chat/completions", `{"x":"` + strings.Repeat("a", 2000) + `"}`, true, nil, 413},
		{"multipart too large", "/v1/audio/transcriptions", strings.Repeat("a", 2000), true, map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, 413},
		{"multipart under cap not parsed", "/v1/audio/transcriptions", `--x`, false, map[string]string{"Content-Type": "multipart/form-data; boundary=x"}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := hits.Load()
			var body io.Reader = strings.NewReader(tt.body)
			if tt.chunked {
				body = io.MultiReader(strings.NewReader(tt.body))
			}
			req := httptest.NewRequest("POST", "http://chat.example.com"+tt.path, body)
			if tt.chunked {
				req.ContentLength = -1
			}
			req.Header.Set("Authorization", "Bearer "+f.key)
			for k, v := range tt.hdr {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			f.gw.Handle(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == 200 {
				if got, _ := last.Load().(string); got != tt.body {
					t.Errorf("engine saw body %q, want %q", got, tt.body)
				}
			} else if !strings.Contains(tt.name, "multipart") && hits.Load() != before {
				t.Error("rejected request reached the engine")
			}
			if tt.want != 200 && !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("body %q is not an OpenAI-style error", rec.Body.String())
			}
		})
	}
}

func TestGateway_LimitsDisabledWithZero(t *testing.T) {
	var hits atomic.Int32
	f := newFixture(t, EngineOllama, countingUpstream(t, &hits, nil), GatewayLimits{})
	rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader(`{"n":9999,"max_tokens":99999999}`))
	if rec.Code != 200 {
		t.Errorf("status = %d, want 200 with every limit disabled", rec.Code)
	}
}

func TestGateway_InflightCap(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()
	lim := DefaultGatewayLimits()
	lim.MaxInflight = 1
	lim.RetryAfter = 7 * time.Second
	f := newFixture(t, EngineOllama, strings.TrimPrefix(srv.URL, "http://"), lim)

	first := make(chan int, 1)
	go func() {
		first <- f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}")).Code
	}()
	<-entered
	rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}"))
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "7" {
		t.Fatalf("status %d Retry-After %q, want 429 and 7", rec.Code, rec.Header().Get("Retry-After"))
	}
	close(release)
	if code := <-first; code != 200 {
		t.Errorf("first request = %d, want 200", code)
	}
	if rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}")); rec.Code != 200 {
		t.Errorf("slot not released, status %d", rec.Code)
	}
}

func TestGateway_InflightCapIsPerModel(t *testing.T) {
	var f inflight
	relA, okA := f.acquire("a", 1)
	if !okA {
		t.Fatal("first acquire failed")
	}
	defer relA()
	if _, ok := f.acquire("a", 1); ok {
		t.Error("second acquire on the same model should fail")
	}
	relB, ok := f.acquire("b", 1)
	if !ok {
		t.Fatal("another model must have its own slots")
	}
	relB()
}

func TestGateway_HeaderTimeoutIs504(t *testing.T) {
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer srv.Close()
	defer close(done)
	lim := DefaultGatewayLimits()
	lim.HeaderTimeout = 150 * time.Millisecond
	f := newFixture(t, EngineOllama, strings.TrimPrefix(srv.URL, "http://"), lim)
	start := time.Now()
	rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}"))
	if rec.Code != http.StatusGatewayTimeout || !strings.Contains(rec.Body.String(), "gateway_timeout") {
		t.Fatalf("status = %d body %s, want 504", rec.Code, rec.Body.String())
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout took far longer than configured")
	}
}

func TestGateway_StreamIdleTimeout(t *testing.T) {
	upstreamDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(upstreamDone)
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	lim := DefaultGatewayLimits()
	lim.IdleTimeout = 150 * time.Millisecond
	f := newFixture(t, EngineOllama, strings.TrimPrefix(srv.URL, "http://"), lim)
	rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}"))
	if !strings.Contains(rec.Body.String(), "data: one") {
		t.Errorf("first chunk lost: %q", rec.Body.String())
	}
	select {
	case <-upstreamDone:
	case <-time.After(3 * time.Second):
		t.Fatal("idle stream was not cancelled upstream")
	}
}

func TestGateway_LongStreamIsNotKilled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for range 10 {
			_, _ = io.WriteString(w, "data: tok\n\n")
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	}))
	defer srv.Close()
	lim := DefaultGatewayLimits()
	lim.IdleTimeout = 250 * time.Millisecond
	f := newFixture(t, EngineOllama, strings.TrimPrefix(srv.URL, "http://"), lim)
	rec := f.do(context.Background(), "POST", "/v1/chat/completions", strings.NewReader("{}"))
	if got := strings.Count(rec.Body.String(), "data: tok"); got != 10 {
		t.Errorf("received %d of 10 chunks: total duration must not be limited, only idleness", got)
	}
}

func TestGateway_ClientCancelReachesEngine(t *testing.T) {
	entered := make(chan struct{})
	upstreamDone := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		first := false
		once.Do(func() { first = true })
		if !first {
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
		close(upstreamDone)
	}))
	defer srv.Close()
	lim := DefaultGatewayLimits()
	lim.HeaderTimeout = time.Minute
	f := newFixture(t, EngineOllama, strings.TrimPrefix(srv.URL, "http://"), lim)

	ctx, cancel := context.WithCancel(context.Background())
	handled := make(chan struct{})
	go func() {
		f.do(ctx, "POST", "/v1/chat/completions", bytes.NewReader([]byte("{}")))
		close(handled)
	}()
	<-entered
	cancel()
	for _, ch := range []chan struct{}{upstreamDone, handled} {
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal("client cancellation did not unwind the proxy and the engine request")
		}
	}
	if rec := f.do(context.Background(), "GET", "/v1/models", nil); rec.Code == http.StatusTooManyRequests {
		t.Error("cancelled request leaked its in-flight slot")
	}
}

func TestLoadGatewayLimits(t *testing.T) {
	t.Setenv(envGatewayMaxBody, "1000")
	t.Setenv(envGatewayMaxN, "2")
	t.Setenv(envGatewayMaxGenLen, "not-a-number")
	t.Setenv(envGatewayIdleTimeout, "45s")
	l := LoadGatewayLimits()
	if l.MaxBodyBytes != 1000 || l.MaxN != 2 || l.IdleTimeout != 45*time.Second {
		t.Errorf("env not applied: %+v", l)
	}
	if l.MaxTokens != DefaultGatewayLimits().MaxTokens {
		t.Errorf("invalid value should fall back to the default, got %d", l.MaxTokens)
	}
	if s := l.Summary(); s.StreamIdleTimeoutS != 45 || s.MaxN != 2 {
		t.Errorf("summary = %+v", s)
	}
}
