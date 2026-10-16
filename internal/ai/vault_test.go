package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type memSecrets struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemSecrets() *memSecrets { return &memSecrets{m: map[string]string{}} }

func (s *memSecrets) SetValue(_ context.Context, svc, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[svc+"/"+k] = v
	return nil
}

func (s *memSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[svc+"/"+k]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (s *memSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[svc+"/"+k]
	return ok, nil
}

func (s *memSecrets) DeleteAll(_ context.Context, svc string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if strings.HasPrefix(k, svc+"/") {
			delete(s.m, k)
		}
	}
	return nil
}

// fakeUpstream answers every provider path: /models validates the key,
// everything else is a minimal chat completion.
func fakeUpstream(t *testing.T, goodKey string, modelsBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if goodKey != "" && r.Header.Get("Authorization") != "Bearer "+goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			// Echo the presented key like real providers do, to prove it is scrubbed.
			msg, _ := json.Marshal(map[string]map[string]string{"error": {"message": "Incorrect API key provided: " + strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")}})
			_, _ = w.Write(msg) //nolint:gosec // test server, JSON-encoded body
			return
		}
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(modelsBody))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testVault(t *testing.T, logBuf *bytes.Buffer) (*Vault, *store.DB, *memSecrets) {
	t.Helper()
	db := openTestDB(t)
	sec := newMemSecrets()
	h := slog.DiscardHandler
	if logBuf != nil {
		h = slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	publicOnly := func(_ context.Context, _ string) ([]netip.Addr, error) { return nil, errors.New("no dns") }
	v := NewVault(db, sec, slog.New(h),
		WithHostResolver(publicOnly),
		WithBackendFactory(func(info ProviderInfo, cfg AdapterConfig) Backend {
			cfg.HTTP = http.DefaultClient
			return DefaultBackendFactory(info, cfg)
		}),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 1}),
	)
	return v, db, sec
}

func TestVault_ValidationFailureDoesNotStoreKey(t *testing.T) {
	var logs bytes.Buffer
	v, db, sec := testVault(t, &logs)
	srv := fakeUpstream(t, "good-key-123456789", `{"data":[{"id":"llama3"}]}`)
	const badKey = "sk-BADKEY-0123456789abcdef"

	_, _, err := v.Set(context.Background(), "ollama", SetInput{APIKey: badKey, BaseURL: srv.URL + "/v1"}, true)
	var se *SaveError
	if !errors.As(err, &se) || se.Code != "invalid_key" {
		t.Fatalf("err = %v, want SaveError invalid_key", err)
	}
	if has, _ := sec.Exists(context.Background(), store.AIProviderSecretsKey("ollama"), SecretsAPIKeyEnvKey); has {
		t.Error("an invalid key was stored")
	}
	if _, gerr := db.GetAIProviderConfig(context.Background(), "ollama"); !errors.Is(gerr, store.ErrAIProviderConfigNotFound) {
		t.Errorf("provider row exists after failed validation: %v", gerr)
	}
	for name, text := range map[string]string{"error": err.Error(), "se.Message": se.Message, "se.Fix": se.Fix, "logs": logs.String()} {
		if strings.Contains(text, "BADKEY") {
			t.Errorf("%s leaks the key: %q", name, text)
		}
	}
}

func TestVault_SetSuccessKeyNeverReturnedOrLogged(t *testing.T) {
	var logs bytes.Buffer
	v, _, sec := testVault(t, &logs)
	const key = "good-key-123456789"
	srv := fakeUpstream(t, key, `{"data":[{"id":"llama3"}]}`)

	view, res, err := v.Set(context.Background(), "ollama", SetInput{APIKey: key, BaseURL: srv.URL + "/v1", DefaultModel: "llama3"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.HasKey || view.Status != store.AIProviderStatusOK || view.DefaultModel != "llama3" || !res.PrivateBaseURL {
		t.Errorf("view = %+v res = %+v", view, res)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), key) || strings.Contains(logs.String(), key) {
		t.Errorf("key leaked in view %s or logs %s", raw, logs.String())
	}
	if !strings.Contains(logs.String(), "provider=ollama") {
		t.Errorf("log lines should carry the provider id: %s", logs.String())
	}
	if got, _ := sec.Resolve(context.Background(), store.AIProviderSecretsKey("ollama"), SecretsAPIKeyEnvKey); got != key {
		t.Error("key not stored")
	}
	// A later base-url-less default model change must not re-validate or lose the key.
	if view, _, err = v.Set(context.Background(), "ollama", SetInput{DefaultModel: "llama3.1"}, true); err != nil || !view.HasKey || view.DefaultModel != "llama3.1" {
		t.Errorf("model-only update: view=%+v err=%v", view, err)
	}
}

func TestVault_PrivateURLPolicyOnSet(t *testing.T) {
	v, _, _ := testVault(t, nil)
	srv := fakeUpstream(t, "", `{"data":[]}`)
	cases := []struct {
		name, id string
		root     bool
		wantCode string
	}{
		{"ollama loopback as root", "ollama", true, ""},
		{"ollama loopback non-root", "ollama", false, "blocked_base_url"},
		{"custom loopback as root", "openai_compatible", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := v.Set(context.Background(), tc.id, SetInput{BaseURL: srv.URL + "/v1"}, tc.root)
			var se *SaveError
			switch {
			case tc.wantCode == "" && err != nil:
				t.Fatalf("err = %v", err)
			case tc.wantCode != "" && (!errors.As(err, &se) || se.Code != tc.wantCode):
				t.Fatalf("err = %v, want %s", err, tc.wantCode)
			}
		})
	}
	if _, _, err := v.Set(context.Background(), "openai", SetInput{BaseURL: "https://evil.example.com"}, true); err == nil {
		t.Error("fixed-URL provider accepted a base_url")
	}
	if _, _, err := v.Set(context.Background(), "nope", SetInput{}, true); !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("unknown provider err = %v", err)
	}
	if _, _, err := v.Set(context.Background(), "openai", SetInput{BaseURL: ""}, true); err != nil && !errors.As(err, new(*SaveError)) {
		t.Errorf("err = %v", err)
	}
}

