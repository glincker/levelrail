package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_PipelinesOIDC_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pipelines/oidc" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.PipelineOIDCResource{Configured: false})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("cli", []string{"pipelines", "oidc", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit %d stderr %q", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not configured") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRun_PipelinesOIDC_Configured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pipelines/oidc" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.PipelineOIDCResource{
			Configured: true, IssuerURL: "https://cp.example.com", JWKSURL: "https://cp.example.com/.well-known/jwks.json",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("cli", []string{"pipelines", "oidc", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit %d stderr %q", got, stderr.String())
	}
	var out apiclient.PipelineOIDCResource
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v, stdout %q", err, stdout.String())
	}
	if !out.Configured || out.IssuerURL != "https://cp.example.com" {
		t.Errorf("got %+v", out)
	}
}
