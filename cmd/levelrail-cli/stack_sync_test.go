package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const syncCanary = "CANARY-app-name-zq9"

func syncServer(t *testing.T, calls *[]string, put *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, put)
			_, _ = w.Write([]byte(`{"config":{"repo_url":"https://git.example.invalid/o/r.git","branch":"main","path":"s","credential_ref":"app:` + syncCanary + `","enabled":false,"apply":false,"prune":false,"deploy":false},"webhook_secret_set":false}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"config":{"repo_url":"https://git.example.invalid/o/r.git","branch":"main","path":"s","credential_ref":"app:` + syncCanary + `","enabled":true,"apply":true,"prune":false,"deploy":false},"webhook_secret_set":true,"status":{"commit":"abc123","checked_at":"2026-11-13T10:00:00Z","pending":true,"counts":{"create":2}}}`))
		case strings.HasSuffix(r.URL.Path, "/refresh"):
			_, _ = w.Write([]byte(`{"commit":"abc123","pending":true,"counts":{"create":2}}`))
		default:
			_, _ = w.Write([]byte(`{"secret":"SECRET-once-value","sync_url":"/api/v1/hooks/stacks/infra/sync","refresh_url":"/api/v1/hooks/stacks/infra/refresh"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestStackSync_SetSendsExplicitFalseAndHidesCredential(t *testing.T) {
	var calls []string
	var put map[string]any
	srv := syncServer(t, &calls, &put)
	code, out, e := runIaC([]string{"stack", "sync", "set", "infra", "--repo", "https://git.example.invalid/o/r.git", "--branch", "main", "--path", "s", "--credential-ref", "app:" + syncCanary, "--api-url", srv.URL}, nil)
	if code != exitOK {
		t.Fatalf("set = %d %s %s", code, out, e)
	}
	for _, k := range []string{"apply", "prune", "deploy"} {
		if v, ok := put[k]; !ok || v != false {
			t.Fatalf("%s = %v (present %v), want explicit false", k, v, ok)
		}
	}
	if put["enabled"] != true {
		t.Fatalf("enabled = %v, want the current state kept", put["enabled"])
	}
	if !strings.Contains(out, "reminder: apply is off") || !strings.Contains(out, "reminder: prune is off") || !strings.Contains(out, "reminder: deploy is off") {
		t.Fatalf("missing reminders: %s", out)
	}
	if strings.Contains(out+e, syncCanary) {
		t.Fatalf("credential_ref leaked: %s %s", out, e)
	}
	code, out, _ = runIaC([]string{"stack", "sync", "set", "infra", "--repo", "https://git.example.invalid/o/r.git", "--branch", "main", "--json", "--api-url", srv.URL}, nil)
	if code != exitOK || strings.Contains(out, syncCanary) || !json.Valid([]byte(out)) {
		t.Fatalf("set --json = %d %s", code, out)
	}
}

func TestStackSync_SetLocalRefusals(t *testing.T) {
	var calls []string
	var put map[string]any
	srv := syncServer(t, &calls, &put)
	base := []string{"stack", "sync", "set", "infra", "--repo", "https://git.example.invalid/o/r.git", "--branch", "main", "--api-url", srv.URL}
	for name, extra := range map[string][]string{
		"prune without apply":  {"--prune"},
		"deploy without apply": {"--deploy"},
		"both toggles":         {"--enable", "--disable"},
	} {
		if code, out, _ := runIaC(append(append([]string{}, base...), extra...), nil); code == exitOK {
			t.Fatalf("%s succeeded: %s", name, out)
		}
	}
	_, out, e := runIaC(append(append([]string{}, base...), "--prune"), nil)
	if !strings.Contains(out+e, "prune needs apply: a sync that does not apply never prunes") {
		t.Fatalf("message = %s %s", out, e)
	}
	if code, _, _ := runIaC([]string{"stack", "sync", "set", "infra", "--api-url", srv.URL}, nil); code == exitOK {
		t.Fatal("set without --repo and --branch succeeded")
	}
	if len(calls) != 0 {
		t.Fatalf("refusals reached the server: %v", calls)
	}
}

func TestStackSync_StatusRefreshAndRotate(t *testing.T) {
	var calls []string
	var put map[string]any
	srv := syncServer(t, &calls, &put)
	code, out, e := runIaC([]string{"stack", "sync", "status", "infra", "--json", "--api-url", srv.URL}, nil)
	var info map[string]any
	if code != exitOK || json.Unmarshal([]byte(out), &info) != nil || info["webhook_secret_set"] != true || strings.Contains(out+e, syncCanary) {
		t.Fatalf("status --json = %d %s %s", code, out, e)
	}
	if code, out, _ = runIaC([]string{"stack", "sync", "status", "infra", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "pending: true") || strings.Contains(out, syncCanary) {
		t.Fatalf("status = %d %s", code, out)
	}
	if code, out, _ = runIaC([]string{"stack", "sync", "refresh", "infra", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "create 2") {
		t.Fatalf("refresh = %d %s", code, out)
	}
	code, out, e = runIaC([]string{"stack", "sync", "webhook-secret", "rotate", "infra", "--api-url", srv.URL}, nil)
	if code != exitOK || strings.Count(out, "SECRET-once-value") != 1 || strings.Contains(e, "SECRET-once-value") || !strings.Contains(e, "will not be shown again") {
		t.Fatalf("rotate = %d out %q err %q", code, out, e)
	}
	code, out, e = runIaC([]string{"stack", "sync", "webhook-secret", "rotate", "infra", "--json", "--api-url", srv.URL}, nil)
	var sec map[string]string
	if code != exitOK || json.Unmarshal([]byte(out), &sec) != nil || sec["secret"] != "SECRET-once-value" || sec["sync_url"] == "" || sec["refresh_url"] == "" || e != "" {
		t.Fatalf("rotate --json = %d %q %q", code, out, e)
	}
}
