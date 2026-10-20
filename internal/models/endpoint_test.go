package models

import (
	"context"
	"encoding/json"
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

type credStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newCredStore() *credStore { return &credStore{values: map[string]string{}} }

func (c *credStore) SetValue(_ context.Context, ns, key, v string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[ns+"/"+key] = v
	return nil
}

func (c *credStore) Resolve(_ context.Context, ns, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[ns+"/"+key], nil
}

func (c *credStore) DeleteAll(_ context.Context, ns string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.values {
		if strings.HasPrefix(k, ns+"/") {
			delete(c.values, k)
		}
	}
	return nil
}

type stubChecker struct{ res EndpointProbe }

func (s stubChecker) Probe(context.Context, string, string, string) EndpointProbe { return s.res }

func openAIServer(t *testing.T, models ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		var data []map[string]string
		for _, m := range models {
			data = append(data, map[string]string{"id": m})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEndpointProber(t *testing.T) {
	t.Setenv(EndpointAllowPrivateEnv, "true")
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			_, _ = io.WriteString(w, `{"models":[{"name":"qwen2.5:0.5b"},{"name":"llama3.2:1b"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer ollama.Close()
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	defer locked.Close()
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "<html>hi</html>") }))
	defer html.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(60 * time.Millisecond)
		_, _ = io.WriteString(w, `{"data":[{"id":"m1"}]}`)
	}))
	defer slow.Close()
	empty := openAIServer(t)
	notfound := httptest.NewServer(http.NotFoundHandler())
	defer notfound.Close()
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()

	p := NewEndpointProber(30 * time.Millisecond)
	tests := []struct {
		name       string
		url, api   string
		cred       string
		wantState  string
		wantCode   string
		wantModels []string
	}{
		{name: "ollama tags", url: ollama.URL, api: store.EndpointAPIOllama, wantState: store.EndpointOnline, wantModels: []string{"llama3.2:1b", "qwen2.5:0.5b"}},
		{name: "wrong api type is route not found", url: ollama.URL, api: store.EndpointAPIOpenAI, wantState: store.EndpointError, wantCode: DiagNotFound},
		{name: "bad credential", url: locked.URL, api: store.EndpointAPIOpenAI, cred: "sk-bad", wantState: store.EndpointAuthFailed, wantCode: DiagAuth},
		{name: "good credential", url: locked.URL, api: store.EndpointAPIOpenAI, cred: "sk-good", wantState: store.EndpointOnline, wantModels: []string{"m1"}},
		{name: "html means wrong port", url: html.URL, api: store.EndpointAPIOpenAI, wantState: store.EndpointError, wantCode: DiagWrongPort},
		{name: "slow endpoint", url: slow.URL, api: store.EndpointAPIOpenAI, wantState: store.EndpointSlow, wantModels: []string{"m1"}},
		{name: "online but no models", url: empty.URL, api: store.EndpointAPIOpenAI, wantState: store.EndpointOnline, wantCode: DiagNoModels},
		{name: "missing route", url: notfound.URL, api: store.EndpointAPIOpenAI, wantState: store.EndpointError, wantCode: DiagNotFound},
		{name: "nothing listening", url: deadURL, api: store.EndpointAPIOpenAI, wantState: store.EndpointUnreachable, wantCode: DiagRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Probe(context.Background(), tt.url, tt.api, tt.cred)
			if got.State != tt.wantState || got.Code != tt.wantCode {
				t.Fatalf("state/code = %s/%s, want %s/%s (%s)", got.State, got.Code, tt.wantState, tt.wantCode, got.Message)
			}
			if len(tt.wantModels) > 0 && strings.Join(got.Models, ",") != strings.Join(tt.wantModels, ",") {
				t.Errorf("models = %v, want %v", got.Models, tt.wantModels)
			}
		})
	}
}

func TestEndpointProber_RefusesMetadataAtDial(t *testing.T) {
	t.Setenv(EndpointAllowPrivateEnv, "true")
	got := NewEndpointProber(0).Probe(context.Background(), "http://169.254.169.254", store.EndpointAPIOpenAI, "")
	if got.Code != DiagBlocked {
		t.Fatalf("code = %q, want %q", got.Code, DiagBlocked)
	}
}

func extSvc(t *testing.T, checker EndpointChecker) (*Service, *store.DB, *credStore) {
	t.Helper()
	svc, db := newSvc(t, nil)
	creds := newCredStore()
	svc.SetEndpoints(EndpointPlumbing{Store: db, Credentials: creds, Checker: checker, Lookup: fixedLookup("93.184.216.34")})
	return svc, db, creds
}

func TestService_CreateExternal(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EndpointAllowPrivateEnv, "true")
	online := stubChecker{EndpointProbe{State: store.EndpointOnline, Latency: 20 * time.Millisecond, Models: []string{"qwen2.5:0.5b"}}}

	t.Run("registers model, endpoint, key and credential", func(t *testing.T) {
		svc, db, creds := extSvc(t, online)
		got, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "mac"}, External: &ExternalInput{BaseURL: "http://100.64.0.7:11434", APIType: "ollama", Credential: "sk-secret"}})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if got.APIKey == "" || got.Model.Compute != ComputeExternal || got.Model.Engine != EngineExternal {
			t.Fatalf("Created = %+v", got)
		}
		e, err := db.GetInferenceEndpoint(ctx, "mac")
		if err != nil || e.HealthState != store.EndpointOnline || len(e.Discovered) != 1 || !e.CredentialSet {
			t.Fatalf("endpoint = %+v, %v", e, err)
		}
		if creds.values["model/mac/"+endpointCredentialKey] != "sk-secret" {
			t.Errorf("credential not stored in the secrets manager: %v", creds.values)
		}
		v, err := svc.Get(ctx, "mac")
		if err != nil || !v.Ready || v.Endpoint == nil {
			t.Fatalf("view = %+v, %v", v, err)
		}
	})

	t.Run("half success: discovery fails but endpoint is registered", func(t *testing.T) {
		svc, db, _ := extSvc(t, stubChecker{EndpointProbe{State: store.EndpointUnreachable, Code: DiagRefused, Message: "nothing is listening"}})
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "asleep"}, External: &ExternalInput{BaseURL: "http://10.0.0.9:11434"}}); err != nil {
			t.Fatalf("Create: %v", err)
		}
		e, err := db.GetInferenceEndpoint(ctx, "asleep")
		if err != nil || e.HealthState != store.EndpointUnreachable || !strings.HasPrefix(e.HealthError, DiagRefused) || len(e.Discovered) != 0 {
			t.Fatalf("endpoint = %+v, %v", e, err)
		}
		views, err := svc.List(ctx)
		if err != nil || len(views) != 1 || views[0].Ready || views[0].Reason != store.EndpointUnreachable {
			t.Fatalf("list = %+v, %v", views, err)
		}
	})

	t.Run("a failed probe later keeps the model list", func(t *testing.T) {
		checker := &swapChecker{res: online.res}
		svc, db, _ := extSvc(t, checker)
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "keep"}, External: &ExternalInput{BaseURL: "http://10.0.0.9:11434"}}); err != nil {
			t.Fatal(err)
		}
		checker.res = EndpointProbe{State: store.EndpointUnreachable, Code: DiagTimeout, Message: "no answer"}
		if _, err := svc.TestEndpoint(ctx, "keep"); err != nil {
			t.Fatal(err)
		}
		e, _ := db.GetInferenceEndpoint(ctx, "keep")
		if e.HealthState != store.EndpointUnreachable || len(e.Discovered) != 1 || e.Failures != 1 || e.LastOKAt.IsZero() {
			t.Fatalf("endpoint = %+v", e)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		svc, _, _ := extSvc(t, online)
		tests := []struct {
			name    string
			in      CreateInput
			wantErr error
		}{
			{"metadata address", CreateInput{Spec: Spec{Name: "x1"}, External: &ExternalInput{BaseURL: "http://169.254.169.254"}}, ErrEndpointBlocked},
			{"bad name", CreateInput{Spec: Spec{Name: "Bad Name"}, External: &ExternalInput{BaseURL: "http://10.0.0.1"}}, ErrInvalid},
			{"bad api type", CreateInput{Spec: Spec{Name: "x2"}, External: &ExternalInput{BaseURL: "http://10.0.0.1", APIType: "grpc"}}, ErrInvalid},
			{"unknown fallback", CreateInput{Spec: Spec{Name: "x3"}, External: &ExternalInput{BaseURL: "http://10.0.0.1", FallbackModel: "nope"}}, ErrInvalid},
			{"budget without fallback", CreateInput{Spec: Spec{Name: "x4"}, External: &ExternalInput{BaseURL: "http://10.0.0.1", LatencyBudget: time.Second}}, ErrInvalid},
		}
		for _, tt := range tests {
			if _, err := svc.Create(ctx, tt.in); !errors.Is(err, tt.wantErr) {
				t.Errorf("%s: err = %v, want %v", tt.name, err, tt.wantErr)
			}
		}
	})

	t.Run("private address needs the opt in", func(t *testing.T) {
		t.Setenv(EndpointAllowPrivateEnv, "false")
		t.Setenv("APP_NOTIFY_ALLOW_PRIVATE_NETWORKS", "false")
		svc, _, _ := extSvc(t, online)
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "lan"}, External: &ExternalInput{BaseURL: "http://192.168.1.4:11434"}}); !errors.Is(err, ErrEndpointPrivate) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("delete removes model, endpoint and credential", func(t *testing.T) {
		svc, db, creds := extSvc(t, online)
		if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "gone"}, External: &ExternalInput{BaseURL: "http://10.0.0.1:1", Credential: "k"}}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Delete(ctx, "gone"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.GetModel(ctx, "gone"); !errors.Is(err, store.ErrModelNotFound) {
			t.Errorf("model still there: %v", err)
		}
		if _, err := db.GetInferenceEndpoint(ctx, "gone"); !errors.Is(err, store.ErrEndpointNotFound) {
			t.Errorf("endpoint still there: %v", err)
		}
		if len(creds.values) != 0 {
			t.Errorf("credential left behind: %v", creds.values)
		}
	})
}

type swapChecker struct {
	mu  sync.Mutex
	res EndpointProbe
}

func (s *swapChecker) Probe(context.Context, string, string, string) EndpointProbe {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.res
}

func TestService_EndpointRouting(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EndpointAllowPrivateEnv, "true")
	svc, db, _ := extSvc(t, stubChecker{EndpointProbe{State: store.EndpointOnline}})
	if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "mac"}, External: &ExternalInput{BaseURL: "http://10.0.0.1:1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("cloud-chat"), Domain: ""}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetEndpointRouting(ctx, "mac", "cloud-chat", "llama3.1:8b", 3*time.Second); err != nil {
		t.Fatalf("set routing: %v", err)
	}
	e, _ := db.GetInferenceEndpoint(ctx, "mac")
	if e.FallbackModel != "cloud-chat" || e.LatencyBudget != 3*time.Second || e.FallbackRef != "llama3.1:8b" {
		t.Fatalf("endpoint = %+v", e)
	}
	for name, fn := range map[string]func() error{
		"self":         func() error { return svc.SetEndpointRouting(ctx, "mac", "mac", "", 0) },
		"unknown":      func() error { return svc.SetEndpointRouting(ctx, "mac", "nope", "", 0) },
		"huge budget":  func() error { return svc.SetEndpointRouting(ctx, "mac", "cloud-chat", "", time.Hour) },
		"not endpoint": func() error { return svc.SetEndpointRouting(ctx, "cloud-chat", "mac", "", 0) },
	} {
		if err := fn(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if err := svc.SetEndpointRouting(ctx, "mac", "", "ignored", time.Second); err == nil {
		t.Error("budget without fallback accepted")
	}
	if err := svc.SetEndpointRouting(ctx, "mac", "", "", 0); err != nil {
		t.Fatalf("turn off: %v", err)
	}
	e, _ = db.GetInferenceEndpoint(ctx, "mac")
	if e.FallbackModel != "" || e.LatencyBudget != 0 {
		t.Fatalf("policy not off: %+v", e)
	}
}

func TestService_EndpointCredential(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EndpointAllowPrivateEnv, "true")
	svc, db, creds := extSvc(t, stubChecker{EndpointProbe{State: store.EndpointAuthFailed, Code: DiagAuth, Message: "rejected"}})
	if _, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "mac"}, External: &ExternalInput{BaseURL: "http://10.0.0.1:1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetEndpointCredential(ctx, "mac", "sk-new"); err != nil {
		t.Fatal(err)
	}
	e, _ := db.GetInferenceEndpoint(ctx, "mac")
	if !e.CredentialSet || creds.values["model/mac/"+endpointCredentialKey] != "sk-new" {
		t.Fatalf("credential not set: %+v %v", e, creds.values)
	}
	if _, err := svc.SetEndpointCredential(ctx, "mac", ""); err != nil {
		t.Fatal(err)
	}
	e, _ = db.GetInferenceEndpoint(ctx, "mac")
	if e.CredentialSet || len(creds.values) != 0 {
		t.Fatalf("credential not removed: %+v %v", e, creds.values)
	}
}

type extGW struct {
	gw    *Gateway
	key   string
	db    *store.DB
	creds *credStore
}

func newExternalGateway(t *testing.T, primaryURL string, ep store.InferenceEndpoint, fallback *store.Model) extGW {
	t.Helper()
	t.Setenv(EndpointAllowPrivateEnv, "true")
	db := openDB(t)
	key, hash, prefix, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	m := store.Model{Name: "mac", Domain: "mac.example.com", Engine: EngineExternal, APIKeyHash: hash, APIKeyPrefix: prefix, Compute: ComputeExternal}
	ep.Name, ep.BaseURL = "mac", primaryURL
	if err := db.SaveInferenceEndpoint(context.Background(), m, ep); err != nil {
		t.Fatal(err)
	}
	if fallback != nil {
		if err := db.SaveModel(context.Background(), *fallback); err != nil {
			t.Fatal(err)
		}
		if err := db.SetModelEndpoint(context.Background(), fallback.Name, fallback.EndpointDial); err != nil {
			t.Fatal(err)
		}
	}
	creds := newCredStore()
	_ = creds.SetValue(context.Background(), store.ModelSecretsKey("mac"), endpointCredentialKey, "sk-upstream")
	gw := NewGateway(db, NewHostResolver("", nil), slog.Default())
	gw.SetCredentialReader(creds)
	return extGW{gw: gw, key: key, db: db, creds: creds}
}

func (f extGW) chat(body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://mac.example.com/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.key)
	rec := httptest.NewRecorder()
	f.gw.Handle(rec, req)
	return rec
}

func fallbackModel(t *testing.T, dial string) *store.Model {
	t.Helper()
	_, hash, prefix, _ := NewAPIKey()
	return &store.Model{Name: "cloud", Engine: EngineOllama, ModelRef: "llama3.1:8b", APIKeyHash: hash, APIKeyPrefix: prefix, EndpointDial: dial, Compute: store.ComputeGPU}
}

func TestGateway_ExternalEndpoint(t *testing.T) {
	var gotAuth, gotBody, gotPath string
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotAuth, gotBody, gotPath = r.Header.Get("Authorization"), string(b), r.URL.Path
		_, _ = io.WriteString(w, `{"served_by":"primary","usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	}))
	defer primary.Close()
	f := newExternalGateway(t, primary.URL, store.InferenceEndpoint{APIType: store.EndpointAPIOpenAI, CredentialSet: true, HealthState: store.EndpointOnline}, nil)

	rec := f.chat(`{"model":"qwen2.5:0.5b","messages":[]}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "primary") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer sk-upstream" || gotPath != "/v1/chat/completions" || !strings.Contains(gotBody, "qwen2.5:0.5b") {
		t.Errorf("upstream saw auth=%q path=%q body=%q", gotAuth, gotPath, gotBody)
	}

	t.Run("gateway key of another model is refused", func(t *testing.T) {
		other, _, _, _ := NewAPIKey()
		req := httptest.NewRequest(http.MethodPost, "http://mac.example.com/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+other)
		rec := httptest.NewRecorder()
		f.gw.Handle(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
	t.Run("admin routes are not forwarded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://mac.example.com/api/pull", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+f.key)
		rec := httptest.NewRecorder()
		f.gw.Handle(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestGateway_ExternalFallback(t *testing.T) {
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"served_by":"cloud","saw":`+string(b)+`}`)
	}))
	defer cloud.Close()
	cloudDial := strings.TrimPrefix(cloud.URL, "http://")
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = io.WriteString(w, `{"served_by":"slow"}`)
	}))
	defer slow.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }))
	defer broken.Close()

	policy := store.InferenceEndpoint{APIType: store.EndpointAPIOpenAI, FallbackModel: "cloud", FallbackRef: "llama3.1:8b", HealthState: store.EndpointOnline, LatencyBudget: 100 * time.Millisecond}
	tests := []struct {
		name       string
		url        string
		mutate     func(*store.InferenceEndpoint)
		wantStatus int
		wantServed string
	}{
		{name: "connection refused falls back", url: deadURL, wantStatus: 200, wantServed: "cloud"},
		{name: "over latency budget falls back", url: slow.URL, wantStatus: 200, wantServed: "cloud"},
		{name: "server error falls back", url: broken.URL, wantStatus: 200, wantServed: "cloud"},
		{name: "marked unreachable skips the endpoint", url: slow.URL, mutate: func(e *store.InferenceEndpoint) { e.HealthState = store.EndpointUnreachable }, wantStatus: 200, wantServed: "cloud"},
		{name: "policy off returns the failure", url: deadURL, mutate: func(e *store.InferenceEndpoint) { e.FallbackModel, e.FallbackRef, e.LatencyBudget = "", "", 0 }, wantStatus: http.StatusBadGateway},
		{name: "no budget waits for the slow endpoint", url: slow.URL, mutate: func(e *store.InferenceEndpoint) { e.LatencyBudget = 0 }, wantStatus: 200, wantServed: "slow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := policy
			if tt.mutate != nil {
				tt.mutate(&ep)
			}
			f := newExternalGateway(t, tt.url, ep, fallbackModel(t, cloudDial))
			rec := f.chat(`{"model":"qwen2.5:0.5b","messages":[]}`)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantServed == "" {
				return
			}
			if !strings.Contains(rec.Body.String(), `"served_by":"`+tt.wantServed+`"`) {
				t.Fatalf("body = %s, want served by %s", rec.Body.String(), tt.wantServed)
			}
			if tt.wantServed == "cloud" {
				if !strings.Contains(rec.Body.String(), `"model":"llama3.1:8b"`) {
					t.Errorf("fallback did not get its own model id: %s", rec.Body.String())
				}
				if rec.Header().Get("X-Fallback-From") != "mac" {
					t.Errorf("missing fallback header")
				}
				if f.gw.external.fallbacks.Load() != 1 {
					t.Errorf("fallback not counted")
				}
			}
		})
	}
}
