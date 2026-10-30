package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/clitree"
)

// routeServer answers "METHOD /path" keys from a table, recording each call's JSON body.
func routeServer(t *testing.T, routes map[string]any, bodies map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if bodies != nil && r.Body != nil {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r.Body)
			bodies[key] = buf.Bytes()
		}
		v, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no route " + key})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func cliArgs(srv *httptest.Server, args ...string) []string {
	return append(args, "--api-url", srv.URL, "--token", "t")
}

func TestAppsScaleSetsResourceLimits(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web": appResource{Name: "web", Image: "x:1", Port: 80, Replicas: 1, Resources: &serviceResources{SwapMemoryBytes: 7}},
		"PUT /api/v1/apps/web": appResource{Name: "web", Image: "x:1", Port: 80},
	}, bodies)
	runCLIExpectOK(t, cliArgs(srv, "apps", "scale", "web", "--memory", "1Gi", "--cpu", "0.5"))
	var sent appResource
	if err := json.Unmarshal(bodies["PUT /api/v1/apps/web"], &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Resources == nil || sent.Resources.MemoryBytes != 1<<30 || sent.Resources.NanoCPUs != 5e8 || sent.Resources.SwapMemoryBytes != 7 {
		t.Errorf("resources = %+v, want memory and cpu set and swap kept", sent.Resources)
	}
	runCLIExpectValidationError(t, cliArgs(srv, "apps", "scale", "web", "--memory", "lots"))
	runCLIExpectValidationError(t, cliArgs(srv, "apps", "scale", "web"))
}

func TestRollbackDefaultsToPreviousSuccessfulDeploy(t *testing.T) {
	t0 := time.Now()
	bodies := map[string][]byte{}
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web": appResource{Name: "web", Image: "r/web:c3", Port: 80},
		"GET /api/v1/apps/web/deploy-attempts": []deployAttemptResource{
			{ID: "3", Image: "r/web:c3", Status: "succeeded", StartedAt: t0},
			{ID: "2x", Image: "r/web:bad", Status: "failed", StartedAt: t0.Add(-time.Minute)},
			{ID: "2", Image: "r/web:c2", Status: "succeeded", StartedAt: t0.Add(-2 * time.Minute)},
			{ID: "1", Image: "r/web:c1", Status: "succeeded", StartedAt: t0.Add(-3 * time.Minute)},
		},
		"POST /api/v1/apps/web/deploys": appResource{Name: "web", Image: "r/web:c2"},
	}, bodies)
	_, stderr := runCLIExpectOK(t, cliArgs(srv, "rollback", "web"))
	if !strings.Contains(stderr, "r/web:c2") {
		t.Errorf("stderr should name the chosen image:\n%s", stderr)
	}
	if !strings.Contains(string(bodies["POST /api/v1/apps/web/deploys"]), "r/web:c2") {
		t.Errorf("deploy body = %s, want image r/web:c2", bodies["POST /api/v1/apps/web/deploys"])
	}

	only := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "r/web:c1", Port: 80},
		"GET /api/v1/apps/web/deploy-attempts": []deployAttemptResource{{ID: "1", Image: "r/web:c1", Status: "succeeded", StartedAt: t0}},
	}, nil)
	if msg := runCLIExpectValidationError(t, cliArgs(only, "rollback", "web")); !strings.Contains(msg, "no earlier successful deploy") {
		t.Errorf("message = %q", msg)
	}
}

func TestSoleBackupTarget(t *testing.T) {
	tests := []struct {
		name    string
		targets []apiclient.BackupTargetResource
		want    string
		wantErr string
	}{
		{"one target is used", []apiclient.BackupTargetResource{{ID: "t1", Name: "s3"}}, "t1", ""},
		{"none explains how to create one", nil, "", "no backup target"},
		{"several ask for --target", []apiclient.BackupTargetResource{{ID: "a"}, {ID: "b"}}, "", "--target"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := routeServer(t, map[string]any{"GET /api/v1/backup-targets": tc.targets}, nil)
			got, err := soleBackupTarget(t.Context(), NewClient(srv.URL, "t"), "lr")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v", got, err)
			}
		})
	}
}

func TestBackupAndRestoreVerbs(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, map[string]any{
		"GET /api/v1/backup-targets":        []apiclient.BackupTargetResource{{ID: "t1"}},
		"POST /api/v1/databases/pg/backups": apiclient.BackupHistoryResource{ID: "b9"},
		"GET /api/v1/databases/pg/backups": []apiclient.BackupHistoryResource{
			{ID: "old", Status: "succeeded", StartedAt: "2026-10-01T00:00:00Z"},
			{ID: "new", Status: "succeeded", StartedAt: "2026-10-03T00:00:00Z"},
			{ID: "failed", Status: "failed", StartedAt: "2026-10-04T00:00:00Z"},
		},
		"POST /api/v1/databases/pg/restore": apiclient.BackupHistoryResource{ID: "r1"},
	}, bodies)
	runCLIExpectOK(t, cliArgs(srv, "backup", "pg"))
	if !strings.Contains(string(bodies["POST /api/v1/databases/pg/backups"]), "t1") {
		t.Errorf("backup body = %s, want the sole target", bodies["POST /api/v1/databases/pg/backups"])
	}
	runCLIExpectValidationError(t, cliArgs(srv, "restore", "pg", "--latest", "--backup", "x", "--confirm", "pg"))
	runCLIExpectValidationError(t, cliArgs(srv, "restore", "pg", "--latest", "--confirm", "wrong"))
}

