package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStackCreateScopeFlag(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "infra", "scope": "install"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"stacks": []any{map[string]any{"name": "infra", "scope": "install"}}})
	}))
	defer srv.Close()
	code, out, errOut := runIaC([]string{"stack", "create", "infra", "--scope", "install", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(out, "install stack infra") {
		t.Fatalf("create: %d %s%s", code, out, errOut)
	}
	if !strings.Contains(body, `"scope":"install"`) || strings.Contains(body, "project") {
		t.Errorf("body = %s", body)
	}
	if code, _, _ := runIaC([]string{"stack", "create", "x", "--scope", "install", "--project", "p", "--api-url", srv.URL}, nil); code == exitOK {
		t.Error("install with --project should fail")
	}
	if code, _, _ := runIaC([]string{"stack", "create", "x", "--scope", "bogus", "--api-url", srv.URL}, nil); code == exitOK {
		t.Error("bad scope should fail")
	}
	if code, out, _ := runIaC([]string{"stack", "list", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(out, "SCOPE") || !strings.Contains(out, "install") {
		t.Errorf("list: %d %s", code, out)
	}
}
