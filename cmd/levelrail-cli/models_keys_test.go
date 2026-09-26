package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func newKeysServer(t *testing.T, gotReq *string, gotBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		*gotReq, *gotBody = r.Method+" "+r.URL.RequestURI(), string(b)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/keys"):
			_ = json.NewEncoder(w).Encode([]apiclient.ModelKeyResource{{ID: "k1", Name: "ci", KeyPrefix: "lr-12345", Status: "active", RPM: 30}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/usage"):
			_ = json.NewEncoder(w).Encode(apiclient.ModelUsageReport{Totals: apiclient.ModelUsageTotals{Requests: 5, InputTokens: 11}, Note: "note text",
				Keys: []apiclient.ModelKeyUsage{{Name: "ci", Status: "active", ModelUsageTotals: apiclient.ModelUsageTotals{Requests: 5}}}})
		default:
			_, _ = io.WriteString(w, `{"id":"k2","name":"ci","api_key":"lr-secretvalue"}`)
		}
	}))
}

func TestRun_ModelsKeys(t *testing.T) {
	var req, body string
	srv := newKeysServer(t, &req, &body)
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"models", "keys", "list", "--api-url", srv.URL, "chat"})
	if req != "GET /api/v1/models/chat/keys" || !strings.Contains(out, "lr-12345") || !strings.Contains(out, "30") {
		t.Errorf("list: req=%q out=%s", req, out)
	}

	out, _ = runCLIExpectOK(t, []string{"models", "keys", "create", "--api-url", srv.URL, "--name", "ci", "--rpm", "30", "--allow-paths", "/v1/chat/completions, /v1/embeddings", "--expires-in", "24h", "chat"})
	var sent apiclient.CreateModelKeyRequest
	_ = json.Unmarshal([]byte(body), &sent)
	if req != "POST /api/v1/models/chat/keys" || sent.RPM != 30 || len(sent.AllowPaths) != 2 || sent.ExpiresAt == nil || time.Until(*sent.ExpiresAt) < 23*time.Hour || !strings.Contains(out, "lr-secretvalue") {
		t.Errorf("create: req=%q body=%s out=%s", req, body, out)
	}

	runCLIExpectOK(t, []string{"models", "keys", "revoke", "--api-url", srv.URL, "chat", "k1"})
	if req != "DELETE /api/v1/models/chat/keys/k1" {
		t.Errorf("revoke req = %q", req)
	}

	runCLIExpectOK(t, []string{"models", "keys", "rotate", "--api-url", srv.URL, "--grace", "10m", "chat", "k1"})
	if req != "POST /api/v1/models/chat/keys/k1/rotate" || !strings.Contains(body, `"grace_seconds":600`) {
		t.Errorf("rotate: req=%q body=%s", req, body)
	}

	out, _ = runCLIExpectOK(t, []string{"models", "usage", "--api-url", srv.URL, "--since", "48h", "chat"})
	if !strings.Contains(req, "/api/v1/models/chat/usage?since=48h0m0s") || !strings.Contains(out, "note text") || !strings.Contains(out, "11 in") {
		t.Errorf("usage: req=%q out=%s", req, out)
	}
}
