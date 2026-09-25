package models

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type listStore struct{ models []store.Model }

func (l listStore) ListModels(context.Context) ([]store.Model, error) { return l.models, nil }

func TestGateway(t *testing.T) {
	var gotAuth, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	dial := strings.TrimPrefix(upstream.URL, "http://")

	key, hash, _, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	ready := store.Model{Name: "chat", Domain: "chat.example.com", APIKeyHash: hash, EndpointDial: dial}
	notReady := store.Model{Name: "cold", Domain: "cold.example.com", APIKeyHash: hash}
	deleting := store.Model{Name: "gone", Domain: "gone.example.com", APIKeyHash: hash, EndpointDial: dial, Deleting: true}
	fallback := store.Model{Name: "fb", APIKeyHash: hash, EndpointDial: dial}
	gw := NewGateway(listStore{[]store.Model{ready, notReady, deleting, fallback}}, NewHostResolver("1-2-3-4", fallbackFn), slog.Default())

	tests := []struct {
		name     string
		host     string
		path     string
		auth     string
		wantCode int
		handled  bool
	}{
		{name: "valid key proxied", host: "chat.example.com", path: "/v1/chat/completions", auth: "Bearer " + key, wantCode: 200, handled: true},
		{name: "host with port", host: "chat.example.com:443", path: "/v1/models", auth: "Bearer " + key, wantCode: 200, handled: true},
		{name: "fallback host", host: "model-fb.1-2-3-4.sslip.io", path: "/v1/models", auth: "Bearer " + key, wantCode: 200, handled: true},
		{name: "missing key", host: "chat.example.com", path: "/v1/models", wantCode: 401, handled: true},
		{name: "wrong key", host: "chat.example.com", path: "/v1/models", auth: "Bearer lr-nope", wantCode: 401, handled: true},
		{name: "non openai path blocked", host: "chat.example.com", path: "/api/pull", auth: "Bearer " + key, wantCode: 404, handled: true},
		{name: "not ready", host: "cold.example.com", path: "/v1/models", auth: "Bearer " + key, wantCode: 503, handled: true},
		{name: "deleting model is not served", host: "gone.example.com", path: "/v1/models", auth: "Bearer " + key, handled: false},
		{name: "unrelated host falls through", host: "dashboard.example.com", path: "/v1/models", auth: "Bearer " + key, handled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://"+tt.host+tt.path, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			rec := httptest.NewRecorder()
			if handled := gw.Handle(rec, req); handled != tt.handled {
				t.Fatalf("handled = %v, want %v", handled, tt.handled)
			}
			if tt.handled && rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d (%s)", rec.Code, tt.wantCode, rec.Body.String())
			}
		})
	}

	if gotAuth != "" {
		t.Errorf("upstream saw Authorization %q, the gateway key must not be forwarded", gotAuth)
	}
	if gotPath == "" {
		t.Error("upstream never received a request")
	}
}

func TestGateway_UpstreamDownIs502(t *testing.T) {
	key, hash, _, _ := NewAPIKey()
	m := store.Model{Name: "chat", Domain: "chat.example.com", APIKeyHash: hash, EndpointDial: "127.0.0.1:1"}
	gw := NewGateway(listStore{[]store.Model{m}}, NewHostResolver("", nil), nil)
	req := httptest.NewRequest(http.MethodGet, "http://chat.example.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	gw.Handle(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestGateway_MiddlewareFallsThrough(t *testing.T) {
	gw := NewGateway(listStore{}, NewHostResolver("", nil), nil)
	called := false
	h := gw.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://dash.example.com/", nil))
	if !called {
		t.Error("next handler not called for an unrelated host")
	}
}