func TestVault_ModelsCacheEmptyListAndRemove(t *testing.T) {
	v, db, _ := testVault(t, nil)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			calls++
			_, _ = w.Write([]byte(`{"data":[]}`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if _, _, err := v.Set(ctx, "vllm", SetInput{BaseURL: srv.URL + "/v1"}, true); err != nil {
		t.Fatal(err)
	}
	callsAfterSet := calls
	for i := 0; i < 3; i++ {
		m, err := v.Models(ctx, "vllm")
		if err != nil || m == nil || len(m) != 0 {
			t.Fatalf("empty list: m=%v err=%v", m, err)
		}
	}
	if calls != callsAfterSet+1 {
		t.Errorf("model list fetched %d times, want cached after the first", calls-callsAfterSet)
	}

	if err := v.SetChoice(ctx, &store.AIModelRef{Provider: "vllm", Model: "any-typed-id"}, &[]store.AIModelRef{}); err != nil {
		t.Fatal(err)
	}
	if err := v.Remove(ctx, "vllm"); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetAIAssistantSettings(ctx)
	if s.Provider != "" || s.Model != "" {
		t.Errorf("removing the active provider left %+v", s)
	}
}

func TestVault_SetChoiceValidation(t *testing.T) {
	v, _, sec := testVault(t, nil)
	ctx := context.Background()
	_ = sec.SetValue(ctx, store.AIProviderSecretsKey("anthropic"), SecretsAPIKeyEnvKey, "k")
	_ = sec.SetValue(ctx, store.AIProviderSecretsKey("groq"), SecretsAPIKeyEnvKey, "k")
	a := store.AIModelRef{Provider: "anthropic", Model: "claude-sonnet-5-5"}
	g := store.AIModelRef{Provider: "groq", Model: "llama"}

	for name, tc := range map[string]struct {
		active   *store.AIModelRef
		fallback *[]store.AIModelRef
		code     string
	}{
		"ok":          {&a, &[]store.AIModelRef{g}, ""},
		"no key":      {&store.AIModelRef{Provider: "openai", Model: "x"}, nil, "key_required"},
		"unknown":     {&store.AIModelRef{Provider: "zzz", Model: "x"}, nil, "unknown_provider"},
		"empty model": {&store.AIModelRef{Provider: "groq"}, nil, "model_required"},
		"duplicate":   {nil, &[]store.AIModelRef{g, g}, "duplicate_fallback"},
		"too long":    {nil, &[]store.AIModelRef{g, g, g, g, g, g}, "fallback_too_long"},
	} {
		t.Run(name, func(t *testing.T) {
			err := v.SetChoice(ctx, tc.active, tc.fallback)
			var se *SaveError
			if tc.code == "" && err != nil || tc.code != "" && (!errors.As(err, &se) || se.Code != tc.code) {
				t.Errorf("err = %v, want code %q", err, tc.code)
			}
		})
	}
	s, _ := v.Settings(ctx)
	if !s.Configured || s.Active == nil || s.Active.Provider != "anthropic" || len(s.Fallback) != 1 {
		t.Errorf("settings = %+v", s)
	}
}

func TestVault_ChainEndToEndFallback(t *testing.T) {
	v, _, sec := testVault(t, nil)
	ctx := context.Background()
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down"}}`))
	}))
	defer limited.Close()
	good := fakeUpstream(t, "", `{"data":[]}`)
	if _, _, err := v.Set(ctx, "ollama", SetInput{BaseURL: limited.URL + "/v1"}, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.Set(ctx, "vllm", SetInput{BaseURL: good.URL + "/v1"}, true); err != nil {
		t.Fatal(err)
	}
	_ = sec
	if err := v.SetChoice(ctx, &store.AIModelRef{Provider: "ollama", Model: "m"}, &[]store.AIModelRef{{Provider: "vllm", Model: "m2"}}); err != nil {
		t.Fatal(err)
	}
	chain, err := v.Chain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var attempts []Attempt
	res, err := chain.Complete(WithAttemptObserver(ctx, func(a Attempt) { attempts = append(attempts, a) }), ChatRequest{Messages: []Message{userText("hi")}}, nil)
	if err != nil || res.Text != "hi" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(attempts) != 2 || attempts[0].ErrorClass != ClassRateLimited || attempts[1].Provider != "vllm" {
		t.Errorf("attempts = %+v", attempts)
	}
}

func TestVault_TestRecordsStatus(t *testing.T) {
	v, db, _ := testVault(t, nil)
	ctx := context.Background()
	good := fakeUpstream(t, "", `{"data":[]}`)
	if _, _, err := v.Set(ctx, "vllm", SetInput{BaseURL: good.URL + "/v1"}, true); err != nil {
		t.Fatal(err)
	}
	good.Close()
	res, err := v.Test(ctx, "vllm")
	if err != nil || res.OK || res.Status != store.AIProviderStatusUnreach {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	cfg, _ := db.GetAIProviderConfig(ctx, "vllm")
	if cfg.Status != store.AIProviderStatusUnreach || cfg.LastCheckedAt.IsZero() || time.Since(cfg.LastCheckedAt) > time.Minute {
		t.Errorf("cfg = %+v", cfg)
	}
}
