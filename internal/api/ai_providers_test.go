package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

type vaultSecrets struct{ m map[string]string }

func (s *vaultSecrets) SetValue(_ context.Context, svc, k, v string) error {
	s.m[svc+"/"+k] = v
	return nil
}
func (s *vaultSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	if v, ok := s.m[svc+"/"+k]; ok {
		return v, nil
	}
	return "", errors.New("missing")
}
func (s *vaultSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	_, ok := s.m[svc+"/"+k]
	return ok, nil
}
func (s *vaultSecrets) DeleteAll(_ context.Context, svc string) error {
	for k := range s.m {
		if strings.HasPrefix(k, svc+"/") {
			delete(s.m, k)
		}
	}
	return nil
}

func providerRouter(t *testing.T) (*Router, *store.DB, *vaultSecrets) {
	t.Helper()
	db := openTestDB(t)
	sec := &vaultSecrets{m: map[string]string{}}
	vault := ai.NewVault(db, sec, slog.New(slog.DiscardHandler),
		ai.WithBackendFactory(func(info ai.ProviderInfo, cfg ai.AdapterConfig) ai.Backend {
			cfg.HTTP = http.DefaultClient
			return ai.DefaultBackendFactory(info, cfg)
		}))
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithAIVault(vault), WithAIUsage(db), WithAIAssistantSecrets(sec)), db, sec
}

func upstream(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" && r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"llama3","context_length":8192}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func aiDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestAIProviders_Lifecycle(t *testing.T) {
	rt, db, sec := providerRouter(t)
	cookie := loginTestSession(t, rt, db)
	srv := upstream(t)
	const base = "/api/v1/settings/ai-assistant"

	rec := aiDo(t, rt, cookie, http.MethodGet, base+"/providers", "")
	var list []ai.ProviderView
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != 200 || len(list) != len(ai.Catalog()) {
		t.Fatalf("list: %d %v len=%d", rec.Code, err, len(list))
	}
	if list[0].ID != "anthropic" || list[0].HasKey || list[0].Status != "untested" || list[0].DefaultModel != "claude-sonnet-5-5" {
		t.Errorf("first provider = %+v", list[0])
	}

	rec = aiDo(t, rt, cookie, http.MethodPut, base+"/providers/vllm", `{"api_key":"bad-key","base_url":"`+srv.URL+`/v1"}`)
	var eb aiErrorBody
	_ = json.Unmarshal(rec.Body.Bytes(), &eb)
	if rec.Code != http.StatusUnprocessableEntity || eb.Code != "invalid_key" || eb.Error == "" || eb.Fix == "" || strings.Contains(rec.Body.String(), "bad-key") {
		t.Fatalf("invalid key: %d %s", rec.Code, rec.Body.String())
	}
	if len(sec.m) != 0 {
		t.Error("invalid key was stored")
	}

	rec = aiDo(t, rt, cookie, http.MethodPut, base+"/providers/vllm", `{"api_key":"good-key","base_url":"`+srv.URL+`/v1"}`)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "good-key") || !strings.Contains(rec.Body.String(), `"has_key":true`) {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}

	rec = aiDo(t, rt, cookie, http.MethodPost, base+"/providers/vllm/test", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Errorf("test: %d %s", rec.Code, rec.Body.String())
	}
	rec = aiDo(t, rt, cookie, http.MethodGet, base+"/providers/vllm/models", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"context_window":8192`) {
		t.Errorf("models: %d %s", rec.Code, rec.Body.String())
	}

	rec = aiDo(t, rt, cookie, http.MethodPut, base, `{"active":{"provider":"vllm","model":"llama3"},"fallback":[]}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"configured":true`) || !strings.Contains(rec.Body.String(), `"max_steps"`) {
		t.Fatalf("choice: %d %s", rec.Code, rec.Body.String())
	}
	rec = aiDo(t, rt, cookie, http.MethodPut, base, `{"active":{"provider":"openai","model":"x"}}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("active without key: %d", rec.Code)
	}

	// Legacy PUT must not wipe the fallback chain.
	_ = db.SetAIFallbackChain(context.Background(), []store.AIModelRef{{Provider: "vllm", Model: "m"}})
	rec = aiDo(t, rt, cookie, http.MethodPut, base, `{"provider":"anthropic","model":"claude-sonnet-5-5","api_key":"sk-ant-x"}`)
	if rec.Code != 200 {
		t.Fatalf("legacy put: %d %s", rec.Code, rec.Body.String())
	}
	if fb, _ := db.GetAIFallbackChain(context.Background()); len(fb) != 1 {
		t.Errorf("legacy PUT dropped the fallback chain: %v", fb)
	}

	rec = aiDo(t, rt, cookie, http.MethodDelete, base+"/providers/vllm", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"has_key":false`) {
		t.Errorf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec = aiDo(t, rt, cookie, http.MethodGet, base+"/providers/nope/models", ""); rec.Code != 404 {
		t.Errorf("unknown provider: %d", rec.Code)
	}
}

func TestAIUsageEndpoint(t *testing.T) {
	rt, db, _ := providerRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	now := time.Now()
	_ = db.RecordAIUsage(ctx, store.AIUsageRecord{CreatedAt: now, Provider: "openai", Model: "m", PromptTokens: 10, CompletionTokens: 5})
	_ = db.RecordAIUsage(ctx, store.AIUsageRecord{CreatedAt: now, Provider: "openai", Model: "m", ErrorClass: "rate_limited"})
	_ = db.RecordAIUsage(ctx, store.AIUsageRecord{CreatedAt: now.AddDate(0, 0, -60), Provider: "old", Model: "m", PromptTokens: 99})

	rec := aiDo(t, rt, cookie, http.MethodGet, "/api/v1/ai/usage?days=30", "")
	var got aiUsageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 {
		t.Fatalf("%d %v", rec.Code, err)
	}
	if len(got.Days) != 1 || got.Totals.PromptTokens != 10 || got.Totals.Requests != 2 || got.Totals.Errors != 1 {
		t.Errorf("usage = %+v", got)
	}
	if rec = aiDo(t, rt, cookie, http.MethodGet, "/api/v1/ai/usage?days=0", ""); rec.Code != 400 {
		t.Errorf("days=0: %d", rec.Code)
	}
}
