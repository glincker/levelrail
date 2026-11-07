package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func driftServer(t *testing.T, drift map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "shop", "drift": drift})
	}))
}

func TestStackDrift_ExitCodes(t *testing.T) {
	drifted := driftServer(t, map[string]any{"status": "drifted", "checked_at": "2026-11-07T10:00:00Z", "summary": []any{map[string]any{"kind": "App", "name": "web"}}})
	defer drifted.Close()
	code, out, _ := runIaC([]string{"stack", "drift", "shop", "--api-url", drifted.URL}, nil)
	if code != exitValidation || !strings.Contains(out, "App web") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	show, sout, _ := runIaC([]string{"stack", "show", "shop", "--api-url", drifted.URL}, nil)
	if show != exitOK || !strings.Contains(sout, "drifted: App web") {
		t.Fatalf("show code=%d out=%s", show, sout)
	}

	clean := driftServer(t, map[string]any{"status": "clean", "checked_at": "2026-11-07T10:00:00Z"})
	defer clean.Close()
	code, out, _ = runIaC([]string{"stack", "drift", "shop", "--api-url", clean.URL}, nil)
	if code != exitOK || !strings.Contains(out, "no drift") {
		t.Fatalf("code=%d out=%s", code, out)
	}
}