func TestNewestSucceededBackup(t *testing.T) {
	if _, ok := newestSucceededBackup([]apiclient.BackupHistoryResource{{ID: "f", Status: "failed"}}); ok {
		t.Error("a failed backup must not be chosen")
	}
	id, ok := newestSucceededBackup([]apiclient.BackupHistoryResource{
		{ID: "a", Status: "succeeded", StartedAt: "2026-10-01T00:00:00Z"}, {ID: "b", Status: "succeeded", StartedAt: "2026-10-02T00:00:00Z"},
	})
	if !ok || id != "b" {
		t.Errorf("got %q, %v", id, ok)
	}
}

func TestEnvSet(t *testing.T) {
	bodies := map[string][]byte{}
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web":                 appResource{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}},
		"GET /api/v1/apps/web/secrets":         []map[string]string{{"key": "TOKEN"}},
		"GET /api/v1/apps/web/env/bound":       []map[string]string{},
		"GET /api/v1/apps/web/pending-changes": map[string]any{},
		"PUT /api/v1/apps/web":                 appResource{Name: "web"},
	}, bodies)
	runCLIExpectOK(t, cliArgs(srv, "env", "set", "web", "B=2", "A=9"))
	var sent appResource
	if err := json.Unmarshal(bodies["PUT /api/v1/apps/web"], &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Env["A"] != "9" || sent.Env["B"] != "2" {
		t.Errorf("env = %v", sent.Env)
	}
	if msg := runCLIExpectValidationError(t, cliArgs(srv, "env", "set", "web", "TOKEN=x")); !strings.Contains(msg, "apps secrets set") {
		t.Errorf("secret key message = %q", msg)
	}
	runCLIExpectValidationError(t, cliArgs(srv, "env", "set", "web", "NOEQUALS"))
}

func TestOpenPrintsURLs(t *testing.T) {
	srv := routeServer(t, map[string]any{
		"GET /api/v1/apps/web": appResource{Name: "web", Domains: []string{"a.example.com"}, FallbackURL: "https://web.1-2-3-4.sslip.io"},
	}, nil)
	out, _ := runCLIExpectOK(t, cliArgs(srv, "open", "web"))
	if strings.TrimSpace(out) != "https://a.example.com\nhttps://web.1-2-3-4.sslip.io" {
		t.Errorf("out = %q", out)
	}
}

func TestLogsAcceptsAnAppName(t *testing.T) {
	var path string
	srv := newListEchoServer(t, &path, map[string]any{"entries": []logEntryResource{}})
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", cliArgs(srv, "logs", "web"), &out, &errb, envMap())
	if code != exitOK || !strings.Contains(path, "/apps/web/logs") {
		t.Errorf("code=%d path=%q stderr=%q", code, path, errb.String())
	}
	if code := run("levelrail-cli-test", []string{"logs", "--bogus"}, &out, &errb, envMap()); code != exitUsage {
		t.Errorf("a flag as the first word must stay a usage error, got %d", code)
	}
}

func TestHelpTiers(t *testing.T) {
	simple := simpleUsage("lr")
	for _, v := range clitree.SimpleVerbs() {
		if _, ok := simpleVerbHelp[v]; !ok {
			t.Errorf("simple verb %q has no help line", v)
		}
		if !strings.Contains(simple, "lr "+v) {
			t.Errorf("simple help lacks %q", v)
		}
	}
	if n := len(clitree.SimpleVerbs()); n != 14 {
		t.Errorf("simple verbs = %d, want 14", n)
	}
	for _, v := range clitree.SimpleVerbs() {
		if _, ok := clitree.Tree[v]; !ok {
			t.Errorf("simple verb %q is not in the command tree", v)
		}
	}
	if strings.Contains(simple, "nodes join-token") || clitree.TierOf("nodes") != clitree.TierAdvanced || clitree.TierOf("up") != clitree.TierSimple {
		t.Error("tiers are wrong")
	}

	var short, adv bytes.Buffer
	if code := run("lr", []string{"help"}, &short, &bytes.Buffer{}, envMap()); code != exitOK || short.String() != simple {
		t.Errorf("help should print the simple list, code=%d", code)
	}
	if code := run("lr", []string{"help", "advanced"}, &adv, &bytes.Buffer{}, envMap()); code != exitOK || !strings.Contains(adv.String(), "nodes join-token") {
		t.Errorf("help advanced should print the full list, code=%d", code)
	}
}
