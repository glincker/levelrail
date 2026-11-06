package main

import (
	"strings"
	"testing"
)

func TestEnvSetPrintsRebuildHint(t *testing.T) {
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80},
		"GET /api/v1/apps/web/secrets":         []map[string]string{},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"PUT /api/v1/apps/web":                 map[string]any{"name": "web", "rebuild_required": true, "rebuild_keys": []string{"NEXT_PUBLIC_API"}},
	}, map[string][]byte{})
	out, _ := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "NEXT_PUBLIC_API=https://a.example"))
	if !strings.Contains(out, "NEXT_PUBLIC_API is baked in at build time") {
		t.Errorf("missing rebuild hint in %q", out)
	}
	if strings.Contains(out, "apps rebuild") || !strings.Contains(out, "deploy <dir>") {
		t.Errorf("hint must name real commands, got %q", out)
	}
}

func TestEnvSetRebuildHintGitApp(t *testing.T) {
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80},
		"GET /api/v1/apps/web/secrets":         []map[string]string{},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"GET /api/v1/apps/web/git-source":      map[string]any{"service_name": "web", "repo_url": "https://x/y.git", "branch": "main"},
		"PUT /api/v1/apps/web":                 map[string]any{"name": "web", "rebuild_required": true, "rebuild_keys": []string{"VITE_API"}},
	}, map[string][]byte{})
	out, _ := runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "VITE_API=https://a.example"))
	if !strings.Contains(out, "deploy web --latest") {
		t.Errorf("missing git app hint in %q", out)
	}
}
