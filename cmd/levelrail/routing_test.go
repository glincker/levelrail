package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComposeMux_HealthzReachesAPIHandlerNotSPAFallback(t *testing.T) {
	apiHit := false
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHit = true
		if r.URL.Path != "/healthz" {
			t.Errorf("apiHandler saw path %q, want /healthz unchanged", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>spa shell</html>"))
	})

	mux := composeMux(apiHandler, nil, webHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if !apiHit {
		t.Fatal("GET /healthz did not reach apiHandler, want it exact-matched ahead of the \"/\" SPA fallback")
	}
	if body := rec.Body.String(); body != `{"status":"ok"}` {
		t.Errorf("body = %q, want the API handler's JSON, not the SPA shell", body)
	}
}

func TestComposeMux_APIPrefixReachesAPIHandlerWithFullPath(t *testing.T) {
	var gotPath string
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux := composeMux(apiHandler, nil, webHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/brand", nil)
	mux.ServeHTTP(rec, req)

	if gotPath != "/api/v1/brand" {
		t.Errorf("apiHandler saw path %q, want the full original path unchanged", gotPath)
	}
}

func TestComposeMux_UnmatchedPathFallsBackToWebHandler(t *testing.T) {
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("api"))
	})
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("spa"))
	})

	mux := composeMux(apiHandler, nil, webHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apps/my-app", nil)
	mux.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "spa" {
		t.Errorf("body = %q, want the SPA fallback to serve an unmatched client-side route", got)
	}
}

func TestComposeMux_NilWebhookHandler_NoWebhookRoute(t *testing.T) {
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("spa"))
	})

	mux := composeMux(apiHandler, nil, webHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	mux.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "spa" {
		t.Errorf("POST /webhook body = %q, want the SPA fallback since no webhookHandler was configured", got)
	}
}

func TestComposeMux_WebhookHandlerConfigured_ReceivesPost(t *testing.T) {
	webhookHit := false
	webhookHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		webhookHit = true
		w.WriteHeader(http.StatusOK)
	})
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	mux := composeMux(apiHandler, webhookHandler, webHandler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	mux.ServeHTTP(rec, req)

	if !webhookHit {
		t.Error("POST /webhook did not reach webhookHandler")
	}
}
